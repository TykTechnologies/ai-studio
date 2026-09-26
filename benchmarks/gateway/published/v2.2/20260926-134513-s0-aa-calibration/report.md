# S0 · A/A calibration: direct vs direct

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s0-aa-calibration |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-26 13:45:13 UTC |
| Duration | 8s |
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
| pass | A/A calibration: no difference between identical arms | rest | median difference 0.012ms, 95% CI [-0.006, 0.028] |
| pass | error rate (direct-a) | stream | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | stream | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | stream | median difference 0.002ms, 95% CI [-0.011, 0.015] |
| pass | analytics recorded every gateway request |  | Studio gained 0 proxy-log rows for 0 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest | direct-b | TTFT | 0.01 [-0.01, 0.03] | 0.01 [0.00, 0.03] | 0.01 [-0.03, 0.04] |  |
| rest | direct-b | TOTAL | 0.01 [-0.01, 0.03] | 0.01 [0.00, 0.03] | 0.01 [-0.03, 0.04] |  |
| stream | direct-b | TTFT | -0.01 [-0.03, 0.01] | 0.01 [-0.01, 0.03] | 0.01 [-0.06, 0.06] |  |
| stream | direct-b | TOTAL | 0.00 [-0.01, 0.02] | 0.02 [-0.00, 0.03] | 0.01 [-0.06, 0.08] |  |

## Cell: rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 863.5 | 0.52 | 0.54 | 0.79 | 0.96 | 1.26 | 0.54 | 0.96 | 1.51 | 0.016 |
| direct-b | 3000 | 0 | 863.5 | 0.53 | 0.55 | 0.80 | 0.97 | 1.17 | 0.55 | 0.97 | 1.60 | 0.016 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 202.3 / 203.6 / 203.6 |
| Goroutines start / end / max | 39 / 39 / 40 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.01 / 0.01 |

## Cell: stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 776.8 | 0.47 | 0.51 | 0.76 | 0.99 | 1.42 | 0.58 | 1.13 | 2.19 | 0.021 |
| direct-b | 3000 | 0 | 776.8 | 0.46 | 0.50 | 0.77 | 1.00 | 1.32 | 0.59 | 1.13 | 4.73 | 0.023 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 203.6 / 204.1 / 204.1 |
| Goroutines start / end / max | 38 / 39 / 39 |
| Open FDs max | 20 |
| CPU cores avg / max | 0.01 / 0.01 |

