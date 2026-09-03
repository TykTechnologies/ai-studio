# Vendor Conformance Test Suite

Live, credential-gated tests that drive **every LLM vendor** through **every
ingress surface** and assert the wire schema on both sides of the translation.

The goal is narrow and specific: **eradicate schema-matching regressions.** Every
schema bug we have shipped has the same shape — a vendor changed a field, or a
translator dropped/renamed one, and nothing failed until a customer's SDK choked
on it. Unit tests with hand-written fixtures cannot catch that, because the
fixture is a copy of our own assumption.

Status: **built and running.** Steps 1-3 of the build order are complete: config,
matrix, pre-flight, the in-process microgateway harness, all three gateway
surfaces, and the equivalence checks. The chat surface (step 4 onward) is not yet
implemented.

Layout:

| Path | Module | Contents |
|---|---|---|
| `pkg/testinfra/vendorconformance/` | root | Config, matrix, schemas, validation, drift, normalization, request builders, SSE reassembly. No network, no credentials, unit-tested in normal CI. |
| `microgateway/tests/vendorconformance/` | microgateway | The live suite: harness, pre-flight, conformance matrix, equivalence. Behind `//go:build vendorlive`. |
| `test-secrets/vendors.env` | - | Credentials (gitignored). |

The split is forced by Go: the root module cannot import `microgateway/internal/*`,
so anything that boots the data plane must live inside that module. Everything
that does not need to is shared.

```bash
make test-vendors-harness     # credential-free unit tests (runs in normal CI)
make test-vendors-preflight   # verify credentials and model IDs, ~1 call per model
make test-vendors             # the full matrix
```

---

## 1. What is under test

### 1.1 The four ingress surfaces

| Surface | Path | What it exercises |
|---|---|---|
| `chat` | Studio `GET /api/v1/chat/:chat_id` (SSE) + `POST /api/v1/chat/:chat_id/messages` | The **langchaingo driver** path — `switches.FetchDriver` → `models.LLMVendorProvider.GetDriver` → `chat_session`. Completely separate code from the proxy translators. |
| `shim` | `POST /ai/{slug}/v1/chat/completions` | The **OpenAI-format translation layer** (`proxy/translator.go`). OpenAI wire in, OpenAI wire out, vendor-native in the middle. |
| `sdk` | `POST /llm/call/{slug}/{rest}` | **Native passthrough.** Body is forwarded verbatim to `LLM.APIEndpoint + /{rest}`; we only add auth, filters, plugins and analytics. Vendor-native wire on both sides. |
| `universal` | `POST /v1/chat/completions` with `model: "{slug}/{model}"` | The **unified router** (`proxy/unified_router.go`). Rewrites to the `shim` path pre-auth, so it must be byte-equivalent to `shim` modulo the model string. |

Plus one bridge that only exists for one vendor:

| Bridge | Path | Vendor |
|---|---|---|
| `anthropic-native` | `POST /anthropic/{slug}/v1/messages` | Bedrock only (`VT_BEDROCK_ANTHROPIC_MODEL`). The Claude Code path. |

**Gateway target: the microgateway (data plane).** `shim`, `sdk` and `universal`
run against the standalone microgateway, not the Studio embedded gateway. Both
mount the same `proxy.Handler()`, but through different routers (`api/` vs
`microgateway/internal/api/router.go`), and the gin-based data-plane router is
where path-prefix and streaming-buffering divergence has actually bitten. Testing
the data plane exercises the shared handler *and* the wrapper most likely to
break it.

`chat` is Studio-only — it does not go through a gateway at all.

### 1.2 The vendors

`openai`, `anthropic`, `google_ai`, `vertex`, `bedrock`, `ollama`, `huggingface`
— the full set from `models/llm.go`, minus `mock`. Plus arbitrary
OpenAI-wire-compatible third parties via the `VT_COMPAT_N_*` slots.

Not every vendor supports every surface. The matrix encodes support explicitly so
an unsupported combination is a **declared skip**, never a silent pass:

| Vendor | chat | shim | sdk | universal | tools | json_mode | vision |
|---|---|---|---|---|---|---|---|
| openai | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| anthropic | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ |
| google_ai | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| vertex | ✓ | ✓ | — | ✓ | ✓ | ✓ | ✓ |
| bedrock | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ |
| ollama | ✓ | ✓ | ✓ | ✓ | model-dep. | ✓ | model-dep. |
| huggingface | ✓ | ✓ | ✓ | ✓ | model-dep. | — | — |

*(The table is the starting hypothesis; the harness prints an "unexpected
support" line when a declared-unsupported combination actually succeeds, so the
table stays honest.)*

### 1.3 The models

Two per vendor: `MODEL_LATEST` and `MODEL_PREV` (latest-1 family). That is the
whole model axis — deliberately small, because the point is translation
correctness, not model coverage. Two optional extras for OpenAI where the *schema
itself* differs: `MODEL_REASONING` (extra usage fields, rejected sampling params)
and `MODEL_VISION`.

A **pre-flight** step lists each vendor's models and fails loudly if a configured
model ID is not offered. This is what keeps the "latest-1" definition from
silently rotting into "some model from two years ago".

---

## 2. How schema mismatch gets eradicated

Four independent mechanisms. Each catches a class the others miss.

### 2.1 Canonical JSON Schema validation (both directions)

`tests/vendorconformance/schema/` holds Draft-07 schemas for every wire format we
speak, validated with `xeipuuv/gojsonschema` (already in `go.sum`):

```
openai_chat_completion.json          openai_chat_completion_chunk.json
openai_error.json                    openai_models_list.json
anthropic_message.json               anthropic_stream_event.json
gemini_generate_content.json         gemini_stream_chunk.json
bedrock_converse.json                bedrock_converse_stream.json
```

Every response the suite receives is validated against the schema for the format
that surface promised. `additionalProperties: false` on the objects we *emit*
(shim/universal/chat responses are ours), and `additionalProperties: true` on the
objects we *receive* from vendors — with unknown fields recorded as drift (§2.3)
rather than failed, until `VENDOR_TESTS_STRICT_UNKNOWN_FIELDS=true` locks it.

The schemas cover the parts that actually break:

- `usage` — every token counter present, integral, non-negative, and
  `prompt + completion == total`. Historically our number-one bug source.
- `choices[].finish_reason` — enum-constrained, never empty on a terminal chunk.
- `tool_calls[].function.arguments` — a **string** containing valid JSON, not an
  object. The single most common SDK-breaking mistake.
- `id`, `object`, `created`, `model` — present and correctly typed; `object` is
  the literal `chat.completion` / `chat.completion.chunk`.
- Streaming: first chunk carries `delta.role`, subsequent chunks do not repeat
  it, the stream terminates with `data: [DONE]`, and no chunk is emitted after it.

### 2.2 Cross-surface equivalence

The strongest single check. The **same logical request** is sent through `shim`,
`universal` and `chat` for one vendor, each response is normalized into a
canonical envelope, and the envelopes are compared field-by-field:

```go
type Envelope struct {
    Role          string
    ContentKind   string   // "text" | "tool_calls" | "mixed"
    ToolCallNames []string
    FinishReason  string
    Model         string
    Usage         UsageTriple
    HasUsage      bool
}
```

Content text is not compared (LLMs are non-deterministic) — **shape** is. A
translator that drops `usage` on the universal path but not the shim path fails
here even though both responses are individually schema-valid. `universal` vs
`shim` is additionally asserted to be *identical*, since the router is a pure
pre-auth rewrite.

The `chat` comparison is the loosest of the three (different code path, different
response shape) but the most valuable: it is the only check that the langchaingo
driver and the proxy translator agree on what a vendor said.

### 2.3 Upstream drift detection

Independently of pass/fail, every raw vendor response is walked and every JSON
pointer not described by the canonical schema is recorded to
`$VENDOR_TESTS_ARTIFACT_DIR/drift/<vendor>-<surface>-<model>.json`. A
`drift-summary.md` aggregates them.

This is the early-warning system: when a vendor adds a field, it shows up as
drift on the next run — before it becomes a translator bug. Reviewing drift is a
release-checklist item; once triaged, the schema is updated and the fields stop
being reported.

### 2.4 Golden envelopes

Normalized envelopes (not raw bodies) are snapshotted per
`(vendor, surface, model, scenario)` under `testdata/golden/`. Refresh with
`VENDOR_TESTS_UPDATE_GOLDEN=true`. A golden diff in a PR is a visible,
reviewable statement that the wire contract changed — which is exactly the review
signal we have been missing.

---

## 3. Scenarios

Each runs on every (vendor, surface, model) the matrix allows.

| Scenario | Asserts |
|---|---|
| `basic` | Non-streaming single-turn. Full schema + usage + finish_reason. |
| `streaming` | SSE framing, chunk schema, role-first delta, `[DONE]`, no post-`[DONE]` data, reassembled content non-empty. |
| `multiturn` | 3-turn conversation; role alternation survives translation, no message loss or duplication. |
| `system` | System prompt honoured — including vendors where it is a top-level field (Anthropic `system`, Gemini `systemInstruction`) rather than a message. Classic translation-drop site. |
| `tools` | Tool definition → `tool_calls` → `role:"tool"` result → final answer. Asserts `arguments` is a JSON string, `tool_call_id` round-trips, `finish_reason == "tool_calls"`. |
| `tools_streaming` | Tool-call deltas accumulate to valid JSON across chunks. |
| `json_mode` | `response_format` / structured output; result parses and matches the requested schema. |
| `vision` | Small inline base64 image; multimodal content array survives translation. |
| `long_context` | ~8k-token prompt. Catches truncation and body-size limits in the shim. |
| `usage` | `stream_options.include_usage` produces a usage-bearing final chunk; non-streaming usage is consistent with it. |
| `errors` | Bad model → 400/404 in **the surface's own error format**; missing token → 401; disallowed model → 403. Error-shape parity is as SDK-breaking as success-shape. |

---

## 4. Harness design

```
tests/vendorconformance/
  env.go            # godotenv load, VendorConfig structs, skip reasons
  matrix.go         # vendor × surface × scenario support table
  harness_gw.go     # in-process microgateway (mirrors gateway_traffic_test.go)
  harness_studio.go # in-process Studio API (api.NewAPI) - chat surface only
  harness_chat.go   # SSE client for the chat surface
  seed.go           # env -> models.LLM / mgwdb.LLM (incl. bedrock Metadata,
                    #   vertex "project:location", ollama base URL)
  normalize.go      # raw response -> Envelope, per wire format
  validate.go       # gojsonschema wrapper + drift walker
  schema/*.json
  testdata/golden/
  basic_test.go  streaming_test.go  tools_test.go  json_test.go
  errors_test.go equivalence_test.go preflight_test.go
```

**Build tag:** `//go:build vendorlive`. Combined with `VENDOR_TESTS_ENABLED=true`
this is a two-key lock — `go test ./...`, `make test-all` and CI cannot trip it.

**Boot mode: in-process.** The harness boots the microgateway in-process exactly
like `microgateway/tests/integration/gateway_traffic_test.go` (real listener on a
free port, real service container, SQLite, seeded LLM + App + APIToken), and the
Studio API in-process for the `chat` surface (`api.NewAPI` + `apitest` helpers, as
in `api/chat_session_test.go`). No Docker, no `make dev`.

This matters more than it looks: the env file becomes the **single source of
truth** for LLM configuration. Against a live stack, a stale LLM config in the
admin DB is indistinguishable from a translation bug — which is the exact class
of false signal this suite exists to remove.

`VENDOR_TESTS_STUDIO_BASE_URL` / `VENDOR_TESTS_MICROGATEWAY_BASE_URL` are
reserved for a later live-stack mode; leave them blank.

**Config loading:** `godotenv.Load` on `test-secrets/vendors.env`, with real
process env taking precedence — so CI injects secrets with no file on disk and no
shell sourcing (important: the shell here is fish; `set -a; source` does not work).

**Skips are loud.** Every skip carries a reason and lands in the run summary:

```
=== VENDOR CONFORMANCE SUMMARY ===
openai      latest+prev   chat ✓  shim ✓  sdk ✓  universal ✓   44/44
anthropic   latest+prev   chat ✓  shim ✓  sdk ✓  universal ✓   40/40  (json_mode n/a)
vertex      SKIPPED       VT_VERTEX_CREDENTIALS_FILE not set
bedrock     latest only   VT_BEDROCK_MODEL_PREV not set        22/22
...
drift: 3 new upstream fields -> test-results/vendor-conformance/drift-summary.md
```

**Cost control:** `VENDOR_TESTS_MAX_TOKENS` caps every completion; prompts are
one sentence; `long_context` is the only large request and runs once per vendor,
not per model.

---

## 5. Commands

```bash
make test-vendors                                   # everything configured
make test-vendors VENDORS=openai,anthropic          # subset
make test-vendors SURFACES=shim,universal           # subset
make test-vendors SCENARIOS=tools,tools_streaming   # subset
make test-vendors-update-golden                     # refresh snapshots
make test-vendors-drift                             # pre-flight + drift only, no assertions
```

All map to `go test -tags vendorlive -count=1 ./tests/vendorconformance/...`
with the corresponding `VENDOR_TESTS_*` variables set.

---

## 6. Build order

1. **Config + matrix + pre-flight** (`env.go`, `matrix.go`, `preflight_test.go`).
   Proves credentials load and every configured model exists. Independently
   useful on day one.
2. **Microgateway harness + `basic` on `shim`** for openai and anthropic.
   Establishes seeding, the schema validator and the drift walker end-to-end.
3. **Remaining surfaces** — `universal`, `sdk` on the microgateway, then `chat`
   on the in-process Studio — plus the equivalence test. This is where the
   highest-value bugs are expected.
4. **Remaining vendors** — google_ai, vertex, bedrock, ollama, huggingface, and
   the bedrock `/anthropic/` bridge.
5. **Remaining scenarios** — tools, streaming tools, json_mode, vision,
   long_context, errors.
6. **Golden envelopes + CI nightly job** (scheduled, secrets from the CI vault,
   non-blocking on PRs). The drift summary and Makefile targets are done.

Steps 1–3 are complete. 4–6 are mechanical extension.

---

## 7. Findings

The first full run against OpenAI, Anthropic, Bedrock and Google AI found
**six bugs**, every one of them a schema-matching defect invisible to unit
tests with hand-written fixtures. All six are fixed; this section is kept as
the record of what the suite caught and where the repairs live, because the
same shapes will be reintroduced if nobody knows they were ever wrong.

Reproduce any historical case with
`make test-vendors VENDORS=<vendor> SCENARIOS=<scenario>`; bodies land in
`test-results/vendor-conformance/failures/`.

| # | Bug | Cases | Fixed in |
|---|---|---|---|
| 1 | Bedrock tool arguments always empty | 4 | `vendors/bedrock/bedrock.go` |
| 2 | Anthropic splits one turn across two choices | 4 | `proxy/translator_models.go` |
| 3 | `stream:true` + `tools` returns a buffered response | 6 | `proxy/translator.go` |
| 4 | Bedrock streaming + tools produces no content at all | 4 | `proxy/bedrock_translator.go` |
| 5 | Error contract: empty `error.type`, upstream 4xx as our 5xx | 6 | `proxy/oai_error_contract.go` |
| 6 | Gemini `finishReason` breaks the analytics decoder | all | `responses/enum_string.go` |

### 7.1 Bedrock tool arguments were always empty

`ExtractToolCallsFromContentBlocks` did `json.Marshal(toolBlock.Value.Input)`,
but `Input` is a `document.Interface` — a smithy document wrapper whose payload
lives in an unexported field — so it marshalled to `{}`. Every Bedrock tool call
reached the client as `"arguments":"{}"`. The marshal error was discarded on the
same line, so nothing logged.

Schema-valid, passing every naive check, and silently breaking every tool-using
client on Bedrock. This is the exact failure mode the suite was built for.

**Fix:** `MarshalToolInput` uses `Input.MarshalSmithyDocument()`, the accessor
that returns the real payload, and logs rather than swallows a failure. Pinned
by `TestMarshalToolInput`, which also asserts that `json.Marshal` still returns
`{}` — so the day the SDK changes, the workaround can go.

### 7.2 Anthropic split one turn across two choices

A response carrying both text and a tool call became `choices[0]` (text,
`finish_reason: "tool_calls"`, **no** `tool_calls`) plus `choices[1]` (the tool
call). OpenAI's contract puts both in the *same* message; `choices` is indexed
by `n`, not by content block. An SDK reads `choices[0]`, sees
`finish_reason: "tool_calls"` and no `tool_calls` array, and breaks.

The cause is that langchaingo drivers disagree about what a `ContentChoice` is:
OpenAI's returns one per requested completion, Anthropic's one per content
block.

**Fix:** `NewChatCompletionResponse` now takes the requested `n` and folds
anything beyond it back into a single turn — text concatenated, tool calls
unioned, `finish_reason` forced to `tool_calls` when the turn produced any.
The same grouping fixed a usage bug riding along with it: each Anthropic block
carries the *whole response's* token counters, and summing across blocks
reported double the tokens the vendor actually charged for.

Two smaller wire defects were repaired in the same place, both found by the
suite rather than reported:

- `tool_calls[].type` was `""` on Anthropic and Gemini (the drivers leave it
  unset); OpenAI's schema pins it to the literal `"function"`.
- `tool_calls[].id` was `""` on Gemini. The id is what a client echoes back as
  `tool_call_id`, so an empty one breaks the round trip; one is now minted when
  the driver supplies none.

### 7.3 `stream: true` plus `tools` returned a buffered response

Content-Type `application/json`, `object: "chat.completion"`, no SSE frames —
the handler explicitly fell back to the non-streaming path whenever tools were
present. Reproduced on OpenAI and Anthropic, shim and universal. A client that
asked for a stream got a content type it did not expect and never saw a token.

**Fix:** the fallback is gone. langchaingo streams text through the callback and
accumulates tool-call fragments internally, so the calls are known once
`GenerateContent` returns and are framed as `tool_calls` deltas before the
terminal chunk. `delta.role` is now decided by whether any frame has actually
gone out, because a tool-only turn never reaches the streaming callback at all
and would otherwise have produced a stream with no role on it.

### 7.4 Bedrock streaming with tools produced nothing

Well-formed OpenAI chunks arrived carrying neither text nor `tool_calls`: the
tool call was dropped entirely rather than merely emptied.

The suspected shared root cause with 7.1 was wrong. `ToolUseBlockDelta.Input` is
a `*string`, not a `document.Interface`. The actual cause was simpler and
entirely local to `handleBedrockChatCompletionStream`: the event loop handled
only `ContentBlockDeltaMemberText` and ignored `ContentBlockStart` and
`ContentBlockDeltaMemberToolUse` outright, so the id, the name and every
argument fragment were discarded.

**Fix:** both events are handled. Because Bedrock numbers all content blocks in
one sequence while OpenAI's `tool_calls[].index` counts tool calls only, the two
numberings are mapped rather than passed through — the index is the only thing a
client can use to reassemble fragments. A tool call that produced no argument
fragments is closed with `{}` rather than `""`, which does not parse.

### 7.5 The error contract

Two defects, on every vendor and both surfaces:

- **`error.type` was always empty.** Every error from `respondWithOAIError`
  carried `"type":""`. OpenAI's contract requires a non-empty type
  (`invalid_request_error`, `authentication_error`, …) and SDKs branch on it.
  `error.code` was a number where OpenAI uses a string.
- **Upstream 4xx surfaced as our 5xx.** An unknown model returned HTTP 500
  (OpenAI, Anthropic) or 502 (Bedrock), telling the caller to retry something
  that can never succeed. Upstream returned 404 in every case.

**Fix:** `proxy/oai_error_contract.go` derives both `type` and a string `code`
from the status, and recovers the upstream status from the driver error —
langchaingo's `"API returned unexpected status code: 404"`, Google's
`"googleapi: Error 429"`, and the `HTTPStatusCode()` smithy puts on every AWS
error. A streaming failure that happens before the first frame now answers with
a real HTTP status and an error envelope instead of a 200 whose only frame says
something went wrong.

### 7.6 Gemini `finishReason` broke the analytics decoder

Logged on every successful Gemini call: `failed to analyze response: json:
cannot unmarshal number into Go struct field .candidates.finishReason of type
string`. The analyzer typed it as a string; the value arrives as a number over
one of Gemini's two transports, so the whole record was dropped — calls that
worked perfectly for the caller produced no usage or cost data at all.

**Fix:** `responses.EnumString` accepts either an enum name or its ordinal.
Applied to every enum field in the Gemini models, not just `finishReason`:
decoding stops at the first error, so `safetyRatings[].category`,
`.probability` and `.severity` were latent failures of exactly the same kind.

### 7.7 Upstream drift

17 undocumented fields, caught automatically, all now triaged into
`pkg/testinfra/vendorconformance/schema/` with a note on each saying whether we
carry it through and why:

- **OpenAI** — `usage.prompt_tokens_details.cache_write_tokens`, and
  `obfuscation` on streaming chunks (random padding against traffic analysis).
- **Anthropic** — `stop_details`, `usage.cache_creation`, `usage.inference_geo`,
  `usage.output_tokens_details`, and their streaming equivalents.
- **Gemini** — `thoughtSignature` on content parts,
  `usageMetadata.promptTokensDetails`, `usageMetadata.serviceTier`.

None changes a billable figure: the flat counters remain what the usage triple
carries. `VENDOR_TESTS_STRICT_UNKNOWN_FIELDS` is deliberately still `false` by
default — the next vendor addition should show up as drift for a human to
triage, not as a red build. Set it to `true` for a run when you want the
contract locked shut.

### 7.8 Vendor throttling is not a conformance signal

Free-tier Gemini allows 20 requests per minute, and a full matrix run exceeds
that comfortably. The harness now skips on an upstream 429 or 503 with the
vendor's own words rather than failing: a refusal to serve says nothing about
whether our translation is correct, and a suite that cries wolf gets ignored.

### 7.9 Not bugs

`vision` is switched off platform-wide via `unsupportedFeatures` in `matrix.go`
— multimodal input is not officially supported. Delete that entry to re-enable
the scenario; the request builder and assertions are still there.

`json_mode` is declared unsupported for Anthropic and Bedrock (no OpenAI-style
`response_format`; structured output goes through tool use, which `tools`
already covers).

## 8. Related

- `features/Proxy.md` — proxy and translation layer
- `features/Chat.md` — chat session system
- `features/LLM.md` — LLM configuration model
- `microgateway/tests/integration/gateway_traffic_test.go` — the in-process boot
  pattern this harness follows
- `pkg/testinfra/mockllm` — the mock upstream used by the non-live tests
