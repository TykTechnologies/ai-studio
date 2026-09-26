# S1 · Gateway overhead floor, unloaded, per endpoint

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s1-overhead-floor |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 03:59:10 UTC |
| Duration | 1m46s |
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
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-rest | black-box median overhead 0.633ms vs Server-Timing gw median 0.299ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-stream | black-box median overhead 0.515ms vs Server-Timing gw median 0.287ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-rest | black-box median overhead 0.557ms vs Server-Timing gw median 0.293ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | llm-call-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | llm-call-stream | black-box median overhead 0.529ms vs Server-Timing gw median 0.304ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | anthropic-native-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | anthropic-native-stream | black-box median overhead 0.538ms vs Server-Timing gw median 0.308ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-rest | black-box median overhead 1.072ms vs Server-Timing gw median 0.700ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | ai-shim-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | ai-shim-stream | black-box median overhead 1.016ms vs Server-Timing gw median 0.806ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-rest | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-rest | black-box median overhead 1.068ms vs Server-Timing gw median 0.740ms; the gap is network and kernel time outside the gateway process |
| pass | error rate (gateway) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | error rate (direct) | unified-stream | 0 of 5000 failed (0.000%) |
| pass | Server-Timing agrees with black-box overhead (gateway) | unified-stream | black-box median overhead 1.009ms vs Server-Timing gw median 0.831ms; the gap is network and kernel time outside the gateway process |
| pass | analytics recorded every gateway request |  | Studio gained 47700 proxy-log rows for 47700 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| llm-rest | gateway | TTFT | 0.63 [0.62, 0.64] | 0.77 [0.76, 0.79] | 1.03 [0.94, 1.17] |  |
| llm-rest | gateway | TOTAL | 0.63 [0.62, 0.64] | 0.77 [0.76, 0.79] | 1.03 [0.94, 1.17] |  |
| llm-stream | gateway | TTFT | 0.51 [0.50, 0.52] | 0.58 [0.57, 0.60] | 0.70 [0.63, 0.78] |  |
| llm-stream | gateway | TOTAL | 0.52 [0.51, 0.52] | 0.59 [0.57, 0.61] | 0.67 [0.63, 0.74] |  |
| llm-call-rest | gateway | TTFT | 0.56 [0.55, 0.57] | 0.63 [0.62, 0.64] | 0.93 [0.86, 1.05] |  |
| llm-call-rest | gateway | TOTAL | 0.56 [0.55, 0.57] | 0.63 [0.62, 0.64] | 0.93 [0.86, 1.05] |  |
| llm-call-stream | gateway | TTFT | 0.53 [0.52, 0.54] | 0.60 [0.59, 0.62] | 0.79 [0.73, 0.86] |  |
| llm-call-stream | gateway | TOTAL | 0.53 [0.52, 0.54] | 0.61 [0.59, 0.62] | 0.74 [0.68, 0.82] |  |
| anthropic-native-stream | gateway | TTFT | 0.54 [0.53, 0.55] | 0.61 [0.60, 0.62] | 0.86 [0.78, 0.99] |  |
| anthropic-native-stream | gateway | TOTAL | 0.54 [0.53, 0.55] | 0.64 [0.63, 0.67] | 0.86 [0.76, 1.06] |  |
| ai-shim-rest | gateway | TTFT | 1.07 [1.06, 1.08] | 1.17 [1.16, 1.19] | 1.88 [1.57, 2.23] |  |
| ai-shim-rest | gateway | TOTAL | 1.07 [1.06, 1.08] | 1.17 [1.16, 1.19] | 1.88 [1.57, 2.23] |  |
| ai-shim-stream | gateway | TTFT | 0.91 [0.90, 0.92] | 1.04 [1.02, 1.06] | 1.53 [1.36, 1.86] |  |
| ai-shim-stream | gateway | TOTAL | 1.02 [1.01, 1.03] | 1.19 [1.17, 1.20] | 1.80 [1.56, 2.04] |  |
| unified-rest | gateway | TTFT | 1.07 [1.06, 1.08] | 1.21 [1.19, 1.23] | 1.90 [1.66, 2.17] |  |
| unified-rest | gateway | TOTAL | 1.07 [1.06, 1.08] | 1.21 [1.19, 1.23] | 1.90 [1.66, 2.17] |  |
| unified-stream | gateway | TTFT | 0.91 [0.90, 0.92] | 1.04 [1.03, 1.06] | 1.49 [1.34, 1.73] |  |
| unified-stream | gateway | TOTAL | 1.01 [1.00, 1.02] | 1.18 [1.15, 1.20] | 1.65 [1.48, 1.80] |  |

## Cell: llm-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 473.3 | 1.29 | 1.32 | 1.60 | 1.97 | 3.58 | 1.32 | 1.97 | 4.84 | 0.015 |
| direct | 5000 | 0 | 473.3 | 0.66 | 0.68 | 0.83 | 0.94 | 1.05 | 0.68 | 0.94 | 1.19 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.288 | 0.685 | 0.299 | 0.711 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 495.1 / 260.3 / 495.1 |
| Goroutines start / end / max | 1046 / 6051 / 6051 |
| Open FDs max | 566 |
| CPU cores avg / max | 0.78 / 0.82 |

## Cell: llm-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 466.1 | 1.11 | 1.16 | 1.43 | 1.73 | 3.72 | 1.24 | 1.88 | 4.43 | 0.021 |
| direct | 5000 | 0 | 466.1 | 0.61 | 0.65 | 0.84 | 1.03 | 1.26 | 0.73 | 1.21 | 1.81 | 0.020 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.270 | 0.511 | 0.287 | 0.530 | 0.278 | 0.518 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 260.1 / 288.9 / 288.9 |
| Goroutines start / end / max | 6025 / 6007 / 6072 |
| Open FDs max | 565 |
| CPU cores avg / max | 0.84 / 0.90 |

## Cell: llm-call-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 506.6 | 1.18 | 1.21 | 1.44 | 1.87 | 3.73 | 1.21 | 1.87 | 5.59 | 0.015 |
| direct | 5000 | 0 | 506.6 | 0.62 | 0.65 | 0.81 | 0.94 | 1.05 | 0.65 | 0.94 | 1.29 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.283 | 0.836 | 0.293 | 0.856 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 282.9 / 260.2 / 287.7 |
| Goroutines start / end / max | 6040 / 6398 / 6398 |
| Open FDs max | 565 |
| CPU cores avg / max | 0.84 / 0.94 |

## Cell: llm-call-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 470.5 | 1.13 | 1.18 | 1.45 | 1.82 | 3.91 | 1.25 | 1.95 | 5.11 | 0.023 |
| direct | 5000 | 0 | 470.5 | 0.60 | 0.65 | 0.84 | 1.03 | 1.22 | 0.72 | 1.21 | 1.58 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.286 | 0.659 | 0.304 | 0.690 | 0.295 | 0.670 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 264.6 / 233.3 / 280.7 |
| Goroutines start / end / max | 6389 / 5033 / 6389 |
| Open FDs max | 565 |
| CPU cores avg / max | 0.87 / 0.96 |

## Cell: anthropic-native-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 463.6 | 1.14 | 1.19 | 1.45 | 1.89 | 4.00 | 1.26 | 2.06 | 5.79 | 0.021 |
| direct | 5000 | 0 | 463.6 | 0.61 | 0.65 | 0.84 | 1.04 | 1.24 | 0.72 | 1.20 | 1.41 | 0.023 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.288 | 0.815 | 0.308 | 0.868 | 0.296 | 0.867 | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 233.4 / 221.6 / 237.2 |
| Goroutines start / end / max | 5031 / 4959 / 5056 |
| Open FDs max | 61 |
| CPU cores avg / max | 0.86 / 0.91 |

## Cell: ai-shim-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 386.2 | 1.72 | 1.75 | 2.00 | 2.84 | 5.55 | 1.75 | 2.84 | 11.49 | 0.017 |
| direct | 5000 | 0 | 386.2 | 0.65 | 0.68 | 0.83 | 0.95 | 1.13 | 0.68 | 0.95 | 1.48 | 0.014 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.580 | 1.076 | 0.700 | 1.426 | – | – | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 228.7 / 233.2 / 235.4 |
| Goroutines start / end / max | 4900 / 4116 / 4900 |
| Open FDs max | 63 |
| CPU cores avg / max | 1.04 / 1.08 |

## Cell: ai-shim-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 365.8 | 1.54 | 1.59 | 1.90 | 2.56 | 5.09 | 1.76 | 3.01 | 8.55 | 0.023 |
| direct | 5000 | 0 | 365.8 | 0.64 | 0.68 | 0.86 | 1.04 | 1.23 | 0.75 | 1.21 | 1.55 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.554 | 1.180 | 0.806 | 1.773 | 0.662 | 1.516 | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 233.3 / 236.3 / 237.9 |
| Goroutines start / end / max | 4123 / 3955 / 4123 |
| Open FDs max | 63 |
| CPU cores avg / max | 1.03 / 1.08 |

## Cell: unified-rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 387.3 | 1.71 | 1.74 | 2.03 | 2.85 | 5.02 | 1.74 | 2.85 | 8.29 | 0.016 |
| direct | 5000 | 0 | 387.3 | 0.65 | 0.67 | 0.82 | 0.95 | 1.14 | 0.67 | 0.95 | 1.23 | 0.015 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.584 | 1.088 | 0.740 | 1.462 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 235.0 / 235.7 / 238.2 |
| Goroutines start / end / max | 3977 / 4113 / 4146 |
| Open FDs max | 63 |
| CPU cores avg / max | 1.05 / 1.10 |

## Cell: unified-stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 5000 | 0 | 362.1 | 1.56 | 1.61 | 1.91 | 2.55 | 5.34 | 1.77 | 2.87 | 9.23 | 0.022 |
| direct | 5000 | 0 | 362.1 | 0.65 | 0.70 | 0.86 | 1.06 | 1.25 | 0.76 | 1.22 | 1.75 | 0.022 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.581 | 1.142 | 0.831 | 1.555 | 0.692 | 1.358 | 0.1% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 236.0 / 237.8 / 239.8 |
| Goroutines start / end / max | 4099 / 3887 / 4099 |
| Open FDs max | 63 |
| CPU cores avg / max | 1.03 / 1.05 |

