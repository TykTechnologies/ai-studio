# AI Gateway benchmark results: v2.2.0-rc10.1

Measured 25 September 2026 on AWS (ap-southeast-2, Sydney), with the released
`tykio/tyk-microgateway-ent:v2.2.0-rc10.1` image running as an edge gateway on
**one 4-vCPU machine** (c7i.xlarge). 2.47 million requests over about 2.5
hours, 18 runs, every one VALID under the suite's own checks.

## Summary

**The gateway adds well under a millisecond to a request, and it holds that
up to about 365 streaming requests per second on 4 vCPU.**

| What you want to know | Result on one 4-vCPU gateway |
|---|---|
| Latency the gateway adds (native `/llm/call`, `/llm/rest`, `/llm/stream`), unloaded | **0.52–0.63 ms p50, 0.81–1.01 ms p99**, including the extra network hop |
| Of that, the gateway's own processing (its Server-Timing header) | **0.30 ms p50, 0.7–0.9 ms p99** |
| Latency the OpenAI-compatible endpoints add (`/ai/…` shim, unified `/v1`) | **0.91–1.07 ms p50, 1.49–1.88 ms p99** |
| End-user overhead against real OpenAI and Anthropic (paired, from Sydney) | _S3 in progress; filled in when it completes_ |
| Sustained streaming load, realistic LLM responses (300 ms to first token, 4.3 s streams) | **365 req/s** (range 316–367 over 3 runs), about **1,600 concurrent streams**, gateway p99 ≤ 3 ms; 0 errors |
| Sustained non-streaming load (upstream answers in 20 ms) | **~1,000 req/s** (range 998–1,182 over 3 runs); 0 errors |
| 3× burst (130 → 390 req/s for 30 s, just above capacity) | 0 errors; gateway p99 peaked at 15 ms for 5 s, back to 1 ms immediately |
| One hour at 70% of capacity (257 req/s, 835,059 requests) | 0 errors; latency and memory flat from first minute to last |
| Analytics delivered to the control plane | 2,092,205 of 2,092,206 gateway responses recorded (one lost, in one run) |

What these numbers mean for sizing:

- **Latency is not the constraint.** A chat user waits hundreds of
  milliseconds to seconds for the first token from the model. The gateway adds
  under a millisecond at p50, and its own p99 stays within a few milliseconds
  until the node runs out of CPU.
- **Plan capacity per node, and scale out.** One 4-vCPU edge sustains about
  365 new streaming requests per second with realistic stream lengths, or about
  1,000 short non-streaming requests per second. Each edge serves requests
  from its own copy of the configuration, so capacity grows with the number of
  nodes behind a load balancer.
- **Run below the knee.** At about 400 streaming req/s the node's CPU is
  saturated and latency rises sharply. The soak at 70% of capacity (257 req/s)
  ran for an hour without any drift.

Two findings qualify these numbers. They are described under
[Behaviour to know about](#behaviour-to-know-about):

1. **rc10.1 edges store request and response bodies in their local database
   even when configured not to**, and the database grows quickly under load
   (3 GB plus a 6.9 GB write-ahead log after 40 minutes of benchmark traffic).
   After the heaviest runs this caused about 80 seconds of slower requests
   (p99 up to 21 ms) and is the likely reason one of three capacity runs
   reached 316 rather than ~365 req/s. It is fixed on main after rc10.1
   (956683b4).
2. **Non-streaming throughput is not CPU-bound.** The REST ceiling was reached
   with the gateway at 2–2.4 of its 4 cores. The limit is elsewhere, most likely
   the edge's per-request database writes.

## What was measured, and why

A buyer asks two questions of a gateway: *what does it add to every request*,
and *how much traffic can it take before that changes*. The suite answers both
with scenarios S0–S7:

| | Question | Load | Upstream |
|---|---|---|---|
| S0 | Is the measurement itself trustworthy? (A/A calibration) | 1 request at a time | mock, instant |
| S1 | What does the gateway add per endpoint? | 1 request at a time, 5,000 per arm and endpoint | mock, instant |
| S2 | Does that depend on prompt size or stream length? | 1 request at a time | mock, instant or 1,000-chunk stream |
| S3 | What does an end user see against real vendors? | paired ABBA blocks | OpenAI, Anthropic |
| S4 | How many streaming requests can one node sustain? | Poisson arrivals, +50 req/s per minute | mock: 300 ms TTFT (p99 900 ms), 200 tokens at 50 tokens/s |
| S5 | How many short requests can one node sustain? | Poisson arrivals, +100 req/s per 30 s | mock: 20 ms |
| S6 | Does it absorb a burst and recover? | base → 3× for 30 s → base | as S4 |
| S7 | Does it drift over time? | 1 hour constant at 70% of S4 | as S4 |

The mock upstream (`mockllm`) stands in for a model everywhere except S3, so
that vendor variability, which is far larger than anything the gateway adds,
does not hide the gateway's own effect. S3 is the only scenario whose absolute
latencies mean something to an end user.

## How it was measured

### Method

The full method is in the [benchmark README](README.md#how-the-numbers-are-kept-honest).
In short:

- **Every number is a difference.** Each gateway request has a matching request
  from the same load generator straight to the same upstream (the "direct"
  arm). Overhead is gateway minus direct, so it includes the extra network hop
  to the gateway as well as the gateway's processing.
- **A/A calibration first.** S0 sends identical traffic down two direct arms.
  Across four S0 runs the tool measured differences of at most 0.07 ms at p50
  and p99, so differences larger than that are real.
- **Open-loop load with coordinated-omission correction.** S4–S7 release
  requests on a Poisson schedule and measure latency from each request's
  intended send time. A slow gateway is charged for the queue it causes. The
  load generator's own release lag stayed under 1 ms p99 in every run, the
  threshold at which a run is marked INVALID.
- **Two independent measures.** The gateway reports its own time on every
  request (`Server-Timing`, enabled with `GATEWAY_SERVER_TIMING=true`). The
  black-box difference and the gateway's own account agree to within the network
  hop (about 0.25–0.35 ms here).
- **Nothing lost silently.** After every run, the proxy-log rows Studio received
  are compared with the requests the gateway answered.
- **Capacity SLO.** A load step counts as sustained when errors are under 0.1%
  and the p99 overhead is within 25 ms, judged by the gateway's own p99 and by
  the black-box p99 difference. The black-box check counts a breach only when
  the 95% confidence interval of the difference lies wholly above 25 ms: with a
  realistic upstream (p99 900 ms) and a 10% baseline sample, p99 differences
  move by 50–100 ms from sampling alone. This rule was fixed during this run
  (#611); the first analysis reported a false knee of 38 req/s.

### Environment

Four dedicated VMs in one AWS cluster placement group (ap-southeast-2a), Ubuntu
24.04, Docker with host networking (no bridge NAT in the path). Built and torn
down with [`cloud/aws.sh`](cloud/aws.sh).

| VM | Instance | Runs | Why this size |
|---|---|---|---|
| gateway | c7i.xlarge: 4 vCPU, 8 GiB | `tykio/tyk-microgateway-ent:v2.2.0-rc10.1`, edge mode | The machine under test. 4 vCPU is a common production size for one edge node, and small enough for one load generator to reach its limit. Capacity scales by adding nodes, so a per-node figure on a modest node is what sizing needs. The gateway has the whole VM, with no CPU limit. |
| loadgen | c7i.2xlarge: 8 vCPU | `gwbench` | Twice the gateway's CPU, so the load generator never becomes the bottleneck (it fell behind at ~300 streaming req/s with 4 CPUs in local tests). |
| mock | c7i.2xlarge: 8 vCPU | `mockllm` | Serves the gateway's traffic plus the direct arm, including ~1,600 open streams in S4, without slowing down; if it slowed, both arms would slow and hide the gateway's limit. |
| hub | m7i.xlarge: 4 vCPU, 16 GiB | `tykio/tyk-ai-studio-ent:v2.2.0-rc10.1` + Postgres 16 | Off the request path, but ingests every analytics record, which the completeness check verifies. |

- **Production configuration**: Enterprise edition, hub-spoke with config from
  Studio over gRPC, analytics shipped to Studio by the analytics pulse, a budget
  on the app, Bearer app credentials, edge token cache at its default,
  `LOG_LEVEL=info`.
- **Network**: loadgen → gateway round trip 0.40 ms average, 0.61 ms p99 (200
  pings). The runbook targets under 0.5 ms p99; this is slightly over, and it
  is included in every overhead figure.
- **Versions**: gateway and Studio images v2.2.0-rc10.1 (commit 5ee92c69). The
  load generator and mock were built from the same commit; the load generator
  was replaced mid-suite with a build carrying the capacity-analysis fix (#611),
  which changes analysis only, not measurement. Every report was then
  regenerated from the raw data with `gwbench report` at main 956683b4.
- **Repeats**: S0–S2 and S4–S5 ran three times each, as the runbook requires.
  Tables give the median of the three runs and the range. S6 and S7 were sized
  from the first S4 run (367 req/s) and ran once.

## Results in detail

### S1: overhead per endpoint, unloaded

One request at a time, 5,000 requests per arm and endpoint. Milliseconds added by
the gateway, median of three runs (range in brackets).

| Endpoint | p50 | p90 | p99 | Gateway's own p50 / p99 (Server-Timing) |
|---|---|---|---|---|
| `/llm/rest` (non-streaming) | 0.63 (0.51–0.63) | 0.76 | 1.01 (0.99–1.03) | 0.30 / 0.71 |
| `/llm/stream` | 0.52 (0.51–0.65) | 0.60 | 0.81 (0.70–13.14) | 0.30 / 0.75 |
| `/llm/call`, OpenAI format, non-streaming | 0.56 (0.55–2.75) | 0.63 | 0.93 (0.87–13.18) | 0.30 / 0.86 |
| `/llm/call`, OpenAI format, streaming | 0.56 (0.53–2.76) | 0.63 | 0.88 (0.79–21.49) | 0.31 / 0.78 |
| `/llm/call`, Anthropic format, streaming | 0.54 (0.54–0.57) | 0.62 | 0.86 (0.83–4.35) | 0.31 / 0.87 |
| `/ai/…` OpenAI shim, non-streaming | 1.05 (1.04–1.07) | 1.17 | 1.88 (1.71–1.88) | 0.70 / 1.43 |
| `/ai/…` OpenAI shim, streaming | 0.91 (0.90–0.93) | 1.03 | 1.53 (1.41–1.58) | 0.81 / 1.66 |
| unified `/v1`, non-streaming | 1.07 (1.03–1.08) | 1.20 | 1.86 (1.66–1.90) | 0.73 / 1.46 |
| unified `/v1`, streaming | 0.92 (0.91–0.95) | 1.05 | 1.49 (1.44–1.49) | 0.83 / 1.56 |

Direct requests to the mock took 0.63–0.67 ms p50, so the gateway roughly doubles
the time of a request to an instant upstream, and adds nothing a model call
would notice. The OpenAI-compatible endpoints cost about 0.4 ms more because they
translate the request and pass it through the gateway a second time over
loopback.

The high ends of the p99 ranges for the native endpoints come from the second of
the three runs, which started right after the heaviest load test. See
[finding 1](#1-edge-database-stalls-after-heavy-load).

### S2: prompt size and stream length

Overhead in milliseconds, median of three runs, `/llm/call` OpenAI format.

| Request | p50 | p99 | Gateway's own p50 |
|---|---|---|---|
| 1 KB prompt | 0.68 | 0.99 | 0.31 |
| 32 KB prompt | 1.38 | 3.06 | 0.86 |
| 256 KB prompt | 5.06 | 10.32 | 4.39 |
| 1,000-chunk stream (time to last chunk) | 0.68 | 2.14 | 0.34 |

Cost grows with the prompt the gateway has to read and inspect: about 0.02 ms
per KB. Relaying a long stream adds almost nothing per chunk.

### S4: streaming capacity

Load rose by 50 req/s every minute until the gateway broke the SLO or the ramp's
stop condition ended it. The upstream behaves like a real model: 300 ms median
time to first token with a tail to 900 ms, then 200 tokens at 50 tokens/s.

| Run | Sustained | First breach | Why | Gateway CPU max |
|---|---|---|---|---|
| 1 | **367 req/s** | 408 req/s | gateway's own p99 54 ms | 3.73 of 4 cores |
| 2 | **316 req/s** | ended by stop condition in the next step (~360 req/s) | p99 more than 500 ms above direct | 3.27 |
| 3 | **365 req/s** | 409 req/s | gateway's own p99 36 ms | 3.81 |

Median **365 req/s**. At that rate about 1,600 streams are open at once
(365 req/s × 4.3 s per stream). Below the knee, time to first token through the
gateway stays within a few milliseconds of the upstream's (p50 301–314 ms against
the upstream's 300 ms), and the gateway's own p99 stays under 3 ms:

| Offered load (run 3) | 38 | 136 | 228 | 315 | 365 | 409 req/s |
|---|---|---|---|---|---|---|
| TTFT p50 through the gateway | 301 | 300 | 301 | 309 | 314 | 324 ms |
| Gateway's own p99 (Server-Timing) | 1.0 | 0.9 | 1.0 | 1.6 | 2.9 | 36.0 ms |

There were no errors in any run, and every request reached Studio's analytics.

### S5: request-rate ceiling

Short non-streaming requests (upstream answers in 20 ms), +100 req/s every 30 s.

| Run | Sustained | Ended by | Gateway CPU max |
|---|---|---|---|
| 1 | 1,182 req/s | stop condition in the next step | 2.37 cores |
| 2 | 998 req/s | breach at 1,093 req/s (p99 +132 ms at 95% confidence) | 2.30 |
| 3 | 1,009 req/s | stop condition in the next step | 1.96 |

Median **~1,000 req/s**, with no errors. The p99 overhead rises steadily from
about 1 ms at 75 req/s to about 20 ms near the ceiling, and then latency
collapses within one step. The gateway never used more than 2.4 of its 4 cores,
so this ceiling is not CPU (see [finding 2](#2-rest-throughput-is-not-cpu-bound)).

### S6: burst

Base load of about 130 req/s, a 30-second burst to about 390 req/s (just above
the S4 knee), then back to base.

- No errors, and all 28,023 requests reached Studio.
- During the burst, TTFT p50 rose by about 10 ms and the gateway's own p99
  peaked at 15 ms in one 5-second window.
- As soon as load returned to base, the gateway's p99 was back at about 1 ms,
  with no backlog.

### S7: one-hour soak

A constant 257 req/s (70% of the S4 knee) for 60 minutes: 835,059 requests
through the gateway.

| | Start | End | Notes |
|---|---|---|---|
| TTFT p50, per 5-minute window | 304 ms | 304 ms | 301–305 ms throughout |
| Gateway's own p99 | 1.02 ms | 0.99 ms | 0.96–1.03 ms throughout |
| Resident memory | 477 MB | 510 MB | slope 0.04 MB/hour, max 600 MB |
| Goroutines | 1,061 | 4,832 | max 9,064; slope +64/hour |
| Errors | 0 | 0 | |
| Analytics received by Studio | | 841,973 of 841,973 | |

No latency drift and no memory growth. The goroutine count settles into a range
of several thousand under this load; its slope is small but not zero, and is
worth watching over longer soaks.

### S3: real vendors

_In progress. This section is filled in when the run completes._

## Behaviour to know about

### 1. Edge database stalls after heavy load

rc10.1 edges store up to `ANALYTICS_MAX_BODY_SIZE` (4 KB) of every request and
response body in their local SQLite database, even when
`ANALYTICS_STORE_REQUESTS` and `ANALYTICS_STORE_RESPONSES` are false (their
default). Rows are kept for `ANALYTICS_RETENTION_DAYS` (90 by default).

- **Storage**: after 40 minutes of benchmark traffic the edge database was
  3.0 GB plus a 6.9 GB write-ahead log, and 12.9 GB after the soak. The
  write-ahead log never shrank. Size the edge's disk for this on rc10.1.
- **Latency**: right after the S5 burst, database writes on the edge took 1.1–2.6
  seconds each. For about 80 seconds, requests waited for the database before
  they were sent upstream: the second S1 run measured 2.2 ms p50 and 12–21 ms p99
  overhead on the native endpoints while the gateway's CPU was nearly idle. The
  lower second S4 run (316 req/s) is most likely the same effect.
- **Privacy**: prompts and responses are kept on the edge although the settings
  say they are not.

Fixed on main after rc10.1: the edge now honours `ANALYTICS_STORE_REQUESTS` /
`ANALYTICS_STORE_RESPONSES` (956683b4). A re-run on a build with the fix will
show how much of the variation above it removes.

### 2. REST throughput is not CPU-bound

The request-rate ceiling (S5) was reached with the gateway at 2.0–2.4 of 4 cores,
whereas streaming (S4) saturated the CPU. Each request also writes analytics,
budget and proxy-log rows to the edge's SQLite database, one write each, and
those writes serialise. That is the most likely limit; it has not been
profiled yet.

### 3. Other behaviour

- **Non-streaming responses are buffered.** For REST requests, time to first
  byte equals total time, because a response hook is always registered.
  Streaming is not affected.
- **The OpenAI-compatible endpoints make a second pass** through
  authentication and policy over loopback, which accounts for their extra
  ~0.4 ms.
- **Network hop.** Every overhead figure includes one extra hop from the load
  generator to the gateway in the same availability zone (about 0.25–0.35 ms
  here). A real deployment adds its own client-to-gateway round trip.

## Reproducing these results

```bash
export BENCH_NAME=rc10-1 BENCH_REF=v2.2.0-rc10.1
benchmarks/gateway/cloud/aws.sh up && benchmarks/gateway/cloud/aws.sh deploy
benchmarks/gateway/cloud/aws.sh seed -vendors
benchmarks/gateway/cloud/aws.sh suite VENDORS=1
benchmarks/gateway/cloud/aws.sh fetch && benchmarks/gateway/cloud/aws.sh down
```

See [RUNBOOK-cloud.md](RUNBOOK-cloud.md). The report and summary of every run
behind this document are in [`published/v2.2.0-rc10.1/`](published/v2.2.0-rc10.1/).
The raw per-request data (about 1 GB) is kept outside the repository. Any
report can be regenerated from it with `gwbench report <run-dir>`.
