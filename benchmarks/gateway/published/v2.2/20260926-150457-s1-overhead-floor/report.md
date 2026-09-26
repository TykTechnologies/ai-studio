# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 15:04:57 UTC |
| Duration | 1m42s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.561ms vs Server-Timing gw median 0.130ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.562ms vs Server-Timing gw median 0.141ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 0.587ms vs Server-Timing gw median 0.142ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 0.558ms vs Server-Timing gw median 0.158ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.564ms vs Server-Timing gw median 0.154ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 0.898ms vs Server-Timing gw median 0.486ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 0.903ms vs Server-Timing gw median 0.608ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 0.907ms vs Server-Timing gw median 0.517ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 0.931ms vs Server-Timing gw median 0.659ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.56 [0.55, 0.57] | 0.69 [0.68, 0.70] | 0.80 [0.77, 0.84] |  |
| llm-rest | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.69 [0.68, 0.70] | 0.80 [0.77, 0.84] |  |
| llm-stream | gateway | TTFT | 0.55 [0.54, 0.56] | 0.66 [0.65, 0.68] | 0.75 [0.71, 0.79] |  |
| llm-stream | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.69 [0.67, 0.71] | 0.77 [0.71, 0.83] |  |
| llm-call-rest | gateway | TTFT | 0.59 [0.58, 0.60] | 0.70 [0.69, 0.72] | 0.84 [0.81, 0.88] |  |
| llm-call-rest | gateway | TOTAL | 0.59 [0.58, 0.60] | 0.70 [0.69, 0.72] | 0.84 [0.81, 0.88] |  |
| llm-call-stream | gateway | TTFT | 0.54 [0.53, 0.56] | 0.64 [0.62, 0.65] | 0.75 [0.70, 0.81] |  |
| llm-call-stream | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.64 [0.62, 0.65] | 0.69 [0.66, 0.75] |  |
| anthropic-native-stream | gateway | TTFT | 0.54 [0.53, 0.55] | 0.63 [0.62, 0.65] | 0.78 [0.73, 0.82] |  |
| anthropic-native-stream | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.65 [0.63, 0.67] | 0.72 [0.69, 0.78] |  |
| ai-shim-rest | gateway | TTFT | 0.90 [0.89, 0.91] | 1.04 [1.03, 1.06] | 1.25 [1.21, 1.28] |  |
| ai-shim-rest | gateway | TOTAL | 0.90 [0.89, 0.91] | 1.04 [1.03, 1.06] | 1.25 [1.21, 1.28] |  |
| ai-shim-stream | gateway | TTFT | 0.77 [0.75, 0.78] | 0.92 [0.91, 0.94] | 1.08 [1.02, 1.11] |  |
| ai-shim-stream | gateway | TOTAL | 0.90 [0.89, 0.91] | 1.04 [1.03, 1.06] | 1.16 [1.10, 1.23] |  |
| unified-rest | gateway | TTFT | 0.91 [0.90, 0.92] | 1.07 [1.06, 1.09] | 1.22 [1.19, 1.26] |  |
| unified-rest | gateway | TOTAL | 0.91 [0.90, 0.92] | 1.07 [1.06, 1.09] | 1.22 [1.19, 1.26] |  |
| unified-stream | gateway | TTFT | 0.78 [0.77, 0.79] | 0.92 [0.90, 0.93] | 1.02 [0.98, 1.07] |  |
| unified-stream | gateway | TOTAL | 0.93 [0.92, 0.94] | 1.06 [1.03, 1.07] | 1.14 [1.08, 1.19] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 496.0 | 1.21 | 1.24 | 1.54 | 1.79 | 2.29 | 1.24 | 1.79 | 3.16 | 0.015 |
| direct | 5000 | 0 | 496.0 | 0.65 | 0.68 | 0.85 | 0.98 | 1.35 | 0.68 | 0.98 | 2.02 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.119 | 0.175 | 0.130 | 0.192 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 201.7 / 268.5 / 270.7 |
| Goroutines start / end / max | 39 / 42 / 44 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.32 / 0.35 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 464.8 | 1.15 | 1.20 | 1.53 | 1.82 | 2.94 | 1.28 | 1.98 | 6.83 | 0.018 |
| direct | 5000 | 0 | 464.8 | 0.60 | 0.65 | 0.87 | 1.07 | 1.52 | 0.72 | 1.21 | 1.83 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.123 | 0.175 | 0.141 | 0.206 | 0.132 | 0.187 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 274.1 / 285.5 / 285.5 |
| Goroutines start / end / max | 42 / 43 / 45 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.39 / 0.42 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 503.8 | 1.21 | 1.24 | 1.53 | 1.81 | 2.52 | 1.24 | 1.81 | 4.13 | 0.016 |
| direct | 5000 | 0 | 503.8 | 0.63 | 0.65 | 0.83 | 0.97 | 1.12 | 0.65 | 0.97 | 1.71 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.131 | 0.179 | 0.142 | 0.192 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 285.6 / 294.5 / 294.5 |
| Goroutines start / end / max | 41 / 41 / 43 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.33 / 0.35 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 469.5 | 1.12 | 1.18 | 1.49 | 1.79 | 2.24 | 1.27 | 1.94 | 3.17 | 0.021 |
| direct | 5000 | 0 | 469.5 | 0.59 | 0.64 | 0.85 | 1.04 | 1.41 | 0.71 | 1.25 | 2.12 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.139 | 0.190 | 0.158 | 0.219 | 0.148 | 0.199 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 295.7 / 294.5 / 295.7 |
| Goroutines start / end / max | 42 / 41 / 45 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.40 / 0.43 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 475.5 | 1.14 | 1.19 | 1.50 | 1.81 | 2.81 | 1.26 | 1.91 | 3.98 | 0.020 |
| direct | 5000 | 0 | 475.5 | 0.60 | 0.65 | 0.86 | 1.04 | 1.38 | 0.69 | 1.19 | 2.14 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.135 | 0.187 | 0.154 | 0.220 | 0.144 | 0.197 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 289.7 / 294.4 / 294.4 |
| Goroutines start / end / max | 41 / 41 / 45 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.40 / 0.43 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 431.2 | 1.50 | 1.53 | 1.87 | 2.21 | 4.05 | 1.53 | 2.21 | 5.63 | 0.015 |
| direct | 5000 | 0 | 431.2 | 0.61 | 0.63 | 0.82 | 0.96 | 1.14 | 0.63 | 0.96 | 1.39 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.334 | 0.606 | 0.486 | 0.815 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 294.4 / 283.8 / 294.4 |
| Goroutines start / end / max | 45 / 45 / 49 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.65 / 0.68 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 393.9 | 1.38 | 1.42 | 1.78 | 2.12 | 3.20 | 1.62 | 2.37 | 4.91 | 0.021 |
| direct | 5000 | 0 | 393.9 | 0.61 | 0.66 | 0.86 | 1.04 | 1.50 | 0.72 | 1.21 | 2.00 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.335 | 0.637 | 0.608 | 0.978 | 0.443 | 0.780 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 283.8 / 290.1 / 290.4 |
| Goroutines start / end / max | 46 / 45 / 49 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.65 / 0.68 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 410.6 | 1.57 | 1.59 | 1.92 | 2.21 | 4.33 | 1.59 | 2.21 | 5.68 | 0.019 |
| direct | 5000 | 0 | 410.6 | 0.66 | 0.69 | 0.85 | 0.99 | 1.21 | 0.69 | 0.99 | 1.55 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.364 | 0.639 | 0.517 | 0.849 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 290.2 / 289.8 / 293.0 |
| Goroutines start / end / max | 46 / 45 / 49 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.63 / 0.66 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 389.1 | 1.40 | 1.45 | 1.78 | 2.07 | 2.84 | 1.66 | 2.36 | 5.56 | 0.023 |
| direct | 5000 | 0 | 389.1 | 0.62 | 0.67 | 0.86 | 1.05 | 1.35 | 0.72 | 1.22 | 1.69 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.363 | 0.647 | 0.659 | 1.002 | 0.473 | 0.798 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 289.5 / 291.7 / 292.6 |
| Goroutines start / end / max | 49 / 42 / 49 |
| Open FDs max | 24 |
| CPU cores avg / max | 0.68 / 0.71 |

