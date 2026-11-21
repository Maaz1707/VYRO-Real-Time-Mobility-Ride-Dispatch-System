# VYRO — Mobility dispatch lab

A ride-dispatch prototype built with Go, Next.js, PostgreSQL, and PostGIS. Request a ride, reserve a nearby driver, and test concurrent demand against a simulated fleet. No Docker, map key, paid service, or real bookings.

Phase 3 adds client-side HTTP benchmarks, structured request logs, readiness/liveness checks, a service-pulse dashboard, Prometheus-format metrics, and a GitHub Actions workflow. Local tests and builds pass; the workflow is configured but has not been run on GitHub. There is no public deployment.

**Review the evidence:** [architecture and tradeoffs](docs/ARCHITECTURE.md), [benchmark results and raw samples](benchmarks/README.md), [90-second demo script and resume bullets](docs/DEMO.md).

## Start

The original development machine's project database is already configured. From this folder, use two PowerShell terminals:

```powershell
# Terminal 1: automatically starts the project database, then the Go API
.\start-backend.ps1
# Terminal 2
.\start-dashboard.ps1
```

Open http://127.0.0.1:3000. The ride panel should show **PostgreSQL + PostGIS / Persistent state**.

On another machine, install Go 1.24+, Node.js 22.17+, and PostgreSQL 17. The Windows setup also uses an existing Git for Windows installation to supply one DLL missing from the PostGIS ZIP. Then run `./setup-database.ps1` once before the commands above. Supply `-PostgreSQLHome` if PostgreSQL 17 is installed elsewhere, or `-Port` when initializing on a different port.

Setup copies PostgreSQL binaries into `.local/postgres`, adds the official [PostGIS Windows bundle](https://postgis.net/documentation/getting_started/install_windows/), and initializes a separate cluster on `127.0.0.1:55432`. Existing PostgreSQL services, databases, users, and authentication settings are untouched. Random credentials, binaries, logs, and data stay under the Git-ignored `.local/` directory. Authentication uses SCRAM. The API uses a non-superuser application role; tests have a separate database and role. This unencrypted loopback setup is for local development.

Use `./stop-database.ps1` and `./start-database.ps1` to stop/start only VYRO's database. Stop application services with Ctrl+C. `start-backend.ps1 -Memory` runs without a database; all memory-mode state resets on backend restart. On the original machine, scripts can discover the portable Go SDK under the workspace's `work/tools` folder. That SDK is not part of the project; install Go when moving it.

To use an existing PostGIS database, apply `database/schema.sql` and grant the API role table access, then set `DATABASE_URL` before starting the backend. An explicitly set URL overrides the project database. A failed configured connection fails startup rather than silently switching to memory. Rerunning setup reapplies the idempotent schema for the project databases.

## Demonstrate it

1. Request MG Road → Koramangala. A nearby driver becomes reserved.
2. Complete or cancel the ride. Its driver becomes available; completion moves the driver to the destination.
3. Open **Dispatch laboratory** and run **50 requests / central demand**. From the initial available fleet, expect 12 assignments and 38 unavailable outcomes.
4. Inspect the p50/p95 operation timing, error count, and duplicate active-assignment check.
5. **Release this run's drivers** cancels its active rides without deleting history.
6. Run **10 requests / mixed coverage**. From the original available fleet, expect 8 assignments and 2 distant pickups with no driver.
7. Restart the backend and database, then reload: rides and saved experiment reports remain.

Runs use the current fleet; existing reservations and driver locations affect results. Releases affect only active rides from that run. Saved measurements describe the run-time snapshot and do not change after cancellation. The API returns the latest ten database reports; the dashboard shows the latest plus up to five previous runs.

## Architecture

```text
Browser → Next.js API proxy → Go HTTP API → Store interface
                                             ├─ Memory: mutex-protected fleet
                                             └─ PostgreSQL + PostGIS
                                                ├─ Spatial discovery
                                                ├─ Atomic reservations
                                                └─ Rides + experiment reports
```

- `ST_DWithin` filters available drivers within 5 km; a GiST index supports radius discovery. Small tables may use a sequential scan if the planner estimates it is cheaper.
- Exact `ST_Distance` ranks candidates, with ID as a deterministic tie-breaker.
- `FOR UPDATE SKIP LOCKED` selects an unlocked eligible driver. Availability update and ride creation commit together.
- A partial unique index on `rides(driver_id) WHERE status='assigned'` additionally prevents two active rides for one driver.
- Completion/cancellation locks the ride, rejects repeated terminal transitions, and releases the driver in one transaction. Completion also updates location.
- Repeatable-read snapshots keep fleet, rides, and experiment history consistent within a response.
- Burst experiments use up to ten concurrent goroutines calling the real dispatch path. Reports persist in JSONB. One burst per API process is allowed, with a maximum of 100 requests and a 30-second context deadline.

The original KNN `<->` sort combined with row locking failed on the installed PostgreSQL 17.11 build. Exact-distance sorting avoids that executor path while retaining indexed radius discovery. PostgreSQL's mailing list documents the related [KNN/row-locking issue](https://www.postgresql.org/message-id/CAM6Zo8wZOLnCWRO_tuuXVX9J4N4JN6GsEnk8WJtT0%3D_0zy-1dw%40mail.gmail.com). Real database integration tests exposed the problem and verified the revised query.

Twelve stationary drivers are seeded around central Bengaluru. Request coordinates are fixed for reproducibility; mixed demand places every fifth pickup in Whitefield. The dashboard polls every two seconds. Streets are schematic; pickup ETA uses straight-line proximity at an assumed 25 km/h, rounded up to at least one minute. This is not road routing or GPS streaming.

## Measurements

| Metric | Definition |
|---|---|
| Ride `dispatchMs` / dashboard average | Matching/reservation preparation including lock wait, before final ride insertion and commit; the average includes matched requests only. |
| Experiment p50/p95 operation | Entire successful `Store.Dispatch` calls through commit, including pool wait. Includes assigned/no-driver outcomes; excludes errors and HTTP transport. Nearest-rank percentiles. |
| Burst duration | Dispatch batch wall time, excluding the final integrity snapshot and report insert. |
| Assignment rate | Matched requests divided by all ride records in the current store. |
| Integrity check | Duplicate active driver assignments in a consistent snapshot after the burst. Marked unavailable if snapshot retrieval fails. |

These bounded local experiments are not production performance claims or HTTP load benchmarks. Worker calls can queue behind the database pool. Results depend on hardware, cache state, current fleet, and other activity. Requests with no available/unlocked eligible driver produce `no_driver`; no dispatch retry queue is implemented.

## API

| Method | Route | Input/purpose |
|---|---|---|
| GET | `/api/health` | Storage readiness |
| GET | `/api/live` | Process liveness independent of storage |
| GET | `/api/ops` | Process counters and recent operation p95 |
| GET | `/api/metrics` | Prometheus text counters and latency histogram |
| GET | `/api/state` | Fleet, rides, storage mode, and experiment reports |
| POST | `/api/rides` | `{pickup:{lat,lng},destination:{lat,lng}}` |
| PATCH | `/api/rides/{id}` | `{status:"completed"}` or `{status:"cancelled"}` |
| POST | `/api/simulations` | `{requests:50,scenario:"central"}`; scenario may also be `"mixed"` |

Ride requests return `assigned` or `no_driver`. Repeated terminal transitions return 409; unknown IDs return 404. Invalid simulation parameters return 400; overlapping runs return 409. If report saving fails, the API explicitly reports failure because individual rides may already have committed. Refresh before retrying: a burst is not one atomic transaction and has no idempotency key.

`VYRO_ADDR` changes the Go bind address (default `127.0.0.1:8080`). `VYRO_BACKEND_URL` changes Next.js's API proxy target; set it before starting/building Next.js. Both services bind to loopback by default. Local database connection strings use `sslmode=disable`; choose TLS appropriately for external databases.

## Verify

```powershell
# Unit/API tests and Go vet
.\test-backend.ps1
# Include the separate project test database
.\test-backend.ps1 -Database
cd frontend
npm ci
npm run build
```

The integration test truncates its configured test database's `drivers`, `rides`, and `simulations` tables. A database name ending in `_test` is required. The helper selects `vyro_test`; the application uses `vyro`. If manually setting `VYRO_TEST_DATABASE_URL`, use a disposable database with the schema applied. Race detection requires a supported Windows C toolchain and is not part of the verified checks here.

Covered: nearest-driver selection, radius exclusion, invalid inputs, lifecycle transitions, 50 concurrent memory dispatches, 50 concurrent database dispatches across two independent pools, simulation accounting, JSONB reports, and committed state read through a fresh connection. Browser checks cover central/mixed demand, driver release, and state retained after restarting both database and API. See `VERIFICATION.md` for observations and limits.

## Scope and resume wording

Implemented: Go API, Next.js dashboard, PostgreSQL/PostGIS persistence, spatial discovery, transactional reservations, concurrent demand experiments, and automated tests. Java, Kafka, Redis, moving-fleet simulation, authentication, and public deployment are not implemented. Ride history is currently unpaginated; reservation expiry and production hardening are also future work.

Suggested resume bullet:

> Built a Go and Next.js ride-dispatch prototype with PostgreSQL/PostGIS geospatial discovery, transactional driver reservations, persistent simulation reports, and tests covering 50 concurrent requests across two database connection pools.

Explain `SKIP LOCKED`, the unique active-assignment index, and the timing definitions in an interview. Add latency claims only after a documented, repeatable benchmark.

## Phase 3: reproduce and share

```powershell
# Six HTTP workloads on a separate API/database, leaving the demo fleet alone
.\benchmark.ps1

# Make a source-only ZIP beside this folder
.\package-source.ps1
```

The benchmark suite records 3 rounds × 100 requests for central saturation, lifecycle reuse, and mixed coverage, each at one and ten concurrent clients. JSON, CSV, and Markdown reports live in `benchmarks/`; the index explains what the numbers mean. The saved run contains 1,800 measured requests, zero request errors, zero cleanup failures, and no duplicate active assignments detected. In its 10-client lifecycle case, 300/300 requests matched at 6.58 ms local POST p95. This is a small local observation, not a capacity guarantee. Warmup is excluded, and the direct Go API is measured without Next.js.

`benchmark.ps1` starts a temporary API on port 8081 and connects it to `vyro_bench`, then stops that API. It cancels only its own assignments. Test data uses `vyro_test`; the dashboard uses `vyro`. The three databases share the local cluster and machine. Repeated benchmarks retain history and cache state. The benchmark CLI can also run directly with `go run ./cmd/benchmark -help` from `backend/`.

The service-pulse panel polls process metrics every five seconds. Recent p95 covers up to the last 512 successful dispatch operations through commit, including no-driver outcomes and internal simulation calls. It excludes HTTP and resets with the process. Request logs include generated request IDs, route templates, statuses, and durations; set `VYRO_ACCESS_LOG=1` to include GET requests. `/api/health` uses a lightweight database ping rather than loading ride history. Metrics need no monitoring service for the local demo; Prometheus-format output can be scraped externally if desired.

`.github/workflows/ci.yml` defines backend database tests, Linux race detection, vet/build, and a separate frontend build. It installs native PostgreSQL/PostGIS on a GitHub-hosted runner without a Docker service. CI execution remains unverified until this source is uploaded to a repository and the workflow runs. No deployment or repository publication is performed by the local scripts.

The source ZIP excludes `.local`, credentials, database files, Node dependencies, build outputs, and logs. Extract it into an empty folder, install prerequisites, run database setup, and use the startup commands above. Your system's Windows DLL versions and package installation may differ; setup reports missing prerequisites rather than modifying the system PostgreSQL installation.
