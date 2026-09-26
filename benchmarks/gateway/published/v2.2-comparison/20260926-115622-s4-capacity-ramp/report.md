# S4 · Capacity ramp with realistic streaming upstream

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s4-capacity-ramp |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 11:56:22 UTC |
| Duration | 22m23s |
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
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.819ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 6 of 708357 failed (0.001%): HTTP 500 x6 |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.813ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 71009 failed (0.000%) |
| WARN | analytics recorded every gateway request |  | Studio gained 708818 proxy-log rows for 708824 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 68.18 [57.16, 73.04] | 132.68 [101.65, 134.87] | 153.41 [79.67, 182.19] |  |
| stream-realistic | gateway | TOTAL | 62.28 [51.74, 66.98] | 101.10 [75.34, 108.33] | 126.53 [59.33, 166.23] |  |

## Cell: stream-realistic

> **Stopped early: gateway p99 TTFT over the last 10s is 509.2ms above direct's recent p99**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 708357 | 6 | 527.7 | 367.97 | 368.02 | 681.84 | 1047.07 | 1441.98 | 4342.07 | 4999.99 | 6964.85 | 0.819 |
| direct | 71009 | 0 | 52.9 | 299.81 | 299.84 | 549.15 | 893.66 | 1259.86 | 4279.79 | 4873.47 | 6706.57 | 0.813 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.248 | 0.828 | 0.287 | 0.978 | 0.258 | 0.857 | 0.3% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 3.5 | 212 | 0.00% | 283.79 | 766.53 | – | 4745.98 | – |
| 0 | gateway | 38.3 | 2300 | 0.00% | 298.20 | 929.63 | 4.46 | 4909.94 | 0.852 |
| 60 | direct | 8.9 | 536 | 0.00% | 296.01 | 858.83 | – | 4838.14 | – |
| 60 | gateway | 93.2 | 5590 | 0.00% | 302.47 | 874.40 | -35.75 | 4854.53 | 0.455 |
| 120 | direct | 13.3 | 799 | 0.00% | 303.16 | 831.65 | – | 4812.41 | – |
| 120 | gateway | 137.9 | 8273 | 0.00% | 302.53 | 893.66 | -22.53 | 4874.44 | 0.431 |
| 180 | direct | 18.5 | 1108 | 0.00% | 290.19 | 896.28 | – | 4876.27 | – |
| 180 | gateway | 182.6 | 10955 | 0.00% | 299.83 | 903.71 | -20.78 | 4883.45 | 0.428 |
| 240 | direct | 23.1 | 1389 | 0.00% | 299.51 | 909.19 | – | 4888.10 | – |
| 240 | gateway | 224.4 | 13467 | 0.00% | 304.33 | 912.58 | -38.09 | 4892.54 | 0.454 |
| 300 | direct | 27.5 | 1652 | 0.00% | 299.58 | 868.33 | – | 4848.26 | – |
| 300 | gateway | 271.6 | 16294 | 0.00% | 300.23 | 915.71 | -21.81 | 4895.70 | 0.483 |
| 360 | direct | 31.9 | 1912 | 0.00% | 306.73 | 887.90 | – | 4868.54 | – |
| 360 | gateway | 318.2 | 19091 | 0.00% | 301.92 | 900.67 | -35.64 | 4882.76 | 0.506 |
| 420 | direct | 36.4 | 2186 | 0.00% | 301.50 | 907.84 | – | 4887.70 | – |
| 420 | gateway | 367.2 | 22033 | 0.00% | 305.01 | 900.37 | -43.50 | 4880.69 | 0.522 |
| 480 | direct | 42.4 | 2542 | 0.00% | 301.54 | 870.03 | – | 4849.12 | – |
| 480 | gateway | 413.4 | 24801 | 0.00% | 304.31 | 903.23 | -55.64 | 4883.77 | 0.554 |
| 540 | direct | 45.8 | 2750 | 0.00% | 299.60 | 869.01 | – | 4848.06 | – |
| 540 | gateway | 453.6 | 27215 | 0.00% | 304.82 | 905.94 | -5.44 | 4886.64 | 0.564 |
| 600 | direct | 48.9 | 2931 | 0.00% | 297.87 | 891.51 | – | 4871.61 | – |
| 600 | gateway | 500.4 | 30025 | 0.00% | 305.76 | 901.01 | 3.28 | 4880.70 | 0.577 |
| 660 | direct | 54.0 | 3242 | 0.00% | 299.86 | 874.60 | – | 4854.33 | – |
| 660 | gateway | 548.1 | 32888 | 0.00% | 309.32 | 912.40 | -17.52 | 4892.12 | 0.660 |
| 720 | direct | 59.6 | 3578 | 0.00% | 299.32 | 892.29 | – | 4872.12 | – |
| 720 | gateway | 590.1 | 35406 | 0.00% | 311.41 | 909.92 | -16.89 | 4882.60 | 0.689 |
| 780 | direct | 63.2 | 3794 | 0.00% | 299.20 | 873.17 | – | 4853.58 | – |
| 780 | gateway | 641.6 | 38498 | 0.00% | 315.87 | 921.12 | -52.23 | 4900.45 | 0.742 |
| 840 | direct | 67.5 | 4048 | 0.00% | 299.20 | 868.87 | – | 4848.35 | – |
| 840 | gateway | 682.6 | 40956 | 0.00% | 323.41 | 935.79 | -21.00 | 4915.58 | 0.708 |
| 900 | direct | 72.3 | 4336 | 0.00% | 298.96 | 872.04 | – | 4852.16 | – |
| 900 | gateway | 728.2 | 43691 | 0.00% | 343.15 | 953.67 | -5.91 | 4929.06 | 0.669 |
| 960 | direct | 77.2 | 4631 | 0.00% | 297.11 | 903.70 | – | 4880.97 | – |
| 960 | gateway | 775.6 | 46536 | 0.00% | 362.87 | 961.18 | 109.01 | 4931.06 | 0.684 |
| 1020 | direct | 83.5 | 5007 | 0.00% | 298.74 | 946.19 | – | 4926.08 | – |
| 1020 | gateway | 815.9 | 48952 | 0.00% | 376.71 | 983.45 | 15.15 | 4947.17 | 0.706 |
| 1080 | direct | 90.6 | 5434 | 0.00% | 299.62 | 885.63 | – | 4865.06 | – |
| 1080 | gateway | 862.3 | 51737 | 0.00% | 393.65 | 993.36 | 64.13 | 4957.79 | 1.274 |
| 1140 | direct | 92.0 | 5521 | 0.00% | 300.80 | 922.78 | – | 4903.57 | – |
| 1140 | gateway | 906.7 | 54404 | 0.00% | 420.52 | 1047.86 | 67.19 | 5003.22 | 1.808 |
| 1200 | direct | 94.7 | 5680 | 0.00% | 298.76 | 903.25 | – | 4883.04 | – |
| 1200 | gateway | 961.1 | 57665 | 0.00% | 473.95 | 1109.76 | 119.50 | 5046.75 | 3.284 |
| 1260 | direct | 98.8 | 5928 | 0.00% | 302.82 | 894.30 | – | 4874.71 | – |
| 1260 | gateway | 993.4 | 59603 | 0.01% | 555.41 | 1218.32 | 218.37 | 5173.10 | 3.392 |

### Capacity

Sustained 728 req/s within the 25ms p99 overhead SLO. First breach at 776 req/s: gateway p99 TTFT 79.6ms above baseline, at least 109.0ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 343.1 / 3394.3 / 3414.2 |
| Goroutines start / end / max | 42 / 6233 / 29729 |
| Open FDs max | 10282 |
| CPU cores avg / max | 2.64 / 5.57 |
| RSS growth (MB/hour) | 7413.0 |
| Goroutine growth (per hour) | 72527.9 |

