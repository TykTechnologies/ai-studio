# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 04:02:03 UTC |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.685ms vs Server-Timing gw median 0.310ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.377ms vs Server-Timing gw median 0.860ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 5.066ms vs Server-Timing gw median 4.391ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.685ms vs Server-Timing gw median 0.343ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.69 [0.67, 0.70] | 0.82 [0.80, 0.84] | 0.99 [0.92, 1.15] |  |
| prompt-1kb | gateway | TOTAL | 0.69 [0.67, 0.70] | 0.82 [0.80, 0.84] | 0.99 [0.92, 1.15] |  |
| prompt-32kb | gateway | TTFT | 1.38 [1.36, 1.39] | 1.56 [1.52, 1.61] | 3.07 [2.50, 3.37] |  |
| prompt-32kb | gateway | TOTAL | 1.38 [1.36, 1.39] | 1.56 [1.52, 1.61] | 3.07 [2.50, 3.37] |  |
| prompt-256kb | gateway | TTFT | 5.07 [5.04, 5.09] | 6.44 [6.03, 6.94] | 10.44 [9.98, 10.84] |  |
| prompt-256kb | gateway | TOTAL | 5.07 [5.04, 5.09] | 6.44 [6.03, 6.94] | 10.44 [9.98, 10.84] |  |
| stream-1000-chunks | gateway | TTFT | 0.62 [0.61, 0.64] | 0.82 [0.78, 0.84] | 1.98 [1.57, 2.59] |  |
| stream-1000-chunks | gateway | TOTAL | 0.68 [0.67, 0.71] | 0.84 [0.78, 0.91] | 2.14 [1.56, 2.50] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 480.1 | 1.31 | 1.33 | 1.61 | 1.93 | 3.38 | 1.33 | 1.93 | 3.55 | 0.010 |
| direct | 2000 | 0 | 480.1 | 0.62 | 0.65 | 0.80 | 0.94 | 1.09 | 0.65 | 0.94 | 1.29 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.299 | 0.643 | 0.310 | 0.658 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 238.5 / 224.6 / 238.5 |
| Goroutines start / end / max | 44 / 2050 / 2050 |
| Open FDs max | 61 |
| CPU cores avg / max | 0.76 / 0.79 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 318.6 | 2.10 | 2.12 | 2.56 | 4.49 | 7.53 | 2.13 | 4.49 | 10.00 | 0.009 |
| direct | 2000 | 0 | 318.6 | 0.72 | 0.75 | 1.00 | 1.42 | 1.66 | 0.75 | 1.42 | 1.69 | 0.009 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.848 | 2.423 | 0.860 | 2.442 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 228.7 / 223.6 / 228.7 |
| Goroutines start / end / max | 2424 / 4087 / 4087 |
| Open FDs max | 61 |
| CPU cores avg / max | 0.95 / 0.97 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 96.4 | 7.08 | 7.10 | 9.31 | 13.66 | 14.79 | 7.10 | 13.66 | 15.05 | 0.009 |
| direct | 2000 | 0 | 96.4 | 2.01 | 2.04 | 2.87 | 3.22 | 3.41 | 2.04 | 3.22 | 3.55 | 0.009 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.377 | 10.778 | 4.391 | 10.856 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 233.0 / 229.8 / 233.9 |
| Goroutines start / end / max | 3783 / 1051 / 3783 |
| Open FDs max | 61 |
| CPU cores avg / max | 0.94 / 1.00 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 153.1 | 1.21 | 1.27 | 1.65 | 3.06 | 4.33 | 3.35 | 5.58 | 9.90 | 0.014 |
| direct | 2000 | 0 | 153.1 | 0.58 | 0.65 | 0.83 | 1.07 | 1.29 | 2.66 | 3.44 | 3.82 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.316 | 0.895 | 0.343 | 0.950 | 0.327 | 0.901 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 228.7 / 228.5 / 230.3 |
| Goroutines start / end / max | 1099 / 1650 / 1652 |
| Open FDs max | 57 |
| CPU cores avg / max | 1.18 / 1.22 |

