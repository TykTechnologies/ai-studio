# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:33:03 UTC |
| Duration | 6m36s |
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
| pass | error rate (gateway) | rest-20ms | 0 of 223807 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.127ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 22410 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 224290 proxy-log rows for 224290 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.80 [0.78, 0.84] | 1.53 [1.35, 1.59] | 1291.12 [122.38, 2520.07] |  |
| rest-20ms | gateway | TOTAL | 0.80 [0.78, 0.84] | 1.53 [1.35, 1.59] | 1291.12 [122.38, 2520.07] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 1446.6ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 223807 | 0 | 564.9 | 22.08 | 22.10 | 23.54 | 1313.67 | 19679.22 | 22.10 | 1313.67 | 27801.79 | 1.122 |
| direct | 22410 | 0 | 56.6 | 21.28 | 21.30 | 22.01 | 22.54 | 22.91 | 21.30 | 22.54 | 29.58 | 1.127 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.306 | 1158.249 | 0.319 | 1292.357 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 7.9 | 236 | 0.00% | 21.75 | 22.67 | – | 22.67 | – |
| 0 | gateway | 74.8 | 2244 | 0.00% | 22.66 | 23.83 | 0.97 | 23.83 | 0.694 |
| 30 | direct | 16.7 | 502 | 0.00% | 21.68 | 22.67 | – | 22.67 | – |
| 30 | gateway | 184.5 | 5536 | 0.00% | 22.43 | 23.64 | 0.89 | 23.64 | 0.557 |
| 60 | direct | 27.3 | 818 | 0.00% | 21.54 | 22.70 | – | 22.70 | – |
| 60 | gateway | 277.5 | 8324 | 0.00% | 22.20 | 23.53 | 0.76 | 23.53 | 0.648 |
| 90 | direct | 37.5 | 1124 | 0.00% | 21.48 | 22.63 | – | 22.63 | – |
| 90 | gateway | 364.9 | 10947 | 0.00% | 22.14 | 23.85 | 1.00 | 23.85 | 1.005 |
| 120 | direct | 44.8 | 1344 | 0.00% | 21.45 | 22.63 | – | 22.63 | – |
| 120 | gateway | 448.9 | 13466 | 0.00% | 22.09 | 23.83 | 1.05 | 23.83 | 1.093 |
| 150 | direct | 53.8 | 1614 | 0.00% | 21.41 | 22.57 | – | 22.57 | – |
| 150 | gateway | 544.8 | 16345 | 0.00% | 22.05 | 24.05 | 1.20 | 24.05 | 1.111 |
| 180 | direct | 64.8 | 1945 | 0.00% | 21.34 | 22.49 | – | 22.49 | – |
| 180 | gateway | 630.9 | 18927 | 0.00% | 22.06 | 25.33 | 2.05 | 25.33 | 1.095 |
| 210 | direct | 74.6 | 2239 | 0.00% | 21.30 | 22.42 | – | 22.42 | – |
| 210 | gateway | 725.6 | 21768 | 0.00% | 22.04 | 26.75 | 3.88 | 26.75 | 1.107 |
| 240 | direct | 81.1 | 2432 | 0.00% | 21.28 | 22.48 | – | 22.48 | – |
| 240 | gateway | 818.1 | 24544 | 0.00% | 22.04 | 32.42 | 7.41 | 32.42 | 1.180 |
| 270 | direct | 91.7 | 2751 | 0.00% | 21.24 | 22.50 | – | 22.50 | – |
| 270 | gateway | 909.7 | 27291 | 0.00% | 22.01 | 28.64 | 4.40 | 28.64 | 1.330 |
| 300 | direct | 98.9 | 2967 | 0.00% | 21.19 | 22.47 | – | 22.47 | – |
| 300 | gateway | 998.2 | 29947 | 0.00% | 22.02 | 51.22 | 22.33 | 51.22 | 28.758 |
| 330 | direct | 110.5 | 3314 | 0.00% | 21.17 | 22.46 | – | 22.46 | – |
| 330 | gateway | 1092.5 | 32776 | 0.00% | 22.21 | 164.18 | 132.36 | 164.18 | 142.069 |

### Capacity

Sustained 998 req/s within the 25ms p99 overhead SLO. First breach at 1093 req/s: gateway p99 TTFT 141.6ms above baseline, at least 132.4ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 512.2 / 434.4 / 513.4 |
| Goroutines start / end / max | 3073 / 4162 / 12658 |
| Open FDs max | 2658 |
| CPU cores avg / max | 0.92 / 2.30 |

