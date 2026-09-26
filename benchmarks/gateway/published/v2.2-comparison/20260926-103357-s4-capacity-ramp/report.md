# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 10:33:57 UTC |
| Duration | 22m24s |
| Code | 4f56d8e2adf4 |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | v2.2 build: edge b385573c (PR1-4, #624, #625; defaults, no tuning env), Studio 9a813a95 (#626); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Arrivals follow a Poisson process whose rate steps up every minute. The mock upstream behaves like a hosted chat model (about 300ms to first token, 50 tokens/s, 200 tokens), so each stream stays open for about 4 seconds and the gateway holds thousands of concurrent streams at the top of the ramp. A 10% sample of traffic goes straight to the mock as the baseline. The capacity is the highest rate at which the gateway's p99 time to first token stays within 25ms of the baseline's with an error rate below 0.1%. The ramp stops early once the gateway is clearly past it. Size S6 and S7 from this result with --rate-scale.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.828ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 1 of 708784 failed (0.000%): HTTP 500 x1 |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.821ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 70591 failed (0.000%) |
| WARN | analytics recorded every gateway request |  | Studio gained 709249 proxy-log rows for 709250 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 65.01 [63.45, 80.64] | 123.74 [99.65, 142.91] | 153.06 [120.07, 261.16] |  |
| stream-realistic | gateway | TOTAL | 58.92 [55.94, 72.34] | 94.19 [68.45, 104.30] | 124.69 [98.14, 211.05] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT over the last 10s is 526.5ms above direct's recent p99**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 708784 | 1 | 527.6 | 366.07 | 366.13 | 677.21 | 1048.13 | 1448.08 | 4340.06 | 4999.77 | 6696.84 | 0.828 |
| direct | 70591 | 0 | 52.5 | 301.08 | 301.11 | 553.47 | 895.07 | 1316.17 | 4281.15 | 4875.08 | 6366.98 | 0.821 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.247 | 0.840 | 0.286 | 0.997 | 0.257 | 0.865 | 0.4% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 4.0 | 238 | 0.00% | 294.92 | 954.93 | – | 4935.13 | – |
| 0 | gateway | 36.7 | 2200 | 0.00% | 301.47 | 904.36 | -189.99 | 4884.84 | 0.872 |
| 60 | direct | 9.3 | 561 | 0.00% | 300.82 | 930.67 | – | 4910.87 | – |
| 60 | gateway | 91.6 | 5495 | 0.00% | 299.79 | 925.27 | -102.28 | 4905.90 | 0.463 |
| 120 | direct | 13.4 | 802 | 0.00% | 297.13 | 871.33 | – | 4852.05 | – |
| 120 | gateway | 136.5 | 8190 | 0.00% | 299.85 | 883.52 | -116.94 | 4863.29 | 0.427 |
| 180 | direct | 17.1 | 1023 | 0.00% | 297.28 | 938.35 | – | 4918.16 | – |
| 180 | gateway | 182.3 | 10936 | 0.00% | 299.98 | 909.48 | -100.07 | 4889.66 | 0.431 |
| 240 | direct | 22.1 | 1325 | 0.00% | 301.50 | 877.83 | – | 4857.45 | – |
| 240 | gateway | 230.6 | 13838 | 0.00% | 299.73 | 906.81 | -108.12 | 4887.11 | 0.472 |
| 300 | direct | 27.0 | 1618 | 0.00% | 302.26 | 862.84 | – | 4843.23 | – |
| 300 | gateway | 272.0 | 16322 | 0.00% | 300.55 | 896.54 | -46.02 | 4875.99 | 0.470 |
| 360 | direct | 31.5 | 1891 | 0.00% | 306.94 | 899.68 | – | 4879.93 | – |
| 360 | gateway | 317.6 | 19054 | 0.00% | 303.12 | 922.77 | -38.17 | 4903.78 | 0.494 |
| 420 | direct | 35.7 | 2142 | 0.00% | 299.76 | 915.47 | – | 4894.72 | – |
| 420 | gateway | 364.0 | 21841 | 0.00% | 304.04 | 889.74 | -100.17 | 4870.10 | 0.511 |
| 480 | direct | 39.8 | 2388 | 0.00% | 302.94 | 923.06 | – | 4902.60 | – |
| 480 | gateway | 410.6 | 24637 | 0.00% | 302.24 | 902.92 | -98.30 | 4882.51 | 0.527 |
| 540 | direct | 45.0 | 2702 | 0.00% | 303.92 | 884.52 | – | 4863.89 | – |
| 540 | gateway | 454.3 | 27256 | 0.00% | 305.04 | 911.23 | -50.39 | 4889.96 | 0.556 |
| 600 | direct | 50.6 | 3038 | 0.00% | 301.49 | 867.68 | – | 4847.84 | – |
| 600 | gateway | 496.1 | 29767 | 0.00% | 305.42 | 910.04 | -57.31 | 4888.08 | 0.566 |
| 660 | direct | 54.8 | 3288 | 0.00% | 300.01 | 873.89 | – | 4853.61 | – |
| 660 | gateway | 547.0 | 32822 | 0.00% | 306.75 | 904.35 | -54.32 | 4885.37 | 0.621 |
| 720 | direct | 60.9 | 3654 | 0.00% | 305.51 | 906.04 | – | 4885.96 | – |
| 720 | gateway | 585.5 | 35132 | 0.00% | 309.53 | 917.33 | -57.23 | 4898.99 | 0.701 |
| 780 | direct | 64.7 | 3883 | 0.00% | 297.13 | 866.82 | – | 4847.20 | – |
| 780 | gateway | 636.5 | 38188 | 0.00% | 316.10 | 918.38 | -58.28 | 4895.79 | 0.817 |
| 840 | direct | 66.4 | 3982 | 0.00% | 305.19 | 877.39 | – | 4857.95 | – |
| 840 | gateway | 682.5 | 40950 | 0.00% | 320.72 | 927.59 | -46.68 | 4907.82 | 0.687 |
| 900 | direct | 72.8 | 4366 | 0.00% | 302.44 | 911.89 | – | 4892.05 | – |
| 900 | gateway | 728.3 | 43699 | 0.00% | 347.96 | 953.94 | 19.44 | 4926.89 | 0.668 |
| 960 | direct | 76.4 | 4583 | 0.00% | 301.28 | 868.57 | – | 4848.20 | – |
| 960 | gateway | 769.2 | 46154 | 0.00% | 356.78 | 955.01 | 5.62 | 4934.51 | 0.688 |
| 1020 | direct | 82.1 | 4926 | 0.00% | 299.30 | 875.55 | – | 4859.01 | – |
| 1020 | gateway | 823.7 | 49420 | 0.00% | 375.25 | 977.58 | 50.33 | 4942.43 | 0.742 |
| 1080 | direct | 84.7 | 5080 | 0.00% | 306.80 | 918.38 | – | 4898.21 | – |
| 1080 | gateway | 865.6 | 51936 | 0.00% | 390.80 | 1004.55 | 28.68 | 4970.67 | 1.050 |
| 1140 | direct | 90.3 | 5418 | 0.00% | 296.53 | 928.67 | – | 4908.59 | – |
| 1140 | gateway | 915.0 | 54897 | 0.00% | 416.37 | 1030.11 | 70.77 | 4995.55 | 1.819 |
| 1200 | direct | 93.7 | 5623 | 0.00% | 298.61 | 867.53 | – | 4847.87 | – |
| 1200 | gateway | 960.2 | 57613 | 0.00% | 462.34 | 1076.29 | 135.70 | 5032.15 | 2.407 |
| 1260 | direct | 102.5 | 6152 | 0.00% | 298.95 | 881.55 | – | 4861.88 | – |
| 1260 | gateway | 999.7 | 59980 | 0.00% | 562.44 | 1232.59 | 302.79 | 5193.39 | 6.283 |

### Capacity

Sustained 769 req/s within the 25ms p99 overhead SLO. First breach at 824 req/s: gateway p99 TTFT 84.5ms above baseline, at least 50.3ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 164.3 / 3468.3 / 3468.3 |
| Goroutines start / end / max | 41 / 6184 / 30075 |
| Open FDs max | 10162 |
| CPU cores avg / max | 2.62 / 5.51 |
| RSS growth (MB/hour) | 7813.3 |
| Goroutine growth (per hour) | 72578.3 |

