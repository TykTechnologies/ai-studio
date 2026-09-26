# S0 · A/A calibration: direct vs direct

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s0-aa-calibration |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 17:29:27 UTC |
| Duration | 9s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct-a |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
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
| pass | A/A calibration: no difference between identical arms | rest | median difference -0.009ms, 95% CI [-0.021, 0.004] |
| pass | error rate (direct-a) | stream | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | stream | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | stream | median difference 0.011ms, 95% CI [-0.005, 0.026] |
| pass | analytics recorded every gateway request |  | Studio gained 0 proxy-log rows for 0 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest | direct-b | TTFT | -0.01 [-0.02, 0.00] | 0.00 [-0.01, 0.01] | 0.01 [-0.03, 0.03] |  |
| rest | direct-b | TOTAL | -0.01 [-0.02, 0.00] | 0.00 [-0.01, 0.01] | 0.01 [-0.03, 0.03] |  |
| stream | direct-b | TTFT | 0.01 [-0.01, 0.02] | 0.00 [-0.02, 0.02] | -0.00 [-0.04, 0.06] |  |
| stream | direct-b | TOTAL | 0.01 [-0.01, 0.03] | -0.00 [-0.02, 0.01] | -0.02 [-0.06, 0.03] |  |

## Cell: rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 860.3 | 0.53 | 0.56 | 0.75 | 0.87 | 1.06 | 0.56 | 0.87 | 1.32 | 0.016 |
| direct-b | 3000 | 0 | 860.3 | 0.53 | 0.55 | 0.75 | 0.88 | 1.18 | 0.55 | 0.88 | 1.89 | 0.016 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 618.5 / 618.5 / 618.5 |
| Goroutines start / end / max | 980 / 982 / 982 |
| Open FDs max | 491 |
| CPU cores avg / max | 0.01 / 0.01 |

## Cell: stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 760.3 | 0.47 | 0.52 | 0.78 | 1.00 | 1.38 | 0.59 | 1.14 | 1.72 | 0.020 |
| direct-b | 3000 | 0 | 760.3 | 0.48 | 0.52 | 0.79 | 1.00 | 1.23 | 0.60 | 1.12 | 1.68 | 0.021 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 618.5 / 619.8 / 619.8 |
| Goroutines start / end / max | 982 / 981 / 982 |
| Open FDs max | 491 |
| CPU cores avg / max | 0.01 / 0.01 |

