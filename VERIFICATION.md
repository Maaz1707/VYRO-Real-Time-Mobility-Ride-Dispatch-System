# Phase 2 verification — 21 November 2026

Environment: Windows, Go 1.27.4, Node.js 22.17.1, Next.js 16.4.6, PostgreSQL 17.11, PostGIS 3.7. Project database on loopback port 55432. No Docker.

## Automated checks

- `test-backend.ps1 -Database`: passed unit, API, simulation, and PostgreSQL integration tests; `go vet` passed.
- 50 simultaneous requests across two independent PostgreSQL pools: 12 drivers assigned, none twice, 50 ride records committed.
- Completion, cancellation, repeated-transition rejection, and out-of-radius behavior passed.
- Saved report and rides readable through a fresh database connection.
- `npm run build`: passed production compilation and strict TypeScript checks.

## Browser and restart checks

| Scenario | Requests | Assigned | No driver | Errors | Duplicate active assignments |
|---|---:|---:|---:|---:|---:|
| Central demand, initial available fleet | 50 | 12 | 38 | 0 | 0 |
| Mixed coverage, after releasing prior run | 10 | 8 | 2 | 0 | 0 |

The first central run recorded p50 **6.47 ms**, p95 **113.06 ms**, and batch duration **134.5 ms**. These measure local dispatch operations through commit, not HTTP latency. They include pool waiting and assigned/no-driver outcomes, exclude failed operations, and are observations from one run rather than benchmark claims.

After the central run, both the API and project PostgreSQL process were stopped and restarted. The API returned the same 50 rides, same report ID, and same 12 active reservations. The browser displayed the saved report after reload. Releasing that run cancelled 12 rides and made those drivers available. A following mixed run assigned eight of ten requests as expected.

## Runtime issues resolved

The initial KNN `<->` sort with row locking failed on this PostgreSQL build with `attempted to lock invisible tuple`. The query now filters with `ST_DWithin`, sorts by exact distance and ID, and applies `FOR UPDATE SKIP LOCKED`. The database integration suite then passed; test fixtures were recreated in the dedicated test database before rerunning.

The Windows PostGIS ZIP also needed its bundled Brotli DLLs and the installed Git for Windows nghttp2 DLL copied into the project runtime. Setup handles these local copies; it does not alter the system PostgreSQL installation.

## Limits

No broker, moving fleet, road routing, production load benchmark, public deployment, or race-detector execution was validated. Tests demonstrate the exercised reservation paths and persistence, not general production reliability. Full ride history is loaded into snapshots and should be paginated for larger datasets.

## Phase 3 verification — 21 November 2026

- Unit, HTTP/API, simulation, telemetry/readiness, benchmark-client, and real PostgreSQL integration tests passed locally; Go vet passed.
- Next.js production build and strict TypeScript checks passed. Browser checks confirmed the service-pulse panel, database-backed assignment, cancellation, and updating process metrics.
- Six direct-API HTTP configurations recorded 1,800 measured requests with zero request errors, zero cleanup failures, and no duplicate active assignments detected. Raw samples, definitions, environment, and source/report hashes are in `benchmarks/`.
- A separate benchmark database and temporary API on port 8081 kept the dashboard fleet independent. All drivers were available after the browser verification.
- The source ZIP was inspected: 56 source/report/documentation files, excluding credentials, database/runtime files, Node dependencies, logs, and build output.
- The GitHub Actions workflow was syntax-formatted but has not run on GitHub. Linux race detection is configured there and remains unverified. No public deployment or demo video recording was performed; a recording script is included.
