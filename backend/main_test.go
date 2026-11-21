package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

var center = Point{12.9716, 77.5946}

func TestConcurrentDispatch(t *testing.T) {
	s := newMemory()
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]bool{}
	matches := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Dispatch(context.Background(), center, center)
			if e != nil {
				t.Error(e)
				return
			}
			if r.Status == "assigned" {
				mu.Lock()
				defer mu.Unlock()
				if seen[r.DriverID] {
					t.Errorf("double reservation: %s", r.DriverID)
				}
				seen[r.DriverID] = true
				matches++
			}
		}()
	}
	wg.Wait()
	if matches != 12 {
		t.Fatalf("matched %d; want 12", matches)
	}
}
func TestLifecycle(t *testing.T) {
	s := newMemory()
	r, _ := s.Dispatch(context.Background(), center, Point{12.95, 77.61})
	if r.Status != "assigned" {
		t.Fatal(r)
	}
	if _, e := s.Transition(context.Background(), r.ID, "completed"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Transition(context.Background(), r.ID, "cancelled"); e != errConflict {
		t.Fatalf("want conflict; got %v", e)
	}
	st, _ := s.Snapshot(context.Background())
	for _, d := range st.Drivers {
		if d.ID == r.DriverID && (d.Status != "available" || d.Lat != 12.95) {
			t.Fatal("driver not released at destination")
		}
	}
}
func TestNearestAndRadius(t *testing.T) {
	s := newMemory()
	r, _ := s.Dispatch(context.Background(), s.drivers[3].Point, center)
	if r.DriverID != s.drivers[3].ID || r.Distance != 0 {
		t.Fatal("did not choose nearest driver")
	}
	r, _ = s.Dispatch(context.Background(), Point{0, 0}, center)
	if r.Status != "no_driver" || r.DriverID != "" {
		t.Fatal("expected no driver outside radius")
	}
}
func TestHTTP(t *testing.T) {
	h := api(newMemory())
	for _, body := range []string{`{}`, `{"pickup":{"lat":91,"lng":1},"destination":{"lat":1,"lng":1}}`, `garbage`} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/rides", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatalf("bad input accepted: %s", body)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/rides", bytes.NewBufferString(`{"pickup":{"lat":12.9716,"lng":77.5946},"destination":{"lat":12.95,"lng":77.61}}`)))
	var r Ride
	json.Unmarshal(w.Body.Bytes(), &r)
	if w.Code != 201 || r.DriverID == "" {
		t.Fatalf("bad response: %d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PATCH", "/api/rides/missing", bytes.NewBufferString(`{"status":"completed"}`)))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestPostgresConcurrentDispatch(t *testing.T) {
	url := os.Getenv("VYRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set VYRO_TEST_DATABASE_URL to a dedicated test database with schema applied")
	}
	ctx := context.Background()
	config, e := pgx.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(config.Database, "_test") {
		t.Fatal("integration test requires a database name ending in _test")
	}
	s, e := newPostgres(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer s.pool.Close()
	var dbName string
	if e = s.pool.QueryRow(ctx, "SELECT current_database()").Scan(&dbName); e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatal("integration test requires a database name ending in _test")
	}
	// Explicit test database only: delete ride and driver test fixtures.
	if _, e = s.pool.Exec(ctx, "TRUNCATE simulations,rides,drivers"); e != nil {
		t.Fatal(e)
	}
	s.pool.Close()
	s, e = newPostgres(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer s.pool.Close()
	var wg sync.WaitGroup
	other, e := newPostgres(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer other.pool.Close()
	var mu sync.Mutex
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pool := s
			if i%2 == 1 {
				pool = other
			}
			r, e := pool.Dispatch(ctx, center, center)
			if e != nil {
				t.Error(e)
				return
			}
			if r.DriverID != "" {
				mu.Lock()
				defer mu.Unlock()
				if seen[r.DriverID] {
					t.Error("double assignment")
				}
				seen[r.DriverID] = true
			}
		}(i)
	}
	wg.Wait()
	if len(seen) != 12 {
		t.Fatalf("assigned %d drivers; want 12", len(seen))
	}
	st, e := s.Snapshot(ctx)
	if e != nil || len(st.Rides) != 50 {
		t.Fatalf("snapshot: %d rides, %v", len(st.Rides), e)
	}
	for _, r := range st.Rides {
		if r.Status == "assigned" {
			if _, e = s.Transition(ctx, r.ID, "completed"); e != nil {
				t.Fatal(e)
			}
			break
		}
	}
	// Confirm cancel/release and rejection of a second terminal transition.
	for _, r := range st.Rides {
		if r.Status == "assigned" {
			current, _ := s.Snapshot(ctx)
			var active bool
			for _, x := range current.Rides {
				if x.ID == r.ID {
					active = x.Status == "assigned"
				}
			}
			if !active {
				continue
			}
			if _, e = s.Transition(ctx, r.ID, "cancelled"); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Transition(ctx, r.ID, "completed"); e != errConflict {
				t.Fatalf("expected transition conflict, got %v", e)
			}
		}
	}
	current, e := s.Snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	nearest, e := s.Dispatch(ctx, current.Drivers[0].Point, center)
	if e != nil || nearest.DriverID != current.Drivers[0].ID || nearest.Distance != 0 {
		t.Fatalf("nearest driver: %+v, %v", nearest, e)
	}
	if _, e = s.Transition(ctx, nearest.ID, "cancelled"); e != nil {
		t.Fatal(e)
	}
	far, e := s.Dispatch(ctx, Point{0, 0}, center)
	if e != nil || far.Status != "no_driver" {
		t.Fatalf("radius check: %+v %v", far, e)
	}
	// A new pool sees committed rides and reports from the original pools.
	run := runSimulation(ctx, s, 25, "mixed")
	if run.Errors != 0 || !run.IntegrityChecked || run.DuplicateAssignments != 0 {
		t.Fatalf("database simulation: %+v", run)
	}
	if e = s.SaveSimulation(ctx, run); e != nil {
		t.Fatal(e)
	}
	reopened, e := newPostgres(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.pool.Close()
	persisted, e := reopened.Snapshot(ctx)
	if e != nil || len(persisted.Rides) != 77 || len(persisted.Simulations) != 1 || persisted.Simulations[0].ID != run.ID {
		t.Fatalf("persistence: %d rides, %d simulations, %v", len(persisted.Rides), len(persisted.Simulations), e)
	}
}
