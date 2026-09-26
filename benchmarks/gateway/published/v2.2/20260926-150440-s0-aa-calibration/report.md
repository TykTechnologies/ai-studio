# S0 · A/A calibration: direct vs direct

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s0-aa-calibration |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 15:04:40 UTC |
| Duration | 9s |
| Code | c94e0f17684e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct-a |
| Label | main-adaptive-c94e0f17: v2.2 release build (edge + Studio from main c94e0f17; adaptive GOGC; defaults, no tuning env); empty hub; c7i.xlarge edge |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Both arms send the same request straight to the mock upstream. Any difference between them is measurement noise, so this run sets the noise floor for every other scenario. The run is INVALID unless the 95% confidence interval for the median difference contains zero. Run it first, on the same hosts, every time.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (direct-a) | rest | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | rest | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | rest | median difference -0.045ms, 95% CI [-0.057, -0.030] |
| pass | error rate (direct-a) | stream | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | stream | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | stream | median difference 0.002ms, 95% CI [-0.013, 0.016] |
| pass | analytics recorded every gateway request |  | Studio gained 0 proxy-log rows for 0 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest | direct-b | TTFT | -0.05 [-0.06, -0.03] | -0.04 [-0.05, -0.02] | -0.01 [-0.06, 0.03] |  |
| rest | direct-b | TOTAL | -0.05 [-0.06, -0.03] | -0.04 [-0.05, -0.02] | -0.01 [-0.06, 0.03] |  |
| stream | direct-b | TTFT | 0.01 [-0.01, 0.03] | 0.01 [-0.00, 0.02] | -0.03 [-0.08, 0.01] |  |
| stream | direct-b | TOTAL | 0.00 [-0.01, 0.02] | 0.01 [-0.01, 0.03] | 0.02 [-0.03, 0.06] |  |

## Cell: rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 812.5 | 0.58 | 0.60 | 0.83 | 1.00 | 1.28 | 0.60 | 1.00 | 1.60 | 0.017 |
| direct-b | 3000 | 0 | 812.5 | 0.53 | 0.55 | 0.80 | 0.99 | 1.12 | 0.55 | 0.99 | 1.29 | 0.015 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 201.7 / 201.7 / 201.7 |
| Goroutines start / end / max | 39 / 40 / 40 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.00 / 0.01 |

## Cell: stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 722.8 | 0.52 | 0.56 | 0.82 | 1.04 | 1.30 | 0.65 | 1.18 | 1.68 | 0.015 |
| direct-b | 3000 | 0 | 722.8 | 0.53 | 0.57 | 0.83 | 1.01 | 1.34 | 0.65 | 1.20 | 1.69 | 0.018 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 201.7 / 201.7 / 201.7 |
| Goroutines start / end / max | 38 / 40 / 40 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.01 / 0.01 |

