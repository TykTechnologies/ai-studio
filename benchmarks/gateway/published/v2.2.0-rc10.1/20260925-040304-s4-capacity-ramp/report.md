# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:03:04 UTC |
| Duration | 9m24s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Arrivals follow a Poisson process whose rate steps up every minute. The mock upstream behaves like a hosted chat model (about 300ms to first token, 50 tokens/s, 200 tokens), so each stream stays open for about 4 seconds and the gateway holds thousands of concurrent streams at the top of the ramp. A 10% sample of traffic goes straight to the mock as the baseline. The capacity is the highest rate at which the gateway's p99 time to first token stays within 25ms of the baseline's with an error rate below 0.1%. The ramp stops early once the gateway is clearly past it. Size S6 and S7 from this result with --rate-scale.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.897ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 130679 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.898ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 13067 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 131153 proxy-log rows for 131153 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 16.74 [15.24, 28.37] | 45.12 [41.20, 80.78] | 121.88 [19.98, 180.33] |  |
| stream-realistic | gateway | TOTAL | 15.75 [15.13, 28.08] | 43.34 [37.26, 77.04] | 122.38 [21.44, 184.64] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT is 527.8ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 130679 | 0 | 231.9 | 318.02 | 318.07 | 591.36 | 1007.39 | 1452.72 | 4297.30 | 4987.26 | 6540.69 | 0.897 |
| direct | 13067 | 0 | 23.2 | 301.31 | 301.34 | 546.24 | 885.52 | 1339.01 | 4281.55 | 4864.87 | 6121.97 | 0.898 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.473 | 363.436 | 0.515 | 363.470 | 0.483 | 363.454 | 1.3% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 3.6 | 217 | 0.00% | 300.30 | 913.63 | – | 4893.51 | – |
| 0 | gateway | 38.5 | 2308 | 0.00% | 303.14 | 931.16 | -161.92 | 4909.85 | 1.123 |
| 60 | direct | 8.2 | 494 | 0.00% | 310.60 | 755.42 | – | 4734.39 | – |
| 60 | gateway | 89.8 | 5388 | 0.00% | 296.48 | 897.92 | -48.50 | 4878.46 | 0.967 |
| 120 | direct | 14.3 | 861 | 0.00% | 305.40 | 932.02 | – | 4912.17 | – |
| 120 | gateway | 136.3 | 8179 | 0.00% | 302.04 | 891.66 | -96.39 | 4872.53 | 0.985 |
| 180 | direct | 19.1 | 1147 | 0.00% | 296.58 | 843.93 | – | 4823.93 | – |
| 180 | gateway | 183.2 | 10994 | 0.00% | 300.84 | 914.50 | -32.32 | 4894.92 | 0.985 |
| 240 | direct | 22.8 | 1370 | 0.00% | 298.92 | 842.99 | – | 4823.53 | – |
| 240 | gateway | 226.0 | 13560 | 0.00% | 305.28 | 916.26 | -10.61 | 4897.59 | 1.120 |
| 300 | direct | 27.8 | 1666 | 0.00% | 300.17 | 805.84 | – | 4785.25 | – |
| 300 | gateway | 271.1 | 16265 | 0.00% | 305.33 | 927.24 | 10.38 | 4905.51 | 1.297 |
| 360 | direct | 32.1 | 1926 | 0.00% | 303.52 | 924.25 | – | 4903.97 | – |
| 360 | gateway | 318.5 | 19111 | 0.00% | 309.55 | 923.82 | -21.86 | 4902.62 | 1.619 |
| 420 | direct | 36.4 | 2184 | 0.00% | 298.91 | 913.06 | – | 4893.06 | – |
| 420 | gateway | 367.4 | 22044 | 0.00% | 311.91 | 912.95 | -54.27 | 4891.93 | 3.247 |
| 480 | direct | 39.5 | 2370 | 0.00% | 302.31 | 903.29 | – | 4883.62 | – |
| 480 | gateway | 408.4 | 24506 | 0.00% | 326.95 | 926.07 | -20.02 | 4901.60 | 54.355 |

### Capacity

Sustained 367 req/s within the 25ms p99 overhead SLO. First breach at 408 req/s: gateway Server-Timing p99 gateway time to first byte 54.3ms (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 224.2 / 529.9 / 796.8 |
| Goroutines start / end / max | 39 / 7899 / 17888 |
| Open FDs max | 4570 |
| CPU cores avg / max | 1.77 / 3.73 |

