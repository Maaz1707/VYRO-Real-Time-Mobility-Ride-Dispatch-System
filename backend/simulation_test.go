package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSimulation(t *testing.T) {
	s := newMemory()
	run := runSimulation(context.Background(), s, 50, "central")
	if run.Assigned != 12 || run.NoDriver != 38 || run.Errors != 0 || !run.IntegrityChecked || run.DuplicateAssignments != 0 {
		t.Fatalf("unexpected report: %+v", run)
	}
	if len(run.RideIDs) != 50 || run.P95 < run.P50 {
		t.Fatal("invalid timing or ride accounting")
	}
	s.SaveSimulation(context.Background(), run)
	st, _ := s.Snapshot(context.Background())
	if len(st.Simulations) != 1 {
		t.Fatal("report not saved")
	}
	// Cancelling the run's active rides makes another experiment possible.
	for _, r := range st.Rides {
		if r.Status == "assigned" {
			if _, e := s.Transition(context.Background(), r.ID, "cancelled"); e != nil {
				t.Fatal(e)
			}
		}
	}
	mixed := runSimulation(context.Background(), s, 10, "mixed")
	if mixed.Assigned != 8 || mixed.NoDriver != 2 {
		t.Fatalf("mixed scenario: %+v", mixed)
	}
}

func TestSimulationHTTP(t *testing.T) {
	s := newMemory()
	h := api(s)
	for _, body := range []string{`{}`, `{"requests":101,"scenario":"central"}`, `{"requests":10,"scenario":"unknown"}`} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/simulations", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatalf("invalid request returned %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/simulations", bytes.NewBufferString(`{"requests":25,"scenario":"central"}`)))
	var run Simulation
	if e := json.Unmarshal(w.Body.Bytes(), &run); e != nil {
		t.Fatal(e)
	}
	if w.Code != 201 || run.Assigned != 12 || run.NoDriver != 13 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
