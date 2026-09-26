# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:54:27 UTC |
| Duration | 6m6s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

The complement of S4: short requests against an upstream that answers in 20ms, so the gateway's per-request cost (authentication, policy, analytics), not the number of open streams, sets the limit. The rate steps up every 30 seconds until p99 overhead or errors breach the limits.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | rest-20ms | p99 release lag 1.122ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-20ms | 0 of 198366 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.126ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 19961 failed (0.000%) |
| WARN | analytics recorded every gateway request |  | Studio gained 198809 proxy-log rows for 198810 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.75 [0.74, 0.80] | 1.19 [1.12, 1.29] | 166.57 [21.92, 1889.32] |  |
| rest-20ms | gateway | TOTAL | 0.75 [0.75, 0.80] | 1.19 [1.12, 1.29] | 166.57 [21.92, 1889.32] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 731.3ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 198366 | 0 | 542.6 | 22.05 | 22.08 | 23.22 | 189.13 | 12437.46 | 22.08 | 189.13 | 19939.69 | 1.122 |
| direct | 19961 | 0 | 54.6 | 21.30 | 21.32 | 22.03 | 22.56 | 22.94 | 21.32 | 22.56 | 27.69 | 1.126 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.301 | 167.372 | 0.313 | 167.383 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 8.5 | 254 | 0.00% | 21.76 | 22.76 | – | 22.76 | – |
| 0 | gateway | 74.8 | 2243 | 0.00% | 22.64 | 23.78 | 0.67 | 23.78 | 0.668 |
| 30 | direct | 18.2 | 547 | 0.00% | 21.64 | 22.66 | – | 22.66 | – |
| 30 | gateway | 183.4 | 5503 | 0.00% | 22.38 | 23.58 | 0.78 | 23.58 | 0.587 |
| 60 | direct | 27.2 | 816 | 0.00% | 21.56 | 22.66 | – | 22.66 | – |
| 60 | gateway | 273.9 | 8218 | 0.00% | 22.18 | 23.53 | 0.76 | 23.53 | 0.802 |
| 90 | direct | 36.4 | 1092 | 0.00% | 21.50 | 22.64 | – | 22.64 | – |
| 90 | gateway | 366.4 | 10993 | 0.00% | 22.12 | 23.68 | 0.86 | 23.68 | 0.995 |
| 120 | direct | 47.9 | 1437 | 0.00% | 21.43 | 22.58 | – | 22.58 | – |
| 120 | gateway | 455.3 | 13660 | 0.00% | 22.08 | 23.75 | 0.96 | 23.75 | 1.080 |
| 150 | direct | 55.5 | 1666 | 0.00% | 21.40 | 22.54 | – | 22.54 | – |
| 150 | gateway | 546.5 | 16396 | 0.00% | 22.05 | 24.11 | 1.31 | 24.11 | 1.132 |
| 180 | direct | 64.4 | 1933 | 0.00% | 21.36 | 22.58 | – | 22.58 | – |
| 180 | gateway | 639.0 | 19171 | 0.00% | 22.05 | 26.79 | 2.96 | 26.79 | 1.170 |
| 210 | direct | 73.1 | 2193 | 0.00% | 21.30 | 22.50 | – | 22.50 | – |
| 210 | gateway | 725.7 | 21770 | 0.00% | 22.03 | 27.54 | 3.56 | 27.54 | 1.158 |
| 240 | direct | 80.6 | 2419 | 0.00% | 21.25 | 22.48 | – | 22.48 | – |
| 240 | gateway | 815.2 | 24455 | 0.00% | 22.02 | 26.84 | 3.54 | 26.84 | 1.263 |
| 270 | direct | 90.8 | 2724 | 0.00% | 21.25 | 22.48 | – | 22.48 | – |
| 270 | gateway | 906.3 | 27189 | 0.00% | 22.05 | 40.75 | 15.90 | 40.75 | 17.131 |
| 300 | direct | 99.7 | 2991 | 0.00% | 21.21 | 22.43 | – | 22.43 | – |
| 300 | gateway | 1009.1 | 30274 | 0.00% | 22.01 | 42.85 | 20.05 | 42.85 | 20.209 |

### Capacity

Sustained 1009 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 731.3ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 569.6 / 439.2 / 570.8 |
| Goroutines start / end / max | 3288 / 3077 / 11352 |
| Open FDs max | 2814 |
| CPU cores avg / max | 0.87 / 1.96 |

