# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 12:30:05 UTC |
| Duration | 20m16s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Arrivals follow a Poisson process whose rate steps up every minute. The mock upstream behaves like a hosted chat model (about 300ms to first token, 50 tokens/s, 200 tokens), so each stream stays open for about 4 seconds and the gateway holds thousands of concurrent streams at the top of the ramp. A 10% sample of traffic goes straight to the mock as the baseline. The capacity is the highest rate at which the gateway's p99 time to first token stays within 25ms of the baseline's with an error rate below 0.1%. The ramp stops early once the gateway is clearly past it. Size S6 and S7 from this result with --rate-scale.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.838ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 581406 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.842ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 58131 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 581860 proxy-log rows for 581860 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 98.51 [92.52, 111.52] | 177.71 [158.37, 203.03] | 206.88 [86.55, 213.13] |  |
| stream-realistic | gateway | TOTAL | 80.47 [77.20, 93.19] | 126.27 [105.55, 145.02] | 137.26 [30.14, 144.67] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT over the last 10s is 592.7ms above direct's recent p99**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 581406 | 0 | 478.3 | 398.65 | 398.71 | 728.68 | 1109.68 | 1494.79 | 4360.74 | 5019.76 | 6921.72 | 0.838 |
| direct | 58131 | 0 | 47.8 | 300.17 | 300.20 | 550.97 | 902.80 | 1296.57 | 4280.27 | 4882.50 | 6195.29 | 0.842 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.255 | 1.368 | 0.296 | 1.862 | 0.266 | 1.450 | 0.4% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 3.5 | 213 | 0.00% | 294.21 | 904.70 | – | 4884.78 | – |
| 0 | gateway | 38.2 | 2291 | 0.00% | 303.39 | 869.36 | -169.58 | 4849.78 | 0.867 |
| 60 | direct | 9.1 | 543 | 0.00% | 304.82 | 895.63 | – | 4875.39 | – |
| 60 | gateway | 89.8 | 5388 | 0.00% | 301.39 | 908.35 | -90.63 | 4888.27 | 0.405 |
| 120 | direct | 14.4 | 863 | 0.00% | 305.55 | 936.65 | – | 4916.70 | – |
| 120 | gateway | 133.9 | 8034 | 0.00% | 304.26 | 907.65 | -83.05 | 4888.39 | 0.435 |
| 180 | direct | 18.3 | 1098 | 0.00% | 310.71 | 925.74 | – | 4905.47 | – |
| 180 | gateway | 182.4 | 10944 | 0.00% | 302.20 | 895.93 | -116.53 | 4877.33 | 0.457 |
| 240 | direct | 22.3 | 1339 | 0.00% | 297.60 | 945.61 | – | 4925.36 | – |
| 240 | gateway | 229.1 | 13745 | 0.00% | 302.13 | 908.70 | -90.00 | 4888.86 | 0.505 |
| 300 | direct | 28.1 | 1688 | 0.00% | 304.16 | 851.63 | – | 4830.73 | – |
| 300 | gateway | 270.0 | 16201 | 0.00% | 300.24 | 896.01 | -104.26 | 4875.69 | 0.508 |
| 360 | direct | 31.9 | 1915 | 0.00% | 299.18 | 950.42 | – | 4929.31 | – |
| 360 | gateway | 316.2 | 18971 | 0.00% | 305.17 | 896.44 | -76.30 | 4875.55 | 0.548 |
| 420 | direct | 36.0 | 2160 | 0.00% | 300.46 | 927.27 | – | 4907.10 | – |
| 420 | gateway | 360.2 | 21613 | 0.00% | 308.71 | 907.63 | -98.14 | 4887.21 | 0.641 |
| 480 | direct | 39.5 | 2368 | 0.00% | 301.10 | 874.97 | – | 4855.13 | – |
| 480 | gateway | 408.7 | 24521 | 0.00% | 306.44 | 903.36 | -131.96 | 4881.97 | 0.701 |
| 540 | direct | 46.4 | 2782 | 0.00% | 302.42 | 855.68 | – | 4835.65 | – |
| 540 | gateway | 458.8 | 27527 | 0.00% | 313.69 | 906.57 | -45.97 | 4890.24 | 0.791 |
| 600 | direct | 50.1 | 3009 | 0.00% | 297.16 | 914.27 | – | 4894.06 | – |
| 600 | gateway | 500.0 | 29998 | 0.00% | 319.30 | 928.23 | -11.11 | 4910.17 | 0.857 |
| 660 | direct | 55.4 | 3321 | 0.00% | 299.72 | 880.07 | – | 4860.22 | – |
| 660 | gateway | 544.0 | 32638 | 0.00% | 324.54 | 927.29 | -55.21 | 4902.25 | 1.472 |
| 720 | direct | 58.5 | 3507 | 0.00% | 296.85 | 908.88 | – | 4888.61 | – |
| 720 | gateway | 592.9 | 35576 | 0.00% | 333.71 | 923.37 | -27.87 | 4898.88 | 1.071 |
| 780 | direct | 62.6 | 3757 | 0.00% | 299.16 | 863.03 | – | 4841.41 | – |
| 780 | gateway | 633.8 | 38026 | 0.00% | 348.31 | 955.05 | 30.30 | 4927.06 | 1.343 |
| 840 | direct | 68.2 | 4094 | 0.00% | 301.42 | 870.84 | – | 4850.65 | – |
| 840 | gateway | 681.1 | 40866 | 0.00% | 372.29 | 978.56 | 47.36 | 4946.99 | 1.681 |
| 900 | direct | 73.3 | 4401 | 0.00% | 295.54 | 903.79 | – | 4883.23 | – |
| 900 | gateway | 723.7 | 43424 | 0.00% | 396.27 | 1004.17 | 51.23 | 4969.71 | 1.581 |
| 960 | direct | 76.8 | 4606 | 0.00% | 302.31 | 922.94 | – | 4902.97 | – |
| 960 | gateway | 771.1 | 46268 | 0.00% | 421.43 | 1036.67 | 48.80 | 4994.05 | 1.728 |
| 1020 | direct | 82.7 | 4963 | 0.00% | 302.99 | 914.00 | – | 4893.65 | – |
| 1020 | gateway | 818.0 | 49083 | 0.00% | 449.80 | 1050.28 | 58.70 | 5006.97 | 2.347 |
| 1080 | direct | 86.3 | 5179 | 0.00% | 296.87 | 894.70 | – | 4874.38 | – |
| 1080 | gateway | 869.5 | 52172 | 0.00% | 524.54 | 1133.89 | 164.74 | 5081.49 | 4.588 |
| 1140 | direct | 89.1 | 5346 | 0.00% | 302.95 | 866.68 | – | 4846.15 | – |
| 1140 | gateway | 906.1 | 54369 | 0.00% | 650.20 | 1257.43 | 266.22 | 5188.87 | 8.441 |

### Capacity

Sustained 593 req/s within the 25ms p99 overhead SLO. First breach at 634 req/s: gateway p99 TTFT 48.5ms above baseline, at least 30.3ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 164.5 / 959.5 / 1355.3 |
| Goroutines start / end / max | 41 / 5790 / 26567 |
| Open FDs max | 9221 |
| CPU cores avg / max | 2.59 / 4.81 |
| RSS growth (MB/hour) | 2618.7 |
| Goroutine growth (per hour) | 72451.6 |

