package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Check the benchmark itself against a tiny contended fleet: saturation
// must retain reservations until the batch ends, then release only its rides.
func TestRoundCleanupAndAccounting(t *testing.T) {
	var mu sync.Mutex
	active := map[string]string{}
	rides := []ride{}
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "POST":
			next++
			v := ride{ID: fmtID(next), Status: "no_driver"}
			if len(active) < 2 {
				v.Status = "assigned"
				v.DriverID = fmtID(len(active))
				active[v.ID] = v.DriverID
			}
			rides = append(rides, v)
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(v)
		case "PATCH":
			delete(active, r.PathValue("id"))
			for key := range active {
				if r.URL.Path == "/api/rides/"+key {
					delete(active, key)
					break
				}
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
		default:
			var state fleet
			for _, v := range rides {
				if _, ok := active[v.ID]; ok {
					state.Rides = append(state.Rides, v)
				}
			}
			json.NewEncoder(w).Encode(state)
		}
	}))
	defer server.Close()
	c := client{server.URL, server.Client()}
	samples, cleanup, duplicates, e := c.round(10, 5, 1, "saturation")
	if e != nil {
		t.Fatal(e)
	}
	s := summarize(samples)
	if s.Assigned != 2 || s.NoDriver != 8 || s.Errors != 0 || cleanup != 0 || duplicates != 0 || len(active) != 0 {
		t.Fatalf("summary %+v, cleanup %d, duplicates %d, active %d", s, cleanup, duplicates, len(active))
	}
}
func fmtID(n int) string { return string(rune('A' + n)) }
