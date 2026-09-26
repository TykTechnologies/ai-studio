# S7 · One-hour soak at 70% of capacity

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s7-soak |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 05:04:58 UTC |
| Duration | 1h0m5s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Constant Poisson load for an hour at about 70% of the S4 capacity. A healthy gateway shows flat p99 latency across the 5-minute windows, a stable goroutine count, bounded memory, no errors, and analytics for every request. Growth in any of them is a leak or a queue that does not drain. The rate below assumes a knee of about 200 req/s. Scale it from your S4 result: --rate-scale <sustained_rps / 200>.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | stream-realistic | p99 release lag 0.934ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | stream-realistic | 0 of 835059 failed (0.000%) |
| pass | load generator kept schedule (direct) | stream-realistic | p99 release lag 0.934ms (limit 5ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | stream-realistic | 0 of 84037 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 841973 proxy-log rows for 841973 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| stream-realistic | gateway | TTFT | 2.21 [-1.15, 12.34] | -0.21 [-22.09, 16.67] | 10.41 [-104.39, 3.37] |  |
| stream-realistic | gateway | TOTAL | 2.28 [-1.43, 12.22] | 0.00 [-22.16, 17.17] | 10.46 [-104.07, 4.58] |  |

## Cell: stream-realistic

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 835059 | 0 | 231.7 | 303.20 | 303.25 | 551.92 | 906.51 | 1305.61 | 4283.39 | 4886.54 | 6807.34 | 0.934 |
| direct | 84037 | 0 | 23.3 | 301.01 | 301.03 | 552.13 | 896.10 | 1296.40 | 4281.11 | 4876.08 | 6085.40 | 0.934 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.440 | 0.986 | 0.478 | 1.061 | 0.450 | 1.000 | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 21.1 | 6319 | 0.00% | 304.33 | 900.21 | – | 4880.14 | – |
| 0 | gateway | 210.7 | 63195 | 0.00% | 304.14 | 918.47 | -53.83 | 4898.35 | 1.082 |
| 300 | direct | 23.4 | 7032 | 0.00% | 304.00 | 884.34 | – | 4864.39 | – |
| 300 | gateway | 232.5 | 69751 | 0.00% | 303.61 | 918.84 | -17.94 | 4898.48 | 1.104 |
| 600 | direct | 23.3 | 6990 | 0.00% | 295.85 | 913.81 | – | 4893.66 | – |
| 600 | gateway | 233.3 | 69976 | 0.00% | 302.71 | 904.60 | -26.29 | 4884.80 | 1.073 |
| 900 | direct | 24.0 | 7189 | 0.00% | 301.38 | 922.86 | – | 4902.75 | – |
| 900 | gateway | 233.0 | 69900 | 0.00% | 301.86 | 891.12 | -77.86 | 4871.38 | 1.014 |
| 1200 | direct | 23.8 | 7128 | 0.00% | 305.07 | 906.53 | – | 4886.69 | – |
| 1200 | gateway | 234.5 | 70361 | 0.00% | 304.81 | 892.18 | -1.92 | 4871.92 | 1.072 |
| 1500 | direct | 23.1 | 6930 | 0.00% | 300.28 | 907.10 | – | 4886.29 | – |
| 1500 | gateway | 234.9 | 70460 | 0.00% | 303.68 | 906.01 | -75.03 | 4885.74 | 1.076 |
| 1800 | direct | 23.3 | 6980 | 0.00% | 298.72 | 907.45 | – | 4887.55 | – |
| 1800 | gateway | 234.2 | 70257 | 0.00% | 302.39 | 905.93 | -71.19 | 4886.38 | 1.062 |
| 2100 | direct | 23.9 | 7156 | 0.00% | 297.82 | 881.30 | – | 4860.55 | – |
| 2100 | gateway | 234.8 | 70425 | 0.00% | 302.47 | 912.91 | -39.35 | 4892.20 | 1.053 |
| 2400 | direct | 23.8 | 7133 | 0.00% | 299.04 | 886.11 | – | 4866.35 | – |
| 2400 | gateway | 233.6 | 70071 | 0.00% | 303.65 | 907.61 | -77.18 | 4887.91 | 1.047 |
| 2700 | direct | 23.6 | 7068 | 0.00% | 301.58 | 889.58 | – | 4868.41 | – |
| 2700 | gateway | 233.7 | 70122 | 0.00% | 301.23 | 901.82 | -89.48 | 4881.39 | 1.063 |
| 3000 | direct | 23.6 | 7080 | 0.00% | 300.13 | 891.68 | – | 4871.61 | – |
| 3000 | gateway | 233.3 | 70001 | 0.00% | 304.23 | 913.07 | -26.69 | 4893.75 | 1.044 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 477.2 / 510.0 / 599.9 |
| Goroutines start / end / max | 1061 / 4832 / 9064 |
| Open FDs max | 2304 |
| CPU cores avg / max | 1.67 / 2.08 |
| RSS growth (MB/hour) | 0.0 |
| Goroutine growth (per hour) | 64.2 |

