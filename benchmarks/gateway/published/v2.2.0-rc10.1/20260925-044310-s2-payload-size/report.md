# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 04:43:10 UTC |
| Duration | 45s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.679ms vs Server-Timing gw median 0.320ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.380ms vs Server-Timing gw median 0.883ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 5.065ms vs Server-Timing gw median 4.439ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.741ms vs Server-Timing gw median 0.349ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.68 [0.67, 0.69] | 0.80 [0.77, 0.82] | 1.01 [0.90, 1.31] |  |
| prompt-1kb | gateway | TOTAL | 0.68 [0.67, 0.69] | 0.80 [0.77, 0.81] | 1.01 [0.90, 1.31] |  |
| prompt-32kb | gateway | TTFT | 1.38 [1.36, 1.40] | 1.57 [1.53, 1.61] | 3.06 [2.67, 3.48] |  |
| prompt-32kb | gateway | TOTAL | 1.38 [1.36, 1.40] | 1.57 [1.53, 1.61] | 3.06 [2.67, 3.48] |  |
| prompt-256kb | gateway | TTFT | 5.06 [5.03, 5.09] | 6.21 [5.76, 6.59] | 10.12 [9.77, 10.50] |  |
| prompt-256kb | gateway | TOTAL | 5.06 [5.03, 5.09] | 6.21 [5.76, 6.59] | 10.12 [9.77, 10.50] |  |
| stream-1000-chunks | gateway | TTFT | 0.65 [0.63, 0.67] | 0.81 [0.78, 0.85] | 2.41 [1.90, 2.87] |  |
| stream-1000-chunks | gateway | TOTAL | 0.74 [0.72, 0.76] | 0.90 [0.83, 0.96] | 2.14 [1.70, 2.43] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 475.3 | 1.32 | 1.35 | 1.61 | 1.96 | 2.81 | 1.35 | 1.96 | 4.59 | 0.016 |
| direct | 2000 | 0 | 475.3 | 0.64 | 0.67 | 0.81 | 0.95 | 1.16 | 0.67 | 0.95 | 1.20 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.309 | 0.718 | 0.320 | 0.729 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 328.1 / 313.0 / 328.1 |
| Goroutines start / end / max | 42 / 2042 / 2042 |
| Open FDs max | 44 |
| CPU cores avg / max | 0.78 / 0.79 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 311.8 | 2.13 | 2.15 | 2.58 | 4.57 | 7.08 | 2.15 | 4.57 | 9.77 | 0.008 |
| direct | 2000 | 0 | 311.8 | 0.75 | 0.77 | 1.01 | 1.51 | 1.79 | 0.77 | 1.51 | 2.01 | 0.008 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.871 | 2.423 | 0.883 | 2.477 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 318.3 / 311.3 / 318.9 |
| Goroutines start / end / max | 2401 / 4031 / 4031 |
| Open FDs max | 44 |
| CPU cores avg / max | 0.95 / 0.97 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 96.2 | 7.12 | 7.15 | 9.08 | 13.34 | 14.49 | 7.15 | 13.34 | 14.51 | 0.009 |
| direct | 2000 | 0 | 96.2 | 2.06 | 2.09 | 2.87 | 3.22 | 3.40 | 2.09 | 3.22 | 3.58 | 0.008 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.424 | 10.622 | 4.439 | 10.634 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 319.5 / 320.2 / 321.9 |
| Goroutines start / end / max | 3771 / 1052 / 3771 |
| Open FDs max | 44 |
| CPU cores avg / max | 0.95 / 1.03 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 153.2 | 1.23 | 1.30 | 1.67 | 3.42 | 6.22 | 3.38 | 5.61 | 8.91 | 0.011 |
| direct | 2000 | 0 | 153.2 | 0.58 | 0.65 | 0.85 | 1.01 | 1.20 | 2.64 | 3.47 | 4.08 | 0.009 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.323 | 1.060 | 0.349 | 1.086 | 0.332 | 1.070 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 315.5 / 318.7 / 320.2 |
| Goroutines start / end / max | 1087 / 1646 / 1647 |
| Open FDs max | 41 |
| CPU cores avg / max | 1.19 / 1.23 |

