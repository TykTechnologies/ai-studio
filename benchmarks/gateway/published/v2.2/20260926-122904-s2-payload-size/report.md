# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 12:29:04 UTC |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.514ms vs Server-Timing gw median 0.142ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.302ms vs Server-Timing gw median 0.749ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 5.385ms vs Server-Timing gw median 4.671ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.644ms vs Server-Timing gw median 0.170ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.51 [0.50, 0.53] | 0.66 [0.63, 0.68] | 0.76 [0.71, 0.80] |  |
| prompt-1kb | gateway | TOTAL | 0.51 [0.50, 0.53] | 0.66 [0.63, 0.68] | 0.76 [0.71, 0.80] |  |
| prompt-32kb | gateway | TTFT | 1.30 [1.29, 1.32] | 1.45 [1.42, 1.48] | 1.60 [1.48, 1.75] |  |
| prompt-32kb | gateway | TOTAL | 1.30 [1.29, 1.32] | 1.45 [1.42, 1.48] | 1.60 [1.48, 1.75] |  |
| prompt-256kb | gateway | TTFT | 5.39 [5.36, 5.41] | 5.61 [5.54, 5.68] | 7.01 [6.77, 7.18] |  |
| prompt-256kb | gateway | TOTAL | 5.39 [5.36, 5.41] | 5.61 [5.54, 5.68] | 7.01 [6.77, 7.18] |  |
| stream-1000-chunks | gateway | TTFT | 0.62 [0.60, 0.64] | 0.75 [0.71, 0.77] | 0.93 [0.84, 1.08] |  |
| stream-1000-chunks | gateway | TOTAL | 0.64 [0.62, 0.66] | 0.68 [0.63, 0.73] | 0.75 [0.63, 0.90] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 480.9 | 1.22 | 1.24 | 1.54 | 1.77 | 1.97 | 1.24 | 1.77 | 2.16 | 0.017 |
| direct | 2000 | 0 | 480.9 | 0.70 | 0.73 | 0.88 | 1.02 | 1.13 | 0.73 | 1.02 | 1.15 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.131 | 0.178 | 0.142 | 0.194 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 160.2 / 163.6 / 163.6 |
| Goroutines start / end / max | 42 / 43 / 44 |
| Open FDs max | 19 |
| CPU cores avg / max | 0.31 / 0.32 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 320.0 | 2.09 | 2.11 | 2.46 | 2.98 | 3.81 | 2.11 | 2.98 | 4.39 | 0.015 |
| direct | 2000 | 0 | 320.0 | 0.79 | 0.81 | 1.01 | 1.38 | 1.87 | 0.81 | 1.38 | 6.05 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.737 | 1.233 | 0.749 | 1.255 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 172.6 / 163.2 / 178.7 |
| Goroutines start / end / max | 44 / 44 / 44 |
| Open FDs max | 19 |
| CPU cores avg / max | 0.51 / 0.52 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 94.5 | 7.56 | 7.59 | 8.35 | 10.17 | 11.04 | 7.59 | 10.17 | 11.10 | 0.017 |
| direct | 2000 | 0 | 94.5 | 2.18 | 2.21 | 2.74 | 3.17 | 3.35 | 2.21 | 3.17 | 3.51 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.658 | 7.111 | 4.671 | 7.130 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 168.6 / 167.4 / 173.4 |
| Goroutines start / end / max | 43 / 42 / 45 |
| Open FDs max | 19 |
| CPU cores avg / max | 0.66 / 0.69 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 152.8 | 1.25 | 1.31 | 1.65 | 2.05 | 2.73 | 3.37 | 4.41 | 5.17 | 0.017 |
| direct | 2000 | 0 | 152.8 | 0.62 | 0.69 | 0.90 | 1.12 | 1.40 | 2.72 | 3.66 | 4.08 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.145 | 0.307 | 0.170 | 0.348 | 0.155 | 0.334 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 162.1 / 163.8 / 166.5 |
| Goroutines start / end / max | 45 / 45 / 46 |
| Open FDs max | 18 |
| CPU cores avg / max | 0.87 / 0.91 |

