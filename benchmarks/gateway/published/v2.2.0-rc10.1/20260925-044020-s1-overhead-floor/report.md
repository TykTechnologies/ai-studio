# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 04:40:20 UTC |
| Duration | 1m44s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

One request at a time against an upstream that answers immediately, so the difference between the gateway arm and the direct arm is the gateway's own cost: authentication, policy, request handling and the extra network hop. Arms alternate request by request so both see the same conditions. Every public LLM endpoint is covered, streaming and non-streaming. The /ai/ shim and the unified /v1 endpoint translate to the OpenAI format through an internal second hop, so expect them to cost more than the native /llm/ routes.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (gateway) | llm-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.629ms vs Server-Timing gw median 0.302ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.549ms vs Server-Timing gw median 0.296ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 0.545ms vs Server-Timing gw median 0.296ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 0.561ms vs Server-Timing gw median 0.310ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.565ms vs Server-Timing gw median 0.310ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 1.049ms vs Server-Timing gw median 0.702ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 1.017ms vs Server-Timing gw median 0.828ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 1.075ms vs Server-Timing gw median 0.730ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 1.068ms vs Server-Timing gw median 0.834ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.63 [0.62, 0.64] | 0.76 [0.75, 0.78] | 0.99 [0.92, 1.06] |  |
| llm-rest | gateway | TOTAL | 0.63 [0.62, 0.64] | 0.76 [0.75, 0.78] | 0.99 [0.92, 1.06] |  |
| llm-stream | gateway | TTFT | 0.52 [0.51, 0.53] | 0.60 [0.59, 0.61] | 0.81 [0.74, 0.94] |  |
| llm-stream | gateway | TOTAL | 0.55 [0.54, 0.56] | 0.68 [0.67, 0.70] | 0.87 [0.77, 0.97] |  |
| llm-call-rest | gateway | TTFT | 0.55 [0.54, 0.55] | 0.63 [0.62, 0.64] | 0.87 [0.81, 0.96] |  |
| llm-call-rest | gateway | TOTAL | 0.55 [0.54, 0.55] | 0.63 [0.62, 0.64] | 0.87 [0.81, 0.96] |  |
| llm-call-stream | gateway | TTFT | 0.56 [0.55, 0.57] | 0.63 [0.62, 0.65] | 0.88 [0.80, 0.95] |  |
| llm-call-stream | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.69 [0.67, 0.71] | 0.83 [0.78, 0.94] |  |
| anthropic-native-stream | gateway | TTFT | 0.54 [0.54, 0.55] | 0.62 [0.61, 0.64] | 0.83 [0.75, 0.97] |  |
| anthropic-native-stream | gateway | TOTAL | 0.57 [0.56, 0.58] | 0.69 [0.68, 0.71] | 0.87 [0.79, 0.95] |  |
| ai-shim-rest | gateway | TTFT | 1.05 [1.04, 1.06] | 1.18 [1.17, 1.19] | 1.71 [1.54, 2.13] |  |
| ai-shim-rest | gateway | TOTAL | 1.05 [1.04, 1.06] | 1.18 [1.17, 1.19] | 1.71 [1.54, 2.13] |  |
| ai-shim-stream | gateway | TTFT | 0.90 [0.89, 0.91] | 1.03 [1.01, 1.04] | 1.58 [1.40, 1.82] |  |
| ai-shim-stream | gateway | TOTAL | 1.02 [1.00, 1.03] | 1.18 [1.16, 1.19] | 1.75 [1.52, 2.03] |  |
| unified-rest | gateway | TTFT | 1.08 [1.07, 1.08] | 1.20 [1.19, 1.22] | 1.86 [1.63, 2.27] |  |
| unified-rest | gateway | TOTAL | 1.08 [1.07, 1.08] | 1.20 [1.19, 1.22] | 1.86 [1.63, 2.27] |  |
| unified-stream | gateway | TTFT | 0.95 [0.94, 0.96] | 1.07 [1.06, 1.09] | 1.49 [1.34, 1.77] |  |
| unified-stream | gateway | TOTAL | 1.07 [1.05, 1.08] | 1.25 [1.23, 1.27] | 1.74 [1.53, 2.06] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 486.0 | 1.25 | 1.28 | 1.56 | 1.90 | 4.07 | 1.28 | 1.90 | 7.86 | 0.018 |
| direct | 5000 | 0 | 486.0 | 0.62 | 0.65 | 0.80 | 0.91 | 1.06 | 0.65 | 0.91 | 1.27 | 0.019 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.291 | 0.617 | 0.302 | 0.658 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 435.1 / 321.8 / 435.1 |
| Goroutines start / end / max | 921 / 6061 / 6061 |
| Open FDs max | 502 |
| CPU cores avg / max | 0.80 / 0.86 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 479.4 | 1.10 | 1.15 | 1.43 | 1.82 | 4.19 | 1.24 | 2.02 | 10.15 | 0.023 |
| direct | 5000 | 0 | 479.4 | 0.58 | 0.63 | 0.83 | 1.01 | 1.20 | 0.69 | 1.15 | 1.77 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.276 | 0.721 | 0.296 | 0.746 | 0.284 | 0.732 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 325.8 / 352.9 / 352.9 |
| Goroutines start / end / max | 6078 / 6030 / 6083 |
| Open FDs max | 502 |
| CPU cores avg / max | 0.88 / 0.95 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 507.3 | 1.18 | 1.20 | 1.44 | 1.79 | 3.76 | 1.20 | 1.79 | 10.39 | 0.018 |
| direct | 5000 | 0 | 507.3 | 0.64 | 0.66 | 0.80 | 0.92 | 1.05 | 0.66 | 0.92 | 1.26 | 0.018 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.285 | 0.655 | 0.296 | 0.683 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 341.4 / 323.4 / 346.2 |
| Goroutines start / end / max | 6011 / 6282 / 6283 |
| Open FDs max | 502 |
| CPU cores avg / max | 0.84 / 0.90 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 482.3 | 1.11 | 1.16 | 1.45 | 1.87 | 4.48 | 1.24 | 1.99 | 11.36 | 0.022 |
| direct | 5000 | 0 | 482.3 | 0.56 | 0.61 | 0.81 | 0.99 | 1.18 | 0.67 | 1.15 | 1.66 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.290 | 0.755 | 0.310 | 0.783 | 0.298 | 0.771 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 313.1 / 332.4 / 336.1 |
| Goroutines start / end / max | 5440 / 5166 / 5440 |
| Open FDs max | 84 |
| CPU cores avg / max | 0.89 / 0.93 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 481.3 | 1.12 | 1.17 | 1.44 | 1.86 | 3.61 | 1.24 | 2.02 | 8.29 | 0.021 |
| direct | 5000 | 0 | 481.3 | 0.58 | 0.62 | 0.82 | 1.03 | 1.31 | 0.67 | 1.15 | 1.68 | 0.021 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.287 | 0.778 | 0.310 | 0.798 | 0.296 | 0.786 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 331.2 / 319.5 / 335.1 |
| Goroutines start / end / max | 5157 / 5154 / 5165 |
| Open FDs max | 56 |
| CPU cores avg / max | 0.88 / 0.95 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 400.3 | 1.66 | 1.68 | 1.97 | 2.64 | 5.54 | 1.68 | 2.64 | 9.20 | 0.017 |
| direct | 5000 | 0 | 400.3 | 0.61 | 0.63 | 0.79 | 0.93 | 1.11 | 0.64 | 0.93 | 1.83 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.550 | 1.096 | 0.702 | 1.433 | – | – | 0.2% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 322.0 / 330.9 / 332.9 |
| Goroutines start / end / max | 5075 / 4320 / 5075 |
| Open FDs max | 51 |
| CPU cores avg / max | 1.06 / 1.09 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 376.5 | 1.50 | 1.55 | 1.87 | 2.61 | 5.13 | 1.73 | 2.92 | 9.48 | 0.024 |
| direct | 5000 | 0 | 376.5 | 0.61 | 0.65 | 0.84 | 1.03 | 1.34 | 0.71 | 1.18 | 1.77 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.544 | 1.203 | 0.828 | 1.663 | 0.652 | 1.349 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 326.0 / 327.7 / 328.0 |
| Goroutines start / end / max | 4295 / 4043 / 4295 |
| Open FDs max | 51 |
| CPU cores avg / max | 1.06 / 1.09 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 387.3 | 1.71 | 1.74 | 2.01 | 2.79 | 5.84 | 1.74 | 2.79 | 11.83 | 0.018 |
| direct | 5000 | 0 | 387.3 | 0.64 | 0.66 | 0.80 | 0.93 | 1.23 | 0.66 | 0.93 | 1.33 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.586 | 1.088 | 0.730 | 1.460 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 327.7 / 325.4 / 332.0 |
| Goroutines start / end / max | 4058 / 4149 / 4176 |
| Open FDs max | 45 |
| CPU cores avg / max | 1.03 / 1.06 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 371.4 | 1.55 | 1.60 | 1.91 | 2.53 | 4.82 | 1.77 | 2.92 | 11.67 | 0.024 |
| direct | 5000 | 0 | 371.4 | 0.60 | 0.65 | 0.84 | 1.05 | 1.36 | 0.70 | 1.18 | 1.78 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.584 | 1.138 | 0.834 | 1.794 | 0.698 | 1.381 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 325.4 / 327.8 / 327.8 |
| Goroutines start / end / max | 4118 / 3987 / 4118 |
| Open FDs max | 43 |
| CPU cores avg / max | 1.05 / 1.08 |

