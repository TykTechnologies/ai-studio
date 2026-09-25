# AI Gateway benchmark results

Measured 25 September 2026 on AWS (ap-southeast-2, Sydney), with the Enterprise
edge gateway running on **one 4-vCPU machine** (c7i.xlarge) under its full
production policy: app credential authentication, access checks, a monthly
budget enforced on every request, cost accounting, and analytics shipped to the
control plane. Two builds were measured:

- **v2.2.0-rc10.1**, the released image: the full suite, about 2.5 million
  requests in 18 complete runs, every one VALID under the suite's own checks,
  plus a real-vendor run that was stopped after three of its eight cells.
- **main at 0b0af5de**: rc10.1 plus the edge fix that stops it storing request
  and response bodies against its settings (#612), with the rc10.1 image's
  binary replaced by one built from main. Used for the throughput figures: six
  more runs, all VALID.

## Summary

**Under full production policy the gateway adds about half a millisecond to a
request, and streaming traffic stays stable from idle to the edge of capacity,
through bursts and over an hour of sustained load.**

### Latency under real policy load

| | One 4-vCPU gateway, every request authenticated, budgeted and recorded |
|---|---|
| Added by the gateway, native endpoints (`/llm/call`, `/llm/rest`, `/llm/stream`) | **0.52–0.63 ms p50, 0.81–1.01 ms p99**, including the extra network hop |
| Of that, the gateway's own processing (its `Server-Timing` header) | **0.30 ms p50**, 0.7–0.9 ms p99 |
| Added by the OpenAI-compatible endpoints (`/ai/…` shim, unified `/v1`) | 0.91–1.07 ms p50, 1.49–1.88 ms p99 |
| Added under streaming load, up to ~365 new streams per second | gateway's own p99 **under 3 ms**; time to first token p50 within 1–8 ms of the upstream's |
| Against real OpenAI, from Sydney (200 paired requests per cell) | **not measurable**: +5 ms [−9, +32] streaming, −12 ms [−30, +6] non-streaming, against 720–1,250 ms vendor response times |

### Streaming stability

| | Result |
|---|---|
| One hour at 257 streaming req/s (835,059 requests, 4.3 s streams) | **0 errors**; time to first token 301–305 ms p50 in every 5-minute window; gateway p99 0.96–1.03 ms throughout; memory flat (+33 MB) |
| 3× burst (130 → 390 req/s for 30 s, just above capacity) | **0 errors**; gateway p99 peaked at 15 ms for 5 s and was back to 1 ms as soon as the burst ended |
| Concurrent streams on one node | about **1,600** (365 new streams/s × 4.3 s), 0 errors in every streaming run |
| Analytics delivered to the control plane (rc10.1 runs) | 2,092,205 of 2,092,206 gateway responses (one lost, in one run) |

What this means in practice:

- **Latency is not the constraint.** A chat user waits hundreds of
  milliseconds to seconds for a model's first token. The gateway adds about
  half a millisecond, and against a real vendor its effect is smaller than the
  vendor's own variation from one request to the next.
- **Streams do not degrade before the node is full.** Up to about 365 new
  streams per second (about 1,600 open at once), time to first token through
  the gateway tracks the upstream's. Past that the 4 vCPUs are saturated; add
  nodes behind a load balancer, each serving from its own copy of the
  configuration.

### Throughput: read with care

Request-rate figures are where gateway comparisons usually start, but these
are not yet a like-for-like number, so they are not the headline:

- With full policy, the non-streaming ceiling on main is **about 1,190 req/s
  on a freshly started edge, falling to 640–730 req/s** once the edge's local
  database has grown over a few minutes of load (rc10.1: about 1,000 req/s).
  The gateway reaches it using 1–2.4 of its 4 cores: the limit is the edge's
  per-request writes to its local SQLite database, not CPU. Work to remove it
  is under way.
- Published gateway throughput figures are usually measured with an instant
  upstream and no authentication, logging or accounting. With what
  configuration alone can remove (instant upstream, no budget, no cost
  accounting, no analytics shipping, `LOG_LEVEL=error`), the ceiling is
  **about 1,545 req/s on a fresh edge**, with the gateway adding 0.5–0.7 ms at
  p50 up to 1,450 req/s. It falls to 720–820 req/s once the database has
  grown, about the same as with full policy. Authentication and the edge's
  local analytics record cannot be switched off by configuration, so this is
  still not a no-auth figure. A benchmark-only build that removes them is in
  progress for that comparison.
- Until the SQLite ceiling is removed, size non-streaming capacity from the
  full-policy figure.

See [Throughput](#throughput) for the runs, and
[Behaviour to know about](#behaviour-to-know-about) for the findings behind
these caveats.

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
| S5m | The same, under minimal configuration (no budget, no cost accounting, no analytics shipping, `LOG_LEVEL=error`) | as S5 | mock, instant |
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
  which changes analysis only, not measurement. Every rc10.1 report was then
  regenerated from the raw data with `gwbench report` using that analysis.
- **The main build** (throughput runs only): the rc10.1 image with its
  `tyk-microgateway` binary replaced by one built from main 0b0af5de the way
  the release builds it (CGO, `-tags=enterprise`, Debian glibc), using
  `aws.sh build-gateway`. Studio stayed on the rc10.1 image; main differs from
  rc10.1 only by the edge fix (#612) and benchmark tooling. The edge was started
  with an empty database before each set of three runs.
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
[finding 1](#1-rc101-edges-store-request-and-response-bodies).

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
gateway stays within 1–8 ms (p50) of the upstream's as the gateway observed it
(301–314 ms through the gateway; the mock's nominal median is 300 ms), and the
gateway's own p99 stays under 3 ms:

| Offered load (run 3) | 38 | 136 | 228 | 315 | 365 | 409 req/s |
|---|---|---|---|---|---|---|
| TTFT p50 through the gateway | 301 | 300 | 301 | 309 | 314 | 324 ms |
| Gateway's own p99 (Server-Timing) | 1.0 | 0.9 | 1.0 | 1.6 | 2.9 | 36.0 ms |

There were no errors in any run, and every request reached Studio's analytics.

### Throughput

Short non-streaming requests, +100 req/s every 30 s until the SLO or the stop
condition ends the ramp. Every run below was VALID with **no errors**.

**S5, full policy, upstream answers in 20 ms.** Three runs on each build, one
after the other on the same edge:

| Build | Run 1 | Run 2 | Run 3 | Gateway CPU at the ceiling |
|---|---|---|---|---|
| v2.2.0-rc10.1 (released image) | 1,182 req/s | 998 req/s | 1,009 req/s | 2.0–2.4 cores |
| main 0b0af5de, edge started fresh before run 1 | **1,190 req/s** | 636 req/s | 727 req/s | 2.3, then 1.1–1.3 cores |

**S5m, minimal configuration, instant upstream** (main 0b0af5de, a fresh edge
before run 1, no budget, no model price, no analytics shipping,
`LOG_LEVEL=error`; authentication and the edge's local analytics record still
run):

| Run 1 | Run 2 | Run 3 | Gateway CPU at the ceiling |
|---|---|---|---|
| **1,545 req/s** (breach at 1,637) | 723 req/s | 815 req/s | 2.4, then 1.0–1.1 cores |

Below the ceiling the gateway is fast. In the first S5m run it added 0.5–0.7 ms
at p50 from 360 to 1,450 req/s (p99 3–13 ms), and in the first full-policy
run 0.6–0.9 ms at p50 up to 1,190 req/s. The ceiling itself is set by the
edge's local database, and it drops once that database has grown: see
[finding 2](#2-the-edges-local-database-sets-the-throughput-ceiling). The
rc10.1 runs followed S0–S2 and a streaming ramp on the same edge rather than a
fresh one, and that build also stored bodies, so they are not directly
comparable with either main set.

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

Paired ABBA blocks from the load generator in Sydney to OpenAI, directly and
through the gateway, 200 pairs per cell after 5 warm-up blocks. The suite was
stopped during the fourth cell (unified `/v1`), so the run is marked INVALID
as a whole and Anthropic was not reached. The three completed cells have no
errors and are reported as measured:

| Cell | Paired overhead, median of within-block differences [95% CI] | Vendor response time (direct, p50) | Gateway's own time p50 |
|---|---|---|---|
| streaming, 64 tokens | TTFT **+5.2 ms** [−9.0, +31.5] | TTFT 722 ms, total 1,141 ms | 0.62 ms |
| non-streaming, 64 tokens | **−11.7 ms** [−30.0, +5.8] | 1,245 ms | 0.72 ms |
| streaming, 512 tokens | TTFT **−8.0 ms** [−18.6, +5.3] | TTFT 802 ms, total 4,009 ms | 1.04 ms |

Every interval contains zero: against a real vendor, the gateway's effect is
smaller than the vendor's own variation between two requests sent seconds
apart. The gateway's own account of its time (0.6–1.0 ms) is the better
measure of what it adds. The absolute times include the round trip from
Sydney to OpenAI's API.

## Behaviour to know about

### 1. rc10.1 edges store request and response bodies

rc10.1 edges store up to `ANALYTICS_MAX_BODY_SIZE` (4 KB) of every request and
response body in their local SQLite database, even when
`ANALYTICS_STORE_REQUESTS` and `ANALYTICS_STORE_RESPONSES` are false (their
default), and keep them for `ANALYTICS_RETENTION_DAYS` (90 by default).

- **Storage**: after 40 minutes of benchmark traffic the edge database was
  3.0 GB plus a 6.9 GB write-ahead log, and 12.9 GB after the soak.
- **Latency**: right after the heaviest load test, database writes on the edge
  took 1.1–2.6 seconds each, and for about 80 seconds requests waited for the
  database before they were sent upstream. The second S1 run measured 2.2 ms
  p50 and 12–21 ms p99 overhead on the native endpoints while the gateway's CPU
  was nearly idle, and the second S4 run reached 316 rather than ~365 req/s.
- **Privacy**: prompts and responses were kept on the edge although the
  settings said they were not.

**Fixed on main** (#612): bodies are stored only when those settings are on.
On main the edge database stayed at about 200 MB over the same kind of load.

### 2. The edge's local database sets the throughput ceiling

On main the non-streaming ceiling is not CPU: the gateway reached it using 1–2.4
of its 4 cores, and simple indexed lookups (the app for the budget check, the
model price, the OAuth token check) were logged taking over a second. What we
found:

- **Every request writes to SQLite several times**, after the response: an
  analytics insert, an update that merges in token counts, and with a priced
  model a budget-usage upsert and update. SQLite has a single writer. The disk
  wrote about 45 KB per request at 1,000–1,200 req/s.
- **Those writes run in one goroutine per request, with no limit, sharing the
  request path's 25 database connections.** Under load, writers waiting for the
  write lock (up to the 5 s busy timeout) hold connections, and the request
  path's reads queue for one.
- **The write-ahead log is starved of checkpoints under constant load** and
  grows without bound: 7.3 GB after 20 minutes on main, beside a 198 MB
  database. It resets when traffic stops, but the file never shrinks, and the
  larger it gets, the slower the edge's reads and checkpoints.

That is why a fresh edge sustains about 1,190 req/s with full policy (1,545
with the minimal configuration) and the same edge a few minutes later about
650–820 in both. Streaming (S4) sends about a third as many requests per
second, so it stays below this limit. Work on a bounded, batched writer with
its own connections and managed checkpoints is under way; the throughput
figures will be re-measured on that build.

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

The throughput runs on another build, and the minimal configuration:

```bash
IMG=$(benchmarks/gateway/cloud/aws.sh build-gateway main | tail -1)
BENCH_GATEWAY_IMAGE=$IMG benchmarks/gateway/cloud/aws.sh deploy      # fresh edge
benchmarks/gateway/cloud/aws.sh seed
benchmarks/gateway/cloud/aws.sh run benchmarks/gateway/scenarios/s5-throughput-ceiling.yaml   # x3

BENCH_GATEWAY_IMAGE=$IMG BENCH_PLUGINS_CONFIG_PATH= BENCH_LOG_LEVEL=error benchmarks/gateway/cloud/aws.sh deploy
benchmarks/gateway/cloud/aws.sh seed -minimal
benchmarks/gateway/cloud/aws.sh run -no-analytics-check benchmarks/gateway/scenarios/s5m-minimal-config.yaml   # x3
```

See [RUNBOOK-cloud.md](RUNBOOK-cloud.md). The report and summary of every run
behind this document are in [`published/v2.2.0-rc10.1/`](published/v2.2.0-rc10.1/)
and [`published/main-0b0af5de/`](published/main-0b0af5de/).
The raw per-request data (about 1 GB) is kept outside the repository. Any
report can be regenerated from it with `gwbench report <run-dir>`.
