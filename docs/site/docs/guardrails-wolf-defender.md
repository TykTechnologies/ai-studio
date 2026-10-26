# Wolf Defender guardrail

The Wolf Defender guardrail is a prompt-injection classifier that plugs into
AI Studio's [HTTP classifier](./guardrails.md#http) provider (Enterprise,
2.3 and later). It runs [Patronus AI's Wolf Defender Jev](https://huggingface.co/patronus-studio/wolf-defender-jev)
and scores what agent traffic carries:

| Stage | What is scored | Typical action |
|---|---|---|
| `tool_result` | what a tool returned (a file, a fetched page, command output), read as untrusted content | quarantine the result, or stop the conversation |
| `tool_call` | the arguments of a call the model wants to run, judged against the user's goal | block the call before the client runs it |
| `user_message` | the user's own text, or a bot runner's prompt around untrusted text | block |
| `tool_definition` | a tool's description and schema (poisoned MCP tools) | remove the tool |

It ships as two images, built from [TykTechnologies/ai-studio-guardrail-wolf](https://github.com/TykTechnologies/ai-studio-guardrail-wolf)
(MIT):

- `tykio/ai-studio-guardrail-wolf`: CPU, linux/amd64 and arm64. Runs next to
  the gateway on a Unix socket with no network.
- `tykio/ai-studio-guardrail-wolf-gpu`: NVIDIA (CUDA 12.6, driver 560 or later),
  linux/amd64. Runs on a GPU host and is reached over HTTPS.

This page walks through enabling it end to end. The guardrail's own
repository documents its internals and its policy file in full.

## How it fits

```
client ──> AI Studio gateway (hub or edge) ──> LLM vendor
                │  request filters: user messages, tool results, tool definitions
                │  response filters: the model's tool calls
                ▼
       Wolf Defender guardrail  (Unix socket, or HTTPS)
```

Each guardrail filter sends the gateway's segments to the guardrail, which
answers with graded findings: `<stage>.review`, `<stage>.block` and
`<stage>.critical`. The filter decides which levels to act on and how
(log, quarantine, block, remove). A filter naming `tool_result.block` acts
on block **and** critical segments; one naming `tool_result.critical` acts
only on the near-certain ones. That lets two filters split the tiers: stop
the conversation on critical, quarantine the result on block.

The guardrail adds three things beyond the model:

- **Rules decided without the model.** A tool call whose arguments read
  credentials (`/proc/*/environ`, `.env`, `~/.ssh`, cloud credential files,
  `$GITHUB_TOKEN`-style variables, `env`, `printenv`) or carry a planted
  canary token is critical at once, whatever the tool is called. Hidden text
  (Unicode tag characters, bidirectional controls, spoofed headers) is
  revealed to the model and flagged.
- **A verdict store.** Every verdict is kept (flags 7 days, clean verdicts
  24 hours). Agents resend their whole history on every request, so the
  history is answered from the store in milliseconds, and a flagged tool
  result stops every later request of its conversation, not just the one it
  first appeared in. False flags are lifted by hash.
- **A versioned policy.** Thresholds, the questions asked per stage, per-app
  system context, per-stage enforcement modes and kill switches live in a
  YAML policy that the guardrail hot-reloads.

## 1. Choose a deployment

| | CPU, next to the gateway | GPU over HTTPS |
|---|---|---|
| Image | `tykio/ai-studio-guardrail-wolf` | `tykio/ai-studio-guardrail-wolf-gpu` |
| Reached over | a Unix socket (no network) | HTTPS with a bearer token and a client-IP allowlist |
| Short message / tool call | 0.15–0.5 s | about 20 ms (plus the network round trip) |
| 6,000-token tool result | 9–18 s | about 0.3 s |
| Fits | shadow rollouts; enforcing short content; low volume | enforcing every stage inline |

The CPU figures range from a 4-CPU arm64 container to 4 shared x86 vCPUs; the
GPU figure is an NVIDIA A40. On CPU, long tool results take seconds, which
the guardrail's **deferred** mode absorbs: content it has not seen is passed
at once and scored in the background, and the next request that carries it
(the agent resends its history) is stopped. On a GPU, score everything
**inline**: a flagged result never reaches the model. Both modes are set in
the policy (see [Tune the policy](#7-tune-the-policy)).

Pin both images by digest (`image@sha256:...`), never by a moving tag.

## 2. Run the guardrail

### CPU, next to the gateway

Add the guardrail to the compose project that runs AI Studio and the
microgateway. AI Studio and the edge reach it only over a shared socket
volume; it has no network at all.

```yaml
volumes:
  guard-sock: {}

services:
  wolfguard:
    image: tykio/ai-studio-guardrail-wolf@sha256:...     # pin by digest
    network_mode: none
    read_only: true
    tmpfs: ["/tmp:rw,noexec,nosuid,size=256m"]
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    cpus: 4
    mem_limit: 4g
    env_file: [./wolfguard.env]                   # WOLFGUARD_TOKEN=..., mode 0600
    environment:
      WOLFGUARD_STATE_DIR: /var/lib/wolfguard      # verdict store, decision log, previews
      WOLFGUARD_PREVIEW: "1"                       # 200-character previews of flagged text, secrets masked
    volumes:
      - guard-sock:/run/wolfguard
      - ./wolfguard-state:/var/lib/wolfguard       # owned by uid 10001, gid 65532
      # - ./policy.yaml:/etc/wolfguard/policy.yaml:ro   # your own policy
    restart: always

  ai-studio:       # your existing Studio service
    environment:
      GUARDRAIL_UNIX_SOCKET_ALLOWLIST: /run/wolfguard/
    volumes: [guard-sock:/run/wolfguard]
    depends_on: {wolfguard: {condition: service_healthy}}

  microgateway:    # and each edge
    environment:
      GUARDRAIL_UNIX_SOCKET_ALLOWLIST: /run/wolfguard/
    volumes: [guard-sock:/run/wolfguard]
    depends_on: {wolfguard: {condition: service_healthy}}
```

1. Create the token: `openssl rand -base64 32`, written to `wolfguard.env` as
   `WOLFGUARD_TOKEN=...` (mode `0600`, never committed). The guardrail will not
   start without one.
2. Create the state directory: `install -d -o 10001 -g 65532 -m 0750 wolfguard-state`.
3. `docker compose up -d`. The guardrail is healthy once it has loaded the
   model and scored one sample per stage, in 20 to 60 seconds.

Give it 4 CPUs and 4 GB: one large worker with four threads for long
content, one small worker for short content. Fewer CPUs are markedly
slower; with less memory the second worker can be killed while it loads.

The socket is created mode `0660` with group 65532, the group of the
distroless AI Studio and microgateway images, so only they can connect.
AI Studio refuses sockets not listed in `GUARDRAIL_UNIX_SOCKET_ALLOWLIST`
(see [HTTP classifier](./guardrails.md#http)); every node that runs the
filter needs the socket mounted and the variable set.

### GPU, over HTTPS

Run `tykio/ai-studio-guardrail-wolf-gpu` on an NVIDIA host. It listens on
TCP 8000 and keeps its state under `/workspace/wolfguard`:

```bash
docker run -d --gpus all -p 127.0.0.1:8000:8000 -v wolfguard-state:/workspace \
  -e WOLFGUARD_TOKEN=... \
  -e WOLFGUARD_ALLOW_IPS=203.0.113.10/32 \
  tykio/ai-studio-guardrail-wolf-gpu@sha256:...
```

- **TLS.** Put a TLS proxy in front. AI Studio's HTTP provider only trusts
  publicly trusted certificates.
- **Client-IP allowlist.** `WOLFGUARD_ALLOW_IPS` lists the addresses (or CIDR
  ranges) AI Studio and your edges call from; everything else gets HTTP 403
  before the token is even checked. Find a gateway host's outbound address
  with `curl -4 https://api.ipify.org` on that host: a reserved or floating
  IP is usually inbound only, and containers usually leave through the host's
  address.
- **Behind a proxy that hides the caller's address**, set
  `WOLFGUARD_CLIENT_IP_HEADER` to a header the proxy sets and clients cannot,
  and `WOLFGUARD_TRUSTED_PROXIES` to the proxy's own addresses. Never trust
  `X-Forwarded-For` from a proxy that appends to the client's value.
- **Policy.** `WOLFGUARD_POLICY_YAML_B64` (your policy, base64) replaces the
  image's default policy at start.

**On Runpod.** The repository's `deploy/runpod/create_pod.sh` creates a pod in
Runpod's Secure Cloud with port 8000 behind Runpod's HTTPS proxy
(`https://POD_ID-8000.proxy.runpod.net`), a 20 GB volume for the state, no
SSH, the token taken from a Runpod secret (create `wolfguard_token` under
Settings, Secrets first), and the allowlist reading `cf-connecting-ip` from
Runpod's proxy range `100.64.0.0/10`. Behind that proxy, `X-Forwarded-For`
keeps whatever the caller sent as its first entry, so it is not used.
Cloudflare, in front of the proxy, refuses some default user agents
(Python's `urllib`); AI Studio's own client is not affected.

A GPU host has no CPU fallback: if it is down, the guardrail is down for every
filter that points at it. See [Operate it](#9-operate-it).

## 3. Store the token in AI Studio

In **Secrets**, add a secret named `WOLFGUARD_TOKEN` with the same value as
the guardrail's `WOLFGUARD_TOKEN`. Filters refer to it as
`$SECRET/WOLFGUARD_TOKEN`, so the token is never stored in a filter, and a
rotation reaches the filters within 30 seconds (an edge with its next
configuration push). See [Secrets](./secrets.md).

## 4. Give the agent its own LLM (recommended)

A filter attaches to an LLM and applies to **every app** that uses it. If the
agent you want to protect shares an LLM with other apps, create a second LLM
entry with the same vendor, endpoint, key and models, give only the agent's
app access to it, and attach the guardrail's filters there. Other apps then
never depend on the guardrail, and a guardrail outage cannot refuse their
requests.

If an app must stay on a shared LLM but should not be scored, `skip` it in the
policy (`apps: {"<app id>": {skip: true}}`): it is answered clean at once.

## 5. Create the filters (shadow first)

Start in **shadow**: the gateway does not wait for the guardrail, never
changes traffic, and records every decision as a compliance event. The
guardrail runs its enforcement modes in shadow too, so the events show what
enforcement would do. Run shadow for a couple of weeks on real traffic before
enforcing anything.

### The request filter

In **Filters**, add a filter:

| Field | Value |
|---|---|
| Name | `Wolf Defender (shadow)` |
| Filter type | **Guardrail** |
| Is this a Response Filter? | off |
| Provider | **HTTP classifier (generic)** |
| Endpoint URL | CPU: `unix:///run/wolfguard/wolfguard.sock`. GPU: the full URL, `https://guard.example.com/v1/classify?policy=2026-10-20.2` |
| Request path | CPU only: `/v1/classify?policy=2026-10-20.2`. Leave empty for an HTTPS endpoint, whose URL already carries the path |
| API key | `$SECRET/WOLFGUARD_TOKEN` |
| On detection | **Log only** |
| Detectors | `tool_result.critical`, `tool_result.block`, `tool_result.review`, `user_message.block`, `user_message.review`, `tool_call.block`, `tool_call.review`, `tool_definition.block`, `tool_definition.review`, each with **Threshold** `0.01` |
| Messages to inspect | **User text of the current turn** |
| Scan tool results | on; **Action on tool results**: Log only |
| Scan tool calls | on |
| Scan tool definitions | on |
| Shadow (don't wait) | on |
| If the provider fails | open |
| Timeout (ms) | `30000` |
| Advanced: Reuse verdicts for text already scanned | **off** |
| Advanced: Max provider pieces per request | `2000` |

Why these values:

- **Threshold 0.01.** The guardrail's policy holds the real thresholds and only
  reports decided segments. The gateway drops findings below a detector's
  threshold (default 0.5), which would lose review-level findings.
- **`?policy=` in the URL** names the policy version you expect. Rolling out a
  new policy then includes a filter edit, which also invalidates any cached
  verdicts, and the guardrail counts calls that name another version
  (`wolfguard_policy_version_mismatch_total`).
- **Verdict reuse off.** The guardrail's own store is the cache. With the
  gateway's cache on, a deferred stage's first "not scored yet" answer would
  be reused for 10 minutes and the stored verdict would never reach the
  request.
- **Max pieces 2000.** With the cache off, the whole history goes to the
  guardrail on every request (answered from its store in milliseconds). A
  long conversation passes the default of 256 pieces, and pieces beyond the
  cap are unscanned, which blocks.
- **Timeout 30000.** Inline scoring of a long comment or a large file can take
  seconds on CPU. Keep the guardrail's `WOLFGUARD_QUEUE_WAIT` (20 seconds by
  default) below it, so a busy guardrail answers before the gateway gives up.

### The response filter (tool calls)

Add a second filter for the model's tool calls: the same provider, endpoint
and key, **Is this a Response Filter?** on, detectors `tool_call.block` and
`tool_call.review` (threshold `0.01`), **Scan tool calls** on, **On
detection** Log only, **Shadow (don't wait)** on, verdict reuse off,
timeout `30000`. Streamed tool calls are held until classified, so in
enforcement the client never sees a blocked call.

### The same through the API

```json
{
  "data": {
    "type": "filters",
    "attributes": {
      "name": "Wolf Defender (shadow)",
      "description": "Prompt injection, log only",
      "kind": "guardrail",
      "response_filter": false,
      "config": {
        "provider": "http",
        "connection": {"endpoint": "unix:///run/wolfguard/wolfguard.sock", "path": "/v1/classify?policy=2026-10-20.2", "api_key": "$SECRET/WOLFGUARD_TOKEN"},
        "detectors": [
          {"name": "tool_result.critical", "threshold": 0.01}, {"name": "tool_result.block", "threshold": 0.01}, {"name": "tool_result.review", "threshold": 0.01},
          {"name": "user_message.block", "threshold": 0.01}, {"name": "user_message.review", "threshold": 0.01},
          {"name": "tool_call.block", "threshold": 0.01}, {"name": "tool_call.review", "threshold": 0.01},
          {"name": "tool_definition.block", "threshold": 0.01}, {"name": "tool_definition.review", "threshold": 0.01}
        ],
        "scope": "last_user",
        "on_detect": "log",
        "tool_results": {"on_detect": "log"},
        "tool_calls": {"scan": true},
        "tool_definitions": {"scan": true},
        "shadow": true,
        "cache": false,
        "budget": {"max_pieces": 2000},
        "timeout_ms": 30000,
        "fail_mode": "open"
      }
    }
  }
}
```

`POST /api/v1/filters` with this body creates it. For an HTTPS endpoint, put
the full URL in `endpoint` and leave out `path`.

## 6. Attach and check

1. Open the LLM, expand **Filters**, and add both filters. Filters run top to
   bottom; the first block wins.
2. On edges, wait for the configuration push (the edge shows as in sync).
3. In the filter's **Test** panel, set **Request body format** to the vendor
   your agent uses and use **Insert a tool-result sample**: the result should
   list a `tool_result.*` compliance event. A test is a real call, so it also
   checks the endpoint, the socket allowlist and the token.
4. Send one real agent turn. **Compliance → Filter Events** shows the
   decisions, marked `shadow`, with the stage, the tool name and the score.

## 7. Tune the policy

Review the shadow decisions weekly before enforcing:

- **Compliance events** in AI Studio: what was flagged, by stage, app and
  tool.
- **The guardrail's decision log** (`decisions.jsonl` in its state
  directory): one line per scored segment with the per-question scores, the
  stage, the app, the tool, the size, the policy version and the text's
  SHA-256, never the text. With previews on, `previews.jsonl` holds the first
  200 characters of each flagged segment, with keys, tokens and high-entropy
  strings masked first; join the two on `sha256`.
- **Metrics** (`/metrics` on the guardrail, bearer token): decisions per
  stage and app, latency, refusals, the store, busy and deferred work.

The policy is a YAML file (`/etc/wolfguard/policy.yaml` in the image; mount
your own over it, or pass `WOLFGUARD_POLICY_YAML_B64` on a GPU host). It is
hot-reloaded within 2 seconds, and an invalid file is refused while the last
good one stays in use. Bump its `version` on every change, and the filters'
`?policy=` with it. The settings most worth tuning:

| Setting | What it does |
|---|---|
| `apps."<id>".system_context` | A sentence describing the app ("A code-review agent for public GitHub repositories…"). It markedly improves user-message accuracy. |
| `apps."<id>".user_messages: untrusted` | For a bot whose "user" turn is its runner's prompt wrapped around untrusted text (an issue, a pull request, a comment). The user-message questions read the runner's own instruction as manipulation; this scores the message as untrusted content instead, and only the critical tier blocks. Set `untrusted_source` (for example `github_comment`) to name it. |
| `apps."<id>".skip` | Answer clean without scoring (an app with no tools that only sends structured prompts). |
| `stages.<stage>.questions.<q>.block` / `.review` / `.critical` | The thresholds. Tool results use `block_when: all`: both questions must agree, which roughly halves false blocks. |
| `enforcement.stages.<stage>.mode` | `inline` (scored before the call is answered) or `deferred` (passed at once, scored in the background, stopped from the next request). `inline_tools` keeps chosen tool calls inline in a deferred stage. |
| `enforcement.stages.<stage>.when_busy` | What an item gets when it cannot be scored in time: `error` (503), `pass`, or `unscanned` (treated as critical). |
| `rules.secret_access` | The credential-access rule on tool calls (on by default). |
| `canaries` | Strings that must never appear in a tool call (plant them in system prompts or documents). |

Things you will likely see in shadow:

- **Security text scores like an attack.** Documentation about prompt
  injection, detection patterns and test fixtures look like attacks to the
  model. That is why the defaults stop a conversation only at the critical
  tier (both questions at 0.95 or more) and quarantine at block.
- **Ordinary issues or comments flagged as user messages.** The app's runner
  wraps them in its own instruction: set `user_messages: untrusted` for that
  app.
- **Decisions one turn late.** That is deferred mode: the content was passed
  and scored in the background. Use `inline` where latency allows (on a GPU,
  everywhere).

## 8. Enforce

Enforce one stage at a time, with a week of decisions in between. Replace the
shadow request filter with two filters, in this order on the LLM:

**A. Stop the conversation:** detectors `tool_result.critical` and
`user_message.block`, **On detection** Block, **Action on tool results**
Block, a **Block message** such as "This conversation was stopped by the
security guardrail.", shadow off, the other settings as in shadow. Start
with only `tool_result.critical` and add `user_message.block` once user
messages look right.

**B. Quarantine the rest:** detector `tool_result.block`, **On detection**
Log only, **Action on tool results** Quarantine, a **Quarantine
placeholder** such as `[content removed by policy: {{tool_name}} returned
text that looked like instructions to the AI]`, shadow off.

**C. Tool calls** (response filter): detector `tool_call.block`, **Action on
tool calls** Block, shadow off. With the default policy, `shell`, `bash`,
`execute`, `write` and `edit` calls are scored inline, and any call that
reads credentials is critical without the model.

**Tool definitions:** add `tool_definition.block` to B with **Scan tool
definitions** on and **Action on tool definitions** Remove the tool.

Keep the shadow filter attached alongside for the review band.

What clients see: a blocked request gets the block message (HTTP 400); a
quarantined tool result reaches the model as the placeholder, and the agent
goes on; a blocked tool call ends the response with the vendor's error event.
Because the guardrail remembers flags, every later request of a stopped
conversation is stopped too, for 7 days or until the flag is lifted.

**Check the reply as well.** The model scores what goes in. For data leaving
in the model's own text, add a response filter with the built-in library:
provider **Built-in pattern library**, detectors `secrets` and `leak`, On
detection Block (shadow it first). `leak` catches system-prompt disclosure
markers and markdown exfiltration links; `secrets` catches credentials and
high-entropy tokens.

## 9. Operate it

**Why was a request blocked?** The caller's `400` says: the reason code and sentence after "Reason:", and the request id. See [When a guardrail blocks a request](/docs/guardrails#when-a-guardrail-blocks-a-request) for the codes. The most common surprise is `guardrail_too_large_to_scan`: a tool result longer than the filter's `budget.max_scan_chars` is never sent to Wolf Defender, so even an App the policy skips is blocked. Raise the limit on the filters (2,000,000 is a reasonable value; long text is scored in pieces). To let an App's owner see their own blocks, turn on **Show this filter's blocks to app owners** on the filters.

**See what was flagged.** In AI Studio, **Compliance → Filter events**, expand
an event: **Flagged text** lists each flagged message or tool result with the
SHA-256 of the text the guardrail scored and the request id (the
`X-Tyk-AI-Request-ID` the gateway answered with; **Open the request log**
goes to its row). Turn on **Show flagged text in
compliance events** on the filters to record a 200-character excerpt as well,
with credentials and personal data masked; Wolf Defender gives no position
inside the text, so the excerpt is the start of the flagged message or tool
result. A `guardrail.unscanned` event (Wolf Defender failed or was not
asked) has no excerpt whatever the setting: the text was never scored, and
the event's headline names the cause, such as the HTTP status Wolf Defender
returned. The same `sha256` is the key of the guardrail's decision log
(`decisions.jsonl`), its previews (`previews.jsonl`) and `GET /v1/admin/flags`,
and the request id is in the decision log's `request_id`. The blocked request
itself is in the app's request logs (status 400), with the first part of its
body.

**Kill switches, fastest first:**

- In AI Studio, switch a filter's actions to Log only (or turn on Shadow), or
  detach it from the LLM. Both take effect at once.
- In the policy: `skip` an app, raise a threshold, or switch a stage's mode;
  effective within 2 seconds.
- Stop the guardrail. With tool results set to block, requests over 32,768
  characters that carry tool results are blocked while it is down (their
  tool results are unscanned), so detach the filters first.

**Lift a false flag.** A flagged tool result keeps stopping its conversation
for 7 days. Find its `sha256` in the compliance event (or the decision log, or `GET /v1/admin/flags`),
then either run `python -m wolfguard.store --db /var/lib/wolfguard/verdicts.db allow SHA256 --note "why"`
inside the container, or call the guardrail:

```bash
curl -s -X POST https://guard.example.com/v1/admin/allow \
  -H "Authorization: Bearer $WOLFGUARD_TOKEN" -H "Content-Type: application/json" \
  -d '{"sha256": "...", "days": 30, "note": "security documentation, not an attack"}'
```

`GET /v1/admin/stats` counts the flags; `POST /v1/admin/unallow` undoes an
allow. Bumping the policy version clears every flag.

**Monitor:**

| Metric | Watch for |
|---|---|
| `wolfguard_decisions_total{stage,app,tool,decision}` | a jump in a stage's rate; every user-message flag while you tune |
| `wolfguard_rejected_total{code,reason}` | any 503 (`saturated`: queue full or waited too long) |
| `wolfguard_busy_total`, `wolfguard_deferred_total{outcome="dropped"}` | content not scored in time |
| `wolfguard_denied_total` | allowlist refusals (a gateway's address changed?) |
| `wolfguard_policy_invalid` | 1 while the policy file on disk is invalid |
| `aistudio_guardrail_errors_total` (gateway) | the guardrail unreachable or failing |
| `aistudio_guardrail_shadow_dropped_total` (gateway) | the gateway's shadow pool full |

The gateway's `/metrics` needs `METRICS_ENABLED=true` and a `METRICS_AUTH_TOKEN`
on Studio, and a `METRICS_AUTH_TOKEN` on the microgateway.

**Logs.** Rotate `decisions.jsonl` and `previews.jsonl` (for example daily,
30 and 7 days, with `copytruncate`). Never rotate or truncate `verdicts.db`
(SQLite); it expires its own rows.

**A GPU host outage.** There is no automatic fallback. Alert on
`aistudio_guardrail_errors_total`, then switch the filters to Log only, or
point their endpoint at a CPU guardrail next to the gateway until the host is
back (a CPU guardrail has its own, empty store).

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Saving the filter fails with a socket error | The socket is not in `GUARDRAIL_UNIX_SOCKET_ALLOWLIST` on Studio (`/run/wolfguard/` covers it). Each edge needs it too, or calls fail there. |
| Every call fails with HTTP 404 | An HTTPS endpoint with the path in **Request path**: for `https://` the endpoint must be the full URL; **Request path** only applies to `unix://`. |
| HTTP 401 | The `WOLFGUARD_TOKEN` secret differs from the guardrail's token, or the filter's API key is not `$SECRET/WOLFGUARD_TOKEN`. |
| HTTP 403 | The client-IP allowlist: the gateway calls from an address not in `WOLFGUARD_ALLOW_IPS` (check with `curl -4 https://api.ipify.org` on the host), or behind a proxy the address header or trusted proxy range is wrong. The guardrail logs each refused address. |
| Tool results flagged `guardrail.unscanned` | The request went over a budget: raise **Max provider pieces per request** (2000), keep the timeout at 30000, and check the guardrail's 503s. A 413 means a segment over 40,000 characters or a call over 4096 segments. |
| No events in shadow for tool results seen before | Verdict reuse is on: turn **Reuse verdicts for text already scanned** off. |
| Decisions arrive a turn late | Deferred mode (the default policy's tool results on CPU). Expected; use `inline` on a GPU. |
| The guardrail restarts while loading | Memory: give the CPU image 4 GB. |
| Slow responses on CPU | Long tool results take seconds on CPU; keep them deferred, or use the GPU image. |
| Everything from one app is clean and unscored | The app is `skip` in the policy (`wolfguard_skipped_total{reason="app_skip"}`). |
| A policy edit had no effect | The file is invalid (`wolfguard_policy_invalid` is 1; the log names the error), or the `version` was not bumped. |

## Data handling

- **CPU deployment:** the guardrail has no network. Text never leaves the
  host; the decision log holds hashes and scores only.
- **GPU deployment:** the text a filter scans goes to the GPU host, over TLS
  to your proxy. Use a host you control, and keep the allowlist and token on.
- The model weights are baked into the image at a pinned revision and
  verified file by file at build time; the guardrail fetches nothing at run
  time.
