# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 04:20:03 UTC |
| Duration | 2m32s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.513ms vs Server-Timing gw median 0.300ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.746ms vs Server-Timing gw median 0.337ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 2.748ms vs Server-Timing gw median 2.251ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 2.785ms vs Server-Timing gw median 2.271ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.608ms vs Server-Timing gw median 0.311ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 1.044ms vs Server-Timing gw median 0.682ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 1.036ms vs Server-Timing gw median 0.786ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 1.031ms vs Server-Timing gw median 0.712ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 1.030ms vs Server-Timing gw median 0.810ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.51 [0.50, 0.52] | 0.64 [0.63, 0.65] | 1.01 [0.93, 1.19] |  |
| llm-rest | gateway | TOTAL | 0.51 [0.50, 0.52] | 0.64 [0.63, 0.65] | 1.01 [0.93, 1.19] |  |
| llm-stream | gateway | TTFT | 0.65 [0.63, 0.68] | 6.71 [6.49, 7.01] | 13.14 [12.51, 13.92] |  |
| llm-stream | gateway | TOTAL | 0.75 [0.72, 0.78] | 6.77 [6.46, 7.03] | 13.05 [12.42, 13.83] |  |
| llm-call-rest | gateway | TTFT | 2.75 [2.67, 2.85] | 7.68 [7.41, 7.94] | 13.18 [12.68, 13.81] |  |
| llm-call-rest | gateway | TOTAL | 2.75 [2.67, 2.85] | 7.68 [7.41, 7.94] | 13.18 [12.68, 13.81] |  |
| llm-call-stream | gateway | TTFT | 2.76 [2.54, 2.96] | 12.05 [11.60, 12.46] | 21.49 [20.49, 22.66] |  |
| llm-call-stream | gateway | TOTAL | 2.78 [2.59, 3.02] | 12.04 [11.61, 12.47] | 21.39 [20.59, 22.63] |  |
| anthropic-native-stream | gateway | TTFT | 0.57 [0.56, 0.58] | 0.71 [0.69, 0.73] | 4.35 [3.51, 5.23] |  |
| anthropic-native-stream | gateway | TOTAL | 0.61 [0.60, 0.62] | 0.78 [0.76, 0.81] | 4.22 [3.40, 5.12] |  |
| ai-shim-rest | gateway | TTFT | 1.04 [1.03, 1.05] | 1.16 [1.14, 1.17] | 1.88 [1.62, 2.20] |  |
| ai-shim-rest | gateway | TOTAL | 1.04 [1.03, 1.05] | 1.16 [1.14, 1.17] | 1.88 [1.62, 2.20] |  |
| ai-shim-stream | gateway | TTFT | 0.93 [0.92, 0.94] | 1.03 [1.01, 1.04] | 1.41 [1.28, 1.61] |  |
| ai-shim-stream | gateway | TOTAL | 1.04 [1.03, 1.05] | 1.17 [1.16, 1.19] | 1.64 [1.44, 1.85] |  |
| unified-rest | gateway | TTFT | 1.03 [1.02, 1.04] | 1.16 [1.15, 1.18] | 1.66 [1.54, 1.95] |  |
| unified-rest | gateway | TOTAL | 1.03 [1.02, 1.04] | 1.16 [1.15, 1.18] | 1.66 [1.54, 1.95] |  |
| unified-stream | gateway | TTFT | 0.92 [0.92, 0.94] | 1.05 [1.03, 1.07] | 1.44 [1.32, 1.73] |  |
| unified-stream | gateway | TOTAL | 1.03 [1.02, 1.04] | 1.20 [1.18, 1.22] | 1.62 [1.40, 1.80] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 534.0 | 1.10 | 1.11 | 1.44 | 1.97 | 3.98 | 1.11 | 1.97 | 5.10 | 0.015 |
| direct | 5000 | 0 | 534.0 | 0.59 | 0.60 | 0.81 | 0.95 | 1.17 | 0.60 | 0.95 | 2.74 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.288 | 0.890 | 0.300 | 0.901 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 401.2 / 310.5 / 401.2 |
| Goroutines start / end / max | 410 / 5519 / 5519 |
| Open FDs max | 244 |
| CPU cores avg / max | 0.87 / 0.94 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 256.5 | 1.23 | 1.29 | 7.56 | 14.18 | 22.00 | 1.45 | 14.25 | 37.85 | 0.022 |
| direct | 5000 | 0 | 256.5 | 0.60 | 0.65 | 0.85 | 1.04 | 1.21 | 0.71 | 1.19 | 1.45 | 0.024 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.317 | 12.903 | 0.337 | 12.928 | 0.324 | 12.910 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 316.9 / 337.6 / 343.6 |
| Goroutines start / end / max | 6034 / 2006 / 6034 |
| Open FDs max | 248 |
| CPU cores avg / max | 0.55 / 0.92 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 189.4 | 3.42 | 3.44 | 8.51 | 14.11 | 18.38 | 3.44 | 14.11 | 21.32 | 0.019 |
| direct | 5000 | 0 | 189.4 | 0.67 | 0.70 | 0.83 | 0.94 | 1.18 | 0.70 | 0.94 | 1.40 | 0.017 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2.234 | 12.398 | 2.251 | 12.412 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 337.6 / 324.5 / 338.9 |
| Goroutines start / end / max | 2012 / 2032 / 2497 |
| Open FDs max | 248 |
| CPU cores avg / max | 0.42 / 0.47 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 148.7 | 3.39 | 3.45 | 12.91 | 22.55 | 30.90 | 3.54 | 22.60 | 41.50 | 0.024 |
| direct | 5000 | 0 | 148.7 | 0.65 | 0.70 | 0.86 | 1.07 | 1.28 | 0.75 | 1.21 | 1.91 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 2.256 | 21.281 | 2.271 | 21.299 | 2.261 | 21.289 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 325.0 / 322.4 / 325.4 |
| Goroutines start / end / max | 2010 / 2183 / 2183 |
| Open FDs max | 62 |
| CPU cores avg / max | 0.36 / 0.83 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 441.0 | 1.15 | 1.20 | 1.55 | 5.40 | 9.85 | 1.30 | 5.41 | 16.02 | 0.023 |
| direct | 5000 | 0 | 441.0 | 0.59 | 0.63 | 0.84 | 1.05 | 1.29 | 0.69 | 1.19 | 1.55 | 0.026 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.291 | 3.002 | 0.311 | 3.036 | 0.298 | 3.008 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 322.4 / 320.2 / 331.1 |
| Goroutines start / end / max | 2442 / 4709 / 4709 |
| Open FDs max | 62 |
| CPU cores avg / max | 0.81 / 0.88 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 391.5 | 1.68 | 1.71 | 1.96 | 2.81 | 5.38 | 1.71 | 2.81 | 9.82 | 0.018 |
| direct | 5000 | 0 | 391.5 | 0.64 | 0.67 | 0.81 | 0.93 | 1.08 | 0.67 | 0.93 | 1.25 | 0.016 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.580 | 0.993 | 0.682 | 1.288 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 323.4 / 330.3 / 335.4 |
| Goroutines start / end / max | 4645 / 4196 / 4645 |
| Open FDs max | 64 |
| CPU cores avg / max | 1.02 / 1.07 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 375.6 | 1.52 | 1.57 | 1.87 | 2.44 | 4.80 | 1.74 | 2.82 | 10.78 | 0.022 |
| direct | 5000 | 0 | 375.6 | 0.59 | 0.64 | 0.84 | 1.03 | 1.28 | 0.70 | 1.18 | 1.67 | 0.024 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.538 | 1.035 | 0.786 | 1.483 | 0.642 | 1.248 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 329.9 / 329.0 / 333.5 |
| Goroutines start / end / max | 4187 / 4035 / 4187 |
| Open FDs max | 64 |
| CPU cores avg / max | 1.01 / 1.04 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 397.2 | 1.66 | 1.69 | 1.97 | 2.59 | 5.49 | 1.69 | 2.59 | 10.13 | 0.018 |
| direct | 5000 | 0 | 397.2 | 0.63 | 0.66 | 0.81 | 0.93 | 1.18 | 0.66 | 0.93 | 1.52 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.564 | 1.021 | 0.712 | 1.363 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 329.0 / 329.9 / 331.0 |
| Goroutines start / end / max | 4046 / 4251 / 4251 |
| Open FDs max | 64 |
| CPU cores avg / max | 1.01 / 1.04 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 379.5 | 1.51 | 1.56 | 1.88 | 2.47 | 4.68 | 1.73 | 2.80 | 9.15 | 0.023 |
| direct | 5000 | 0 | 379.5 | 0.58 | 0.63 | 0.83 | 1.03 | 1.28 | 0.70 | 1.17 | 1.62 | 0.024 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.565 | 1.095 | 0.810 | 1.518 | 0.672 | 1.266 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 329.9 / 329.7 / 330.9 |
| Goroutines start / end / max | 4265 / 4057 / 4265 |
| Open FDs max | 64 |
| CPU cores avg / max | 1.03 / 1.07 |

