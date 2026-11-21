package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Timings wrap the entire Store.Dispatch call, including PostgreSQL commit.
// This is a bounded, single-process experiment, not an HTTP load benchmark.
type Simulation struct {
	ID                   string    `json:"id"`
	Scenario             string    `json:"scenario"`
	Requests             int       `json:"requests"`
	Workers              int       `json:"workers"`
	Assigned             int       `json:"assigned"`
	NoDriver             int       `json:"noDriver"`
	Errors               int       `json:"errors"`
	P50                  float64   `json:"p50Ms"`
	P95                  float64   `json:"p95Ms"`
	Elapsed              float64   `json:"elapsedMs"`
	DuplicateAssignments int       `json:"duplicateAssignments"`
	IntegrityChecked     bool      `json:"integrityChecked"`
	RideIDs              []string  `json:"rideIds"`
	CreatedAt            time.Time `json:"createdAt"`
}

func (s *Memory) SaveSimulation(_ context.Context, run Simulation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulations = append([]Simulation{run}, s.simulations...)
	return nil
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[int(math.Ceil(float64(len(values))*p))-1]
}

func runSimulation(ctx context.Context, s Store, count int, scenario string) Simulation {
	start := time.Now()
	run := Simulation{ID: id(), Scenario: scenario, Requests: count, Workers: min(10, count), CreatedAt: time.Now().UTC(), RideIDs: []string{}}
	jobs := make(chan int, count)
	for i := 0; i < count; i++ {
		jobs <- i
	}
	close(jobs)
	var mu sync.Mutex
	var wg sync.WaitGroup
	timings := []float64{}
	for w := 0; w < run.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// Fixed, reproducible request coordinates. The mixed scenario puts every
				// fifth pickup in Whitefield, beyond the initial fleet's search radius.
				p := Point{12.9716 + float64(i%5-2)*.002, 77.5946 + float64(i%7-3)*.002}
				if scenario == "mixed" && i%5 == 0 {
					p = Point{12.9698, 77.75}
				}
				destination := Point{12.9756, 77.6067}
				before := time.Now()
				r, e := s.Dispatch(ctx, p, destination)
				ms := float64(time.Since(before).Nanoseconds()) / 1e6
				mu.Lock()
				if e != nil {
					run.Errors++
				} else {
					timings = append(timings, ms)
					run.RideIDs = append(run.RideIDs, r.ID)
					if r.Status == "assigned" {
						run.Assigned++
					} else {
						run.NoDriver++
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Float64s(timings)
	run.P50 = percentile(timings, .5)
	run.P95 = percentile(timings, .95)
	run.Elapsed = float64(time.Since(start).Nanoseconds()) / 1e6
	if snapshot, e := s.Snapshot(ctx); e == nil {
		run.IntegrityChecked = true
		active := map[string]int{}
		for _, r := range snapshot.Rides {
			if r.Status == "assigned" {
				active[r.DriverID]++
				if active[r.DriverID] > 1 {
					run.DuplicateAssignments++
				}
			}
		}
	}
	return run
}

func registerSimulations(mux *http.ServeMux, s Store) {
	// Only one burst per API process. Database row locking still coordinates
	// dispatches across processes; this gate just bounds demo resource use.
	gate := make(chan struct{}, 1)
	mux.HandleFunc("POST /api/simulations", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Requests int    `json:"requests"`
			Scenario string `json:"scenario"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || body.Requests < 1 || body.Requests > 100 || (body.Scenario != "central" && body.Scenario != "mixed") {
			write(w, 400, map[string]string{"error": "requests must be 1–100 and scenario central or mixed"})
			return
		}
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			write(w, 409, map[string]string{"error": "a simulation is already running"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		run := runSimulation(ctx, s, body.Requests, body.Scenario)
		if e := s.SaveSimulation(ctx, run); e != nil {
			write(w, 500, map[string]any{"error": "Rides may have been created, but the simulation report could not be saved. Refresh before retrying.", "result": run})
			return
		}
		write(w, 201, run)
	})
}
