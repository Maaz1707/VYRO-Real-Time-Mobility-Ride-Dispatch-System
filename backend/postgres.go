package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Postgres struct{ pool *pgxpool.Pool }

func (s *Postgres) Ready(ctx context.Context) error { return s.pool.Ping(ctx) }
func (s *Postgres) Mode() string                    { return "postgres" }

func newPostgres(ctx context.Context, url string) (*Postgres, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	s := &Postgres{p}
	for _, d := range seedDrivers() {
		_, e = p.Exec(ctx, `INSERT INTO drivers(id,name,vehicle,location,status) VALUES($1,$2,$3,ST_SetSRID(ST_MakePoint($4,$5),4326)::geography,'available') ON CONFLICT(id) DO NOTHING`, d.ID, d.Name, d.Vehicle, d.Lng, d.Lat)
		if e != nil {
			p.Close()
			return nil, e
		}
	}
	return s, nil
}
func (s *Postgres) Snapshot(ctx context.Context) (State, error) {
	st := State{Drivers: []Driver{}, Rides: []Ride{}, Mode: "postgres", Simulations: []Simulation{}}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return st, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT id,name,vehicle,ST_Y(location::geometry),ST_X(location::geometry),status FROM drivers ORDER BY id`)
	if e != nil {
		return st, e
	}
	for rows.Next() {
		var d Driver
		if e = rows.Scan(&d.ID, &d.Name, &d.Vehicle, &d.Lat, &d.Lng, &d.Status); e != nil {
			rows.Close()
			return st, e
		}
		st.Drivers = append(st.Drivers, d)
	}
	if e = rows.Err(); e != nil {
		return st, e
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT id,pickup_lat,pickup_lng,dest_lat,dest_lng,status,COALESCE(driver_id,''),driver_name,distance_km,eta_minutes,dispatch_ms,created_at FROM rides ORDER BY created_at DESC`)
	if e != nil {
		return st, e
	}
	for rows.Next() {
		var r Ride
		if e = rows.Scan(&r.ID, &r.Pickup.Lat, &r.Pickup.Lng, &r.Destination.Lat, &r.Destination.Lng, &r.Status, &r.DriverID, &r.DriverName, &r.Distance, &r.ETA, &r.Latency, &r.CreatedAt); e != nil {
			rows.Close()
			return st, e
		}
		st.Rides = append(st.Rides, r)
	}
	if e = rows.Err(); e != nil {
		return st, e
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT report FROM simulations ORDER BY created_at DESC LIMIT 10`)
	if e != nil {
		return st, e
	}
	for rows.Next() {
		var payload []byte
		var run Simulation
		if e = rows.Scan(&payload); e != nil {
			rows.Close()
			return st, e
		}
		if e = json.Unmarshal(payload, &run); e != nil {
			rows.Close()
			return st, e
		}
		st.Simulations = append(st.Simulations, run)
	}
	if e = rows.Err(); e != nil {
		return st, e
	}
	rows.Close()
	return st, tx.Commit(ctx)
}
func (s *Postgres) SaveSimulation(ctx context.Context, run Simulation) error {
	payload, e := json.Marshal(run)
	if e != nil {
		return e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO simulations(id,created_at,report) VALUES($1,$2,$3)`, run.ID, run.CreatedAt, payload)
	return e
}
func (s *Postgres) Dispatch(ctx context.Context, p, d Point) (Ride, error) {
	start := time.Now()
	r := makeRide(p, d)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return r, e
	}
	defer tx.Rollback(ctx)
	var driver Driver
	var metres float64
	// Keep ST_DWithin's indexed radius filter, then sort the bounded candidates
	// by exact distance. KNN (<->) plus row locking hits an executor bug on the
	// installed PostgreSQL 17.11 build. The ID provides deterministic ties.
	e = tx.QueryRow(ctx, `SELECT id,name,ST_Distance(location,ST_SetSRID(ST_MakePoint($1,$2),4326)::geography) AS distance_m FROM drivers WHERE status='available' AND ST_DWithin(location,ST_SetSRID(ST_MakePoint($1,$2),4326)::geography,5000) ORDER BY distance_m,id LIMIT 1 FOR UPDATE SKIP LOCKED`, p.Lng, p.Lat).Scan(&driver.ID, &driver.Name, &metres)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return r, e
	}
	if e == nil {
		assign(&r, driver, metres/1000)
		if _, e = tx.Exec(ctx, `UPDATE drivers SET status='busy' WHERE id=$1`, driver.ID); e != nil {
			return r, e
		}
	}
	r.Latency = float64(time.Since(start).Microseconds()) / 1000
	_, e = tx.Exec(ctx, `INSERT INTO rides(id,pickup_lat,pickup_lng,dest_lat,dest_lng,status,driver_id,driver_name,distance_km,eta_minutes,dispatch_ms,created_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12)`, r.ID, p.Lat, p.Lng, d.Lat, d.Lng, r.Status, r.DriverID, r.DriverName, r.Distance, r.ETA, r.Latency, r.CreatedAt)
	if e != nil {
		return r, e
	}
	return r, tx.Commit(ctx)
}
func (s *Postgres) Transition(ctx context.Context, id, status string) (Ride, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Ride{}, e
	}
	defer tx.Rollback(ctx)
	var r Ride
	e = tx.QueryRow(ctx, `SELECT id,pickup_lat,pickup_lng,dest_lat,dest_lng,status,COALESCE(driver_id,''),driver_name,distance_km,eta_minutes,dispatch_ms,created_at FROM rides WHERE id=$1 FOR UPDATE`, id).Scan(&r.ID, &r.Pickup.Lat, &r.Pickup.Lng, &r.Destination.Lat, &r.Destination.Lng, &r.Status, &r.DriverID, &r.DriverName, &r.Distance, &r.ETA, &r.Latency, &r.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, errNotFound
	}
	if e != nil {
		return r, e
	}
	if r.Status != "assigned" {
		return r, errConflict
	}
	if _, e = tx.Exec(ctx, `UPDATE rides SET status=$2 WHERE id=$1`, id, status); e != nil {
		return r, e
	}
	if status == "completed" {
		_, e = tx.Exec(ctx, `UPDATE drivers SET status='available',location=ST_SetSRID(ST_MakePoint($2,$3),4326)::geography WHERE id=$1`, r.DriverID, r.Destination.Lng, r.Destination.Lat)
	} else {
		_, e = tx.Exec(ctx, `UPDATE drivers SET status='available' WHERE id=$1`, r.DriverID)
	}
	if e != nil {
		return r, e
	}
	r.Status = status
	return r, tx.Commit(ctx)
}
