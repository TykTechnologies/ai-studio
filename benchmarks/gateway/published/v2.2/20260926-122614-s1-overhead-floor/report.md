# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 12:26:14 UTC |
| Duration | 1m41s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.560ms vs Server-Timing gw median 0.128ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.543ms vs Server-Timing gw median 0.141ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 0.582ms vs Server-Timing gw median 0.141ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 0.560ms vs Server-Timing gw median 0.158ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.564ms vs Server-Timing gw median 0.156ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 0.888ms vs Server-Timing gw median 0.488ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 0.838ms vs Server-Timing gw median 0.622ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 0.904ms vs Server-Timing gw median 0.508ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 0.887ms vs Server-Timing gw median 0.625ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.56 [0.55, 0.57] | 0.69 [0.69, 0.71] | 0.82 [0.79, 0.84] |  |
| llm-rest | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.69 [0.69, 0.71] | 0.82 [0.79, 0.84] |  |
| llm-stream | gateway | TTFT | 0.54 [0.52, 0.55] | 0.67 [0.65, 0.68] | 0.77 [0.74, 0.80] |  |
| llm-stream | gateway | TOTAL | 0.54 [0.53, 0.56] | 0.68 [0.65, 0.70] | 0.76 [0.73, 0.81] |  |
| llm-call-rest | gateway | TTFT | 0.58 [0.57, 0.59] | 0.71 [0.70, 0.72] | 0.84 [0.81, 0.87] |  |
| llm-call-rest | gateway | TOTAL | 0.58 [0.57, 0.59] | 0.71 [0.70, 0.72] | 0.84 [0.81, 0.87] |  |
| llm-call-stream | gateway | TTFT | 0.56 [0.55, 0.57] | 0.66 [0.65, 0.68] | 0.76 [0.73, 0.79] |  |
| llm-call-stream | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.65 [0.63, 0.66] | 0.71 [0.66, 0.77] |  |
| anthropic-native-stream | gateway | TTFT | 0.56 [0.55, 0.57] | 0.67 [0.65, 0.68] | 0.77 [0.74, 0.80] |  |
| anthropic-native-stream | gateway | TOTAL | 0.56 [0.55, 0.58] | 0.65 [0.63, 0.67] | 0.75 [0.70, 0.78] |  |
| ai-shim-rest | gateway | TTFT | 0.89 [0.88, 0.90] | 1.05 [1.03, 1.06] | 1.25 [1.22, 1.30] |  |
| ai-shim-rest | gateway | TOTAL | 0.89 [0.88, 0.90] | 1.05 [1.03, 1.06] | 1.25 [1.22, 1.30] |  |
| ai-shim-stream | gateway | TTFT | 0.73 [0.72, 0.74] | 0.88 [0.87, 0.89] | 1.02 [0.97, 1.04] |  |
| ai-shim-stream | gateway | TOTAL | 0.84 [0.83, 0.85] | 0.95 [0.93, 0.97] | 1.05 [1.01, 1.10] |  |
| unified-rest | gateway | TTFT | 0.90 [0.89, 0.91] | 1.05 [1.04, 1.07] | 1.20 [1.15, 1.24] |  |
| unified-rest | gateway | TOTAL | 0.90 [0.89, 0.91] | 1.05 [1.04, 1.07] | 1.20 [1.15, 1.24] |  |
| unified-stream | gateway | TTFT | 0.78 [0.76, 0.79] | 0.93 [0.92, 0.95] | 1.07 [1.03, 1.11] |  |
| unified-stream | gateway | TOTAL | 0.89 [0.88, 0.90] | 0.99 [0.98, 1.01] | 1.15 [1.08, 1.18] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 521.3 | 1.17 | 1.19 | 1.51 | 1.77 | 1.94 | 1.19 | 1.77 | 2.95 | 0.017 |
| direct | 5000 | 0 | 521.3 | 0.61 | 0.63 | 0.82 | 0.95 | 1.05 | 0.63 | 0.95 | 1.34 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.118 | 0.169 | 0.128 | 0.183 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 98.5 / 164.6 / 165.2 |
| Goroutines start / end / max | 43 / 43 / 45 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.33 / 0.34 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 476.7 | 1.10 | 1.15 | 1.51 | 1.80 | 2.04 | 1.25 | 2.00 | 3.07 | 0.021 |
| direct | 5000 | 0 | 476.7 | 0.57 | 0.62 | 0.84 | 1.03 | 1.23 | 0.71 | 1.23 | 2.36 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.122 | 0.173 | 0.141 | 0.204 | 0.132 | 0.186 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 165.0 / 160.0 / 168.8 |
| Goroutines start / end / max | 43 / 47 / 47 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.40 / 0.43 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 500.2 | 1.22 | 1.24 | 1.56 | 1.81 | 2.09 | 1.24 | 1.81 | 3.40 | 0.017 |
| direct | 5000 | 0 | 500.2 | 0.64 | 0.66 | 0.85 | 0.97 | 1.09 | 0.66 | 0.97 | 1.14 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.131 | 0.180 | 0.141 | 0.195 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 160.1 / 164.1 / 167.7 |
| Goroutines start / end / max | 45 / 44 / 45 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.32 / 0.34 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 460.1 | 1.15 | 1.21 | 1.53 | 1.81 | 2.07 | 1.29 | 1.96 | 3.19 | 0.024 |
| direct | 5000 | 0 | 460.1 | 0.60 | 0.65 | 0.87 | 1.04 | 1.22 | 0.73 | 1.25 | 1.66 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.139 | 0.190 | 0.158 | 0.223 | 0.149 | 0.205 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 167.0 / 160.4 / 169.6 |
| Goroutines start / end / max | 43 / 47 / 48 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.39 / 0.44 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 462.6 | 1.15 | 1.21 | 1.53 | 1.81 | 2.10 | 1.29 | 1.98 | 3.12 | 0.021 |
| direct | 5000 | 0 | 462.6 | 0.60 | 0.64 | 0.86 | 1.04 | 1.21 | 0.73 | 1.23 | 1.61 | 0.024 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.136 | 0.190 | 0.156 | 0.226 | 0.146 | 0.205 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 160.8 / 170.7 / 194.2 |
| Goroutines start / end / max | 48 / 46 / 48 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.40 / 0.43 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 422.1 | 1.52 | 1.55 | 1.88 | 2.20 | 2.83 | 1.55 | 2.20 | 4.91 | 0.018 |
| direct | 5000 | 0 | 422.1 | 0.64 | 0.66 | 0.83 | 0.96 | 1.05 | 0.66 | 0.96 | 1.23 | 0.019 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.332 | 0.626 | 0.488 | 0.834 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 167.2 / 162.9 / 172.6 |
| Goroutines start / end / max | 46 / 44 / 48 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.63 / 0.65 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 388.5 | 1.35 | 1.40 | 1.75 | 2.05 | 2.38 | 1.61 | 2.30 | 4.74 | 0.024 |
| direct | 5000 | 0 | 388.5 | 0.63 | 0.67 | 0.87 | 1.04 | 1.21 | 0.77 | 1.25 | 1.74 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.333 | 0.619 | 0.622 | 0.971 | 0.441 | 0.766 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 166.8 / 156.4 / 174.6 |
| Goroutines start / end / max | 49 / 47 / 49 |
| Open FDs max | 22 |
| CPU cores avg / max | 0.64 / 0.67 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 424.1 | 1.53 | 1.56 | 1.89 | 2.18 | 2.53 | 1.56 | 2.18 | 4.08 | 0.020 |
| direct | 5000 | 0 | 424.1 | 0.63 | 0.66 | 0.84 | 0.98 | 1.11 | 0.66 | 0.98 | 1.17 | 0.017 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.357 | 0.615 | 0.508 | 0.809 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 158.0 / 168.4 / 179.1 |
| Goroutines start / end / max | 44 / 44 / 49 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.65 / 0.69 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 382.5 | 1.40 | 1.45 | 1.80 | 2.10 | 3.43 | 1.65 | 2.38 | 4.34 | 0.023 |
| direct | 5000 | 0 | 382.5 | 0.62 | 0.67 | 0.86 | 1.03 | 1.18 | 0.76 | 1.23 | 1.50 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.364 | 0.654 | 0.625 | 0.984 | 0.470 | 0.790 | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 172.6 / 159.4 / 176.6 |
| Goroutines start / end / max | 49 / 41 / 50 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.65 / 0.67 |

