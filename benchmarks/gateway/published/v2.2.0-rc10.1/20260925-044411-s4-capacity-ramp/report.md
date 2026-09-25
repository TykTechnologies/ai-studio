# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 04:44:11 UTC |
| Duration | 9m51s |
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
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.887ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 142713 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.893ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 14353 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 143183 proxy-log rows for 143183 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 20.92 [17.42, 30.45] | 48.16 [36.38, 70.33] | 161.47 [33.74, 176.30] |  |
| stream-realistic | gateway | TOTAL | 20.22 [16.60, 30.20] | 46.94 [35.96, 68.15] | 158.08 [34.41, 168.54] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT is 539.8ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 142713 | 0 | 241.5 | 322.20 | 322.26 | 599.82 | 1056.42 | 1543.75 | 4301.61 | 5033.30 | 6257.29 | 0.887 |
| direct | 14353 | 0 | 24.3 | 301.31 | 301.34 | 551.67 | 894.96 | 1254.99 | 4281.39 | 4875.21 | 6136.03 | 0.893 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.475 | 414.062 | 0.516 | 414.463 | 0.485 | 414.078 | 1.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 4.1 | 244 | 0.00% | 312.62 | 959.37 | – | 4939.42 | – |
| 0 | gateway | 37.6 | 2256 | 0.00% | 300.66 | 907.89 | -341.78 | 4887.97 | 0.986 |
| 60 | direct | 8.6 | 513 | 0.00% | 294.45 | 841.53 | – | 4822.19 | – |
| 60 | gateway | 90.5 | 5428 | 0.00% | 304.61 | 928.38 | -123.21 | 4908.84 | 0.954 |
| 120 | direct | 13.8 | 827 | 0.00% | 293.96 | 884.26 | – | 4863.64 | – |
| 120 | gateway | 135.7 | 8139 | 0.00% | 299.64 | 928.30 | -53.42 | 4908.19 | 0.902 |
| 180 | direct | 18.4 | 1105 | 0.00% | 307.18 | 932.49 | – | 4912.75 | – |
| 180 | gateway | 181.9 | 10914 | 0.00% | 300.60 | 908.31 | -99.51 | 4888.87 | 0.953 |
| 240 | direct | 23.3 | 1397 | 0.00% | 302.00 | 982.78 | – | 4961.81 | – |
| 240 | gateway | 228.2 | 13690 | 0.00% | 300.94 | 898.89 | -122.50 | 4878.63 | 1.061 |
| 300 | direct | 27.8 | 1668 | 0.00% | 301.91 | 881.24 | – | 4861.14 | – |
| 300 | gateway | 273.7 | 16420 | 0.00% | 303.81 | 903.85 | -110.61 | 4884.31 | 1.285 |
| 360 | direct | 31.3 | 1876 | 0.00% | 300.80 | 853.92 | – | 4833.49 | – |
| 360 | gateway | 314.6 | 18876 | 0.00% | 308.73 | 892.51 | -55.34 | 4873.54 | 1.728 |
| 420 | direct | 37.2 | 2232 | 0.00% | 297.26 | 921.35 | – | 4901.91 | – |
| 420 | gateway | 364.6 | 21879 | 0.00% | 313.56 | 931.96 | -12.67 | 4907.16 | 3.323 |
| 480 | direct | 40.9 | 2451 | 0.00% | 305.83 | 868.58 | – | 4849.34 | – |
| 480 | gateway | 408.9 | 24532 | 0.00% | 323.93 | 924.11 | -70.93 | 4900.85 | 36.209 |

### Capacity

Sustained 365 req/s within the 25ms p99 overhead SLO. First breach at 409 req/s: gateway Server-Timing p99 gateway time to first byte 36.0ms (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 317.1 / 603.6 / 873.6 |
| Goroutines start / end / max | 39 / 7562 / 17745 |
| Open FDs max | 4516 |
| CPU cores avg / max | 1.79 / 3.81 |

