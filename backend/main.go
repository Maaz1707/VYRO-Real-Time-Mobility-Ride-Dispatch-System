package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}
type Driver struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Vehicle string `json:"vehicle"`
	Point
	Status string `json:"status"`
}
type Ride struct {
	ID          string    `json:"id"`
	Pickup      Point     `json:"pickup"`
	Destination Point     `json:"destination"`
	Status      string    `json:"status"`
	DriverID    string    `json:"driverId"`
	DriverName  string    `json:"driverName"`
	Distance    float64   `json:"pickupDistanceKm"`
	ETA         int       `json:"etaMinutes"`
	Latency     float64   `json:"dispatchMs"`
	CreatedAt   time.Time `json:"createdAt"`
}
type State struct {
	Drivers     []Driver     `json:"drivers"`
	Rides       []Ride       `json:"rides"`
	Mode        string       `json:"mode"`
	Simulations []Simulation `json:"simulations"`
}
type Store interface {
	Ready(context.Context) error
	Mode() string
	Snapshot(context.Context) (State, error)
	Dispatch(context.Context, Point, Point) (Ride, error)
	Transition(context.Context, string, string) (Ride, error)
	SaveSimulation(context.Context, Simulation) error
}

var errNotFound = errors.New("ride not found")
var errConflict = errors.New("only assigned rides can be completed or cancelled")

type Memory struct {
	mu          sync.Mutex
	drivers     []Driver
	rides       []Ride
	simulations []Simulation
}

func newMemory() *Memory {
	return &Memory{drivers: seedDrivers(), rides: []Ride{}, simulations: []Simulation{}}
}
func (s *Memory) Ready(context.Context) error { return nil }
func (s *Memory) Mode() string                { return "memory" }
func seedDrivers() []Driver {
	names := []string{"Aarav", "Ishaan", "Kabir", "Neha", "Rohan", "Ananya", "Dev", "Meera", "Arjun", "Zoya", "Vikram", "Diya"}
	offsets := [][2]float64{{.005, .003}, {-.009, .008}, {.014, -.006}, {-.003, -.012}, {.022, .018}, {-.018, -.004}, {.009, .023}, {-.024, .015}, {.032, -.015}, {-.012, -.023}, {.018, -.028}, {-.031, -.013}}
	result := []Driver{}
	for i, n := range names {
		result = append(result, Driver{ID: n, Name: n, Vehicle: []string{"Swift Dzire", "Hyundai Aura", "Toyota Etios"}[i%3], Point: Point{12.9716 + offsets[i][0], 77.5946 + offsets[i][1]}, Status: "available"})
	}
	return result
}
func distance(a, b Point) float64 {
	r := math.Pi / 180
	dlat := (b.Lat - a.Lat) * r
	dlng := (b.Lng - a.Lng) * r
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(a.Lat*r)*math.Cos(b.Lat*r)*math.Sin(dlng/2)*math.Sin(dlng/2)
	return 6371 * 2 * math.Asin(math.Sqrt(math.Min(1, h)))
}
func id() string {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func makeRide(p, d Point) Ride {
	return Ride{ID: id(), Pickup: p, Destination: d, Status: "no_driver", CreatedAt: time.Now().UTC()}
}
func assign(r *Ride, d Driver, km float64) {
	r.Status = "assigned"
	r.DriverID = d.ID
	r.DriverName = d.Name
	r.Distance = math.Round(km*100) / 100
	r.ETA = int(math.Max(1, math.Ceil(km/25*60)))
}
func (s *Memory) Snapshot(_ context.Context) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return State{Drivers: append([]Driver{}, s.drivers...), Rides: append([]Ride{}, s.rides...), Mode: "memory", Simulations: append([]Simulation{}, s.simulations...)}, nil
}
func (s *Memory) Dispatch(_ context.Context, p, d Point) (Ride, error) {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	r := makeRide(p, d)
	best := -1
	km := 5.0
	for i, v := range s.drivers {
		if v.Status == "available" {
			dist := distance(p, v.Point)
			if dist <= km {
				best = i
				km = dist
			}
		}
	}
	if best >= 0 {
		assign(&r, s.drivers[best], km)
		s.drivers[best].Status = "busy"
	}
	r.Latency = float64(time.Since(start).Microseconds()) / 1000
	s.rides = append([]Ride{r}, s.rides...)
	return r, nil
}
func (s *Memory) Transition(_ context.Context, rid, status string) (Ride, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rides {
		r := &s.rides[i]
		if r.ID != rid {
			continue
		}
		if r.Status != "assigned" {
			return Ride{}, errConflict
		}
		r.Status = status
		for j := range s.drivers {
			if s.drivers[j].ID == r.DriverID {
				s.drivers[j].Status = "available"
				if status == "completed" {
					s.drivers[j].Point = r.Destination
				}
			}
		}
		return *r, nil
	}
	return Ride{}, errNotFound
}
func valid(p Point) bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lng) && p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func api(store Store) http.Handler {
	metrics := newTelemetry()
	store = &observedStore{Store: store, metrics: metrics}
	mux := http.NewServeMux()
	registerOperations(mux, store, metrics)
	registerSimulations(mux, store)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		e := store.Ready(ctx)
		if e != nil {
			write(w, 503, map[string]string{"error": "storage unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ok", "storage": store.Mode(), "version": "0.3.0"})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s, e := store.Snapshot(r.Context())
		if e != nil {
			log.Print(e)
			write(w, 500, map[string]string{"error": "could not load fleet"})
			return
		}
		write(w, 200, s)
	})
	mux.HandleFunc("POST /api/rides", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Pickup      *Point `json:"pickup"`
			Destination *Point `json:"destination"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil || body.Pickup == nil || body.Destination == nil || !valid(*body.Pickup) || !valid(*body.Destination) {
			write(w, 400, map[string]string{"error": "valid pickup and destination coordinates required"})
			return
		}
		ride, e := store.Dispatch(r.Context(), *body.Pickup, *body.Destination)
		if e != nil {
			log.Print(e)
			write(w, 500, map[string]string{"error": "dispatch failed"})
			return
		}
		write(w, 201, ride)
	})
	mux.HandleFunc("PATCH /api/rides/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status string `json:"status"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || (body.Status != "completed" && body.Status != "cancelled") {
			write(w, 400, map[string]string{"error": "status must be completed or cancelled"})
			return
		}
		ride, e := store.Transition(r.Context(), r.PathValue("id"), body.Status)
		if e != nil {
			code := 500
			if errors.Is(e, errConflict) {
				code = 409
			}
			if errors.Is(e, errNotFound) {
				code = 404
			}
			write(w, code, map[string]string{"error": http.StatusText(code)})
			return
		}
		write(w, 200, ride)
	})
	return metrics.handler(mux)
}
func main() {
	var store Store = newMemory()
	if url := os.Getenv("DATABASE_URL"); url != "" {
		s, e := newPostgres(context.Background(), url)
		if e != nil {
			log.Fatal(e)
		}
		defer s.pool.Close()
		store = s
	}
	addr := os.Getenv("VYRO_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	log.Printf("VYRO dispatch listening on http://%s", addr)
	log.Fatal((&http.Server{Addr: addr, Handler: api(store), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 45 * time.Second}).ListenAndServe())
}
