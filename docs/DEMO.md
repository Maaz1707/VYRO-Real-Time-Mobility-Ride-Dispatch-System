# A 90-second recruiter walkthrough

Start the database-backed API and dashboard using the main README. Open the browser at localhost:3000. Release active rides from the last experiment before beginning. A screen recording is not included; this is a script you can use when recording your own demonstration.

## 0–15 seconds: frame the problem

“VYRO is a ride-dispatch prototype built with Go, Next.js, PostgreSQL, and PostGIS. The interesting problem is choosing a nearby available driver while preventing competing requests from assigning that driver twice.”

Show the map, twelve simulated vehicles, and PostgreSQL storage indicator. Be explicit that the streets are schematic and vehicles are simulated.

## 15–35 seconds: demonstrate one ride

Submit MG Road → Koramangala. Show the assignment, pickup distance, and the driver becoming reserved. Cancel the ride to return that driver without changing its location.

“Matching and reservation commit in one database transaction. A row lock coordinates concurrent callers, and a unique index prevents two active rides for one driver.”

## 35–60 seconds: create competition

Open Dispatch laboratory, select central demand and fifty requests, and run it. From the initial fleet, expect twelve assignments, thirty-eight unavailable outcomes, zero errors, and no duplicate active assignments. Existing reservations can change the counts.

“This deliberately creates more demand than available drivers. The report makes both successful assignments and unavailable outcomes visible. Its operation timings include database commit; they are not HTTP timings.”

Release that run's drivers. Briefly show that the saved report remains while drivers become available again.

## 60–75 seconds: show engineering evidence

Show `benchmarks/README.md`, a raw CSV, and `docs/ARCHITECTURE.md`. Explain the lifecycle benchmark: ten clients, three rounds of one hundred requests, with cancellation between each worker's requests. All three hundred matched in the saved local run.

“I keep client HTTP timing separate from server timing, and record raw samples. The results describe this machine and workload, not production capacity.”

## 75–90 seconds: explain one tradeoff

“I kept this as one Go service so I could test and explain the reservation path thoroughly. The next reliability work would be idempotency and reservation expiry. Kafka and Java are possible extensions, not completed components.”

Mention the database integration tests across two independent pools and the checked-in CI workflow. The CI workflow has not been executed on GitHub yet; local tests and the frontend production build have passed.

## Resume bullets

- Built a Go/Next.js ride-dispatch prototype using PostgreSQL/PostGIS spatial discovery, transactional driver reservations, and persistent fleet experiments.
- Validated reservation consistency with fifty concurrent dispatches across two database connection pools; added structured request logs, health checks, and Prometheus-format metrics.
- Created a reproducible HTTP benchmark suite with 1,800 recorded requests across six local workload/concurrency configurations, with no request errors or duplicate active assignments detected in those runs.

Optional measured wording, always with context: “In a local twelve-driver lifecycle workload, matched 300/300 requests at ten concurrent clients with 6.58 ms POST p95.” Treat that as the saved run's observation and update it if benchmark reports are regenerated.

## Be ready to answer

**Why not Redis locks?** The relational database owns both ride and driver state, so its transaction and unique index enforce the invariant in one place.

**Why can `no_driver` occur even under light demand?** Drivers may be outside the five-kilometre radius or already reserved. `SKIP LOCKED` also skips rows another transaction currently holds; this prototype does not retry them.

**Does fast no-driver latency prove high capacity?** No. A rejected assignment is less work than a successful ride, and the fleet is tiny. Lifecycle and saturation results must be interpreted separately.

**What happens when a client times out after commit?** The reservation may exist despite the missing response. An idempotency key and reservation expiry are the next improvements.

**Is it a distributed ride-hailing platform?** It is a database-backed dispatch prototype. It does not include real riders, road routing, a distributed event pipeline, or production deployment.
