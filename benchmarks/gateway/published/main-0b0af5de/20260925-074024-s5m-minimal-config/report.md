# S5m · Request-rate ceiling, minimal configuration

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5m-minimal-config |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:40:24 UTC |
| Duration | 4m51s |
| Code | acafd3580957 |
| Load generator | ip-10-77-1-242, 8 CPUs, linux/amd64, Linux 7.0.0-1013-aws |
| Baseline arm | direct |
| Label | aws ap-southeast-2 ap-southeast-2a, cluster PG; gateway c7i.xlarge (whole VM), loadgen c7i.2xlarge, mock c7i.2xlarge, hub m7i.xlarge; images tykio/tyk-ai-studio-ent:v2.2.0-rc10.1 + gwbench-microgateway:0b0af5de; ping p99 0.578ms; MINIMAL CONFIG: instant mock, no budget, unpriced model, no analytics pulse, LOG_LEVEL=error (auth + local analytics insert still on) |
| Target gateway | http://10.77.1.175:8080 |
| Target gateway-minimal | http://10.77.1.175:8080 |
| Target mock | http://10.77.1.50:9999 |

S5 under the conditions gateway throughput figures are usually published with: an upstream that answers instantly, no budget on the app, a model with no price (so no cost or budget-usage accounting), no analytics shipped to the control plane and LOG_LEVEL=error. It is what configuration alone can take off the request path. Authentication and the edge's local analytics insert still run on every request, so this is not a no-auth, no-logging figure. Needs `gwbench seed -minimal`, a gateway deployed with BENCH_PLUGINS_CONFIG_PATH= and BENCH_LOG_LEVEL=error, and `gwbench run -no-analytics-check`.

Latencies are in milliseconds, measured by the load generator from each request's intended send time. TTFT is time to the first content token (for non-streaming requests, the complete response). Overhead is the arm's percentile minus the baseline arm's, with a 95% bootstrap confidence interval. Server-Timing figures are the gateway's own account of its time, excluding network hops.

## Validity checks

| Result | Check | Cell | Detail |
| --- | --- | --- | --- |
| pass | load generator kept schedule (gateway) | rest-instant | p99 release lag 1.121ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-instant | 0 of 140008 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-instant | p99 release lag 1.121ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-instant | 0 of 13841 failed (0.000%) |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-instant | gateway | TTFT | 0.58 [0.56, 0.61] | 0.73 [0.67, 0.75] | 161.67 [92.15, 358.46] |  |
| rest-instant | gateway | TOTAL | 0.58 [0.56, 0.61] | 0.73 [0.67, 0.75] | 161.67 [92.15, 358.46] |  |

## Cell: rest-instant

> **Stopped early: gateway p99 total is 696.1ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 140008 | 0 | 480.6 | 1.58 | 1.60 | 2.28 | 163.50 | 2090.90 | 1.60 | 163.50 | 3537.37 | 1.121 |
| direct | 13841 | 0 | 47.5 | 1.00 | 1.02 | 1.55 | 1.83 | 2.06 | 1.02 | 1.83 | 5.28 | 1.121 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.235 | 157.933 | 0.246 | 159.387 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 6.7 | 202 | 0.00% | 1.25 | 1.92 | – | 1.92 | – |
| 0 | gateway | 76.5 | 2295 | 0.00% | 1.96 | 2.91 | 0.82 | 2.91 | 0.636 |
| 30 | direct | 17.6 | 529 | 0.00% | 1.14 | 1.90 | – | 1.90 | – |
| 30 | gateway | 182.5 | 5475 | 0.00% | 1.79 | 2.87 | 0.87 | 2.87 | 0.463 |
| 60 | direct | 26.8 | 803 | 0.00% | 1.13 | 1.82 | – | 1.82 | – |
| 60 | gateway | 277.4 | 8321 | 0.00% | 1.70 | 2.73 | 0.76 | 2.73 | 0.534 |
| 90 | direct | 34.9 | 1047 | 0.00% | 1.07 | 1.86 | – | 1.86 | – |
| 90 | gateway | 365.2 | 10956 | 0.00% | 1.62 | 2.89 | 0.85 | 2.89 | 0.827 |
| 120 | direct | 45.1 | 1353 | 0.00% | 1.10 | 1.81 | – | 1.81 | – |
| 120 | gateway | 454.0 | 13619 | 0.00% | 1.58 | 3.06 | 0.97 | 3.06 | 0.957 |
| 150 | direct | 54.8 | 1643 | 0.00% | 1.03 | 1.85 | – | 1.85 | – |
| 150 | gateway | 548.7 | 16461 | 0.00% | 1.57 | 3.40 | 1.36 | 3.40 | 0.955 |
| 180 | direct | 62.5 | 1875 | 0.00% | 1.02 | 1.82 | – | 1.82 | – |
| 180 | gateway | 648.2 | 19446 | 0.00% | 1.55 | 3.73 | 1.25 | 3.73 | 0.909 |
| 210 | direct | 73.5 | 2204 | 0.00% | 0.98 | 1.83 | – | 1.83 | – |
| 210 | gateway | 730.8 | 21924 | 0.00% | 1.56 | 4.07 | 1.88 | 4.07 | 0.935 |
| 240 | direct | 80.5 | 2414 | 0.00% | 0.95 | 1.76 | – | 1.76 | – |
| 240 | gateway | 815.6 | 24468 | 0.00% | 1.54 | 4.73 | 2.31 | 4.73 | 0.912 |

### Capacity

Sustained 816 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 696.1ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 369.3 / 396.5 / 396.5 |
| Goroutines start / end / max | 254 / 7987 / 9807 |
| Open FDs max | 743 |
| CPU cores avg / max | 0.57 / 1.07 |

