# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:12:30 UTC |
| Duration | 3m58s |
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
| pass | load generator kept schedule (gateway) | rest-20ms | p99 release lag 1.128ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-20ms | 0 of 93815 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.128ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 9472 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 94304 proxy-log rows for 94304 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.67 [0.62, 0.68] | 0.88 [0.82, 0.91] | 866.80 [676.37, 1191.57] |  |
| rest-20ms | gateway | TOTAL | 0.67 [0.62, 0.68] | 0.88 [0.82, 0.91] | 866.80 [676.37, 1191.57] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 1015.2ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 93815 | 0 | 393.8 | 22.09 | 22.11 | 23.03 | 889.44 | 2849.54 | 22.11 | 889.44 | 4445.62 | 1.128 |
| direct | 9472 | 0 | 39.8 | 21.42 | 21.44 | 22.15 | 22.64 | 23.01 | 21.44 | 22.64 | 26.09 | 1.128 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.285 | 791.543 | 0.296 | 866.549 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 7.4 | 222 | 0.00% | 21.72 | 22.74 | – | 22.74 | – |
| 0 | gateway | 78.3 | 2349 | 0.00% | 22.61 | 23.76 | 0.90 | 23.76 | 0.648 |
| 30 | direct | 20.2 | 605 | 0.00% | 21.67 | 22.82 | – | 22.82 | – |
| 30 | gateway | 181.0 | 5430 | 0.00% | 22.38 | 23.63 | 0.76 | 23.63 | 0.695 |
| 60 | direct | 26.5 | 795 | 0.00% | 21.63 | 22.68 | – | 22.68 | – |
| 60 | gateway | 277.5 | 8326 | 0.00% | 22.21 | 23.63 | 0.79 | 23.63 | 0.844 |
| 90 | direct | 35.6 | 1069 | 0.00% | 21.53 | 22.60 | – | 22.60 | – |
| 90 | gateway | 359.3 | 10779 | 0.00% | 22.13 | 23.66 | 0.82 | 23.66 | 0.988 |
| 120 | direct | 46.5 | 1394 | 0.00% | 21.45 | 22.66 | – | 22.66 | – |
| 120 | gateway | 449.6 | 13488 | 0.00% | 22.07 | 23.86 | 1.12 | 23.86 | 1.087 |
| 150 | direct | 54.7 | 1641 | 0.00% | 21.39 | 22.57 | – | 22.57 | – |
| 150 | gateway | 546.2 | 16387 | 0.00% | 22.04 | 24.53 | 1.45 | 24.53 | 1.115 |
| 180 | direct | 62.5 | 1874 | 0.00% | 21.35 | 22.60 | – | 22.60 | – |
| 180 | gateway | 636.2 | 19086 | 0.00% | 22.01 | 24.93 | 1.76 | 24.93 | 1.073 |

### Capacity

Sustained 636 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 1015.2ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 413.2 / 360.4 / 413.2 |
| Goroutines start / end / max | 745 / 6853 / 8077 |
| Open FDs max | 1157 |
| CPU cores avg / max | 0.59 / 1.13 |

