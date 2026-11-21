// A closed-loop HTTP benchmark. It creates only simulated rides, and cancels
// only its own assignments. Run it against the separate benchmark database.
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type ride struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	DriverID string `json:"driverId"`
}
type fleet struct {
	Drivers []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"drivers"`
	Rides []ride `json:"rides"`
	Mode  string `json:"mode"`
}
type sample struct {
	Round      int     `json:"round"`
	Index      int     `json:"index"`
	HTTPStatus int     `json:"httpStatus"`
	Outcome    string  `json:"outcome"`
	Latency    float64 `json:"latencyMs"`
	RideID     string  `json:"rideId,omitempty"`
	DriverID   string  `json:"driverId,omitempty"`
}
type summary struct {
	Requests      int     `json:"requests"`
	Assigned      int     `json:"assigned"`
	NoDriver      int     `json:"noDriver"`
	Errors        int     `json:"errors"`
	CleanupErrors int     `json:"cleanupErrors"`
	Duplicates    int     `json:"duplicateActiveAssignments"`
	P50           float64 `json:"p50Ms"`
	P95           float64 `json:"p95Ms"`
	P99           float64 `json:"p99Ms"`
	Elapsed       float64 `json:"elapsedSeconds"`
}
type report struct {
	CreatedAt        time.Time `json:"createdAt"`
	Version          string    `json:"version"`
	Storage          string    `json:"storage"`
	Scenario         string    `json:"scenario"`
	Concurrency      int       `json:"concurrency"`
	Rounds           int       `json:"rounds"`
	RequestsPerRound int       `json:"requestsPerRound"`
	Warmup           int       `json:"warmupRequests"`
	FleetSize        int       `json:"fleetSize"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	CPUs             int       `json:"logicalCPUs"`
	Go               string    `json:"goVersion"`
	Notes            string    `json:"notes"`
	Summary          summary   `json:"summary"`
	Samples          []sample  `json:"samples"`
}
type client struct {
	url  string
	http *http.Client
}

func (c client) request(method, path string, body any, target any) (int, error) {
	var reader io.Reader
	if body != nil {
		data, e := json.Marshal(body)
		if e != nil {
			return 0, e
		}
		reader = bytes.NewReader(data)
	}
	req, e := http.NewRequest(method, c.url+path, reader)
	if e != nil {
		return 0, e
	}
	req.Header.Set("Content-Type", "application/json")
	response, e := c.http.Do(req)
	if e != nil {
		return 0, e
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return response.StatusCode, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if target != nil {
		e = json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(target)
	} else {
		_, e = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	}
	return response.StatusCode, e
}
func (c client) dispatch(i, round int, mixed bool) sample {
	lat, lng := 12.9716+float64(i%5-2)*.002, 77.5946+float64(i%7-3)*.002
	if mixed && i%5 == 0 {
		lat, lng = 12.9698, 77.75
	}
	body := map[string]any{"pickup": map[string]float64{"lat": lat, "lng": lng}, "destination": map[string]float64{"lat": 12.9756, "lng": 77.6067}}
	start := time.Now()
	var r ride
	code, e := c.request("POST", "/api/rides", body, &r)
	s := sample{Round: round, Index: i, HTTPStatus: code, Outcome: r.Status, Latency: float64(time.Since(start).Nanoseconds()) / 1e6, RideID: r.ID, DriverID: r.DriverID}
	if e != nil || code != 201 || (r.Status != "assigned" && r.Status != "no_driver") || r.ID == "" {
		s.Outcome = "error"
	}
	return s
}
func (c client) release(s sample) error {
	if s.Outcome != "assigned" {
		return nil
	}
	_, e := c.request("PATCH", "/api/rides/"+s.RideID, map[string]string{"status": "cancelled"}, nil)
	return e
}
func quantile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[int(math.Ceil(float64(len(v))*p))-1]
}
func summarize(samples []sample) summary {
	s := summary{Requests: len(samples)}
	latencies := []float64{}
	for _, v := range samples {
		switch v.Outcome {
		case "assigned":
			s.Assigned++
		case "no_driver":
			s.NoDriver++
		default:
			s.Errors++
		}
		latencies = append(latencies, v.Latency)
	}
	sort.Float64s(latencies)
	s.P50 = quantile(latencies, .5)
	s.P95 = quantile(latencies, .95)
	s.P99 = quantile(latencies, .99)
	return s
}
func (c client) round(n, workers, round int, scenario string) ([]sample, int, int, error) {
	jobs := make(chan int, n)
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	results := make([]sample, n)
	var wg sync.WaitGroup
	var mu sync.Mutex
	cleanupErrors := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := c.dispatch(i, round, scenario == "mixed")
				results[i] = r
				if scenario == "lifecycle" {
					if e := c.release(r); e != nil {
						mu.Lock()
						cleanupErrors++
						mu.Unlock()
					}
				}
			}
		}()
	}
	wg.Wait()
	var state fleet
	_, e := c.request("GET", "/api/state", nil, &state)
	if e != nil {
		return results, cleanupErrors, 0, fmt.Errorf("integrity snapshot failed: %w", e)
	}
	active := map[string]int{}
	duplicates := 0
	for _, r := range state.Rides {
		if r.Status == "assigned" {
			active[r.DriverID]++
			if active[r.DriverID] > 1 {
				duplicates++
			}
		}
	}
	// Saturation and mixed scenarios retain all assignments until after the
	// integrity snapshot so contention remains visible for the entire round.
	if scenario != "lifecycle" {
		for _, r := range results {
			if e := c.release(r); e != nil {
				cleanupErrors++
			}
		}
	}
	return results, cleanupErrors, duplicates, nil
}
func writeReport(path string, r report) error {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	data, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(path, append(data, '\n'), 0644); e != nil {
		return e
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	f, e := os.Create(base + ".csv")
	if e != nil {
		return e
	}
	w := csv.NewWriter(f)
	w.Write([]string{"round", "index", "http_status", "outcome", "latency_ms", "ride_id", "driver_id"})
	for _, s := range r.Samples {
		w.Write([]string{fmt.Sprint(s.Round), fmt.Sprint(s.Index), fmt.Sprint(s.HTTPStatus), s.Outcome, fmt.Sprintf("%.6f", s.Latency), s.RideID, s.DriverID})
	}
	w.Flush()
	e = w.Error()
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	md := fmt.Sprintf("# HTTP benchmark — %s\n\nStorage: **%s**. Scenario: **%s**. %d rounds × %d requests, %d concurrent workers, %d warmup requests excluded. Fleet: %d drivers. Runtime: %s / %s, %s, %d logical CPUs.\n\n| Requests | Assigned | No driver | Errors | Cleanup errors | Duplicate active assignments |\n|---:|---:|---:|---:|---:|---:|\n| %d | %d | %d | %d | %d | %d |\n\n| POST p50 | POST p95 | POST p99 | Total experiment time |\n|---:|---:|---:|---:|\n| %.2f ms | %.2f ms | %.2f ms | %.2f s |\n\n%s\n\nRaw samples: [%s.csv](%s.csv). Machine-readable report: [%s.json](%s.json).\n", r.CreatedAt.Format(time.RFC3339), r.Storage, r.Scenario, r.Rounds, r.RequestsPerRound, r.Concurrency, r.Warmup, r.FleetSize, r.OS, r.Arch, r.Go, r.CPUs, r.Summary.Requests, r.Summary.Assigned, r.Summary.NoDriver, r.Summary.Errors, r.Summary.CleanupErrors, r.Summary.Duplicates, r.Summary.P50, r.Summary.P95, r.Summary.P99, r.Summary.Elapsed, r.Notes, filepath.Base(base), filepath.Base(base), filepath.Base(base), filepath.Base(base))
	return os.WriteFile(base+".md", []byte(md), 0644)
}
func main() {
	target := flag.String("url", "http://127.0.0.1:8081", "Direct Go API URL (dedicated benchmark instance)")
	n := flag.Int("requests", 100, "Requests per round, 1–5000")
	workers := flag.Int("concurrency", 10, "Concurrent clients, 1–100")
	rounds := flag.Int("rounds", 3, "Rounds, 1–20")
	warmup := flag.Int("warmup", 10, "Unmeasured requests, 0–100")
	scenario := flag.String("scenario", "saturation", "saturation, lifecycle, or mixed")
	output := flag.String("out", "../benchmarks/latest.json", "Report JSON path; also writes CSV and Markdown")
	flag.Parse()
	if *n < 1 || *n > 5000 || *workers < 1 || *workers > 100 || *rounds < 1 || *rounds > 20 || *warmup < 0 || *warmup > 100 || (*scenario != "saturation" && *scenario != "lifecycle" && *scenario != "mixed") {
		fmt.Fprintln(os.Stderr, "invalid benchmark parameters")
		os.Exit(2)
	}
	transport := &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: *workers, MaxConnsPerHost: *workers}
	defer transport.CloseIdleConnections()
	c := client{strings.TrimRight(*target, "/"), &http.Client{Transport: transport, Timeout: 15 * time.Second}}
	var state fleet
	if _, e := c.request("GET", "/api/state", nil, &state); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	for _, d := range state.Drivers {
		if d.Status != "available" {
			fmt.Fprintln(os.Stderr, "benchmark requires an idle fleet; release active rides first")
			os.Exit(1)
		}
	}
	for i := 0; i < *warmup; i++ {
		s := c.dispatch(i, 0, false)
		if s.Outcome == "error" || c.release(s) != nil {
			fmt.Fprintln(os.Stderr, "warmup failed")
			os.Exit(1)
		}
	}
	r := report{CreatedAt: time.Now().UTC(), Version: "0.3.0", Storage: state.Mode, Scenario: *scenario, Concurrency: *workers, Rounds: *rounds, RequestsPerRound: *n, Warmup: *warmup, FleetSize: len(state.Drivers), OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Go: runtime.Version(), Samples: []sample{}, Notes: "Closed-loop local HTTP experiment against the direct Go API, excluding Next.js. POST latency includes connection, response headers/body, and JSON decoding; cancellation, integrity snapshots, and warmup are excluded from individual POST samples. Percentiles include failed requests if any. Total experiment time includes cancellation and integrity snapshots. No requests-per-second capacity claim is made. Saturation retains reservations until round end; lifecycle cancels each assigned ride before that worker's next POST. Mixed demand uses every fifth pickup outside the original fleet radius. Existing history, logging, pool size, and cache state can affect results. This does not model real traffic, routing, or production capacity."}
	start := time.Now()
	cleanupErrors, duplicates := 0, 0
	for i := 1; i <= *rounds; i++ {
		samples, cleanup, dups, e := c.round(*n, *workers, i, *scenario)
		r.Samples = append(r.Samples, samples...)
		cleanupErrors += cleanup
		duplicates += dups
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	r.Summary = summarize(r.Samples)
	r.Summary.Elapsed = time.Since(start).Seconds()
	r.Summary.CleanupErrors = cleanupErrors
	r.Summary.Duplicates = duplicates
	if e := writeReport(*output, r); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Printf("%s: %d requests, %d assigned, %d no driver, %d errors; p95 %.2f ms; report %s\n", *scenario, r.Summary.Requests, r.Summary.Assigned, r.Summary.NoDriver, r.Summary.Errors, r.Summary.P95, *output)
	if r.Summary.Errors > 0 || cleanupErrors > 0 || duplicates > 0 {
		os.Exit(1)
	}
}
