# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 15:08:57 UTC |
| Duration | 20m20s |
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
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.835ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 585259 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.820ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 58200 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 585748 proxy-log rows for 585748 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 99.59 [94.40, 110.19] | 185.67 [174.53, 212.20] | 214.32 [155.88, 299.35] |  |
| stream-realistic | gateway | TOTAL | 82.00 [77.64, 93.76] | 130.44 [118.63, 155.72] | 145.83 [44.97, 199.56] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT over the last 10s is 512.3ms above direct's recent p99**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 585259 | 0 | 480.3 | 399.42 | 399.48 | 734.28 | 1109.24 | 1489.34 | 4361.95 | 5021.32 | 7089.53 | 0.835 |
| direct | 58200 | 0 | 47.8 | 299.85 | 299.89 | 548.61 | 894.91 | 1302.80 | 4279.95 | 4875.49 | 6251.10 | 0.820 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.256 | 1.136 | 0.297 | 1.756 | 0.267 | 1.248 | 0.4% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 3.7 | 222 | 0.00% | 313.76 | 810.52 | – | 4789.82 | – |
| 0 | gateway | 37.8 | 2269 | 0.00% | 298.54 | 937.11 | -32.59 | 4916.70 | 0.886 |
| 60 | direct | 9.4 | 566 | 0.00% | 297.76 | 774.30 | – | 4754.79 | – |
| 60 | gateway | 90.7 | 5443 | 0.00% | 306.14 | 892.16 | -2.24 | 4872.17 | 0.420 |
| 120 | direct | 13.8 | 826 | 0.00% | 298.30 | 876.89 | – | 4856.88 | – |
| 120 | gateway | 137.3 | 8236 | 0.00% | 300.87 | 909.19 | -13.02 | 4889.27 | 0.416 |
| 180 | direct | 17.9 | 1075 | 0.00% | 296.60 | 890.06 | – | 4869.86 | – |
| 180 | gateway | 182.6 | 10958 | 0.00% | 302.08 | 910.92 | -5.00 | 4891.49 | 0.444 |
| 240 | direct | 22.8 | 1368 | 0.00% | 305.39 | 888.19 | – | 4867.80 | – |
| 240 | gateway | 231.4 | 13885 | 0.00% | 302.13 | 888.22 | -53.84 | 4869.08 | 0.506 |
| 300 | direct | 26.4 | 1581 | 0.00% | 300.79 | 913.46 | – | 4892.98 | – |
| 300 | gateway | 273.9 | 16435 | 0.00% | 302.67 | 891.96 | -48.34 | 4871.15 | 0.616 |
| 360 | direct | 31.5 | 1892 | 0.00% | 301.86 | 792.29 | – | 4772.57 | – |
| 360 | gateway | 322.2 | 19332 | 0.00% | 302.79 | 911.40 | -11.02 | 4891.22 | 0.610 |
| 420 | direct | 35.6 | 2136 | 0.00% | 305.32 | 912.39 | – | 4892.20 | – |
| 420 | gateway | 367.8 | 22065 | 0.00% | 307.85 | 889.68 | -47.15 | 4874.65 | 0.673 |
| 480 | direct | 40.5 | 2432 | 0.00% | 300.30 | 881.73 | – | 4861.72 | – |
| 480 | gateway | 408.8 | 24529 | 0.00% | 309.90 | 918.59 | -30.75 | 4899.14 | 0.766 |
| 540 | direct | 45.3 | 2720 | 0.00% | 298.94 | 904.13 | – | 4884.61 | – |
| 540 | gateway | 455.6 | 27333 | 0.00% | 313.70 | 920.95 | 18.71 | 4900.40 | 0.946 |
| 600 | direct | 50.9 | 3056 | 0.00% | 305.42 | 886.66 | – | 4866.77 | – |
| 600 | gateway | 500.0 | 30001 | 0.00% | 316.73 | 916.01 | -48.01 | 4893.23 | 0.952 |
| 660 | direct | 53.9 | 3231 | 0.00% | 296.55 | 924.75 | – | 4904.72 | – |
| 660 | gateway | 550.9 | 33053 | 0.00% | 326.51 | 918.42 | -14.71 | 4892.16 | 1.466 |
| 720 | direct | 58.5 | 3512 | 0.00% | 297.91 | 854.79 | – | 4834.52 | – |
| 720 | gateway | 587.9 | 35272 | 0.00% | 332.31 | 940.14 | -0.82 | 4917.80 | 1.150 |
| 780 | direct | 62.5 | 3751 | 0.00% | 301.54 | 892.73 | – | 4872.62 | – |
| 780 | gateway | 633.9 | 38031 | 0.00% | 347.81 | 956.87 | -14.17 | 4927.84 | 1.378 |
| 840 | direct | 68.3 | 4096 | 0.00% | 300.46 | 889.12 | – | 4869.68 | – |
| 840 | gateway | 687.4 | 41241 | 0.00% | 375.99 | 973.37 | 11.01 | 4943.03 | 1.371 |
| 900 | direct | 71.2 | 4270 | 0.00% | 297.49 | 866.31 | – | 4846.15 | – |
| 900 | gateway | 736.7 | 44204 | 0.00% | 404.84 | 1000.62 | 84.32 | 4974.01 | 1.171 |
| 960 | direct | 78.2 | 4689 | 0.00% | 299.38 | 901.63 | – | 4881.43 | – |
| 960 | gateway | 774.6 | 46474 | 0.00% | 422.88 | 1035.84 | 91.89 | 4998.71 | 1.658 |
| 1020 | direct | 82.7 | 4959 | 0.00% | 298.45 | 907.84 | – | 4888.11 | – |
| 1020 | gateway | 811.3 | 48676 | 0.00% | 447.46 | 1053.25 | 70.28 | 5013.16 | 2.122 |
| 1080 | direct | 87.4 | 5244 | 0.00% | 300.85 | 901.35 | – | 4881.32 | – |
| 1080 | gateway | 862.4 | 51741 | 0.00% | 515.35 | 1126.23 | 192.56 | 5080.19 | 3.529 |
| 1140 | direct | 89.5 | 5369 | 0.00% | 297.10 | 901.64 | – | 4881.60 | – |
| 1140 | gateway | 907.0 | 54420 | 0.00% | 661.30 | 1269.15 | 340.72 | 5192.19 | 10.759 |

### Capacity

Sustained 687 req/s within the 25ms p99 overhead SLO. First breach at 737 req/s: gateway p99 TTFT 111.0ms above baseline, at least 84.3ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 286.4 / 804.4 / 1445.4 |
| Goroutines start / end / max | 45 / 5842 / 27229 |
| Open FDs max | 9434 |
| CPU cores avg / max | 2.61 / 4.87 |
| RSS growth (MB/hour) | 2470.1 |
| Goroutine growth (per hour) | 72114.6 |

