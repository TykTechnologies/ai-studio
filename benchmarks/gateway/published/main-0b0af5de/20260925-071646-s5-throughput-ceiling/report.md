# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:16:46 UTC |
| Duration | 4m32s |
| Code | 0b0af5decb42 |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + gwbench-microgateway:0b0af5de; ping p99 0.598ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

The complement of S4: short requests against an upstream that answers in 20ms, so the gateway's per-request cost (authentication, policy, analytics), not the number of open streams, sets the limit. The rate steps up every 30 seconds until p99 overhead or errors breach the limits.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | rest-20ms | p99 release lag 1.125ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-20ms | 0 of 120408 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.134ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 12279 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 120845 proxy-log rows for 120845 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.69 [0.66, 0.72] | 0.92 [0.89, 1.01] | 512.34 [22.13, 764.63] |  |
| rest-20ms | gateway | TOTAL | 0.69 [0.66, 0.72] | 0.92 [0.89, 1.01] | 512.34 [22.13, 764.63] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 1052.0ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 120408 | 0 | 443.3 | 22.07 | 22.09 | 23.03 | 534.95 | 2813.91 | 22.09 | 534.95 | 4686.53 | 1.125 |
| direct | 12279 | 0 | 45.2 | 21.38 | 21.40 | 22.11 | 22.61 | 23.04 | 21.40 | 22.61 | 25.10 | 1.134 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.284 | 480.114 | 0.295 | 512.599 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 7.8 | 234 | 0.00% | 21.83 | 22.81 | – | 22.81 | – |
| 0 | gateway | 74.9 | 2247 | 0.00% | 22.62 | 23.79 | 0.72 | 23.79 | 0.562 |
| 30 | direct | 18.3 | 550 | 0.00% | 21.67 | 22.80 | – | 22.80 | – |
| 30 | gateway | 179.7 | 5392 | 0.00% | 22.36 | 23.67 | 0.76 | 23.67 | 0.611 |
| 60 | direct | 28.7 | 862 | 0.00% | 21.57 | 22.69 | – | 22.69 | – |
| 60 | gateway | 272.2 | 8165 | 0.00% | 22.23 | 23.58 | 0.68 | 23.58 | 0.852 |
| 90 | direct | 37.0 | 1109 | 0.00% | 21.50 | 22.65 | – | 22.65 | – |
| 90 | gateway | 366.4 | 10992 | 0.00% | 22.12 | 23.63 | 0.77 | 23.63 | 1.029 |
| 120 | direct | 45.8 | 1373 | 0.00% | 21.44 | 22.59 | – | 22.59 | – |
| 120 | gateway | 456.1 | 13683 | 0.00% | 22.08 | 23.93 | 1.00 | 23.93 | 1.095 |
| 150 | direct | 56.4 | 1692 | 0.00% | 21.39 | 22.55 | – | 22.55 | – |
| 150 | gateway | 549.3 | 16479 | 0.00% | 22.03 | 24.46 | 1.19 | 24.46 | 1.087 |
| 180 | direct | 63.5 | 1906 | 0.00% | 21.34 | 22.51 | – | 22.51 | – |
| 180 | gateway | 631.9 | 18958 | 0.00% | 22.02 | 25.11 | 2.02 | 25.11 | 1.082 |
| 210 | direct | 74.1 | 2222 | 0.00% | 21.33 | 22.55 | – | 22.55 | – |
| 210 | gateway | 727.9 | 21838 | 0.00% | 22.02 | 28.88 | 4.73 | 28.88 | 1.048 |

### Capacity

Sustained 728 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 1052.0ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 349.4 / 361.8 / 362.0 |
| Goroutines start / end / max | 403 / 7235 / 8887 |
| Open FDs max | 1007 |
| CPU cores avg / max | 0.67 / 1.29 |

