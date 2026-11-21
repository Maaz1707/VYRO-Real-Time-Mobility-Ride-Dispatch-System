CREATE EXTENSION IF NOT EXISTS postgis;
CREATE TABLE IF NOT EXISTS drivers (
 id text PRIMARY KEY, name text NOT NULL, vehicle text NOT NULL,
 location geography(Point,4326) NOT NULL,
 status text NOT NULL CHECK(status IN ('available','busy'))
);
CREATE INDEX IF NOT EXISTS drivers_location_idx ON drivers USING gist(location);
CREATE TABLE IF NOT EXISTS rides (
 id text PRIMARY KEY, pickup_lat double precision NOT NULL, pickup_lng double precision NOT NULL,
 dest_lat double precision NOT NULL, dest_lng double precision NOT NULL,
 status text NOT NULL CHECK(status IN ('assigned','completed','cancelled','no_driver')),
 driver_id text REFERENCES drivers(id), driver_name text NOT NULL DEFAULT '',
 distance_km double precision NOT NULL DEFAULT 0, eta_minutes integer NOT NULL DEFAULT 0,
 dispatch_ms double precision NOT NULL, created_at timestamptz NOT NULL,
 CHECK ((status='no_driver' AND driver_id IS NULL) OR (status<>'no_driver' AND driver_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_ride_per_driver ON rides(driver_id) WHERE status='assigned';
CREATE TABLE IF NOT EXISTS simulations (
 id text PRIMARY KEY,
 created_at timestamptz NOT NULL,
 report jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS simulations_created_at_idx ON simulations(created_at DESC);
