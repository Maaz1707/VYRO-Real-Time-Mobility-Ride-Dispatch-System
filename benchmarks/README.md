# Reproducible HTTP benchmark results

Run `./benchmark.ps1` from the project root to regenerate six reports against a separate database and API on port 8081. Each configuration uses 3 rounds of 100 measured requests, 10 excluded warmup requests, and a 12-driver fleet. Results below are local measurements, not production capacity estimates.

| Scenario | Clients | Requests | Assigned | No driver | Errors | POST p50 | POST p95 | Details |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| saturation | 1 | 300 | 36 | 264 | 0 | 1.43 ms | 2.79 ms | [Report](saturation-c1.md) |
| saturation | 10 | 300 | 36 | 264 | 0 | 4.69 ms | 46.67 ms | [Report](saturation-c10.md) |
| lifecycle | 1 | 300 | 300 | 0 | 0 | 1.20 ms | 2.28 ms | [Report](lifecycle-c1.md) |
| lifecycle | 10 | 300 | 300 | 0 | 0 | 3.40 ms | 6.58 ms | [Report](lifecycle-c10.md) |
| mixed | 1 | 300 | 36 | 264 | 0 | 1.13 ms | 1.78 ms | [Report](mixed-c1.md) |
| mixed | 10 | 300 | 36 | 264 | 0 | 2.85 ms | 5.01 ms | [Report](mixed-c10.md) |

POST timings include the HTTP exchange and decoding against the direct Go API, excluding Next.js. They exclude release calls, integrity snapshots, and warmup; failed requests would be included in percentiles. Total experiment time in each report includes cleanup and integrity checks. No requests-per-second capacity claim is made.

Saturation and mixed demand hold reservations until the end of each round, then cancel only their own rides. With 12 drivers, most requests return no-driver, so low latency must not be read as high assignment capacity. Mixed demand also has distant pickups, but saturation masks much of that distinction. Lifecycle cancels each assignment before that worker sends its next request, allowing every central request to be matched.

Database history and caches are retained between configurations. Concurrency 1 and 10 are different closed-loop workloads, not a controlled optimization comparison. The benchmark client and API share a machine and database cluster. Runtime/environment and raw per-request JSON/CSV are included in the linked reports. `manifest.json` fingerprints backend sources and report JSON files; the sources are the code used to generate these measurements.
