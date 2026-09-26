# S7 · One-hour soak at 70% of capacity

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s7-soak |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 16:28:30 UTC |
| Duration | 1h0m7s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Constant Poisson load for an hour at about 70% of the S4 capacity. A healthy gateway shows flat p99 latency across the 5-minute windows, a stable goroutine count, bounded memory, no errors, and analytics for every request. Growth in any of them is a leak or a queue that does not drain. The rate below assumes a knee of about 200 req/s. Scale it from your S4 result: --rate-scale <sustained_rps / 200>.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.865ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 1348588 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.863ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 135261 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 1359968 proxy-log rows for 1359968 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 6.12 [-0.37, 13.24] | 7.34 [-16.16, 23.38] | 9.59 [-34.09, 76.88] |  |
| stream-realistic | gateway | TOTAL | 5.78 [0.02, 14.03] | 7.03 [-16.38, 23.87] | 8.86 [-33.42, 74.27] |  |

## Cell: stream-realistic

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 1348588 | 0 | 374.1 | 306.56 | 306.62 | 556.34 | 908.91 | 1300.10 | 4286.29 | 4888.40 | 7446.90 | 0.865 |
| direct | 135261 | 0 | 37.5 | 300.46 | 300.50 | 549.00 | 899.32 | 1302.55 | 4280.51 | 4879.54 | 6183.55 | 0.863 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.244 | 0.520 | 0.284 | 0.654 | 0.255 | 0.536 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 33.5 | 10039 | 0.00% | 301.46 | 878.55 | – | 4859.21 | – |
| 0 | gateway | 340.1 | 102037 | 0.00% | 307.08 | 907.98 | -46.21 | 4887.36 | 0.673 |
| 300 | direct | 37.6 | 11268 | 0.00% | 300.96 | 915.18 | – | 4895.09 | – |
| 300 | gateway | 377.9 | 113359 | 0.00% | 307.38 | 919.75 | -77.89 | 4899.76 | 0.693 |
| 600 | direct | 37.6 | 11274 | 0.00% | 299.79 | 899.37 | – | 4879.16 | – |
| 600 | gateway | 380.0 | 114011 | 0.00% | 307.30 | 905.39 | -47.59 | 4885.15 | 0.657 |
| 900 | direct | 37.7 | 11321 | 0.00% | 302.22 | 900.80 | – | 4881.30 | – |
| 900 | gateway | 377.5 | 113244 | 0.00% | 306.74 | 905.52 | -116.95 | 4885.79 | 0.662 |
| 1200 | direct | 38.0 | 11401 | 0.00% | 302.07 | 900.17 | – | 4880.34 | – |
| 1200 | gateway | 376.9 | 113066 | 0.00% | 305.99 | 906.45 | -24.77 | 4885.06 | 0.646 |
| 1500 | direct | 37.7 | 11311 | 0.00% | 301.88 | 875.99 | – | 4856.08 | – |
| 1500 | gateway | 379.6 | 113878 | 0.00% | 306.89 | 907.79 | -27.44 | 4887.37 | 0.649 |
| 1800 | direct | 38.5 | 11565 | 0.00% | 300.32 | 911.04 | – | 4890.95 | – |
| 1800 | gateway | 378.4 | 113522 | 0.00% | 306.69 | 906.17 | -26.90 | 4885.98 | 0.664 |
| 2100 | direct | 37.9 | 11361 | 0.00% | 299.10 | 892.91 | – | 4872.51 | – |
| 2100 | gateway | 375.9 | 112784 | 0.00% | 305.39 | 917.94 | -62.55 | 4895.57 | 0.616 |
| 2400 | direct | 37.7 | 11299 | 0.00% | 298.60 | 881.22 | – | 4861.14 | – |
| 2400 | gateway | 376.1 | 112837 | 0.00% | 306.10 | 904.83 | -38.18 | 4884.45 | 0.659 |
| 2700 | direct | 38.2 | 11460 | 0.00% | 300.23 | 924.26 | – | 4904.11 | – |
| 2700 | gateway | 378.7 | 113608 | 0.00% | 305.81 | 905.89 | -41.40 | 4886.05 | 0.648 |
| 3000 | direct | 37.9 | 11368 | 0.00% | 298.76 | 904.18 | – | 4883.81 | – |
| 3000 | gateway | 376.6 | 112973 | 0.00% | 306.98 | 913.46 | -25.60 | 4892.90 | 0.622 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 561.1 / 617.2 / 653.6 |
| Goroutines start / end / max | 1049 / 3443 / 10809 |
| Open FDs max | 3625 |
| CPU cores avg / max | 2.22 / 2.59 |
| RSS growth (MB/hour) | -0.7 |
| Goroutine growth (per hour) | -15.2 |

