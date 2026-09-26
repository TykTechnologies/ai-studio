# S0 · A/A calibration: direct vs direct

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s0-aa-calibration |
| Mode | closed loop (fixed concurrency, back-to-back requests) |
| Started | 2026-09-25 03:58:54 UTC |
| Duration | 8s |
| Code | 5ee92c69a04e |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct-a |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + tykio/tyk-microgateway-ent:v2.2.0-rc10.1; ping p99 0.610ms |
| Target gateway | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

Both arms send the same request straight to the mock upstream. Any difference between them is measurement noise, so this run sets the noise floor for every other scenario. The run is INVALID unless the 95% confidence interval for the median difference contains zero. Run it first, on the same hosts, every time.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | error rate (direct-a) | rest | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | rest | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | rest | median difference 0.016ms, 95% CI [0.000, 0.030] |
| pass | error rate (direct-a) | stream | 0 of 3000 failed (0.000%) |
| pass | error rate (direct-b) | stream | 0 of 3000 failed (0.000%) |
| pass | A/A calibration: no difference between identical arms | stream | median difference 0.041ms, 95% CI [0.024, 0.056] |
| pass | analytics recorded every gateway request |  | Studio gained 0 proxy-log rows for 0 gateway responses |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest | direct-b | TTFT | 0.02 [0.00, 0.03] | -0.01 [-0.02, 0.00] | -0.01 [-0.05, 0.01] |  |
| rest | direct-b | TOTAL | 0.02 [0.00, 0.03] | -0.01 [-0.02, 0.00] | -0.01 [-0.05, 0.01] |  |
| stream | direct-b | TTFT | 0.05 [0.04, 0.07] | 0.02 [0.01, 0.04] | -0.00 [-0.05, 0.04] |  |
| stream | direct-b | TOTAL | 0.04 [0.02, 0.06] | 0.02 [0.00, 0.04] | 0.01 [-0.04, 0.06] |  |

## Cell: rest

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 802.2 | 0.57 | 0.59 | 0.82 | 0.97 | 1.12 | 0.59 | 0.97 | 1.43 | 0.015 |
| direct-b | 3000 | 0 | 802.2 | 0.58 | 0.60 | 0.81 | 0.95 | 1.10 | 0.60 | 0.95 | 1.30 | 0.015 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 493.4 / 494.1 / 494.1 |
| Goroutines start / end / max | 1046 / 1047 / 1047 |
| Open FDs max | 565 |
| CPU cores avg / max | 0.01 / 0.01 |

## Cell: stream

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| direct-a | 3000 | 0 | 693.9 | 0.53 | 0.58 | 0.84 | 1.03 | 1.21 | 0.65 | 1.16 | 1.91 | 0.016 |
| direct-b | 3000 | 0 | 693.9 | 0.59 | 0.63 | 0.86 | 1.03 | 1.26 | 0.69 | 1.17 | 1.69 | 0.014 |

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 494.6 / 495.1 / 495.1 |
| Goroutines start / end / max | 1047 / 1046 / 1047 |
| Open FDs max | 565 |
| CPU cores avg / max | 0.01 / 0.01 |

