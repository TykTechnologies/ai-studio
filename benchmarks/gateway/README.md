# Gateway latency benchmarks

This suite measures how much latency the AI Gateway adds for an end user, and
whether the gateway keeps that latency flat under sustained and bursty load. It
targets the production data plane: an **edge microgateway** in hub-spoke mode,
configured by Studio, with clients authenticating with an app's Bearer token.

It answers four questions:

| Question | Scenario |
|---|---|
| How much time does the gateway itself add to a request? | S1 overhead floor, S2 payload size |
| What does an end user see against a real vendor? | S3 OpenAI and Anthropic |
| How much load can one gateway take before latency climbs? | S4 capacity ramp, S5 request-rate ceiling |
| Does it degrade under bursts or over time? | S6 spike, S7 one-hour soak |

S0 (A/A calibration) runs first on every host and sets the noise floor. It checks
the measurement setup, not the gateway.

## How the numbers are kept honest

- **Always a baseline.** Every gateway request has a matching request sent from
  the same load generator straight to the same upstream. Overhead is the
  difference between the two, never an absolute number.
- **A/A calibration.** S0 sends identical traffic down both arms. If the tool
  "finds" a difference there, every result from that setup is suspect, and the
  report marks the run INVALID.
- **Open-loop load with coordinated-omission correction.** In the stress
  scenarios, requests are released on a fixed (Poisson) schedule, not when the
  previous one returns. Latency is measured from each request's *intended* send
  time. A slow gateway is charged for the queue it causes. Closed-loop tools such
  as `hey` or `ab` hide that queue.
- **The load generator is checked too.** Every request records how late it was
  released. If the p99 release lag passes 1ms, the load generator, not the
  gateway, is shaping the results, and the run is INVALID.
- **Paired sampling against real vendors.** Vendor latency swings by hundreds of
  milliseconds between requests, far more than the gateway adds. S3 sends ABBA
  blocks (direct, gateway, gateway, direct, order randomised). The overhead is
  the median of the within-block differences, with a bootstrap confidence
  interval.
- **Two independent measurements.** The gateway's opt-in `Server-Timing`
  header reports its own time, split from upstream time, on every request. The
  report puts it next to the black-box difference and flags any disagreement.
  The gap between the two is the network hop to the gateway.
- **Nothing silently lost.** After each run, the number of proxy-log rows Studio
  received is compared with the number of requests the gateway answered.
- **Everything recorded.** Each run directory holds every request's raw
  measurements (`results.jsonl`), the gateway's resource use once a second
  (`samples.jsonl`), and a manifest with the scenario, git SHA, load-generator
  host and targets. Reports are regenerated from the raw data with
  `gwbench report`, so anyone can re-check the analysis.
- **Production settings.** Production images (not hot-reload dev builds), the
  Enterprise edition, analytics on and shipped to Studio, a budget on the app,
  the edge token cache at its default, `LOG_LEVEL=info`, and explicit CPU and
  memory limits.

## Running it locally

Prerequisites:
- Docker with at least 8 CPUs and 16GB available to it.
- An Enterprise licence in a file with `TYK_AI_LICENSE=...`. It defaults to
  `dev/.env.secrets`; set `BENCH_ENV_FILE` to use another file.
- For S3 only: `test-secrets/vendors.env` from the vendor conformance suite.
  Set `BENCH_VENDORS_DIR` if it lives elsewhere.

```bash
make bench-up          # build production images and start Studio, edge, mock
make bench-seed        # first admin, LLMs, prices, app + credential, push to edge
make bench-smoke       # 5-minute end-to-end check; NOT publishable

make bench-overhead    # S0 + S1 + S2        (~25 min)
make bench-stress      # S4 + S5             (up to ~1h; stops at the knee)
make bench-spike  BENCH_RATE_SCALE=<S4 sustained rps / 200>
make bench-soak   BENCH_RATE_SCALE=<S4 sustained rps / 200>   (1h)

make bench-seed-vendors && make bench-real   # S3 against OpenAI + Anthropic (~1-2h, costs tokens)

make bench-down        # stop and delete all benchmark data
```

Each run writes `benchmarks/gateway/results/<timestamp>-<scenario>/` with
`report.html`, `report.md`, `summary.json` and the raw data. Start with the
**Validity checks** and **Overhead summary** sections.

### Running from a git worktree

A fresh worktree has neither `dev/.env.secrets` nor `test-secrets/`, which are
gitignored. Point the targets at the main checkout's copies:

```bash
export BENCH_ENV_FILE=/path/to/main-checkout/dev/.env.secrets
export BENCH_VENDORS_DIR=/path/to/main-checkout/test-secrets
make bench-up
```

The Go build also needs the enterprise submodule (`git submodule update --init
enterprise`). The images are built from the checkout that runs `make bench-up`.

### Coming back to it later

- **The stack is still running** (`docker ps --filter name=gwbench`): run
  scenarios straight away. The seed state is in `.state/state.json`.
- **The stack was stopped with `make bench-down`, or the machine restarted:**
  `make bench-up` then `make bench-seed`. Data is not kept between `bench-down`
  and `bench-up`: every start is a fresh Studio and edge. That is deliberate,
  so earlier runs cannot affect a new one.
- **The code under test changed:** `make bench-up` rebuilds the images from the
  current checkout, then run `make bench-seed`.

### Comparing two gateway builds

To measure a change to the gateway, run the same scenarios against a build
from each branch on the same stack:

```bash
make bench-build-gateway SRC=/path/to/main-checkout TAG=before
make bench-build-gateway SRC=/path/to/feature-worktree TAG=after

make bench-swap TAG=before     # recreate the edge from that image, re-seed
GWBENCH_LABEL="before: main @ <sha>" make bench-smoke
make bench-swap TAG=after
GWBENCH_LABEL="after: <branch> @ <sha>" make bench-smoke
```

`bench-swap` re-seeds because the edge's database lives in its container. The
git SHA in each manifest is the load generator's checkout, not the swapped
gateway's, so record the gateway build in `GWBENCH_LABEL`. A build without this
suite's `GATEWAY_SERVER_TIMING` support still runs every scenario. Its reports
have no Server-Timing columns, and capacity is then judged against the
baseline alone.

**Local numbers are for development, not for customers.** On one laptop the
gateway, the mock and the load generator compete for the same CPUs, and Docker
Desktop adds a virtualised network. Use [RUNBOOK-cloud.md](RUNBOOK-cloud.md) for
publishable numbers.

## Scenarios

| | Load | Upstream | Endpoints | Result |
|---|---|---|---|---|
| S0 | 1 request at a time | mock, instant | direct vs direct | noise floor; must be ~0 |
| S1 | 1 request at a time, 5000 per arm | mock, instant | `/llm/rest`, `/llm/stream`, `/llm/call` (OpenAI + Anthropic), `/ai/…` shim, unified `/v1` | overhead per endpoint, p50/p90/p99 |
| S2 | 1 request at a time | mock, instant / 1000-chunk stream | `/llm/call` | overhead vs prompt size (1KB–256KB) and stream length |
| S3 | ABBA pairs, 2 workers | OpenAI, Anthropic | `/llm/call`, unified `/v1` | end-user overhead on TTFT and total time |
| S4 | Poisson, +50 req/s per minute | mock, 300ms TTFT, 50 tok/s, 200 tokens | `/llm/call` streaming | highest sustainable rate (p99 overhead ≤ 25ms, errors ≤ 0.1%) |
| S5 | Poisson, +100 req/s per 30s | mock, 20ms | `/llm/call` REST | request-rate ceiling |
| S6 | base → 3× for 30s → base | mock, realistic | `/llm/call` streaming | burst absorption and recovery |
| S7 | 1h constant at ~70% of S4 | mock, realistic | `/llm/call` streaming | latency drift, memory and goroutine growth, analytics loss |

Scenario files live in `scenarios/`. They are plain YAML; `{mock:<profile>}`
expands to the mock upstream's profile path and `{model:<vendor>}` to the seeded
model.

### Feature-cost variants (S8)

Run S1 again against a gateway configured differently, and label the run:

```bash
EDGE_TOKEN_CACHE_ENABLED=false make bench-up   # cold credential path on every request
GWBENCH_LABEL="token cache off" make bench-overhead
```

Use the same pattern for `ENABLE_TRACING=true`, or for an LLM with a filter or
guardrail attached (add it in Studio after seeding, then push config). Compare
the reports with the default run.

## Reading a report

- **TTFT** is the time to the first content token, which is what a chat user
  waits for. For non-streaming requests it is the complete response. **Total**
  is the time to the last byte.
- **Overhead** is the gateway arm's percentile minus the baseline arm's. The
  brackets give the 95% bootstrap confidence interval. At p99 the interval is
  wide unless the run is long, so quote p99 only with its interval.
- **Server-Timing** comes from the gateway itself:
  - `gw-pre`: time before the upstream request is sent.
  - `gw`: all gateway time outside the upstream call.
  - `gw-ttfb`: the gateway's share of time to the first byte.
  - **New upstream conns**: how often the gateway had to open a new TCP/TLS
    connection to the vendor.
- **Capacity** (S4/S5) is the last load step that met the SLO. Quote it with
  the gateway's CPU and memory allocation, which appear in the manifest and in
  the compose file.

## What the numbers do and do not include

- They include everything between the client and the vendor that the gateway
  adds: the extra network hop, TLS termination (if you run TLS), authentication,
  budget and policy checks, request inspection, and the relay of every streamed
  chunk.
- Analytics, cost and budget accounting run after the response is sent, so they
  cost CPU, not latency. S4 and S7 capture their effect under load, and the
  analytics check shows whether any record was dropped.
- The mock numbers exclude vendor variability on purpose. S3 is the only
  scenario whose absolute latencies mean anything to an end user; everywhere
  else, read the overhead.

## Behaviour to know about when reading results

These were found while building the suite. They are measured as they are, not
worked around:

- **Buffered REST responses.** Microgateway non-streaming responses are fully
  buffered, because a response hook is always registered. For REST, time to
  first byte equals total time. Streaming is not affected.
- **Second hop on OpenAI-compatible endpoints.** The `/ai/…` shim and the
  unified `/v1` endpoint translate to OpenAI format and call back into the
  gateway's own `/llm/call/` route over loopback. That is two passes through
  authentication and policy. The shim also streams from the upstream even when
  the client asked for a non-streaming response.
- **`reasoning_effort` on the unified endpoint.** The OpenAI-compatible
  translation (`/v1`, `/ai/…`) does not forward `reasoning_effort`, so a
  reasoning model reasons at its default effort. A request that returns text
  via `/llm/call` can return empty content through `/v1` when the token budget
  is spent on reasoning. S3's unified cell omits the parameter in both arms so
  they stay comparable.
- **Vendor defaults change.** Current Claude models think by default, and both
  vendors' newest models reject `temperature`. The S3 bodies disable thinking
  and send no sampling parameters. Check them when the configured models change.

### Found by this suite and fixed

The first runs found these; #592 fixed them. Builds from before that PR still
show them, which is worth knowing when comparing old builds:

- **SQLite lock contention on the edge.** The edge's database used SQLite's
  shared-cache mode, and every request ran about 30 configuration queries.
  From about 50 streaming requests/s, lookups failed with "database table is
  locked". The Enterprise budget check turned that into **403 "Budget limit
  exceeded"** (budget $1M), and about 15% of analytics never reached Studio.
  After the fix, on the same stack: unloaded overhead fell from ~3ms to
  ~0.3ms p50, the ramp ran without errors, and 58,727 of 58,727 requests
  reached Studio.
- **Upstream keep-alive pool.** The gateway kept at most 2 idle connections per
  upstream host, so with more requests in flight it opened new TCP+TLS
  connections. The **New upstream conns** column shows how often.

## Code

| Path | What |
|---|---|
| `cmd/gwbench` | load generator, seeder, report generator |
| `cmd/mockllm` | mock upstream (`pkg/testinfra/mockllm.Server`) |
| `internal/sched` | open-loop, closed-loop and paired schedulers |
| `internal/probe` | one timed request: TTFB, TTFT, total, Server-Timing |
| `internal/stats` | percentiles, bootstrap confidence intervals |
| `internal/report` | analysis, validity checks, Markdown/HTML output |
| `internal/seed` | headless Studio setup over the admin API |
| `compose/` | benchmark stack |
| `proxy/server_timing.go` | the gateway's opt-in `Server-Timing` (`GATEWAY_SERVER_TIMING=true`) |

`make bench-unit` runs the tooling's own tests, including one that proves the
coordinated-omission correction works.
