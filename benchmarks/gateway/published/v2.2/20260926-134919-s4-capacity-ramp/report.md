# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 13:49:19 UTC |
| Duration | 20m23s |
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
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.828ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 588494 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.833ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 59222 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 588953 proxy-log rows for 588953 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 98.90 [89.15, 104.22] | 183.24 [152.58, 197.12] | 205.71 [198.08, 297.37] |  |
| stream-realistic | gateway | TOTAL | 80.61 [71.47, 88.08] | 128.26 [104.88, 136.71] | 140.65 [134.85, 219.82] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT over the last 10s is 559.8ms above direct's recent p99**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 588494 | 0 | 481.4 | 399.66 | 399.73 | 732.14 | 1099.81 | 1481.97 | 4361.54 | 5014.79 | 7240.64 | 0.828 |
| direct | 59222 | 0 | 48.4 | 300.80 | 300.83 | 548.90 | 894.10 | 1280.94 | 4280.93 | 4874.13 | 6153.66 | 0.833 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.256 | 1.237 | 0.296 | 1.762 | 0.266 | 1.330 | 0.4% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 4.2 | 249 | 0.00% | 328.44 | 957.16 | – | 4937.64 | – |
| 0 | gateway | 37.5 | 2249 | 0.00% | 306.12 | 881.31 | -159.11 | 4861.68 | 0.869 |
| 60 | direct | 9.1 | 545 | 0.00% | 298.38 | 886.87 | – | 4866.70 | – |
| 60 | gateway | 90.2 | 5410 | 0.00% | 299.88 | 934.46 | -113.64 | 4914.81 | 0.467 |
| 120 | direct | 14.2 | 853 | 0.00% | 307.48 | 865.66 | – | 4845.56 | – |
| 120 | gateway | 136.2 | 8175 | 0.00% | 299.33 | 903.49 | -70.75 | 4882.93 | 0.430 |
| 180 | direct | 18.2 | 1094 | 0.00% | 301.81 | 953.56 | – | 4933.71 | – |
| 180 | gateway | 182.5 | 10950 | 0.00% | 302.15 | 892.78 | -140.66 | 4873.38 | 0.448 |
| 240 | direct | 23.4 | 1405 | 0.00% | 298.83 | 861.27 | – | 4840.84 | – |
| 240 | gateway | 227.3 | 13641 | 0.00% | 298.97 | 895.72 | -59.97 | 4875.50 | 0.488 |
| 300 | direct | 27.2 | 1631 | 0.00% | 302.10 | 901.31 | – | 4881.47 | – |
| 300 | gateway | 275.1 | 16506 | 0.00% | 305.92 | 914.92 | -109.56 | 4895.00 | 0.524 |
| 360 | direct | 32.1 | 1924 | 0.00% | 300.11 | 882.51 | – | 4862.83 | – |
| 360 | gateway | 318.2 | 19093 | 0.00% | 304.42 | 905.30 | -76.70 | 4884.61 | 0.648 |
| 420 | direct | 35.6 | 2138 | 0.00% | 296.26 | 936.31 | – | 4916.34 | – |
| 420 | gateway | 361.9 | 21716 | 0.00% | 306.36 | 909.18 | -52.67 | 4888.76 | 0.602 |
| 480 | direct | 41.7 | 2503 | 0.00% | 303.29 | 934.04 | – | 4912.46 | – |
| 480 | gateway | 408.1 | 24489 | 0.00% | 308.99 | 910.85 | -41.60 | 4889.00 | 0.742 |
| 540 | direct | 45.4 | 2723 | 0.00% | 306.55 | 885.57 | – | 4865.51 | – |
| 540 | gateway | 454.7 | 27282 | 0.00% | 312.27 | 923.46 | -81.83 | 4901.42 | 0.804 |
| 600 | direct | 50.2 | 3014 | 0.00% | 294.85 | 869.77 | – | 4849.84 | – |
| 600 | gateway | 501.1 | 30067 | 0.00% | 320.36 | 917.48 | -30.32 | 4896.07 | 0.980 |
| 660 | direct | 55.3 | 3317 | 0.00% | 307.08 | 866.23 | – | 4845.87 | – |
| 660 | gateway | 548.6 | 32917 | 0.00% | 326.09 | 935.80 | -90.51 | 4905.52 | 1.120 |
| 720 | direct | 58.4 | 3501 | 0.00% | 299.63 | 936.81 | – | 4915.56 | – |
| 720 | gateway | 588.8 | 35325 | 0.00% | 335.11 | 935.50 | -17.98 | 4914.51 | 1.311 |
| 780 | direct | 63.7 | 3820 | 0.00% | 300.79 | 871.83 | – | 4852.51 | – |
| 780 | gateway | 635.2 | 38111 | 0.00% | 349.18 | 954.24 | 1.44 | 4927.91 | 1.334 |
| 840 | direct | 68.4 | 4102 | 0.00% | 301.64 | 923.10 | – | 4903.66 | – |
| 840 | gateway | 684.4 | 41064 | 0.00% | 375.78 | 982.58 | 7.49 | 4953.54 | 1.251 |
| 900 | direct | 72.8 | 4366 | 0.00% | 300.37 | 886.27 | – | 4866.10 | – |
| 900 | gateway | 728.6 | 43719 | 0.00% | 398.23 | 1006.23 | 5.61 | 4973.10 | 1.284 |
| 960 | direct | 77.4 | 4645 | 0.00% | 301.16 | 911.66 | – | 4891.47 | – |
| 960 | gateway | 771.5 | 46289 | 0.00% | 415.86 | 1015.42 | 73.49 | 4974.11 | 1.406 |
| 1020 | direct | 82.6 | 4955 | 0.00% | 303.63 | 888.66 | – | 4868.12 | – |
| 1020 | gateway | 812.3 | 48739 | 0.00% | 443.70 | 1067.82 | 146.99 | 5018.80 | 1.840 |
| 1080 | direct | 88.5 | 5309 | 0.00% | 301.27 | 866.62 | – | 4846.57 | – |
| 1080 | gateway | 866.5 | 51988 | 0.00% | 517.06 | 1111.48 | 135.56 | 5075.22 | 5.094 |
| 1140 | direct | 90.8 | 5449 | 0.00% | 298.21 | 878.92 | – | 4858.85 | – |
| 1140 | gateway | 907.5 | 54448 | 0.00% | 634.66 | 1230.90 | 218.62 | 5164.68 | 10.481 |

### Capacity

Sustained 729 req/s within the 25ms p99 overhead SLO. First breach at 771 req/s: gateway p99 TTFT 112.9ms above baseline, at least 73.5ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 285.3 / 986.8 / 1317.6 |
| Goroutines start / end / max | 42 / 5803 / 26519 |
| Open FDs max | 9183 |
| CPU cores avg / max | 2.61 / 4.73 |
| RSS growth (MB/hour) | 2467.2 |
| Goroutine growth (per hour) | 72287.2 |

