# S6 · Burst to three times the base rate

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s6-spike |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 05:01:36 UTC |
| Duration | 3m5s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Steady traffic at a base rate, a 30-second burst to three times that, then back to base. It shows whether the gateway absorbs a burst (latency rises, then recovers) or falters (errors, or latency that stays high after the burst). The rates below assume a knee of about 200 req/s. Scale them from your S4 result: --rate-scale <sustained_rps / 200>.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.927ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 26879 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.937ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 2730 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 28023 proxy-log rows for 28023 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | -3.88 [-12.51, 4.78] | -2.55 [-27.45, 18.62] | 11.70 [-37.06, 116.88] |  |
| stream-realistic | gateway | TOTAL | -4.05 [-12.40, 4.22] | -2.47 [-27.76, 17.30] | 11.67 [-37.12, 116.94] |  |

## Cell: stream-realistic

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 26879 | 0 | 145.7 | 304.80 | 304.84 | 555.46 | 910.26 | 1294.35 | 4284.43 | 4890.12 | 6109.68 | 0.927 |
| direct | 2730 | 0 | 14.8 | 308.69 | 308.72 | 558.01 | 898.57 | 1302.23 | 4288.48 | 4878.46 | 5506.61 | 0.937 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.431 | 1.292 | 0.469 | 1.448 | 0.440 | 1.305 | 3.9% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 10 | direct | 10.8 | 54 | 0.00% | 315.42 | 754.03 | – | 4733.59 | – |
| 10 | gateway | 119.4 | 597 | 0.00% | 293.90 | 801.64 | – | 4781.88 | 0.887 |
| 15 | direct | 13.4 | 67 | 0.00% | 316.84 | 981.85 | – | 4962.26 | – |
| 15 | gateway | 121.8 | 609 | 0.00% | 292.45 | 848.09 | – | 4828.75 | 0.800 |
| 20 | direct | 13.2 | 66 | 0.00% | 306.23 | 928.01 | – | 4907.82 | – |
| 20 | gateway | 118.6 | 593 | 0.00% | 309.67 | 913.40 | – | 4894.31 | 0.906 |
| 25 | direct | 11.8 | 59 | 0.00% | 305.52 | 899.36 | – | 4879.28 | – |
| 25 | gateway | 122.0 | 610 | 0.00% | 295.93 | 830.05 | -387.25 | 4810.00 | 0.998 |
| 30 | direct | 9.6 | 48 | 0.00% | 345.32 | 757.86 | – | 4737.88 | – |
| 30 | gateway | 116.4 | 582 | 0.00% | 301.94 | 906.65 | -280.90 | 4887.65 | 0.876 |
| 35 | direct | 11.8 | 59 | 0.00% | 340.96 | 662.50 | – | 4642.74 | – |
| 35 | gateway | 116.2 | 581 | 0.00% | 301.89 | 913.88 | -268.31 | 4893.50 | 0.789 |
| 40 | direct | 11.8 | 59 | 0.00% | 278.15 | 824.22 | – | 4803.81 | – |
| 40 | gateway | 115.4 | 577 | 0.00% | 310.41 | 1001.78 | -166.19 | 4981.49 | 1.074 |
| 45 | direct | 14.6 | 73 | 0.00% | 292.67 | 759.41 | – | 4739.48 | – |
| 45 | gateway | 117.0 | 585 | 0.00% | 292.64 | 910.50 | -217.92 | 4890.77 | 0.989 |
| 50 | direct | 13.0 | 65 | 0.00% | 312.17 | 944.83 | – | 4924.64 | – |
| 50 | gateway | 121.4 | 607 | 0.00% | 307.91 | 967.37 | -147.70 | 4947.14 | 0.903 |
| 55 | direct | 12.2 | 61 | 0.00% | 288.68 | 816.56 | – | 4796.18 | – |
| 55 | gateway | 122.4 | 612 | 0.00% | 303.38 | 880.26 | -179.88 | 4860.46 | 0.975 |
| 60 | direct | 36.2 | 181 | 0.00% | 293.83 | 926.27 | – | 4906.52 | – |
| 60 | gateway | 360.0 | 1800 | 0.00% | 297.67 | 895.46 | -199.76 | 4888.93 | 1.415 |
| 65 | direct | 38.2 | 191 | 0.00% | 292.17 | 783.75 | – | 4764.01 | – |
| 65 | gateway | 355.8 | 1779 | 0.00% | 311.93 | 928.63 | -120.29 | 4908.61 | 2.087 |
| 70 | direct | 34.2 | 171 | 0.00% | 318.04 | 724.25 | – | 4704.22 | – |
| 70 | gateway | 349.0 | 1745 | 0.00% | 315.55 | 937.76 | -87.73 | 4913.43 | 3.314 |
| 75 | direct | 34.2 | 171 | 0.00% | 297.62 | 931.90 | – | 4912.64 | – |
| 75 | gateway | 337.2 | 1686 | 0.00% | 315.89 | 905.50 | -103.95 | 4880.79 | 15.081 |
| 80 | direct | 38.0 | 190 | 0.00% | 308.95 | 821.78 | – | 4802.06 | – |
| 80 | gateway | 347.0 | 1735 | 0.00% | 309.62 | 891.67 | -118.12 | 4871.53 | 2.170 |
| 85 | direct | 36.6 | 183 | 0.00% | 314.86 | 990.08 | – | 4969.37 | – |
| 85 | gateway | 337.2 | 1686 | 0.00% | 307.37 | 911.81 | -116.86 | 4892.41 | 1.508 |
| 90 | direct | 9.8 | 49 | 0.00% | 287.20 | 711.51 | – | 4691.06 | – |
| 90 | gateway | 110.6 | 553 | 0.00% | 307.03 | 901.81 | -152.07 | 4881.65 | 1.093 |
| 95 | direct | 11.4 | 57 | 0.00% | 332.91 | 893.96 | – | 4873.66 | – |
| 95 | gateway | 123.4 | 617 | 0.00% | 311.31 | 961.30 | -94.49 | 4941.05 | 0.856 |
| 100 | direct | 11.0 | 55 | 0.00% | 316.29 | 750.41 | – | 4730.68 | – |
| 100 | gateway | 112.2 | 561 | 0.00% | 300.20 | 899.70 | -134.57 | 4879.15 | 1.074 |
| 105 | direct | 9.0 | 45 | 0.00% | 308.56 | 822.70 | – | 4802.61 | – |
| 105 | gateway | 114.4 | 572 | 0.00% | 296.49 | 864.23 | -184.86 | 4844.24 | 0.835 |
| 110 | direct | 12.0 | 60 | 0.00% | 348.32 | 792.77 | – | 4772.11 | – |
| 110 | gateway | 117.2 | 586 | 0.00% | 306.05 | 926.07 | -109.31 | 4906.28 | 1.080 |
| 115 | direct | 13.8 | 69 | 0.00% | 289.24 | 937.94 | – | 4917.78 | – |
| 115 | gateway | 110.2 | 551 | 0.00% | 318.02 | 774.81 | -229.47 | 4754.31 | 0.835 |
| 120 | direct | 9.2 | 46 | 0.00% | 313.86 | 1239.98 | – | 5219.93 | – |
| 120 | gateway | 123.4 | 617 | 0.00% | 295.90 | 915.12 | -179.81 | 4894.95 | 0.945 |
| 125 | direct | 10.4 | 52 | 0.00% | 337.78 | 639.08 | – | 4619.67 | – |
| 125 | gateway | 113.2 | 566 | 0.00% | 297.53 | 959.52 | -151.29 | 4939.70 | 0.847 |
| 130 | direct | 12.4 | 62 | 0.00% | 300.68 | 826.78 | – | 4807.17 | – |
| 130 | gateway | 111.6 | 558 | 0.00% | 299.64 | 912.29 | -151.59 | 4892.17 | 0.934 |
| 135 | direct | 10.2 | 51 | 0.00% | 303.21 | 1062.30 | – | 5042.02 | – |
| 135 | gateway | 127.8 | 639 | 0.00% | 301.62 | 898.30 | -127.58 | 4878.30 | 0.769 |
| 140 | direct | 11.2 | 56 | 0.00% | 328.85 | 909.61 | – | 4889.18 | – |
| 140 | gateway | 120.2 | 601 | 0.00% | 307.68 | 982.07 | -122.25 | 4962.73 | 1.059 |
| 145 | direct | 10.8 | 54 | 0.00% | 302.97 | 845.72 | – | 4825.23 | – |
| 145 | gateway | 116.2 | 581 | 0.00% | 301.72 | 905.40 | -129.26 | 4885.06 | 0.878 |
| 150 | direct | 12.0 | 60 | 0.00% | 300.12 | 791.30 | – | 4771.62 | – |
| 150 | gateway | 119.4 | 597 | 0.00% | 298.88 | 879.45 | -176.51 | 4859.38 | 1.017 |
| 155 | direct | 12.8 | 64 | 0.00% | 289.82 | 833.04 | – | 4812.99 | – |
| 155 | gateway | 112.2 | 561 | 0.00% | 296.29 | 887.20 | -117.30 | 4867.26 | 0.785 |
| 160 | direct | 11.4 | 57 | 0.00% | 294.42 | 713.70 | – | 4693.27 | – |
| 160 | gateway | 115.8 | 579 | 0.00% | 297.68 | 941.50 | -137.57 | 4920.46 | 0.906 |
| 165 | direct | 13.8 | 69 | 0.00% | 270.34 | 584.59 | – | 4564.96 | – |
| 165 | gateway | 116.2 | 581 | 0.00% | 308.75 | 827.72 | -219.62 | 4806.95 | 0.725 |
| 170 | direct | 12.2 | 61 | 0.00% | 323.02 | 794.56 | – | 4775.43 | – |
| 170 | gateway | 118.0 | 590 | 0.00% | 302.53 | 886.60 | -154.87 | 4866.27 | 0.720 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 376.2 / 475.5 / 704.5 |
| Goroutines start / end / max | 130 / 2990 / 12939 |
| Open FDs max | 3264 |
| CPU cores avg / max | 1.15 / 2.75 |

