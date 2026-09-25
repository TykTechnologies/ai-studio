# S5m · Request-rate ceiling, minimal configuration

> **VALID: every validity check passed.**

|  |  |
| --- | --- |
| Scenario | s5m-minimal-config |
| Mode | open loop (fixed arrival schedule, coordinated-omission corrected) |
| Started | 2026-09-25 07:26:09 UTC |
| Duration | 9m9s |
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
| pass | load generator kept schedule (gateway) | rest-instant | p99 release lag 1.113ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (gateway) | rest-instant | 0 of 481155 failed (0.000%) |
| pass | load generator kept schedule (direct) | rest-instant | p99 release lag 1.117ms (limit 2ms); beyond it the load generator, not the target, shapes the results |
| pass | error rate (direct) | rest-instant | 0 of 47824 failed (0.000%) |

## Overhead summary

| Cell | Arm | Metric | p50 overhead | p90 overhead | p99 overhead | Paired median |
| --- | --- | --- | --- | --- | --- | --- |
| rest-instant | gateway | TTFT | 0.74 [0.71, 0.77] | 3.19 [3.02, 3.99] | 109.10 [60.85, 105.77] |  |
| rest-instant | gateway | TOTAL | 0.74 [0.71, 0.77] | 3.19 [3.02, 3.99] | 109.10 [60.85, 105.77] |  |

## Cell: rest-instant

> **Stopped early: gateway p99 total is 524.3ms above direct over the last 10s**

| Arm | n | Errors | req/s | TTFB p50 | TTFT p50 | TTFT p90 | TTFT p99 | TTFT p99.9 | Total p50 | Total p99 | Total max | Lag p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 481155 | 0 | 877.1 | 1.63 | 1.65 | 4.66 | 110.91 | 440.56 | 1.65 | 110.91 | 1088.74 | 1.113 |
| direct | 47824 | 0 | 87.2 | 0.89 | 0.90 | 1.46 | 1.81 | 2.52 | 0.91 | 1.81 | 9.30 | 1.117 |

### Gateway Server-Timing (self-reported)

| Arm | gw-pre p50 | gw-pre p99 | gw p50 | gw p99 | gw-ttfb p50 | gw-ttfb p99 | New upstream conns |
| --- | --- | --- | --- | --- | --- | --- | --- |
| gateway | 0.244 | 109.235 | 0.254 | 109.479 | – | – | 0.0% |

gw-pre: request received to upstream request sent. gw: all gateway time outside the upstream call. gw-ttfb: gateway share of time to first body byte. New upstream conns: requests on which the gateway opened a fresh connection (TCP+TLS) to the upstream instead of reusing one.

### Over time

| t (s) | Arm | req/s | n | Errors | TTFT p50 | TTFT p99 | p99 over baseline ≥ (95%) | Total p99 | gw p99 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | direct | 6.6 | 199 | 0.00% | 1.20 | 1.82 | – | 1.82 | – |
| 0 | gateway | 77.0 | 2311 | 0.00% | 1.91 | 2.83 | – | 2.83 | 0.490 |
| 30 | direct | 19.5 | 584 | 0.00% | 1.19 | 1.87 | – | 1.87 | – |
| 30 | gateway | 184.2 | 5527 | 0.00% | 1.82 | 2.81 | 0.84 | 2.81 | 0.521 |
| 60 | direct | 28.5 | 854 | 0.00% | 1.08 | 1.87 | – | 1.87 | – |
| 60 | gateway | 279.4 | 8381 | 0.00% | 1.68 | 2.78 | 0.81 | 2.78 | 0.593 |
| 90 | direct | 35.0 | 1051 | 0.00% | 1.12 | 1.84 | – | 1.84 | – |
| 90 | gateway | 360.5 | 10816 | 0.00% | 1.63 | 3.03 | 1.01 | 3.03 | 0.668 |
| 120 | direct | 44.2 | 1326 | 0.00% | 1.09 | 1.88 | – | 1.88 | – |
| 120 | gateway | 453.4 | 13601 | 0.00% | 1.59 | 3.10 | 1.07 | 3.10 | 0.722 |
| 150 | direct | 54.7 | 1640 | 0.00% | 1.02 | 1.85 | – | 1.85 | – |
| 150 | gateway | 545.4 | 16363 | 0.00% | 1.56 | 3.28 | 1.06 | 3.28 | 0.800 |
| 180 | direct | 66.3 | 1989 | 0.00% | 1.03 | 1.87 | – | 1.87 | – |
| 180 | gateway | 640.6 | 19217 | 0.00% | 1.56 | 3.51 | 1.52 | 3.51 | 0.833 |
| 210 | direct | 72.5 | 2175 | 0.00% | 0.98 | 1.84 | – | 1.84 | – |
| 210 | gateway | 725.1 | 21753 | 0.00% | 1.54 | 3.99 | 1.76 | 3.99 | 0.790 |
| 240 | direct | 79.4 | 2383 | 0.00% | 1.01 | 1.83 | – | 1.83 | – |
| 240 | gateway | 821.7 | 24651 | 0.00% | 1.54 | 4.22 | 1.87 | 4.22 | 0.784 |
| 270 | direct | 90.2 | 2705 | 0.00% | 0.95 | 1.81 | – | 1.81 | – |
| 270 | gateway | 914.6 | 27439 | 0.00% | 1.54 | 5.75 | 3.17 | 5.75 | 0.775 |
| 300 | direct | 97.0 | 2911 | 0.00% | 0.93 | 1.78 | – | 1.78 | – |
| 300 | gateway | 1008.5 | 30256 | 0.00% | 1.55 | 8.39 | 5.08 | 8.39 | 0.880 |
| 330 | direct | 109.7 | 3291 | 0.00% | 0.91 | 1.79 | – | 1.79 | – |
| 330 | gateway | 1091.8 | 32754 | 0.00% | 1.55 | 11.95 | 8.57 | 11.95 | 1.003 |
| 360 | direct | 117.5 | 3524 | 0.00% | 0.90 | 1.84 | – | 1.84 | – |
| 360 | gateway | 1181.7 | 35450 | 0.00% | 1.53 | 9.30 | 6.73 | 9.30 | 1.022 |
| 390 | direct | 128.5 | 3856 | 0.00% | 0.87 | 1.82 | – | 1.82 | – |
| 390 | gateway | 1274.6 | 38239 | 0.00% | 1.53 | 13.66 | 9.40 | 13.66 | 1.658 |
| 420 | direct | 135.5 | 4066 | 0.00% | 0.87 | 1.76 | – | 1.76 | – |
| 420 | gateway | 1366.5 | 40995 | 0.00% | 1.53 | 12.73 | 8.59 | 12.73 | 4.793 |
| 450 | direct | 148.2 | 4447 | 0.00% | 0.83 | 1.72 | – | 1.72 | – |
| 450 | gateway | 1452.3 | 43569 | 0.00% | 1.57 | 13.16 | 9.16 | 13.16 | 7.281 |
| 480 | direct | 153.0 | 4589 | 0.00% | 0.80 | 1.73 | – | 1.73 | – |
| 480 | gateway | 1545.0 | 46351 | 0.00% | 1.77 | 17.37 | 14.47 | 17.37 | 13.506 |
| 510 | direct | 162.9 | 4887 | 0.00% | 0.78 | 1.73 | – | 1.73 | – |
| 510 | gateway | 1636.9 | 49107 | 0.00% | 3.18 | 32.77 | 26.97 | 32.77 | 31.160 |

### Capacity

Sustained 1545 req/s within the 25ms p99 overhead SLO. First breach at 1637 req/s: gateway p99 TTFT 31.0ms above baseline, at least 27.0ms at 95% confidence (SLO 25ms). A step breaches on errors, or on p99 overhead measured by the gateway's Server-Timing or against the baseline pooled over the ramp (only when the 95% confidence interval of the difference is wholly above the SLO); latency is judged only from 200 or more samples.

### Gateway resources

|  |  |
| --- | --- |
| RSS (MB) start / end / max | 75.5 / 432.4 / 432.4 |
| Goroutines start / end / max | 40 / 18597 / 18597 |
| Open FDs max | 783 |
| CPU cores avg / max | 1.08 / 2.37 |

