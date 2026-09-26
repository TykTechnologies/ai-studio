# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:24:41 UTC |
| Duration | 7m57s |
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
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.909ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 95153 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.904ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 9474 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 95629 proxy-log rows for 95629 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 7.90 [-2.33, 11.28] | 23.90 [2.93, 35.66] | 96.12 [-34.89, 148.38] |  |
| stream-realistic | gateway | TOTAL | 7.82 [-2.14, 10.80] | 24.49 [2.82, 35.50] | 98.60 [-26.50, 160.87] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT is 600.5ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 95153 | 0 | 199.3 | 308.61 | 308.65 | 569.76 | 1024.85 | 1879.91 | 4288.47 | 5006.78 | 6797.02 | 0.909 |
| direct | 9474 | 0 | 19.8 | 300.71 | 300.75 | 545.86 | 928.73 | 1334.97 | 4280.65 | 4908.18 | 5807.30 | 0.904 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.450 | 53.099 | 0.490 | 53.134 | 0.460 | 53.108 | 1.8% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 3.8 | 228 | 0.00% | 305.30 | 973.95 | – | 4954.42 | – |
| 0 | gateway | 38.8 | 2329 | 0.00% | 292.19 | 881.66 | -186.33 | 4861.28 | 0.968 |
| 60 | direct | 9.1 | 545 | 0.00% | 302.08 | 975.26 | – | 4955.42 | – |
| 60 | gateway | 92.7 | 5562 | 0.00% | 303.20 | 886.31 | -175.04 | 4866.36 | 0.949 |
| 120 | direct | 13.3 | 797 | 0.00% | 288.39 | 843.33 | – | 4823.16 | – |
| 120 | gateway | 137.5 | 8248 | 0.00% | 305.49 | 888.15 | -136.34 | 4868.49 | 0.887 |
| 180 | direct | 18.4 | 1103 | 0.00% | 298.04 | 924.45 | – | 4904.36 | – |
| 180 | gateway | 182.2 | 10935 | 0.00% | 302.82 | 914.69 | -84.94 | 4894.87 | 0.973 |
| 240 | direct | 22.7 | 1362 | 0.00% | 294.95 | 1003.90 | – | 4984.06 | – |
| 240 | gateway | 227.2 | 13635 | 0.00% | 303.63 | 906.92 | -139.48 | 4888.13 | 1.157 |
| 300 | direct | 27.3 | 1640 | 0.00% | 306.49 | 927.58 | – | 4907.79 | – |
| 300 | gateway | 272.2 | 16333 | 0.00% | 305.27 | 903.92 | -99.32 | 4884.50 | 1.349 |
| 360 | direct | 32.0 | 1921 | 0.00% | 301.85 | 821.41 | – | 4802.01 | – |
| 360 | gateway | 315.6 | 18938 | 0.00% | 308.31 | 914.47 | -67.64 | 4897.54 | 2.045 |

### Capacity

Sustained 316 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 TTFT is 600.5ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 321.4 / 586.4 / 896.1 |
| Goroutines start / end / max | 39 / 6784 / 14741 |
| Open FDs max | 4087 |
| CPU cores avg / max | 1.51 / 3.27 |

