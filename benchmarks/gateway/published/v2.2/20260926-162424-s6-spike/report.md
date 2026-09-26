# S6 · Burst to three times the base rate

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s6-spike |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-26 16:24:24 UTC |
| Duration | 3m22s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Steady traffic at a base rate, a 30-second burst to three times that, then back to base. It shows whether the gateway absorbs a burst (latency rises, then recovers) or falters (errors, or latency that stays high after the burst). The rates below assume a knee of about 200 req/s. Scale them from your S4 result: --rate-scale <sustained_rps / 200>.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.931ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 43340 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.944ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 4344 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 45191 proxy-log rows for 45191 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 10.82 [5.19, 18.90] | 14.10 [-11.11, 23.14] | 13.72 [-2.22, 130.83] |  |
| stream-realistic | gateway | TOTAL | 10.05 [5.55, 18.92] | 12.30 [-11.37, 22.90] | 17.12 [-0.42, 137.70] |  |

## Cell: stream-realistic

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 43340 | 0 | 234.1 | 312.78 | 312.83 | 560.70 | 901.61 | 1330.62 | 4291.99 | 4884.44 | 5976.55 | 0.931 |
| direct | 4344 | 0 | 23.5 | 301.98 | 302.02 | 546.60 | 887.89 | 1183.14 | 4281.94 | 4867.32 | 5375.09 | 0.944 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.227 | 0.533 | 0.266 | 0.629 | 0.237 | 0.547 | 3.5% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 10 | direct | 19.8 | 99 | 0.00% | 282.92 | 645.17 | – | 4624.68 | – |
| 10 | gateway | 193.8 | 969 | 0.00% | 303.86 | 919.08 | – | 4899.37 | 0.456 |
| 15 | direct | 19.0 | 95 | 0.00% | 295.70 | 982.52 | – | 4962.80 | – |
| 15 | gateway | 178.6 | 893 | 0.00% | 300.84 | 875.76 | – | 4855.34 | 0.474 |
| 20 | direct | 18.6 | 93 | 0.00% | 277.96 | 873.65 | – | 4853.52 | – |
| 20 | gateway | 195.0 | 975 | 0.00% | 300.72 | 896.61 | -153.67 | 4876.77 | 0.443 |
| 25 | direct | 17.0 | 85 | 0.00% | 291.12 | 964.99 | – | 4945.11 | – |
| 25 | gateway | 189.6 | 948 | 0.00% | 304.27 | 1041.93 | -94.31 | 5021.79 | 0.457 |
| 30 | direct | 20.0 | 100 | 0.00% | 256.84 | 766.16 | – | 4745.79 | – |
| 30 | gateway | 199.8 | 999 | 0.00% | 300.05 | 901.77 | -125.38 | 4880.83 | 0.571 |
| 35 | direct | 20.2 | 101 | 0.00% | 307.64 | 854.91 | – | 4834.27 | – |
| 35 | gateway | 179.0 | 895 | 0.00% | 303.51 | 950.13 | -80.21 | 4930.51 | 0.485 |
| 40 | direct | 18.0 | 90 | 0.00% | 302.05 | 736.99 | – | 4717.00 | – |
| 40 | gateway | 184.2 | 921 | 0.00% | 303.69 | 836.85 | -136.58 | 4816.91 | 0.470 |
| 45 | direct | 22.0 | 110 | 0.00% | 298.05 | 747.02 | – | 4726.90 | – |
| 45 | gateway | 176.2 | 881 | 0.00% | 305.99 | 947.79 | -70.57 | 4927.76 | 0.430 |
| 50 | direct | 19.4 | 97 | 0.00% | 325.95 | 926.90 | – | 4907.42 | – |
| 50 | gateway | 175.6 | 878 | 0.00% | 305.19 | 913.77 | -129.14 | 4893.71 | 0.466 |
| 55 | direct | 17.6 | 88 | 0.00% | 318.31 | 1081.58 | – | 5061.24 | – |
| 55 | gateway | 185.6 | 928 | 0.00% | 302.67 | 882.15 | -129.31 | 4862.48 | 0.549 |
| 60 | direct | 55.2 | 276 | 0.00% | 306.68 | 931.26 | – | 4911.07 | – |
| 60 | gateway | 569.2 | 2846 | 0.00% | 311.42 | 882.83 | -147.61 | 4873.38 | 0.680 |
| 65 | direct | 54.8 | 274 | 0.00% | 304.84 | 806.89 | – | 4787.24 | – |
| 65 | gateway | 570.8 | 2854 | 0.00% | 329.75 | 911.31 | -68.47 | 4893.98 | 1.182 |
| 70 | direct | 56.4 | 282 | 0.00% | 308.59 | 862.83 | – | 4842.89 | – |
| 70 | gateway | 564.0 | 2820 | 0.00% | 327.86 | 928.15 | -49.23 | 4898.43 | 0.784 |
| 75 | direct | 63.8 | 319 | 0.00% | 292.50 | 846.83 | – | 4826.66 | – |
| 75 | gateway | 582.6 | 2913 | 0.00% | 333.54 | 944.67 | -22.46 | 4919.23 | 0.944 |
| 80 | direct | 56.2 | 281 | 0.00% | 334.18 | 861.40 | – | 4841.24 | – |
| 80 | gateway | 571.4 | 2857 | 0.00% | 334.18 | 864.83 | -70.42 | 4849.17 | 1.469 |
| 85 | direct | 52.4 | 262 | 0.00% | 297.71 | 785.88 | – | 4766.50 | – |
| 85 | gateway | 552.8 | 2764 | 0.00% | 330.17 | 910.58 | -35.93 | 4891.11 | 0.736 |
| 90 | direct | 20.0 | 100 | 0.00% | 326.49 | 747.38 | – | 4728.10 | – |
| 90 | gateway | 188.0 | 940 | 0.00% | 308.51 | 884.25 | -45.95 | 4864.03 | 0.558 |
| 95 | direct | 17.2 | 86 | 0.00% | 281.90 | 732.36 | – | 4712.30 | – |
| 95 | gateway | 191.8 | 959 | 0.00% | 299.19 | 852.10 | -82.25 | 4836.92 | 0.463 |
| 100 | direct | 18.2 | 91 | 0.00% | 310.93 | 655.55 | – | 4635.44 | – |
| 100 | gateway | 189.6 | 948 | 0.00% | 292.54 | 825.97 | -142.30 | 4805.64 | 0.487 |
| 105 | direct | 17.2 | 86 | 0.00% | 316.79 | 991.95 | – | 4972.23 | – |
| 105 | gateway | 195.6 | 978 | 0.00% | 306.44 | 898.28 | -74.47 | 4877.55 | 0.494 |
| 110 | direct | 17.2 | 86 | 0.00% | 287.68 | 810.54 | – | 4790.60 | – |
| 110 | gateway | 192.2 | 961 | 0.00% | 308.93 | 880.36 | -99.78 | 4857.51 | 0.447 |
| 115 | direct | 16.4 | 82 | 0.00% | 299.15 | 695.26 | – | 4676.00 | – |
| 115 | gateway | 200.2 | 1001 | 0.00% | 296.86 | 872.90 | -90.32 | 4852.28 | 0.451 |
| 120 | direct | 17.0 | 85 | 0.00% | 294.93 | 971.15 | – | 4951.13 | – |
| 120 | gateway | 184.6 | 923 | 0.00% | 311.14 | 906.49 | -66.04 | 4886.42 | 0.421 |
| 125 | direct | 19.0 | 95 | 0.00% | 307.12 | 904.05 | – | 4884.62 | – |
| 125 | gateway | 183.8 | 919 | 0.00% | 303.84 | 899.87 | -46.92 | 4879.70 | 0.471 |
| 130 | direct | 18.2 | 91 | 0.00% | 312.01 | 845.15 | – | 4824.92 | – |
| 130 | gateway | 179.4 | 897 | 0.00% | 309.08 | 859.56 | -93.17 | 4839.65 | 0.441 |
| 135 | direct | 20.8 | 104 | 0.00% | 316.08 | 884.77 | – | 4864.68 | – |
| 135 | gateway | 198.8 | 994 | 0.00% | 307.67 | 963.34 | -70.56 | 4943.06 | 0.450 |
| 140 | direct | 20.6 | 103 | 0.00% | 294.86 | 683.47 | – | 4664.06 | – |
| 140 | gateway | 180.4 | 902 | 0.00% | 304.08 | 863.47 | -104.06 | 4845.21 | 0.473 |
| 145 | direct | 20.2 | 101 | 0.00% | 286.90 | 774.67 | – | 4754.93 | – |
| 145 | gateway | 196.0 | 980 | 0.00% | 296.87 | 884.49 | -110.53 | 4864.69 | 0.482 |
| 150 | direct | 20.6 | 103 | 0.00% | 273.08 | 601.69 | – | 4581.66 | – |
| 150 | gateway | 185.6 | 928 | 0.00% | 299.87 | 946.57 | -42.38 | 4926.63 | 0.461 |
| 155 | direct | 18.4 | 92 | 0.00% | 342.32 | 663.48 | – | 4644.01 | – |
| 155 | gateway | 182.4 | 912 | 0.00% | 298.99 | 889.41 | -56.69 | 4868.78 | 0.452 |
| 160 | direct | 17.2 | 86 | 0.00% | 323.45 | 824.06 | – | 4803.40 | – |
| 160 | gateway | 183.8 | 919 | 0.00% | 306.37 | 1011.85 | -51.30 | 4992.35 | 0.507 |
| 165 | direct | 18.6 | 93 | 0.00% | 313.63 | 908.09 | – | 4888.01 | – |
| 165 | gateway | 193.0 | 965 | 0.00% | 306.68 | 878.46 | -111.57 | 4858.54 | 0.469 |
| 170 | direct | 19.4 | 97 | 0.00% | 270.81 | 610.93 | – | 4590.95 | – |
| 170 | gateway | 186.6 | 933 | 0.00% | 297.79 | 872.98 | -95.70 | 4858.28 | 0.429 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 203.3 / 559.9 / 782.5 |
| Goroutines start / end / max | 40 / 1965 / 15300 |
| Open FDs max | 5130 |
| CPU cores avg / max | 1.45 / 3.58 |

