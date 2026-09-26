# S3 · Real-world overhead against OpenAI and Anthropic

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s3-real-vendors |
| Mode | paired ABBA blocks against real upstreams |
| Started | 2026-09-26 17:29:45 UTC |
| Duration | 2h17m6s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target anthropic | https://api.anthropic.com/v1 |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |
| Target openai | https://api.openai.com/v1 |

The end-user view: the same request sent straight to the vendor, and through the gateway to the same vendor, from the same host. Vendor latency varies by hundreds of milliseconds from one request to the next, far more than the gateway adds, so requests are sent in ABBA blocks (direct, gateway, gateway, direct, or the reverse, chosen at random) and the overhead is the median of the within-block differences. Both arms see the same vendor conditions within a block. The prompt makes every reply run to max_tokens, so output length is the same in both arms. Compare the paired overhead with the gateway's own Server-Timing: they should agree to within the network hop to the gateway. Needs test-secrets/vendors.env and `--vendors` on both seed and run.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (gateway) | openai-stream-64 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | openai-stream-64 | 0 of 400 failed (0.000%) |
| pass | error rate (gateway) | openai-rest-64 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | openai-rest-64 | 0 of 400 failed (0.000%) |
| pass | error rate (gateway) | openai-stream-512 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | openai-stream-512 | 0 of 400 failed (0.000%) |
| pass | error rate (gateway) | openai-unified-stream | 0 of 400 failed (0.000%) |
| WARN | error rate (direct) | openai-unified-stream | 1 of 400 failed (0.250%): stream ended without content x1 |
| pass | error rate (gateway) | anthropic-stream-64 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | anthropic-stream-64 | 0 of 400 failed (0.000%) |
| pass | error rate (gateway) | anthropic-rest-64 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | anthropic-rest-64 | 0 of 400 failed (0.000%) |
| pass | error rate (gateway) | anthropic-stream-512 | 0 of 400 failed (0.000%) |
| pass | error rate (direct) | anthropic-stream-512 | 0 of 400 failed (0.000%) |
| pass | analytics recorded every gateway request |  | Studio gained 2870 proxy-log rows for 2870 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| openai-stream-64 | gateway | TTFT | -5.63 [-16.24, 9.97] | -1.96 [-29.63, 23.70] | -251.98 [-1198.88, 849.67] | -10.54 [-21.57, 4.70] (200 pairs) |
| openai-stream-64 | gateway | TOTAL | -2.48 [-24.61, 19.63] | -27.09 [-58.29, 6.19] | -145.76 [-1072.32, 702.76] | -12.92 [-30.49, 6.32] (200 pairs) |
| openai-rest-64 | gateway | TTFT | -5.95 [-24.05, 8.34] | -14.88 [-54.20, 21.08] | 326.87 [-1206.83, 848.71] | -5.42 [-20.14, 19.09] (200 pairs) |
| openai-rest-64 | gateway | TOTAL | -5.95 [-24.05, 8.34] | -14.88 [-54.20, 21.08] | 326.87 [-1206.83, 848.71] | -5.42 [-20.14, 19.09] (200 pairs) |
| openai-stream-512 | gateway | TTFT | -10.74 [-22.64, 3.89] | -11.11 [-48.74, 21.71] | -399.80 [-736.38, -40.09] | -9.75 [-21.82, 1.85] (200 pairs) |
| openai-stream-512 | gateway | TOTAL | -22.47 [-72.23, 19.34] | 6.21 [-101.19, 100.53] | 145.87 [-562.40, 384.72] | -16.80 [-69.19, 48.31] (200 pairs) |
| openai-unified-stream | gateway | TTFT | 55.47 [-111.85, 157.83] | 5.74 [-248.18, 191.96] | 724.47 [-653.84, 1605.18] | 25.66 [-113.18, 107.48] (199 pairs) |
| openai-unified-stream | gateway | TOTAL | 16.95 [-77.32, 128.87] | 36.17 [-107.24, 230.17] | 264.76 [-517.27, 782.04] | 26.27 [-44.91, 105.94] (199 pairs) |
| anthropic-stream-64 | gateway | TTFT | 9.01 [-11.74, 23.43] | -3.33 [-61.17, 63.77] | -102.17 [-272.10, 150.49] | 11.09 [-10.84, 29.41] (200 pairs) |
| anthropic-stream-64 | gateway | TOTAL | 5.25 [-15.29, 26.04] | 10.91 [-51.37, 71.55] | -133.25 [-334.92, 52.86] | 14.05 [-3.74, 25.52] (200 pairs) |
| anthropic-rest-64 | gateway | TTFT | -8.56 [-24.11, 11.67] | -10.33 [-50.74, 40.17] | -119.53 [-579.91, 189.33] | 11.02 [-15.40, 26.05] (200 pairs) |
| anthropic-rest-64 | gateway | TOTAL | -8.56 [-24.11, 11.67] | -10.33 [-50.74, 40.17] | -119.53 [-579.91, 189.33] | 11.02 [-15.40, 26.05] (200 pairs) |
| anthropic-stream-512 | gateway | TTFT | 3.22 [-12.06, 16.45] | 11.52 [-29.40, 51.59] | -50.67 [-134.09, 85.47] | 9.29 [-10.13, 22.48] (200 pairs) |
| anthropic-stream-512 | gateway | TOTAL | -56.56 [-156.71, 22.26] | -37.59 [-169.10, 106.82] | -203.41 [-528.94, 297.16] | -77.26 [-123.06, -24.91] (200 pairs) |

## Cell: openai-stream-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.9 | 623.28 | 647.47 | 746.73 | 997.80 | 2132.90 | 1067.94 | 1553.53 | 2568.45 | 0.029 |
| direct | 400 | 0 | 0.9 | 624.43 | 653.10 | 748.69 | 1249.78 | 3729.65 | 1070.43 | 1699.29 | 5294.72 | 0.026 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.239 | 1.156 | 0.283 | 1.198 | 0.260 | 1.189 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 619.8 / 286.9 / 619.8 |
| Goroutines start / end / max | 981 / 45 / 992 |
| Open FDs max | 494 |
| CPU cores avg / max | 0.01 / 0.07 |

## Cell: openai-rest-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.9 | 1079.21 | 1079.27 | 1229.58 | 1712.34 | 2316.78 | 1079.27 | 1712.34 | 2371.62 | 0.019 |
| direct | 400 | 0 | 0.9 | 1085.16 | 1085.22 | 1244.46 | 1385.47 | 3642.85 | 1085.22 | 1385.47 | 4192.29 | 0.025 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.248 | 1.112 | 0.465 | 1.220 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 286.9 / 296.2 / 296.2 |
| Goroutines start / end / max | 44 / 45 / 50 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.01 / 0.05 |

## Cell: openai-stream-512

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.3 | 586.72 | 607.87 | 720.84 | 825.39 | 1236.24 | 3611.90 | 4922.68 | 4996.10 | 0.023 |
| direct | 400 | 0 | 0.3 | 594.28 | 618.61 | 731.94 | 1225.19 | 2329.81 | 3634.37 | 4776.81 | 5627.44 | 0.033 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.977 | 1.331 | 1.023 | 1.387 | 0.999 | 1.346 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 296.2 / 305.4 / 305.4 |
| Goroutines start / end / max | 42 / 45 / 53 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.02 / 0.07 |
| RSS growth (MB/hour) | 12.2 |
| Goroutine growth (per hour) | -0.2 |

## Cell: openai-unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.1 | 2504.83 | 2523.90 | 3541.46 | 6169.50 | 6630.86 | 6534.75 | 8476.95 | 10476.30 | 0.029 |
| direct | 400 | 1 | 0.1 | 2454.22 | 2468.44 | 3535.73 | 5445.03 | 8095.82 | 6517.81 | 8212.18 | 9957.63 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 1.635 | 2.096 | 1.764 | 2.269 | 1.780 | 3.262 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 305.4 / 306.8 / 307.0 |
| Goroutines start / end / max | 48 / 45 / 60 |
| Open FDs max | 29 |
| CPU cores avg / max | 0.02 / 0.09 |
| RSS growth (MB/hour) | 3.2 |
| Goroutine growth (per hour) | 0.0 |

## Cell: anthropic-stream-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.6 | 875.51 | 875.63 | 1065.16 | 1258.39 | 1475.23 | 1678.32 | 2095.42 | 2369.98 | 0.035 |
| direct | 400 | 0 | 0.6 | 866.54 | 866.62 | 1068.49 | 1360.55 | 1622.80 | 1673.06 | 2228.66 | 2511.28 | 0.029 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.709 | 1.145 | 0.756 | 1.195 | 0.726 | 1.162 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 306.8 / 307.5 / 307.5 |
| Goroutines start / end / max | 46 / 42 / 53 |
| Open FDs max | 26 |
| CPU cores avg / max | 0.01 / 0.05 |
| RSS growth (MB/hour) | 1.7 |
| Goroutine growth (per hour) | -12.6 |

## Cell: anthropic-rest-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.6 | 1682.87 | 1682.94 | 1865.47 | 2083.22 | 2469.89 | 1682.94 | 2083.22 | 2611.17 | 0.036 |
| direct | 400 | 0 | 0.6 | 1691.40 | 1691.49 | 1875.80 | 2202.75 | 4330.89 | 1691.49 | 2202.75 | 5438.35 | 0.026 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.504 | 1.138 | 0.549 | 1.279 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 307.5 / 298.0 / 307.5 |
| Goroutines start / end / max | 41 / 42 / 48 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.01 / 0.05 |
| RSS growth (MB/hour) | -58.6 |
| Goroutine growth (per hour) | -0.3 |

## Cell: anthropic-stream-512

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.2 | 862.70 | 862.76 | 1036.37 | 1229.99 | 1276.71 | 4163.17 | 5368.00 | 6106.92 | 0.034 |
| direct | 400 | 0 | 0.2 | 859.47 | 859.54 | 1024.85 | 1280.66 | 1516.48 | 4219.73 | 5571.41 | 7085.07 | 0.035 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.931 | 1.151 | 0.984 | 1.193 | 0.950 | 1.165 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 298.0 / 298.0 / 298.0 |
| Goroutines start / end / max | 45 / 46 / 49 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.01 / 0.06 |
| RSS growth (MB/hour) | 0.0 |
| Goroutine growth (per hour) | -0.1 |

