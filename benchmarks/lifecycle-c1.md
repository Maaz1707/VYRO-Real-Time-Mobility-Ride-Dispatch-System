# HTTP benchmark — 2026-11-21T07:06:15Z

Storage: **postgres**. Scenario: **lifecycle**. 3 rounds × 100 requests, 1 concurrent workers, 10 warmup requests excluded. Fleet: 12 drivers. Runtime: windows / amd64, go1.27.4, 4 logical CPUs.

| Requests | Assigned | No driver | Errors | Cleanup errors | Duplicate active assignments |
|---:|---:|---:|---:|---:|---:|
| 300 | 300 | 0 | 0 | 0 | 0 |

| POST p50 | POST p95 | POST p99 | Total experiment time |
|---:|---:|---:|---:|
| 1.20 ms | 2.28 ms | 2.66 ms | 0.82 s |

Closed-loop local HTTP experiment against the direct Go API, excluding Next.js. POST latency includes connection, response headers/body, and JSON decoding; cancellation, integrity snapshots, and warmup are excluded from individual POST samples. Percentiles include failed requests if any. Total experiment time includes cancellation and integrity snapshots. No requests-per-second capacity claim is made. Saturation retains reservations until round end; lifecycle cancels each assigned ride before that worker's next POST. Mixed demand uses every fifth pickup outside the original fleet radius. Existing history, logging, pool size, and cache state can affect results. This does not model real traffic, routing, or production capacity.

Raw samples: [lifecycle-c1.csv](lifecycle-c1.csv). Machine-readable report: [lifecycle-c1.json](lifecycle-c1.json).
