# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 13:48:19 UTC |
| Duration | 45s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

The gateway reads and inspects request bodies (model validation, filters, analytics) and relays every streamed chunk. This scenario checks how its overhead grows with a large prompt (long context) and with a long stream of small chunks. It uses the native /llm/call/ route, unloaded, one request at a time.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (gateway) | prompt-1kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-1kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.607ms vs Server-Timing gw median 0.144ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.313ms vs Server-Timing gw median 0.756ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 5.498ms vs Server-Timing gw median 4.727ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.638ms vs Server-Timing gw median 0.172ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.61 [0.59, 0.62] | 0.72 [0.69, 0.74] | 0.84 [0.80, 0.91] |  |
| prompt-1kb | gateway | TOTAL | 0.61 [0.59, 0.62] | 0.72 [0.69, 0.74] | 0.84 [0.80, 0.91] |  |
| prompt-32kb | gateway | TTFT | 1.31 [1.30, 1.33] | 1.47 [1.43, 1.50] | 1.46 [1.17, 1.78] |  |
| prompt-32kb | gateway | TOTAL | 1.31 [1.30, 1.33] | 1.47 [1.43, 1.49] | 1.46 [1.17, 1.78] |  |
| prompt-256kb | gateway | TTFT | 5.50 [5.47, 5.52] | 5.67 [5.59, 5.78] | 8.94 [8.03, 9.43] |  |
| prompt-256kb | gateway | TOTAL | 5.50 [5.47, 5.52] | 5.67 [5.59, 5.78] | 8.94 [8.03, 9.43] |  |
| stream-1000-chunks | gateway | TTFT | 0.62 [0.60, 0.64] | 0.74 [0.70, 0.77] | 1.04 [0.89, 1.24] |  |
| stream-1000-chunks | gateway | TOTAL | 0.64 [0.62, 0.66] | 0.76 [0.70, 0.82] | 0.89 [0.73, 1.07] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 503.4 | 1.23 | 1.25 | 1.53 | 1.77 | 2.13 | 1.25 | 1.77 | 5.75 | 0.021 |
| direct | 2000 | 0 | 503.4 | 0.62 | 0.65 | 0.81 | 0.94 | 1.05 | 0.65 | 0.94 | 1.43 | 0.019 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.132 | 0.173 | 0.144 | 0.185 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 286.3 / 286.8 / 286.8 |
| Goroutines start / end / max | 46 / 45 / 46 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.33 / 0.34 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 320.0 | 2.08 | 2.11 | 2.48 | 3.26 | 4.84 | 2.11 | 3.26 | 5.50 | 0.011 |
| direct | 2000 | 0 | 320.0 | 0.78 | 0.80 | 1.02 | 1.80 | 2.29 | 0.80 | 1.80 | 2.56 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.743 | 1.317 | 0.756 | 1.330 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 288.6 / 284.5 / 298.6 |
| Goroutines start / end / max | 48 / 47 / 48 |
| Open FDs max | 26 |
| CPU cores avg / max | 0.52 / 0.54 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 92.7 | 7.59 | 7.62 | 8.88 | 12.72 | 14.41 | 7.62 | 12.72 | 14.84 | 0.017 |
| direct | 2000 | 0 | 92.7 | 2.09 | 2.12 | 3.20 | 3.78 | 4.15 | 2.12 | 3.78 | 4.19 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.714 | 9.914 | 4.727 | 9.926 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 301.5 / 282.3 / 301.5 |
| Goroutines start / end / max | 47 / 43 / 48 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.66 / 0.69 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 152.6 | 1.20 | 1.27 | 1.61 | 2.10 | 3.44 | 3.37 | 4.53 | 6.48 | 0.014 |
| direct | 2000 | 0 | 152.6 | 0.58 | 0.65 | 0.88 | 1.06 | 1.37 | 2.73 | 3.64 | 4.20 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.144 | 0.313 | 0.172 | 0.352 | 0.157 | 0.332 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 286.4 / 283.8 / 290.2 |
| Goroutines start / end / max | 42 / 42 / 46 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.93 / 0.95 |

