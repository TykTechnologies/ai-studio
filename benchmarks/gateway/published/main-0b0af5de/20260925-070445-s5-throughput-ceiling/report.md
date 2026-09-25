# S5 · Request-rate ceiling with a fast upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5-throughput-ceiling |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:04:45 UTC |
| Duration | 7m10s |
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
| pass | load generator kept schedule (gateway) | rest-20ms | p99 release lag 1.116ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-20ms | 0 of 298983 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-20ms | p99 release lag 1.114ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-20ms | 0 of 29785 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 299468 proxy-log rows for 299466 gateway responses; extra rows come from other traffic to the same app during the run |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-20ms | gateway | TTFT | 0.86 [0.82, 0.88] | 3.86 [3.40, 4.82] | 240.15 [213.94, 276.12] |  |
| rest-20ms | gateway | TOTAL | 0.86 [0.82, 0.88] | 3.86 [3.40, 4.82] | 240.15 [213.94, 276.12] |  |

## Cell: rest-20ms

> **Stopped early: gateway p99 total is 586.5ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 298983 | 0 | 695.8 | 22.10 | 22.13 | 25.83 | 262.68 | 636.52 | 22.13 | 262.68 | 1310.93 | 1.116 |
| direct | 29785 | 0 | 69.3 | 21.25 | 21.27 | 21.97 | 22.52 | 22.96 | 21.27 | 22.52 | 26.20 | 1.114 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.300 | 239.406 | 0.312 | 240.264 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 7.8 | 234 | 0.00% | 21.74 | 22.77 | – | 22.77 | – |
| 0 | gateway | 75.0 | 2251 | 0.00% | 22.62 | 23.74 | 0.68 | 23.74 | 0.539 |
| 30 | direct | 17.9 | 536 | 0.00% | 21.68 | 22.73 | – | 22.73 | – |
| 30 | gateway | 181.2 | 5435 | 0.00% | 22.37 | 23.67 | 0.79 | 23.67 | 0.648 |
| 60 | direct | 25.8 | 774 | 0.00% | 21.57 | 22.73 | – | 22.73 | – |
| 60 | gateway | 269.8 | 8095 | 0.00% | 22.23 | 23.59 | 0.71 | 23.59 | 0.690 |
| 90 | direct | 36.0 | 1080 | 0.00% | 21.48 | 22.70 | – | 22.70 | – |
| 90 | gateway | 359.7 | 10790 | 0.00% | 22.12 | 23.68 | 0.78 | 23.68 | 0.870 |
| 120 | direct | 45.1 | 1354 | 0.00% | 21.46 | 22.61 | – | 22.61 | – |
| 120 | gateway | 456.3 | 13689 | 0.00% | 22.07 | 23.84 | 0.91 | 23.84 | 1.045 |
| 150 | direct | 55.4 | 1662 | 0.00% | 21.39 | 22.60 | – | 22.60 | – |
| 150 | gateway | 554.9 | 16647 | 0.00% | 22.02 | 23.99 | 1.20 | 23.99 | 0.999 |
| 180 | direct | 63.6 | 1907 | 0.00% | 21.36 | 22.50 | – | 22.50 | – |
| 180 | gateway | 630.8 | 18923 | 0.00% | 22.02 | 25.19 | 1.86 | 25.19 | 0.994 |
| 210 | direct | 70.3 | 2109 | 0.00% | 21.32 | 22.52 | – | 22.52 | – |
| 210 | gateway | 725.9 | 21776 | 0.00% | 22.02 | 27.43 | 4.25 | 27.43 | 1.009 |
| 240 | direct | 80.7 | 2421 | 0.00% | 21.30 | 22.49 | – | 22.49 | – |
| 240 | gateway | 824.3 | 24728 | 0.00% | 22.00 | 29.62 | 5.04 | 29.62 | 1.007 |
| 270 | direct | 89.8 | 2694 | 0.00% | 21.25 | 22.42 | – | 22.42 | – |
| 270 | gateway | 912.1 | 27362 | 0.00% | 21.97 | 31.19 | 7.37 | 31.19 | 1.160 |
| 300 | direct | 101.2 | 3037 | 0.00% | 21.23 | 22.43 | – | 22.43 | – |
| 300 | gateway | 1009.2 | 30275 | 0.00% | 21.97 | 31.78 | 6.78 | 31.78 | 1.824 |
| 330 | direct | 109.8 | 3293 | 0.00% | 21.20 | 22.42 | – | 22.42 | – |
| 330 | gateway | 1095.0 | 32850 | 0.00% | 21.97 | 30.26 | 6.54 | 30.26 | 4.640 |
| 360 | direct | 119.8 | 3593 | 0.00% | 21.15 | 22.42 | – | 22.42 | – |
| 360 | gateway | 1190.4 | 35711 | 0.00% | 22.08 | 34.02 | 9.57 | 34.02 | 10.887 |
| 390 | direct | 128.2 | 3846 | 0.00% | 21.13 | 22.40 | – | 22.40 | – |
| 390 | gateway | 1265.5 | 37965 | 0.00% | 23.04 | 55.77 | 30.49 | 55.77 | 34.107 |

### Capacity

Sustained 1190 req/s within the 25ms p99 overhead SLO. First breach at 1266 req/s: gateway p99 TTFT 33.2ms above baseline, at least 30.5ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 76.1 / 395.1 / 395.1 |
| Goroutines start / end / max | 42 / 14789 / 15293 |
| Open FDs max | 1194 |
| CPU cores avg / max | 1.09 / 2.34 |

