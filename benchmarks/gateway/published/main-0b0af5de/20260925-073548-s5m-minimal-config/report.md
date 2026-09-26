# S5m · Request-rate ceiling, minimal configuration

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5m-minimal-config |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:35:48 UTC |
| Duration | 4m20s |
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
| pass | load generator kept schedule (gateway) | rest-instant | p99 release lag 1.124ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-instant | 0 of 113363 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-instant | p99 release lag 1.123ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-instant | 0 of 11465 failed (0.000%) |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-instant | gateway | TTFT | 0.59 [0.56, 0.61] | 0.76 [0.72, 0.80] | 89.29 [30.83, 102.84] |  |
| rest-instant | gateway | TOTAL | 0.59 [0.56, 0.61] | 0.76 [0.72, 0.80] | 89.29 [30.83, 102.84] |  |

## Cell: rest-instant

> **Stopped early: gateway p99 total is 543.3ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 113363 | 0 | 435.3 | 1.60 | 1.62 | 2.31 | 91.12 | 976.51 | 1.62 | 91.12 | 1749.47 | 1.124 |
| direct | 11465 | 0 | 44.0 | 1.01 | 1.03 | 1.55 | 1.83 | 2.08 | 1.03 | 1.83 | 4.02 | 1.123 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.238 | 85.547 | 0.249 | 86.230 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 8.2 | 247 | 0.00% | 1.18 | 1.94 | – | 1.94 | – |
| 0 | gateway | 79.0 | 2369 | 0.00% | 1.91 | 2.78 | 0.71 | 2.78 | 0.436 |
| 30 | direct | 18.0 | 539 | 0.00% | 1.16 | 1.84 | – | 1.84 | – |
| 30 | gateway | 184.0 | 5519 | 0.00% | 1.78 | 2.86 | 0.86 | 2.86 | 0.508 |
| 60 | direct | 27.6 | 828 | 0.00% | 1.12 | 1.85 | – | 1.85 | – |
| 60 | gateway | 270.7 | 8120 | 0.00% | 1.70 | 2.83 | 0.85 | 2.83 | 0.682 |
| 90 | direct | 35.0 | 1051 | 0.00% | 1.08 | 1.82 | – | 1.82 | – |
| 90 | gateway | 368.7 | 11062 | 0.00% | 1.62 | 2.89 | 0.84 | 2.89 | 0.823 |
| 120 | direct | 46.4 | 1392 | 0.00% | 1.05 | 1.83 | – | 1.83 | – |
| 120 | gateway | 455.3 | 13660 | 0.00% | 1.58 | 3.05 | 1.04 | 3.05 | 0.940 |
| 150 | direct | 54.7 | 1640 | 0.00% | 1.02 | 1.81 | – | 1.81 | – |
| 150 | gateway | 544.5 | 16335 | 0.00% | 1.56 | 3.26 | 1.17 | 3.26 | 0.928 |
| 180 | direct | 65.8 | 1973 | 0.00% | 1.00 | 1.81 | – | 1.81 | – |
| 180 | gateway | 639.9 | 19198 | 0.00% | 1.56 | 3.56 | 1.39 | 3.56 | 0.912 |
| 210 | direct | 74.2 | 2225 | 0.00% | 0.98 | 1.81 | – | 1.81 | – |
| 210 | gateway | 723.8 | 21715 | 0.00% | 1.56 | 3.82 | 1.63 | 3.82 | 0.895 |

### Capacity

Sustained 724 req/s within the 25ms p99 overhead SLO. The run's stop condition ended the ramp during the next step: gateway p99 total is 543.3ms above direct over the last 10s. A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 425.7 / 368.7 / 426.7 |
| Goroutines start / end / max | 132 / 8392 / 9002 |
| Open FDs max | 659 |
| CPU cores avg / max | 0.52 / 0.99 |

