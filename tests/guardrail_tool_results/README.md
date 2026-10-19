# Tool-result guardrails: live E2E

`live_e2e.py` checks the tool-result guardrails (epic `midsommar-clean-4uc`)
end to end, against an Enterprise Studio (hub with its embedded gateway) and a
microgateway edge, with a recording mock upstream standing in for the
vendors. Every check reads what actually reached the vendor (the mock's
`/_recorded`), not just the gateway's status code. The mock rejects a
conversation whose tool calls and results no longer pair up, as the vendors
do, so a rewrite that breaks the request shape fails the suite.

The fixtures are the shared vendor bodies in `pkg/testinfra/toolfixtures`.
Each one plants an injection canary (`[canary:...]`) only in tool output.

## Run

```bash
# 1. Build the binaries (and the frontend first if you want the UI).
(cd ui/admin-frontend && CI=false npm run build)        # optional, for the UI
go build -tags enterprise -o "$BIN/studio" .
(cd microgateway && go build -tags enterprise -o "$BIN/mgw" ./cmd/microgateway)

# 2. Start a clean stack. E2E_SECRETS_FILE is a dotenv file holding
#    TYK_AI_LICENSE (for example the main checkout's dev/.env.secrets;
#    worktrees have none). It prints the private stack directory it made.
E2E_SECRETS_FILE=/path/to/dev/.env.secrets BIN=$BIN tests/guardrail_tool_results/start.sh

# 3. Run the suite (repeatable: each run makes its own LLMs, filters and app).
E2E_DIR=<the directory start.sh printed> python3 tests/guardrail_tool_results/live_e2e.py
```

The stack directory is created with `mktemp -d` (or taken from `E2E_DIR`),
made 0700, and everything in it is 0600: the env files carry the license and
the stack's random keys, and the databases and logs carry request data.
Passing an existing `E2E_DIR` to `start.sh` wipes its databases. Ports
default to Studio 18080, embedded gateway 19090, gRPC 50071, edge 18081 and
mock 14031 (`E2E_*_PORT`). To browse the UI, use `http://localhost:18080`,
not `127.0.0.1`, for CSRF.

**Local tests only.** The suite registers and logs in as
`admin@example.com` / `Test#2025` on a fresh stack. `start.sh` generates
the encryption key, the gRPC token and the edge's `JWT_SECRET` randomly on
every run. The stack also runs with settings that are only acceptable on a
developer machine: plaintext gRPC between hub and edge
(`GRPC_TLS_INSECURE`, `EDGE_TLS_ENABLED=false`, `EDGE_ALLOW_INSECURE`),
`ALLOW_INTERNAL_NETWORK_ACCESS` and `DEVMODE`. Never reuse any of this
outside a throwaway local stack.

To run a second stack beside one someone else is using (for example a UI
review), give it other ports and its own directory:
`E2E_STUDIO_PORT=28080 E2E_PROXY_PORT=29090 E2E_GRPC_PORT=50072 E2E_EDGE_PORT=28081 E2E_MOCK_PORT=24031`
with a different `E2E_DIR`, for both `start.sh` and `live_e2e.py`. Set
`E2E_NATIVE_MAX_GUARDRAIL_CALLS=3` on both to include the batch-over-budget
check. Sessions are one per user: give a person browsing the stack their own
admin rather than `admin@example.com`, which the suite logs in as.

`restart.sh [studio] [edge] [mock]` restarts parts of the stack and keeps the
databases. Restart the edge after the Studio. Both scripts stop only
*listeners* (`lsof -sTCP:LISTEN`). A plain `lsof -ti :PORT` also lists
processes holding a client connection to the port, and killing those takes
the Studio down along with the mock.

## Scenarios

| | What | Epic acceptance |
|---|---|---|
| S1 | Anthropic tool_result (string, blocks plus image, parallel with is_error, nested search_result/document): quarantined on hub and edge, buffered and streamed; ids, flags and unknown fields kept | injection only in a tool result is quarantined (Anthropic), hub and edge |
| S2 | OpenAI role tool (string, array parts, parallel, legacy function, Ollama shape) | same, OpenAI |
| S3 | Gemini functionResponse (nested leaf, parallel, Vertex role function) | same, Google |
| S13 | `last_user`, next turn: the history's poisoned result quarantined again (D1) | same |
| S5 | `tool_results.on_detect: block` refuses (400), nothing sent | |
| S6 | `tool_results.scan: false` sends the tool result unchanged | |
| S7 | A PII redact on tool conversations changes only the address; the vendor accepts the body | redact keeps every tool block and id |
| S8 | The seeded "Prompt injection heuristics (log)", with its config stored without `tool_results` (v2.2 shape), records a tool-result compliance event on hub and edge (D3) | pre-existing filter records an event |
| S8b | The seeded "Prompt injection in tool results (quarantine)" exists unattached; attached, it quarantines a tool-result injection and only logs a typed one, on hub and edge | |
| S9 | Studio chat with a REST tool returning an injection: the model's follow-up carries the placeholder and the history stores it; with block the turn stops | Studio chat session with a tool |
| S10 | OpenAI Responses tool outputs (function_call_output, custom, mcp_call, mcp_list_tools, file_search, code interpreter) | |
| S11 | A clean agent loop reaches the vendor byte for byte and raises no event | |
| S14 | A phrase split across two tool_result text blocks, at a space and inside a word (D2) | |
| S16 | A remote classifier's unpinned finding quarantines every tool result of the call; user text in its own call is kept (D4) | |
| S17 | Server tool results in assistant history (web_fetch, mcp, web_search): quarantined, accepted by the vendor (D7) | |
| S18 | Duplicate keys refused; a hostile tool name never raw in events or the placeholder; a flagged user message beside a flagged tool result still blocks | |
| S20 | Anthropic Message Batches: only the poisoned item rewritten | |
| S20b | OpenAI batch input file (Files API upload): only the poisoned lines rewritten, ids kept; with `E2E_NATIVE_MAX_GUARDRAIL_CALLS` set, a file needing more remote guardrail calls is refused with 400 naming the limit, nothing sent and no content echoed | |

## Agent actions: `live_agent_e2e.py`

`live_agent_e2e.py` checks the guardrails on agent actions (epic
`midsommar-clean-q0z`) on the same stack, hub and edge:

```bash
E2E_DIR=<the directory start.sh printed> python3 tests/guardrail_tool_results/live_agent_e2e.py
```

Here the mock also scripts the vendor's reply (`POST /_respond` with a
`text` or a `tool_call`, streamed in several events in every format) and
plays an HTTP guardrail classifier (`/classify`, flagging segments that hold
`[canary:`, with `pin`, `kinds` and `delay_ms` query options). It also serves
everything on the Unix socket `$E2E_DIR/sock/classifier.sock`, which
`start.sh` allowlists on both nodes (`GUARDRAIL_UNIX_SOCKET_ALLOWLIST`).

| | What | Bead |
|---|---|---|
| A1 | A flagged tool call in a buffered response is blocked (OpenAI Chat, Responses, Anthropic, Gemini); a clean one passes; the classifier sees `kind: tool_call`, the tool name and the user's goal | q0z.13 |
| A2 | A flagged streamed tool call never reaches the client and the stream ends with the vendor's error event (SSE, Responses events, Gemini SSE and JSON array); a clean one streams through | q0z.14 |
| A3 | A flagged tool call in the request history: `block` refuses it with nothing sent (OpenAI, Anthropic, Gemini); `log` passes it and records a `tool_call` event | q0z.13 |
| A4 | A poisoned tool definition is removed and the others reach the vendor (OpenAI, Responses, Anthropic, Gemini); a `tool_choice` forcing it, or removing the only one, is refused; `block` refuses; `tool_definitions` on a response filter is refused when saved | q0z.16 |
| A5 | Shadow: a 1.5 s classifier adds no latency and its event arrives later marked `shadow`, from the hub and the edge; the same filter inline waits; a shadow filter that could block is refused | q0z.17 |
| A6 | The classifier over the allowlisted Unix socket (through the symlinked stack directory on macOS), hub and edge; a socket outside the allowlist, or climbing out with `..`, is refused when saved | q0z.18 |
| A7 | A flagged text stream ends with the vendor's error event and no clean finish, on `/llm/stream` and on `/llm/rest` with `stream: true`, in every streamed format; clean text streams through | q0z.20 |
| A8 | A flagged tool call or text from an Anthropic upstream is blocked on `/ai/` and the unified `/v1`, streamed and not, hub and edge | q0z.23 |
| A9 | Studio chat: the model's flagged tool call is not run and the turn stops; a clean one runs; a poisoned tool definition is not offered to the model | q0z.15, q0z.16 |
| A10 | An OpenAI-shaped agent loop (parallel tool calls and results) over `/ai/` and `/v1` reaches OpenAI, Anthropic and Gemini upstreams in their own shape, and its poisoned tool result is quarantined on the inner hop (the mock refuses empty turns and broken pairing, as the vendors do) | q0z.24 |
| A11 | Buffered Responses API text reaches response guardrails: flagged text is blocked, clean text passes | q0z.25 |

Not covered live: real vendor accounts (the provider adapters are checked
against fakes in `enterprise/guardrails/providers` and live with
`make test-guardrails`), Bedrock upstreams (Converse, the native streams and the bridges are covered by the `proxy` tests against a fake runtime), and a v2.2.2 edge
binary. Old-edge snapshots are checked in `guardrails/edge_compat_*_test.go`
against the vendored v2.2.2 normaliser.
