# S2 · Sensitivity to request and response size

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s2-payload-size |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 15:07:51 UTC |
| Duration | 46s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-1kb | black-box median overhead 0.605ms vs Server-Timing gw median 0.142ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-32kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-32kb | black-box median overhead 1.327ms vs Server-Timing gw median 0.757ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | prompt-256kb | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | prompt-256kb | black-box median overhead 5.437ms vs Server-Timing gw median 4.735ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | error rate (direct) | stream-1000-chunks | 0 of 2000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | stream-1000-chunks | black-box median overhead 0.676ms vs Server-Timing gw median 0.173ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 8400 proxy-log rows for 8400 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| prompt-1kb | gateway | TTFT | 0.61 [0.59, 0.62] | 0.75 [0.73, 0.76] | 0.84 [0.79, 0.95] |  |
| prompt-1kb | gateway | TOTAL | 0.61 [0.59, 0.62] | 0.75 [0.73, 0.76] | 0.84 [0.79, 0.95] |  |
| prompt-32kb | gateway | TTFT | 1.33 [1.31, 1.34] | 1.45 [1.40, 1.48] | 1.90 [1.53, 2.31] |  |
| prompt-32kb | gateway | TOTAL | 1.33 [1.31, 1.34] | 1.45 [1.40, 1.48] | 1.90 [1.53, 2.31] |  |
| prompt-256kb | gateway | TTFT | 5.44 [5.42, 5.46] | 5.53 [5.46, 5.61] | 8.75 [8.00, 9.61] |  |
| prompt-256kb | gateway | TOTAL | 5.44 [5.42, 5.46] | 5.53 [5.46, 5.61] | 8.75 [8.00, 9.61] |  |
| stream-1000-chunks | gateway | TTFT | 0.66 [0.64, 0.67] | 0.75 [0.73, 0.79] | 1.06 [0.85, 1.37] |  |
| stream-1000-chunks | gateway | TOTAL | 0.68 [0.65, 0.69] | 0.74 [0.68, 0.78] | 1.01 [0.87, 1.34] |  |

## Cell: prompt-1kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 511.4 | 1.21 | 1.23 | 1.55 | 1.79 | 2.60 | 1.23 | 1.79 | 3.71 | 0.018 |
| direct | 2000 | 0 | 511.4 | 0.60 | 0.63 | 0.80 | 0.95 | 1.07 | 0.63 | 0.95 | 1.38 | 0.018 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.132 | 0.181 | 0.142 | 0.193 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 291.7 / 291.9 / 292.2 |
| Goroutines start / end / max | 45 / 47 / 47 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.34 / 0.35 |

## Cell: prompt-32kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 315.3 | 2.11 | 2.14 | 2.47 | 3.55 | 4.96 | 2.14 | 3.55 | 6.74 | 0.011 |
| direct | 2000 | 0 | 315.3 | 0.79 | 0.81 | 1.02 | 1.66 | 2.05 | 0.81 | 1.66 | 2.07 | 0.012 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.744 | 1.186 | 0.757 | 1.195 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 292.2 / 283.9 / 299.1 |
| Goroutines start / end / max | 46 / 47 / 47 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.52 / 0.55 |

## Cell: prompt-256kb

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 91.8 | 7.61 | 7.64 | 8.89 | 12.55 | 14.91 | 7.64 | 12.55 | 15.85 | 0.011 |
| direct | 2000 | 0 | 91.8 | 2.18 | 2.20 | 3.36 | 3.80 | 4.09 | 2.20 | 3.80 | 4.21 | 0.013 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 4.723 | 9.590 | 4.735 | 9.601 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 296.3 / 285.1 / 298.5 |
| Goroutines start / end / max | 47 / 43 / 48 |
| Open FDs max | 25 |
| CPU cores avg / max | 0.66 / 0.68 |

## Cell: stream-1000-chunks

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2000 | 0 | 150.9 | 1.20 | 1.28 | 1.62 | 2.20 | 3.97 | 3.42 | 4.64 | 9.48 | 0.018 |
| direct | 2000 | 0 | 150.9 | 0.55 | 0.62 | 0.87 | 1.14 | 1.53 | 2.75 | 3.63 | 4.22 | 0.017 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.144 | 0.331 | 0.173 | 0.365 | 0.157 | 0.342 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 283.5 / 286.1 / 288.3 |
| Goroutines start / end / max | 42 / 46 / 46 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.94 / 0.97 |

