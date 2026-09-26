# AI Gateway benchmark results: Tyk AI Studio 2.2

Measured 26 September 2026 on AWS (ap-southeast-2, Sydney). The Enterprise
edge gateway ran on **one 4-vCPU machine** (c7i.xlarge) under full production
policy: app credential authentication, access checks, a monthly budget
enforced on every request, cost accounting, and every request's analytics
record shipped to the control plane. The gateway and AI Studio were built from
the 2.2 release commit (main at `c94e0f17`) and ran with their default
settings. Studio started with an empty database.

The full runbook suite ran once, start to finish: 19 runs and 44.6 million
requests through the gateway, every run VALID under the suite's own checks, no
errors through the gateway, and every analytics record accounted for in
Studio.

## Summary

**On one 4-vCPU node under full production policy, the 2.2 gateway adds about
half a millisecond to a request, sustains about 8,550 short requests per
second or 690 new streams per second (about 3,000 open streams), and holds its
latency through bursts and an hour of sustained load, with no errors.**

### Latency under real policy load

| | One 4-vCPU gateway, every request authenticated, budgeted and recorded |
|---|---|
| Added by the gateway, native endpoints (`/llm/call`, `/llm/rest`, `/llm/stream`) | **0.54–0.58 ms p50, 0.75–0.84 ms p99**, including the extra network hop |
| Of that, the gateway's own processing (its `Server-Timing` header) | **0.13–0.16 ms p50**, 0.18–0.23 ms p99 |
| Added by the OpenAI-compatible endpoints (`/ai/…` shim, unified `/v1`) | 0.77–0.93 ms p50, 1.05–1.25 ms p99 |
| Added at 6,000 short requests per second | **+0.55 ms p50**, +8 ms p99 |
| Added to time to first token at 450 new streams per second (~2,000 open) | **+14 ms p50**; gateway's own p99 0.6 ms |
| Against real OpenAI and Anthropic, from Sydney (200 paired requests per cell) | **not measurable**: paired medians −17 to +26 ms against 0.6–6.5 s vendor responses, with 11 of the 12 distinct intervals containing zero (the other has the gateway arm faster); the gateway's own time was 0.3–1.8 ms |

### Streaming stability

| | Result |
|---|---|
| One hour at 377 streaming req/s (1,348,588 requests, 4.3 s streams) | **0 errors**; time to first token 305–307 ms p50 in every 5-minute window (direct 299–302 ms); gateway's own p99 0.52–0.55 ms throughout; memory flat (561 → 617 MB, max 654 MB) |
| 3× burst (190 → 570 new streams/s for 30 s, then back) | **0 errors**; gateway's own p99 peaked at 1.5 ms; time to first token rose ~30 ms during the burst and was back to baseline in the next 5 seconds |
| Streaming ramps to saturation, three runs | **0 errors** in 1.76 million streams, carried on past the capacity point until the ramp was stopped |
| Analytics delivered to the control plane | **all 44,657,389** records (every gateway request, warm-ups included), in every run |

### Capacity per 4-vCPU node

| | Sustained, median of three runs (range) | Limited by |
|---|---|---|
| Short non-streaming requests, full policy, 20 ms upstream | **8,551 req/s** (8,546–8,570) | CPU: 3.5 of 4 cores |
| Streaming (300 ms to first token, 4.3 s streams) | **687 req/s** (593–729), about 3,000 open streams | CPU: all 4 cores |
| Short requests with what configuration can switch off ("competitor conditions") | **over 9,000 req/s** (one run, pre-release build) | not reached: the ramp's top is ~9,000 |

"Sustained" is the highest load step at which errors stayed under 0.1% and the
added p99 latency stayed within 25 ms (see [Method](#method)).

What this means in practice:

- **Latency is not the constraint.** A chat user waits hundreds of
  milliseconds to seconds for a model's first token. The gateway adds about
  half a millisecond, and against a real vendor its effect is smaller than the
  vendor's own variation from one request to the next.
- **Capacity is CPU, and it scales out.** For both kinds of traffic the limit
  is the node's CPU, not the gateway's local database, locks or the control
  plane. Each edge serves from its own copy of the configuration, so add
  nodes behind a load balancer for more.
- **Streams degrade gradually, not suddenly.** Time to first token through the
  gateway tracks the upstream's to about 450 new streams per second, then
  rises as the node's CPU fills (+25 ms at 550, +75 ms at 690). Nothing
  failed at any load the ramps reached. Plan streaming nodes at about 400 new
  streams per second, a little above the rate the one-hour soak held without
  drift.
- **The control plane keeps up.** Studio ingested every record, at up to 8,600
  records a second, on a 4-vCPU machine with a load average under 0.8, and
  its periodic database work no longer grows with the number of rows stored.

## Changes since the first measurement

The first run of this suite, on v2.2.0-rc10.1 the day before, found the
throughput limit in the edge's local SQLite database rather than CPU. The 2.2
release fixes what that run and the investigation after it found:

| | v2.2.0-rc10.1 | 2.2 |
|---|---|---|
| Short requests, full policy | ~1,000 req/s; 640–730 once the local database had grown (main, 25 Sept) | **8,551 req/s**, the same in all three runs |
| Short requests, competitor conditions | ~1,545 req/s on a fresh edge, then 720–820 | over 9,000 req/s |
| Streaming | ~365 req/s | **687 req/s** |
| Gateway's own processing, p50 | 0.30 ms | 0.14 ms |
| Edge write-ahead log under load | grew without bound (6.9–7.3 GB) | capped at 64 MB (4.6 MB after the suite) |
| Request and response bodies | stored on the edge against its settings | stored only when enabled |
| Hub budget sync on a large database | summed every App's period every 30 s (53 s per pass at 80M rows) | reads only new rows |

What changed in the gateway and Studio:

- **Edge database** (#612, #617, #619, #621, #628): bodies stored only when
  `ANALYTICS_STORE_REQUESTS/RESPONSES` are on; one analytics row per request,
  written in batches by a single writer on its own connection, so request-path
  reads never wait for writes; budget usage kept in memory and flushed with
  those batches; per-request reads (the App, the model price) cached;
  analytics retention enforced (7 days by default when the analytics pulse
  ships every row to Studio); the write-ahead log capped at 64 MB; SQLite
  connections never recycled.
- **Token validation** (#624): requests that miss the edge's token cache
  together share one call to Studio; a busy token is revalidated in the
  background before it expires; calls to Studio time out after 5 s. Before
  this, every request in flight missed at once when a busy token's cache
  entry expired, and the edge stalled for about a second every five minutes.
- **Garbage collector** (#625, #628): `GOGC` adapted to the live heap, from
  400 while the heap is small down to Go's default of 100 as it grows, and a
  soft `GOMEMLIMIT` at 90% of the container's memory limit. With Go's default
  the collector ran about 20 times a second under load and stalled requests
  in mark assist.
- **Overload protection** (#622): past a memory threshold the edge refuses new
  requests with 503 and `Retry-After` instead of being killed for memory.
- **Upstream failures** (#629): a dropped or refused upstream connection is
  answered with 502, or 504 on a timeout, instead of 500.
- **Control plane** (#626, #630): Studio's budget sync and usage telemetry
  read only the rows added since their last pass, instead of summing the
  analytics tables in full.

The rc10.1 figures, and the runs behind them, are kept in
[`published/v2.2.0-rc10.1/`](published/v2.2.0-rc10.1/) and
[`published/main-0b0af5de/`](published/main-0b0af5de/).

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
| S5 | How many short requests can one node sustain? | Poisson arrivals, +100 req/s per 30 s, to 10,000 | mock: 20 ms |
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
  In all three S0 runs the tool measured differences of at most 0.05 ms at p50
  and p99, so differences larger than that are real.
- **Open-loop load with coordinated-omission correction.** S4–S7 release
  requests on a Poisson schedule and measure latency from each request's
  intended send time. A slow gateway is charged for the queue it causes. A run
  is marked INVALID if the load generator's own release lag exceeds its limit.
- **Two independent measures.** The gateway reports its own time on every
  request (`Server-Timing`, enabled with `GATEWAY_SERVER_TIMING=true`). The
  black-box difference and the gateway's own account agree to within the
  network hop (about 0.4 ms here).
- **Nothing lost silently.** After every run, the analytics records Studio
  received are compared with the requests the gateway answered.
- **Capacity SLO.** A load step counts as sustained when errors are under 0.1%
  and the p99 overhead is within 25 ms, judged by the gateway's own p99 and by
  the black-box p99 difference. The black-box check counts a breach only when
  the 95% confidence interval of the difference lies wholly above 25 ms: with a
  realistic upstream (p99 900 ms) and a 10% baseline sample, p99 differences
  move by 50–100 ms from sampling alone.

### Environment

Four dedicated VMs in one AWS cluster placement group (ap-southeast-2a), Ubuntu
24.04, Docker with host networking (no bridge NAT in the path). Built and torn
down with [`cloud/aws.sh`](cloud/aws.sh).

| VM | Instance | Runs | Why this size |
|---|---|---|---|
| gateway | c7i.xlarge: 4 vCPU, 8 GiB | Microgateway (Enterprise), edge mode | The machine under test. 4 vCPU is a common production size for one edge node, and small enough for one load generator to reach its limit. Capacity scales by adding nodes, so a per-node figure on a modest node is what sizing needs. The gateway has the whole VM, with no CPU or memory limit. |
| loadgen | c7i.2xlarge: 8 vCPU | `gwbench` | Twice the gateway's CPU, so the load generator never becomes the bottleneck (it fell behind at ~300 streaming req/s with 4 CPUs in local tests). |
| mock | c7i.2xlarge: 8 vCPU | `mockllm` | Serves the gateway's traffic plus the direct arm, including ~3,000 open streams in S4, without slowing down; if it slowed, both arms would slow and hide the gateway's limit. |
| hub | m7i.xlarge: 4 vCPU, 16 GiB | AI Studio (Enterprise) + Postgres 16 | Off the request path, but ingests every analytics record, which the completeness check verifies. |

- **Production configuration**: Enterprise edition, hub-spoke with
  configuration from Studio over gRPC, analytics shipped to Studio by the
  analytics pulse, a budget on the App and a priced model, Bearer app
  credentials, the edge token cache at its default, `LOG_LEVEL=info`, and no
  tuning settings (`GOGC`, `GOMEMLIMIT` and the overload limits at their
  defaults).
- **Network**: loadgen → gateway round trip 0.40 ms average, 0.61 ms p99. It
  is included in every overhead figure.
- **Versions**: Microgateway and AI Studio built from main at `c94e0f17` the
  way the release builds them (CGO, `-tags=enterprise`, Debian glibc), with
  `aws.sh build-gateway` and `aws.sh build-studio`, inside the published
  v2.2.0-rc10.1 images. The load generator and mock were built from the same
  commit.
- **State**: Studio started with an empty database and the edge with an empty
  local database. The suite then ran in one sequence on the same edge without
  restarts, so later runs saw a database holding every earlier run's rows
  (15.7 GB on the edge by the end of S6).
- **Repeats**: S0–S2 and S4–S5 ran three times each, as the runbook requires.
  Tables give the median of the three runs and the range. S6 and S7 were sized
  from the first S4 run (593 req/s) and ran once.

## Results in detail

### S1: overhead per endpoint, unloaded

One request at a time, 5,000 requests per arm and endpoint. Milliseconds added
by the gateway, median of three runs (range in brackets).

| Endpoint | p50 | p90 | p99 | Gateway's own p50 / p99 (Server-Timing) |
|---|---|---|---|---|
| `/llm/rest` (non-streaming) | 0.56 (0.56–0.58) | 0.69 | 0.81 (0.80–0.82) | 0.13 / 0.18 |
| `/llm/stream` | 0.54 (0.53–0.55) | 0.66 | 0.76 (0.75–0.77) | 0.14 / 0.21 |
| `/llm/call`, OpenAI format, non-streaming | 0.58 (0.58–0.59) | 0.70 | 0.84 (0.80–0.84) | 0.14 / 0.19 |
| `/llm/call`, OpenAI format, streaming | 0.56 (0.54–0.56) | 0.66 | 0.75 (0.75–0.76) | 0.16 / 0.22 |
| `/llm/call`, Anthropic format, streaming | 0.55 (0.54–0.56) | 0.66 | 0.77 (0.75–0.78) | 0.16 / 0.22 |
| `/ai/…` OpenAI shim, non-streaming | 0.90 (0.89–0.91) | 1.05 | 1.25 (1.23–1.25) | 0.49 / 0.81 |
| `/ai/…` OpenAI shim, streaming | 0.77 (0.73–0.78) | 0.91 | 1.05 (1.02–1.08) | 0.62 / 0.97 |
| unified `/v1`, non-streaming | 0.91 (0.90–0.93) | 1.07 | 1.22 (1.20–1.26) | 0.51 / 0.83 |
| unified `/v1`, streaming | 0.78 (0.78–0.81) | 0.93 | 1.07 (1.02–1.07) | 0.66 / 1.00 |

Direct requests to the mock took 0.63–0.68 ms p50. Of the ~0.55 ms the gateway
adds on the native endpoints, about 0.4 ms is the extra network hop and
0.13–0.16 ms is the gateway's own work: authentication, access and budget
checks, cost accounting and queueing the analytics record. The
OpenAI-compatible endpoints cost about 0.3 ms more because they translate the
request and pass it through the gateway a second time over loopback.

The three runs agree to within 0.05 ms at p99 on the native endpoints, although
the second and third ran after the heaviest load tests, with the edge's
database holding tens of millions of rows.

### S2: prompt size and stream length

Overhead in milliseconds, median of three runs, `/llm/call` OpenAI format.

| Request | p50 | p99 | Gateway's own p50 |
|---|---|---|---|
| 1 KB prompt | 0.61 | 0.84 | 0.14 |
| 32 KB prompt | 1.31 | 1.60 | 0.76 |
| 256 KB prompt | 5.44 | 8.75 | 4.73 |
| 1,000-chunk stream (time to last chunk) | 0.64 | 0.89 | 0.17 |

Cost grows with the prompt the gateway has to read and inspect: about 0.02 ms
per KB. Relaying a long stream adds almost nothing per chunk.

### S4: streaming capacity

Load rose by 50 req/s every minute until the gateway broke the SLO and the
ramp's stop condition ended it. The upstream behaves like a real model: 300 ms
median time to first token with a tail to 900 ms, then 200 tokens at
50 tokens/s.

| Run | Sustained | First breach | Gateway CPU at the breach | Memory (RSS) max | Errors |
|---|---|---|---|---|---|
| 1 | **593 req/s** | 634 req/s | all 4 cores | 1.36 GB | 0 |
| 2 | **729 req/s** | 771 req/s | all 4 cores | 1.32 GB | 0 |
| 3 | **687 req/s** | 737 req/s | all 4 cores | 1.45 GB | 0 |

Median **687 req/s**: about 3,000 streams open at once (687 req/s × 4.3 s per
stream). Every run carried on past its breach to about 900 req/s before the
stop condition ended it, with no errors, and every request reached Studio's
analytics.

Time to first token (TTFT) through the gateway in run 3 (the median run),
against about 300 ms direct:

| Load (req/s through the gateway) | 90 | 182 | 273 | 367 | 455 | 550 | 633 | 687 | 736 | 811 |
|---|---|---|---|---|---|---|---|---|---|---|
| TTFT p50 through the gateway (ms) | 306 | 302 | 303 | 308 | 314 | 327 | 348 | 376 | 405 | 448 |
| Gateway's own p99 (ms) | 0.4 | 0.4 | 0.5 | 0.6 | 0.6 | 0.9 | 0.9 | 0.7 | 0.7 | 1.8 |

The gateway's own time, from receiving a request to sending it upstream, stays
under 2 ms at every load. What grows past ~450 req/s is the time to relay the
first chunk back while the node's four cores serve 2,000–3,000 open streams:
the limit is CPU. The three runs differ mostly in where the p99 test, which is
noisy with a realistic upstream, first counts a breach on a curve that rises
gradually. At the same load their p50 figures agree to within a few
milliseconds.

### S5: non-streaming capacity

Short requests (1 KB prompt, 16 tokens, 20 ms upstream), +100 req/s every
30 s up to 10,000 req/s offered. Full policy, three runs one after the other on
the same edge:

| Run | Sustained | First breach | Gateway CPU max | Memory (RSS) max | Requests | Errors |
|---|---|---|---|---|---|---|
| 1 | **8,551 req/s** | 8,632 req/s | 3.53 cores | 856 MB | 13,772,785 | 0 |
| 2 | **8,570 req/s** | 8,655 req/s | 3.55 cores | 724 MB | 13,772,440 | 0 |
| 3 | **8,546 req/s** | 8,637 req/s | 3.46 cores | 762 MB | 13,777,960 | 0 |

Median **8,551 req/s**, and the three runs agree to within 0.3%: the edge's
database, which held about 30 million rows by run 3, no longer affects the
result. Added latency by load, median of the three runs:

| Load (req/s) | 1,000 | 2,000 | 4,000 | 6,000 | 7,000 | 8,000 | 8,500 |
|---|---|---|---|---|---|---|---|
| Added p50 (ms) | 0.48 | 0.41 | 0.43 | 0.55 | 0.67 | 0.88 | 1.06 |
| p99 through the gateway / direct (ms) | 23 / 22 | 23 / 22 | 26 / 22 | 31 / 23 | 37 / 23 | 43 / 24 | 44 / 25 |
| Gateway's own p99 (ms) | 0.3 | 0.8 | 0.9 | 1.2 | 1.5 | 2.2 | 3.0 |

The added p50 stays near half a millisecond up to the ceiling. The p99 grows as
the CPU fills, and crosses the SLO's 25 ms at about 8,600 req/s. Studio
received every one of the 41.3 million analytics records.

### S5m: competitor conditions

Published gateway throughput figures are usually measured with an instant
upstream and no authentication, logging or accounting. S5m removes what
configuration can remove: no budget, no model price, no analytics shipping,
`LOG_LEVEL=error`, and an instant upstream. Authentication and the edge's local
analytics record still run.

S5m was not repeated on the release build. On the pre-release build
`b385573c` (the same edge changes except the last two, #628 and #629, and
`GOGC` fixed at 400), one S5m run reached **9,023 req/s with no SLO breach**:
the ramp's top of 10,000 req/s offered (about 9,000 through the gateway) was
the limit, at 3.15 cores and 267 MB. The same build's full-policy S5 run
sustained 8,553 req/s, like the release build. So full policy costs little
throughput, and a like-for-like figure is above 9,000 req/s per 4-vCPU node;
this ramp cannot show how far above.

### S6: burst

A base load of about 190 req/s (a third of the first S4 run's capacity), a
30-second burst to about 570 req/s, then back to base.

| Phase | Load | TTFT p50 (direct ~300 ms) | Gateway's own p99 |
|---|---|---|---|
| Base | 175–200 req/s | 300–306 ms | 0.4 ms |
| Burst | 552–582 req/s | 311–334 ms | 0.7–1.5 ms |
| After the burst | 179–200 req/s | 293–311 ms | 0.4–0.6 ms |

No errors, and all 43,340 requests reached Studio. The first 5-second window
after the burst was already back at base latency, with no backlog to clear.
The burst peaked just below the first S4 run's breach, so this tests recovery
at the edge of capacity, not overload.

### S7: one-hour soak

A constant 415 req/s offered (70% of the first S4 run's 593 req/s), of which
377 req/s went through the gateway and the rest to the direct arm, for 60
minutes: 1,348,588 streams through the gateway.

| | Start | End | Range over the hour |
|---|---|---|---|
| TTFT p50 per 5-minute window (direct 299–302 ms) | 307 ms | 307 ms | 305–307 ms |
| Gateway's own p99 | 0.53 ms | 0.53 ms | 0.52–0.55 ms |
| Resident memory | 561 MB | 617 MB | max 654 MB, slope −0.7 MB/hour |
| Goroutines | 1,049 | 3,443 | max 10,809, slope −15/hour |
| Gateway CPU | | | 2.2 cores average, 2.6 max |
| Errors | 0 | 0 | |
| Analytics received by Studio | | 1,359,968 of 1,359,968 | |

No latency drift, no memory growth and no goroutine growth. The added time to
first token (+6 ms p50 over the direct arm) is the same in the first and last
windows.

### S3: real vendors

Paired ABBA blocks from the load generator in Sydney to OpenAI and Anthropic,
directly and through the gateway: 200 pairs per cell after 5 warm-up blocks.
The paired overhead is the median of the within-block differences, with its
95% confidence interval.

| Cell | Paired overhead: TTFT | Paired overhead: total | Vendor response (direct, p50) | Gateway's own time p50 |
|---|---|---|---|---|
| OpenAI, streaming, 64 tokens | −10.5 ms [−21.6, +4.7] | −12.9 ms [−30.5, +6.3] | TTFT 653 ms, total 1,070 ms | 0.28 ms |
| OpenAI, non-streaming, 64 tokens | −5.4 ms [−20.1, +19.1] | same | 1,085 ms | 0.47 ms |
| OpenAI, streaming, 512 tokens | −9.8 ms [−21.8, +1.9] | −16.8 ms [−69.2, +48.3] | TTFT 619 ms, total 3,634 ms | 1.02 ms |
| OpenAI through unified `/v1`, streaming, up to 1,024 tokens | +25.7 ms [−113.2, +107.5] | +26.3 ms [−44.9, +105.9] | TTFT 2,468 ms, total 6,518 ms | 1.76 ms |
| Anthropic, streaming, 64 tokens | +11.1 ms [−10.8, +29.4] | +14.1 ms [−3.7, +25.5] | TTFT 867 ms, total 1,673 ms | 0.76 ms |
| Anthropic, non-streaming, 64 tokens | +11.0 ms [−15.4, +26.1] | same | 1,691 ms | 0.55 ms |
| Anthropic, streaming, 512 tokens | +9.3 ms [−10.1, +22.5] | −77.3 ms [−123.1, −24.9] | TTFT 860 ms, total 4,220 ms | 0.98 ms |

Eleven of the twelve distinct intervals contain zero (for non-streaming
requests TTFT and total are the same measurement). The other says the gateway
arm finished a 4-second Anthropic stream 77 ms *sooner*, which the gateway
cannot cause: over a long generation the vendor's own variation outweighs the
pairing. Against a real vendor, the gateway's effect is smaller than the
vendor's variation between two requests sent seconds apart, and the gateway's
own account of its time (0.3–1.8 ms) is the better measure of what it adds.
The absolute times include the round trip from Sydney to each vendor's API.

There were no errors through the gateway in 2,800 requests. One direct
request to OpenAI failed on the vendor's side (the stream ended without
content).

### The control plane

Studio ran on a 4-vCPU m7i.xlarge with Postgres on the same machine. Over the
suite it ingested 44.6 million analytics records, at up to about 8,600 a
second during S5, with a 1-minute load average between 0.1 and 0.8 in every
5-minute sample, from an empty database to 31 GB: about 700 bytes per request
across `llm_chat_records` and `proxy_logs`, with bodies off. The budget sync
and usage telemetry read only rows added since their last pass. Before that
fix, one budget sync pass took 53 s on an 80-million-row database and kept
Postgres busy continuously.

## Tuning and behaviour to know about

### Garbage collection: the default favours memory

With `GOGC` unset, the gateway adapts it every second so that the heap goal is
about the live heap plus 256 MB, between 400 and Go's default of 100 (the
`microgateway_gogc` metric shows the value in effect). A small live heap, as
with non-streaming traffic, gets 400 and collects rarely. Thousands of open
streams raise the live heap, and `GOGC` steps down towards 100 to keep memory
in check: in S4 it reached 100 at about 400 req/s.

Setting `GOGC=400` yourself trades memory for streaming capacity. The same
suite on the pre-release build, whose default was a fixed 400:

| | Adaptive (2.2 default) | `GOGC=400` fixed |
|---|---|---|
| S4 streaming, sustained | 687 req/s (593–729, three runs) | 749 req/s (728 and 769, two runs) |
| S4 memory (RSS) max | 1.32–1.45 GB | 3.41–3.47 GB |
| S5 non-streaming, sustained | 8,551 req/s | 8,553 req/s (one run) |
| Errors | 0 | 7 in the S4 runs, past the breach: upstream connections dropped (EOF), answered with 500 by that build and with 502 since #629 |

For streaming-heavy edges with memory to spare, `GOGC=400` gives about 9% more
streaming capacity for about 2.4 times the memory. Keep the default where
memory is tight. A `GOGC` you set is used as is, and `GOMEMLIMIT` still caps
the heap.

### Memory limits and overload

The benchmark gateway had no container memory limit, so neither the soft
`GOMEMLIMIT` nor memory-based shedding was active. In a container with a
memory limit, the gateway sets `GOMEMLIMIT` to 90% of it, and above 85%
(`OVERLOAD_MEMORY_THRESHOLD`) refuses new requests with 503 and
`Retry-After: 1` until memory falls, instead of being killed. Give streaming
edges a limit well above the 1.45 GB the S4 runs reached at saturation, so
that CPU, not the memory threshold, is what limits them.
`MAX_INFLIGHT_REQUESTS` adds an optional cap on requests in progress. It is off
by default, because long streams make any single default wrong for some
deployments.

### Edge disk use

The edge keeps one analytics row per request in its local database, about
360 bytes each here (15.7 GB after about 43 million requests), for
`ANALYTICS_RETENTION_DAYS`: by default 7 days on an edge whose analytics pulse
ships every row to Studio, and 90 days otherwise. At a steady 1,000 req/s that
is about 31 GB a day. The rows back the edge's own analytics API; Studio's
dashboards and budgets use the copies shipped to Studio. Size the volume for
your retention, or set `ANALYTICS_RETENTION_DAYS=1` on busy edges. The
write-ahead log stays at or below 64 MB.

### Other behaviour

- **Non-streaming responses are buffered.** For REST requests, time to first
  byte equals total time, because a response hook is always registered.
  Streaming is not affected.
- **The OpenAI-compatible endpoints make a second pass** through
  authentication and policy over loopback, which accounts for their extra
  ~0.3 ms.
- **The analytics pulse sends early under load.** The benchmark's pulse file
  (every 10 s, with the default `max_buffer_size` of 10,000) held about
  9 seconds of traffic at the soak's rate, since each request buffers two to
  three records. Busy edges therefore send when the buffer fills and log
  `Analytics buffer is full` at WARN each time (12,929 times in this suite).
  Nothing is lost while Studio is reachable, but during a Studio outage the
  buffer holds only those seconds of traffic. Raise `max_buffer_size` on busy
  edges.
- **Network hop.** Every overhead figure includes one extra hop from the load
  generator to the gateway in the same availability zone (about 0.4 ms here).
  A real deployment adds its own client-to-gateway round trip.

## Reproducing these results

```bash
export BENCH_NAME=gwbench BENCH_REF=v2.2.0-rc10.1 BENCH_TOOLS_REF=c94e0f17
benchmarks/gateway/cloud/aws.sh up
IMG=$(benchmarks/gateway/cloud/aws.sh build-gateway c94e0f17 | tail -1)
benchmarks/gateway/cloud/aws.sh build-studio c94e0f17      # pinned for later deploys
BENCH_FRESH_HUB=1 BENCH_GATEWAY_IMAGE=$IMG benchmarks/gateway/cloud/aws.sh deploy
benchmarks/gateway/cloud/aws.sh seed -vendors
benchmarks/gateway/cloud/aws.sh suite REPEATS=3 VENDORS=1 SOAK=1
```

`build-gateway` and `build-studio` put binaries built from a commit into the
published images; once the 2.2 images are published, `BENCH_REF` set to the
2.2 tag runs them directly, without those steps. The S5m variant needs its own
deploy and seed:

```bash
BENCH_GATEWAY_IMAGE=$IMG BENCH_PLUGINS_CONFIG_PATH= BENCH_LOG_LEVEL=error benchmarks/gateway/cloud/aws.sh deploy
benchmarks/gateway/cloud/aws.sh seed -minimal
benchmarks/gateway/cloud/aws.sh run -no-analytics-check benchmarks/gateway/scenarios/s5m-minimal-config.yaml
benchmarks/gateway/cloud/aws.sh down    # deletes only resources tagged with this BENCH_NAME
```

See [RUNBOOK-cloud.md](RUNBOOK-cloud.md) for the full procedure.

The report, summary and manifest of every run behind this document are in
[`published/v2.2/`](published/v2.2/), and the comparison runs cited above
(fixed `GOGC=400`, S5m) in [`published/v2.2-comparison/`](published/v2.2-comparison/).
The raw per-request data (several GB per S5 run) is kept outside the
repository; any report can be regenerated from it with `gwbench report <run-dir>`.
