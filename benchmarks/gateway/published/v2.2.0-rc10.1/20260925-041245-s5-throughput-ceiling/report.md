# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:12:45 UTC |
| Duration | 6m40s |
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
| pass | load generator kept schedule (gateway) | rest-20ms | p99 release lag 1.127ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-20ms | 0 of 260384 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.120ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 25766 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 260845 proxy-log rows for 260845 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.89 [0.89, 0.96] | 4.06 [3.19, 4.71] | 230.23 [152.46, 253.23] |  |
| rest-20ms | gateway | TOTAL | 0.89 [0.89, 0.96] | 4.06 [3.19, 4.71] | 230.23 [152.46, 253.23] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 595.9ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 260384 | 0 | 650.5 | 22.16 | 22.18 | 26.05 | 252.76 | 583.73 | 22.18 | 252.76 | 1278.47 | 1.127 |
| direct | 25766 | 0 | 64.4 | 21.26 | 21.28 | 21.99 | 22.53 | 22.95 | 21.28 | 22.53 | 27.43 | 1.120 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.316 | 228.995 | 0.329 | 230.034 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 7.8 | 233 | 0.00% | 21.74 | 22.88 | – | 22.88 | – |
| 0 | gateway | 75.4 | 2262 | 0.00% | 22.68 | 23.80 | 0.82 | 23.80 | 0.655 |
| 30 | direct | 16.9 | 506 | 0.00% | 21.65 | 22.73 | – | 22.73 | – |
| 30 | gateway | 184.7 | 5540 | 0.00% | 22.40 | 23.63 | 0.76 | 23.63 | 0.586 |
| 60 | direct | 28.5 | 855 | 0.00% | 21.53 | 22.58 | – | 22.58 | – |
| 60 | gateway | 273.7 | 8211 | 0.00% | 22.22 | 23.59 | 0.79 | 23.59 | 0.974 |
| 90 | direct | 34.9 | 1048 | 0.00% | 21.53 | 22.57 | – | 22.57 | – |
| 90 | gateway | 366.9 | 11008 | 0.00% | 22.15 | 23.70 | 0.90 | 23.70 | 1.063 |
| 120 | direct | 46.2 | 1386 | 0.00% | 21.43 | 22.67 | – | 22.67 | – |
| 120 | gateway | 450.9 | 13526 | 0.00% | 22.10 | 23.79 | 1.02 | 23.79 | 1.089 |
| 150 | direct | 56.7 | 1701 | 0.00% | 21.41 | 22.63 | – | 22.63 | – |
| 150 | gateway | 540.9 | 16226 | 0.00% | 22.06 | 24.27 | 1.37 | 24.27 | 1.108 |
| 180 | direct | 60.6 | 1819 | 0.00% | 21.36 | 22.51 | – | 22.51 | – |
| 180 | gateway | 636.8 | 19103 | 0.00% | 22.06 | 25.16 | 2.05 | 25.16 | 1.135 |
| 210 | direct | 73.0 | 2189 | 0.00% | 21.32 | 22.58 | – | 22.58 | – |
| 210 | gateway | 727.3 | 21819 | 0.00% | 22.05 | 28.73 | 5.36 | 28.73 | 1.106 |
| 240 | direct | 80.7 | 2421 | 0.00% | 21.25 | 22.46 | – | 22.46 | – |
| 240 | gateway | 824.2 | 24725 | 0.00% | 22.01 | 27.61 | 3.76 | 27.61 | 1.129 |
| 270 | direct | 89.4 | 2683 | 0.00% | 21.25 | 22.46 | – | 22.46 | – |
| 270 | gateway | 902.1 | 27062 | 0.00% | 22.01 | 32.03 | 6.88 | 32.03 | 1.327 |
| 300 | direct | 99.2 | 2975 | 0.00% | 21.22 | 22.49 | – | 22.49 | – |
| 300 | gateway | 997.3 | 29920 | 0.00% | 22.04 | 35.36 | 8.95 | 35.36 | 7.188 |
| 330 | direct | 105.8 | 3175 | 0.00% | 21.17 | 22.43 | – | 22.43 | – |
| 330 | gateway | 1097.9 | 32938 | 0.00% | 22.12 | 34.56 | 10.35 | 34.56 | 10.337 |
| 360 | direct | 119.7 | 3590 | 0.00% | 21.17 | 22.44 | – | 22.44 | – |
| 360 | gateway | 1182.1 | 35464 | 0.00% | 22.75 | 48.40 | 22.82 | 48.40 | 26.692 |

### Capacity

Sustained 1182 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 595.9ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 485.4 / 440.3 / 486.0 |
| Goroutines start / end / max | 3315 / 14333 / 14333 |
| Open FDs max | 2839 |
| CPU cores avg / max | 1.07 / 2.37 |

