# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 13:45:29 UTC |
| Duration | 1m43s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

One request at a time against an upstream that answers immediately, so the difference between the gateway arm and the direct arm is the gateway's own cost: authentication, policy, request handling and the extra network hop. Arms alternate request by request so both see the same conditions. Every public LLM endpoint is covered, streaming and non-streaming. The /ai/ shim and the unified /v1 endpoint translate to the OpenAI format through an internal second hop, so expect them to cost more than the native /llm/ routes.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (gateway) | llm-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.580ms vs Server-Timing gw median 0.130ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.550ms vs Server-Timing gw median 0.142ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 0.577ms vs Server-Timing gw median 0.141ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 0.570ms vs Server-Timing gw median 0.160ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.578ms vs Server-Timing gw median 0.156ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 0.913ms vs Server-Timing gw median 0.484ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 0.920ms vs Server-Timing gw median 0.629ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 0.926ms vs Server-Timing gw median 0.512ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 0.953ms vs Server-Timing gw median 0.660ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.58 [0.57, 0.59] | 0.71 [0.70, 0.73] | 0.81 [0.78, 0.84] |  |
| llm-rest | gateway | TOTAL | 0.58 [0.57, 0.59] | 0.71 [0.70, 0.73] | 0.81 [0.78, 0.84] |  |
| llm-stream | gateway | TTFT | 0.53 [0.52, 0.54] | 0.65 [0.64, 0.67] | 0.76 [0.71, 0.81] |  |
| llm-stream | gateway | TOTAL | 0.55 [0.54, 0.56] | 0.68 [0.66, 0.70] | 0.80 [0.76, 0.85] |  |
| llm-call-rest | gateway | TTFT | 0.58 [0.57, 0.59] | 0.69 [0.68, 0.71] | 0.80 [0.77, 0.84] |  |
| llm-call-rest | gateway | TOTAL | 0.58 [0.57, 0.59] | 0.69 [0.68, 0.71] | 0.80 [0.77, 0.84] |  |
| llm-call-stream | gateway | TTFT | 0.56 [0.55, 0.57] | 0.66 [0.65, 0.67] | 0.75 [0.71, 0.81] |  |
| llm-call-stream | gateway | TOTAL | 0.57 [0.56, 0.58] | 0.67 [0.66, 0.69] | 0.75 [0.71, 0.79] |  |
| anthropic-native-stream | gateway | TTFT | 0.55 [0.54, 0.56] | 0.66 [0.64, 0.67] | 0.75 [0.71, 0.78] |  |
| anthropic-native-stream | gateway | TOTAL | 0.58 [0.57, 0.59] | 0.71 [0.69, 0.73] | 0.80 [0.75, 0.86] |  |
| ai-shim-rest | gateway | TTFT | 0.91 [0.90, 0.92] | 1.05 [1.04, 1.07] | 1.23 [1.18, 1.27] |  |
| ai-shim-rest | gateway | TOTAL | 0.91 [0.90, 0.92] | 1.05 [1.04, 1.07] | 1.23 [1.18, 1.27] |  |
| ai-shim-stream | gateway | TTFT | 0.78 [0.77, 0.79] | 0.91 [0.90, 0.93] | 1.05 [1.01, 1.09] |  |
| ai-shim-stream | gateway | TOTAL | 0.92 [0.91, 0.93] | 1.04 [1.03, 1.06] | 1.13 [1.08, 1.18] |  |
| unified-rest | gateway | TTFT | 0.93 [0.92, 0.94] | 1.07 [1.06, 1.09] | 1.26 [1.21, 1.30] |  |
| unified-rest | gateway | TOTAL | 0.93 [0.92, 0.94] | 1.07 [1.06, 1.09] | 1.26 [1.21, 1.30] |  |
| unified-stream | gateway | TTFT | 0.81 [0.80, 0.82] | 0.93 [0.92, 0.95] | 1.07 [1.03, 1.11] |  |
| unified-stream | gateway | TOTAL | 0.95 [0.94, 0.96] | 1.07 [1.05, 1.09] | 1.18 [1.13, 1.23] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 493.9 | 1.22 | 1.25 | 1.55 | 1.80 | 2.35 | 1.25 | 1.80 | 3.34 | 0.015 |
| direct | 5000 | 0 | 493.9 | 0.64 | 0.67 | 0.84 | 1.00 | 1.32 | 0.67 | 1.00 | 1.40 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.119 | 0.173 | 0.130 | 0.184 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 204.1 / 279.7 / 279.7 |
| Goroutines start / end / max | 41 / 41 / 44 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.32 / 0.37 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 474.1 | 1.11 | 1.17 | 1.50 | 1.80 | 2.54 | 1.25 | 1.99 | 6.75 | 0.020 |
| direct | 5000 | 0 | 474.1 | 0.59 | 0.64 | 0.85 | 1.04 | 1.41 | 0.70 | 1.19 | 3.61 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.122 | 0.173 | 0.142 | 0.206 | 0.133 | 0.185 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 281.0 / 294.2 / 296.8 |
| Goroutines start / end / max | 43 / 45 / 45 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.41 / 0.43 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 498.9 | 1.21 | 1.23 | 1.53 | 1.79 | 2.36 | 1.23 | 1.79 | 3.29 | 0.014 |
| direct | 5000 | 0 | 498.9 | 0.63 | 0.66 | 0.84 | 0.98 | 1.21 | 0.66 | 0.98 | 1.52 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.131 | 0.180 | 0.141 | 0.190 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 294.2 / 297.1 / 297.1 |
| Goroutines start / end / max | 41 / 41 / 43 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.33 / 0.35 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 459.6 | 1.17 | 1.22 | 1.53 | 1.81 | 2.16 | 1.30 | 1.98 | 3.07 | 0.023 |
| direct | 5000 | 0 | 459.6 | 0.62 | 0.66 | 0.87 | 1.07 | 1.32 | 0.73 | 1.23 | 1.61 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.139 | 0.193 | 0.160 | 0.227 | 0.149 | 0.204 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 297.1 / 297.4 / 297.4 |
| Goroutines start / end / max | 45 / 42 / 45 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.40 / 0.42 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 467.0 | 1.16 | 1.21 | 1.52 | 1.82 | 2.32 | 1.28 | 1.99 | 3.77 | 0.022 |
| direct | 5000 | 0 | 467.0 | 0.61 | 0.66 | 0.87 | 1.07 | 1.38 | 0.71 | 1.19 | 1.66 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.134 | 0.189 | 0.156 | 0.221 | 0.145 | 0.200 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 298.5 / 289.6 / 298.9 |
| Goroutines start / end / max | 41 / 42 / 46 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.41 / 0.44 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 424.4 | 1.53 | 1.56 | 1.89 | 2.20 | 3.31 | 1.56 | 2.20 | 8.09 | 0.016 |
| direct | 5000 | 0 | 424.4 | 0.62 | 0.65 | 0.84 | 0.97 | 1.22 | 0.65 | 0.97 | 1.55 | 0.018 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.339 | 0.610 | 0.484 | 0.779 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 289.6 / 289.0 / 290.8 |
| Goroutines start / end / max | 45 / 48 / 50 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.64 / 0.70 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 393.3 | 1.39 | 1.44 | 1.77 | 2.08 | 3.24 | 1.64 | 2.35 | 8.49 | 0.020 |
| direct | 5000 | 0 | 393.3 | 0.61 | 0.66 | 0.85 | 1.03 | 1.41 | 0.72 | 1.22 | 2.23 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.336 | 0.614 | 0.629 | 0.955 | 0.442 | 0.738 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 290.7 / 287.6 / 291.5 |
| Goroutines start / end / max | 41 / 45 / 50 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.67 / 0.73 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 413.6 | 1.56 | 1.59 | 1.91 | 2.24 | 4.34 | 1.59 | 2.24 | 5.39 | 0.017 |
| direct | 5000 | 0 | 413.6 | 0.64 | 0.66 | 0.84 | 0.98 | 1.34 | 0.66 | 0.98 | 1.66 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.365 | 0.654 | 0.512 | 0.828 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 288.4 / 293.6 / 293.6 |
| Goroutines start / end / max | 48 / 41 / 48 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.64 / 0.68 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 383.1 | 1.44 | 1.48 | 1.80 | 2.12 | 2.59 | 1.69 | 2.40 | 4.47 | 0.020 |
| direct | 5000 | 0 | 383.1 | 0.63 | 0.67 | 0.87 | 1.06 | 1.36 | 0.73 | 1.21 | 1.69 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.363 | 0.638 | 0.660 | 1.000 | 0.471 | 0.781 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 294.0 / 285.1 / 294.4 |
| Goroutines start / end / max | 44 / 45 / 48 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.66 / 0.70 |

