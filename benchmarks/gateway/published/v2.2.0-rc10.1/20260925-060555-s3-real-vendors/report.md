# S3 · Real-world overhead against OpenAI and Anthropic

> **INVALID: a validity check failed (see below). Do not quote these numbers.**

|  |  |
| --- | --- |
| Scenario | s3-real-vendors |
| Mode | paired ABBA blocks against real upstreams |
| Started | 2026-09-25 06:05:55 UTC |
| Duration | 50m19s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
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
| WARN | error rate (gateway) | openai-unified-stream | 1 of 16 failed (6.250%): canceled x1 |
| FAIL | few enough failures to compare (gateway) | openai-unified-stream | 6.2% of requests failed; latency figures would describe only the survivors |
| WARN | error rate (direct) | openai-unified-stream | 1 of 16 failed (6.250%): canceled x1 |
| FAIL | few enough failures to compare (direct) | openai-unified-stream | 6.2% of requests failed; latency figures would describe only the survivors |
| WARN | analytics recorded every gateway request |  | Studio gained -2118225 proxy-log rows for 1255 gateway responses (GET /api/v1/analytics/proxy-logs-for-app?app_id=1&start_date=2026-09-24&end_date=2026-09-26&page_size=1: Get "http://10.77.1.94:8080/api/v1/analytics/proxy-logs-for-app?app_id=1&start_date=2026-09-24&end_date=2026-09-26&page_size=1": interrupt signal received) |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| openai-stream-64 | gateway | TTFT | 15.29 [-2.05, 27.90] | 26.69 [-11.13, 75.51] | 415.67 [-734.51, 750.53] | 5.21 [-8.97, 31.51] (200 pairs) |
| openai-stream-64 | gateway | TOTAL | 19.52 [-1.98, 38.27] | 36.58 [-8.34, 111.73] | 366.42 [-935.09, 682.20] | 17.62 [-4.02, 39.49] (200 pairs) |
| openai-rest-64 | gateway | TTFT | -8.53 [-30.86, 12.01] | 29.09 [-11.43, 92.76] | 380.49 [-921.88, 3400.11] | -11.73 [-30.00, 5.81] (200 pairs) |
| openai-rest-64 | gateway | TOTAL | -8.53 [-30.86, 12.01] | 29.09 [-11.43, 92.76] | 380.49 [-921.88, 3400.11] | -11.73 [-30.00, 5.81] (200 pairs) |
| openai-stream-512 | gateway | TTFT | -11.35 [-21.50, 4.51] | -29.20 [-102.09, 31.88] | -19.73 [-518.31, 386.26] | -8.01 [-18.59, 5.30] (200 pairs) |
| openai-stream-512 | gateway | TOTAL | 34.13 [-29.69, 98.61] | -18.63 [-130.16, 93.20] | 55.57 [-259.82, 389.28] | 2.33 [-88.81, 62.03] (200 pairs) |
| openai-unified-stream | gateway | TTFT | -391.18 [-1075.88, 284.07] | -551.44 [-1619.91, 1214.13] | 36.57 [-1500.86, 1008.15] | -309.87 [-823.50, 448.20] (7 pairs) |
| openai-unified-stream | gateway | TOTAL | -181.61 [-1118.44, 537.34] | -836.14 [-6567.59, 629.10] | -4891.84 [-6574.45, 453.00] | -257.53 [-1415.35, 644.95] (7 pairs) |

## Cell: openai-stream-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.8 | 712.68 | 737.90 | 890.32 | 1634.18 | 2562.72 | 1160.76 | 2015.33 | 3388.10 | 0.034 |
| direct | 400 | 0 | 0.8 | 699.57 | 722.62 | 863.63 | 1218.51 | 3011.81 | 1141.24 | 1648.92 | 3847.22 | 0.031 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.570 | 1.226 | 0.621 | 1.274 | 0.594 | 1.248 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 498.2 / 330.0 / 498.6 |
| Goroutines start / end / max | 1061 / 48 / 1078 |
| Open FDs max | 558 |
| CPU cores avg / max | 0.01 / 0.37 |

## Cell: openai-rest-64

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.8 | 1236.43 | 1236.50 | 1458.49 | 2307.18 | 8828.62 | 1236.50 | 2307.18 | 11175.11 | 0.034 |
| direct | 400 | 0 | 0.8 | 1244.99 | 1245.04 | 1429.40 | 1926.69 | 3449.21 | 1245.04 | 1926.69 | 3850.84 | 0.035 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.553 | 1.180 | 0.720 | 1.329 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 330.0 / 331.0 / 331.3 |
| Goroutines start / end / max | 48 / 47 / 53 |
| Open FDs max | 18 |
| CPU cores avg / max | 0.01 / 0.04 |

## Cell: openai-stream-512

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 400 | 0 | 0.2 | 764.95 | 790.71 | 936.17 | 1558.05 | 2546.34 | 4043.14 | 5521.51 | 7021.52 | 0.033 |
| direct | 400 | 0 | 0.2 | 776.95 | 802.06 | 965.37 | 1577.78 | 2099.83 | 4009.02 | 5465.94 | 5853.75 | 0.032 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.984 | 5.876 | 1.038 | 5.936 | 1.010 | 5.901 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 331.4 / 334.7 / 335.3 |
| Goroutines start / end / max | 50 / 44 / 50 |
| Open FDs max | 17 |
| CPU cores avg / max | 0.01 / 0.06 |
| RSS growth (MB/hour) | 2.3 |
| Goroutine growth (per hour) | 0.4 |

## Cell: openai-unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 16 | 1 | 0.1 | 3557.32 | 3588.28 | 4791.76 | 5971.82 | 6119.54 | 9879.72 | 11629.24 | 11772.31 | 0.017 |
| direct | 16 | 1 | 0.1 | 3893.93 | 3979.47 | 5343.20 | 5935.25 | 6007.46 | 10061.33 | 16521.08 | 17313.38 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 1.732 | 1.993 | 1.832 | 2.220 | 1.983 | 2.154 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 334.7 / 331.5 / 334.8 |
| Goroutines start / end / max | 48 / 47 / 57 |
| Open FDs max | 21 |
| CPU cores avg / max | 0.01 / 0.05 |

