package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

var durationBuckets = []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2, 5}

type requestKey struct {
	Route  string
	Status int
}
type telemetry struct {
	mu                                 sync.Mutex
	started                            time.Time
	requests                           map[requestKey]uint64
	assigned, noDriver, dispatchErrors uint64
	durations                          []float64
	histogram                          []uint64
	count                              uint64
	sum                                float64
}

func newTelemetry() *telemetry {
	return &telemetry{started: time.Now(), requests: map[requestKey]uint64{}, histogram: make([]uint64, len(durationBuckets)), durations: []float64{}}
}

type observedStore struct {
	Store
	metrics *telemetry
}

func (s *observedStore) Dispatch(ctx context.Context, p, d Point) (Ride, error) {
	start := time.Now()
	r, e := s.Store.Dispatch(ctx, p, d)
	elapsed := time.Since(start).Seconds()
	m := s.metrics
	m.mu.Lock()
	defer m.mu.Unlock()
	if e != nil {
		m.dispatchErrors++
		return r, e
	}
	if r.Status == "assigned" {
		m.assigned++
	} else {
		m.noDriver++
	}
	m.count++
	m.sum += elapsed
	for i, b := range durationBuckets {
		if elapsed <= b {
			m.histogram[i]++
		}
	}
	if len(m.durations) < 512 {
		m.durations = append(m.durations, elapsed*1000)
	} else {
		m.durations[(m.count-1)%512] = elapsed * 1000
	}
	return r, e
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *responseRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (m *telemetry) handler(next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := id()
		w.Header().Set("X-Request-ID", requestID)
		rw := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		code := rw.status
		if code == 0 {
			code = 200
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		m.mu.Lock()
		m.requests[requestKey{route, code}]++
		m.mu.Unlock()
		if r.Method != "GET" || code >= 500 || os.Getenv("VYRO_ACCESS_LOG") == "1" {
			logger.Info("http_request", "request_id", requestID, "route", route, "status", code, "duration_ms", float64(time.Since(start).Nanoseconds())/1e6)
		}
	})
}

type operations struct {
	Version        string  `json:"version"`
	Storage        string  `json:"storage"`
	Uptime         float64 `json:"uptimeSeconds"`
	Requests       uint64  `json:"httpRequests"`
	HTTPErrors     uint64  `json:"httpServerErrors"`
	Assigned       uint64  `json:"assigned"`
	NoDriver       uint64  `json:"noDriver"`
	DispatchErrors uint64  `json:"dispatchErrors"`
	SampleCount    int     `json:"sampleCount"`
	P95            float64 `json:"recentP95Ms"`
	Goroutines     int     `json:"goroutines"`
}

func (m *telemetry) snapshot(mode string) operations {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := operations{Version: "0.3.0", Storage: mode, Uptime: time.Since(m.started).Seconds(), Assigned: m.assigned, NoDriver: m.noDriver, DispatchErrors: m.dispatchErrors, SampleCount: len(m.durations), Goroutines: runtime.NumGoroutine()}
	for k, n := range m.requests {
		result.Requests += n
		if k.Status >= 500 {
			result.HTTPErrors += n
		}
	}
	values := append([]float64{}, m.durations...)
	sort.Float64s(values)
	result.P95 = percentile(values, .95)
	return result
}
func registerOperations(mux *http.ServeMux, s Store, m *telemetry) {
	mux.HandleFunc("GET /api/live", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "alive"}) })
	mux.HandleFunc("GET /api/ops", func(w http.ResponseWriter, r *http.Request) { write(w, 200, m.snapshot(s.Mode())) })
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.mu.Lock()
		defer m.mu.Unlock()
		fmt.Fprintln(w, "# HELP vyro_http_requests_total Completed HTTP requests by route and status.\n# TYPE vyro_http_requests_total counter")
		keys := make([]requestKey, 0, len(m.requests))
		for k := range m.requests {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Route == keys[j].Route {
				return keys[i].Status < keys[j].Status
			}
			return keys[i].Route < keys[j].Route
		})
		for _, k := range keys {
			fmt.Fprintf(w, "vyro_http_requests_total{route=%q,status=%q} %d\n", k.Route, fmt.Sprint(k.Status), m.requests[k])
		}
		fmt.Fprintln(w, "# HELP vyro_dispatch_total Dispatch outcomes including internal simulation calls.\n# TYPE vyro_dispatch_total counter")
		fmt.Fprintf(w, "vyro_dispatch_total{outcome=\"assigned\"} %d\nvyro_dispatch_total{outcome=\"no_driver\"} %d\nvyro_dispatch_total{outcome=\"error\"} %d\n", m.assigned, m.noDriver, m.dispatchErrors)
		fmt.Fprintln(w, "# HELP vyro_dispatch_duration_seconds Successful dispatch operation duration through commit, excluding HTTP.\n# TYPE vyro_dispatch_duration_seconds histogram")
		for i, b := range durationBuckets {
			fmt.Fprintf(w, "vyro_dispatch_duration_seconds_bucket{le=%q} %d\n", fmt.Sprint(b), m.histogram[i])
		}
		fmt.Fprintf(w, "vyro_dispatch_duration_seconds_bucket{le=\"+Inf\"} %d\nvyro_dispatch_duration_seconds_sum %g\nvyro_dispatch_duration_seconds_count %d\n", m.count, m.sum, m.count)
		fmt.Fprintf(w, "# TYPE vyro_uptime_seconds gauge\nvyro_uptime_seconds %g\n# TYPE vyro_goroutines gauge\nvyro_goroutines %d\n", time.Since(m.started).Seconds(), runtime.NumGoroutine())
	})
}
