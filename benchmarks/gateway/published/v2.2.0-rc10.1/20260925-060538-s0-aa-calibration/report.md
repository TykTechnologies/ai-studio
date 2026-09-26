# S0 · A/A calibration: direct vs direct

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s0-aa-calibration |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 06:05:38 UTC |
| Duration | 8s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct-a |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target anthropic | https://api.anthropic.com/v1 |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |
| Target openai | https://api.openai.com/v1 |

Both arms send the same request straight to the mock upstream. Any difference between them is measurement noise, so this run sets the noise floor for every other scenario. The run is INVALID unless the 95% confidence interval for the median difference contains zero. Run it first, on the same hosts, every time.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (direct-a) | rest | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | rest | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | rest | median difference 0.001ms, 95% CI [-0.008, 0.010] |
| pass | error rate (direct-a) | stream | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | stream | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | stream | median difference 0.000ms, 95% CI [-0.014, 0.016] |
| pass | analytics recorded every gateway request |  | Studio gained 0 proxy-log rows for 0 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest | direct-b | TTFT | 0.00 [-0.01, 0.01] | 0.02 [0.01, 0.03] | 0.04 [0.00, 0.07] |  |
| rest | direct-b | TOTAL | 0.00 [-0.01, 0.01] | 0.02 [0.01, 0.03] | 0.04 [0.00, 0.07] |  |
| stream | direct-b | TTFT | 0.02 [-0.00, 0.03] | 0.02 [0.01, 0.04] | 0.05 [0.00, 0.08] |  |
| stream | direct-b | TOTAL | 0.00 [-0.01, 0.02] | 0.03 [0.01, 0.04] | 0.04 [-0.03, 0.07] |  |

## Cell: rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 758.7 | 0.62 | 0.64 | 0.81 | 0.95 | 1.09 | 0.64 | 0.95 | 1.19 | 0.016 |
| direct-b | 3000 | 0 | 758.7 | 0.62 | 0.64 | 0.83 | 0.99 | 1.12 | 0.64 | 0.99 | 1.66 | 0.015 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 496.9 / 497.6 / 497.6 |
| Goroutines start / end / max | 1061 / 1061 / 1061 |
| Open FDs max | 556 |
| CPU cores avg / max | 0.00 / 0.01 |

## Cell: stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 706.1 | 0.52 | 0.56 | 0.83 | 1.00 | 1.23 | 0.66 | 1.16 | 1.40 | 0.021 |
| direct-b | 3000 | 0 | 706.1 | 0.54 | 0.58 | 0.85 | 1.05 | 1.23 | 0.67 | 1.19 | 1.48 | 0.021 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 497.6 / 498.2 / 498.2 |
| Goroutines start / end / max | 1060 / 1060 / 1061 |
| Open FDs max | 555 |
| CPU cores avg / max | 0.01 / 0.01 |

