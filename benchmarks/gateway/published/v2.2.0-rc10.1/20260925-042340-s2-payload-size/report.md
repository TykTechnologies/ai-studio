# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 04:23:40 UTC |
| Duration | 44s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

The gateway reads and inspects request bodies (model validation, filters, analytics) and relays every streamed chunk. This scenario checks how its overhead grows with a large prompt (long context) and with a long stream of small chunks. It uses the native /llm/call/ route, unloaded, one request at a time.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (gateway) | prompt-1kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-1kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.658ms vs Server-Timing gw median 0.310ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.343ms vs Server-Timing gw median 0.854ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 4.943ms vs Server-Timing gw median 4.334ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.681ms vs Server-Timing gw median 0.336ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.66 [0.64, 0.67] | 0.78 [0.76, 0.79] | 0.97 [0.87, 1.40] |  |
| prompt-1kb | gateway | TOTAL | 0.66 [0.64, 0.67] | 0.78 [0.76, 0.79] | 0.97 [0.87, 1.40] |  |
| prompt-32kb | gateway | TTFT | 1.34 [1.33, 1.36] | 1.49 [1.45, 1.53] | 2.89 [2.44, 3.11] |  |
| prompt-32kb | gateway | TOTAL | 1.34 [1.33, 1.36] | 1.49 [1.45, 1.53] | 2.89 [2.44, 3.11] |  |
| prompt-256kb | gateway | TTFT | 4.94 [4.92, 4.97] | 6.27 [5.85, 6.84] | 10.32 [9.97, 10.51] |  |
| prompt-256kb | gateway | TOTAL | 4.94 [4.92, 4.97] | 6.27 [5.85, 6.84] | 10.32 [9.97, 10.51] |  |
| stream-1000-chunks | gateway | TTFT | 0.63 [0.61, 0.64] | 0.82 [0.78, 0.85] | 2.02 [1.53, 2.33] |  |
| stream-1000-chunks | gateway | TOTAL | 0.68 [0.66, 0.70] | 0.78 [0.73, 0.85] | 1.80 [1.38, 2.01] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 489.4 | 1.27 | 1.30 | 1.56 | 1.90 | 4.01 | 1.30 | 1.90 | 7.27 | 0.016 |
| direct | 2000 | 0 | 489.4 | 0.62 | 0.64 | 0.79 | 0.92 | 1.08 | 0.64 | 0.92 | 1.27 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.299 | 0.668 | 0.310 | 0.680 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 329.9 / 317.7 / 329.9 |
| Goroutines start / end / max | 42 / 2098 / 2098 |
| Open FDs max | 65 |
| CPU cores avg / max | 0.77 / 0.78 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 317.3 | 2.09 | 2.12 | 2.51 | 4.41 | 7.13 | 2.12 | 4.41 | 7.98 | 0.015 |
| direct | 2000 | 0 | 317.3 | 0.75 | 0.77 | 1.02 | 1.51 | 1.88 | 0.77 | 1.51 | 2.21 | 0.012 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.842 | 2.292 | 0.854 | 2.349 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 323.5 / 316.0 / 323.5 |
| Goroutines start / end / max | 2450 / 4116 / 4116 |
| Open FDs max | 65 |
| CPU cores avg / max | 0.94 / 0.96 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 97.0 | 6.99 | 7.02 | 9.14 | 13.57 | 14.42 | 7.02 | 13.57 | 15.08 | 0.010 |
| direct | 2000 | 0 | 97.0 | 2.05 | 2.07 | 2.87 | 3.25 | 3.57 | 2.07 | 3.25 | 3.76 | 0.009 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.321 | 10.852 | 4.334 | 10.862 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 318.9 / 320.3 / 324.5 |
| Goroutines start / end / max | 3786 / 1059 / 3786 |
| Open FDs max | 65 |
| CPU cores avg / max | 0.94 / 0.99 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 153.4 | 1.18 | 1.25 | 1.65 | 3.10 | 3.98 | 3.35 | 5.33 | 6.77 | 0.016 |
| direct | 2000 | 0 | 153.4 | 0.55 | 0.62 | 0.83 | 1.08 | 1.23 | 2.67 | 3.54 | 4.24 | 0.019 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.309 | 1.010 | 0.336 | 1.043 | 0.320 | 1.015 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 320.4 / 321.3 / 321.3 |
| Goroutines start / end / max | 1115 / 1658 / 1658 |
| Open FDs max | 62 |
| CPU cores avg / max | 1.16 / 1.20 |

