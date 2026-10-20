# Coding-harness session ids: recorded traffic

These fixtures are real requests from coding harnesses, recorded through a local recording proxy on 2026-10-20 (bead `midsommar-clean-jyx.52`). The session-id extractors (precedence level 5, `correlation.harness_session_headers`) are tested against them. An extractor ships only for an id verified here.

Each line in the files is one request with these fields:
- `run`: the capture step;
- `method`, `path`, `status`;
- all request headers, with credentials replaced by `<redacted>`;
- `body`: a content-stripped skeleton of the JSON body.

The skeleton keeps the structure, ids, roles, types, model names and `metadata`. Every other string becomes `<str:N>`, where N is its length. The Claude Code `device_id` and `account_uuid` are replaced by placeholders. Session ids are random and are kept.

## What was verified

| Harness | Version | API | Signal | Stability |
|---|---|---|---|---|
| Claude Code | 2.1.304 | Anthropic Messages | Header `X-Claude-Code-Session-Id: <uuid>` | Per session. The same across `--resume` and `--continue`, which start a new process. Distinct for parallel sessions. |
| Claude Code | 2.1.304 | Anthropic Messages | Body `metadata.user_id`: a JSON **string** `{"device_id":"<sha256 hex>","account_uuid":"","session_id":"<uuid>"}` | The same `session_id` as the header |
| Claude Code | 2.1.304 | Anthropic Messages | Header `X-Claude-Code-Agent-Id: <17 hex>` | Only on sub-agent (Agent/Task tool) requests, one id per sub-agent. Main-thread requests have none. |
| OpenCode | 1.18.23 | Anthropic Messages | Headers `X-Session-Id: ses_<26 chars>` and `X-Session-Affinity` (the same value) | Per session. The same across `--session` and `--continue`. Distinct for parallel sessions. |
| OpenCode | 1.18.23 | OpenAI Responses | Headers `X-Session-Id`, `Session-Id` and `X-Session-Affinity` (the same value), `Originator: opencode`; body `prompt_cache_key` = the session id; `store: false` | As above. `store:false` means there is no `previous_response_id` chain. |
| Codex CLI | not tested | | | Not installed on the capture host. No extractor ships; clients send `X-Tyk-Session-Id`. |

Notes for the extractors and the assembler:
- **Header precedence.** OpenCode's `X-Session-Id` is the same header as the OpenRouter compatibility family (`x-session-id`, precedence level 3). It is already grouped while compat headers are on, but labelled with the OpenRouter source. Identify OpenCode by `User-Agent: opencode/<version>` to label it correctly.
- **Side calls share the session id.** OpenCode's first request in each session is a title-generation call with no tools. On OpenAI it goes to a different, smaller model than the one configured (`gpt-5.4-nano` here).
- **Claude Code sub-agents.** Sub-agent requests carry the parent's session id plus `X-Claude-Code-Agent-Id`. That identifies a declared sub-agent; linking it to the parent's Agent tool call still needs R5 or the tool result.
- **Legacy metadata shape.** Older Claude Code versions are reported to send `metadata.user_id` as `user_<hash>_account_<uuid>_session_<uuid>`. That shape was not captured here, so an extractor for it must stay off by default until it is verified.

## Re-capturing after a harness upgrade

The tooling is in `capture/`. It never writes keys: they are sourced into the harness process only. Run it from a scratch directory outside the repo.

```bash
export OUT=/tmp/harness-capture VENDORS_ENV=/path/to/vendors.env   # VT_ANTHROPIC_API_KEY, VT_OPENAI_API_KEY
mkdir -p "$OUT/proxy" && cp testdata/agent_tracing/harness/capture/proxy.go.txt "$OUT/proxy/main.go"
(cd "$OUT/proxy" && go mod init capproxy && go build -o "$OUT/capproxy" .)
"$OUT/capproxy" -addr 127.0.0.1:47911 -out "$OUT/capture.jsonl" &

bash testdata/agent_tracing/harness/capture/run-claude.sh            # fresh, --resume, --continue, two parallel
bash testdata/agent_tracing/harness/capture/run-claude.sh subagent   # one Agent/Task delegation
bash testdata/agent_tracing/harness/capture/run-opencode.sh anthropic/claude-haiku-5-5 ant
bash testdata/agent_tracing/harness/capture/run-opencode.sh openai/gpt-5.5 oai

python3 testdata/agent_tracing/harness/capture/summarize.py cc- "$OUT/capture.jsonl"   # eyeball the signals
python3 testdata/agent_tracing/harness/capture/sanitize.py "$OUT/capture.jsonl" testdata/agent_tracing/harness
```

Update the version directories in `sanitize.py` first. Isolation:
- Claude Code runs with its own `CLAUDE_CONFIG_DIR`.
- OpenCode runs with its own `XDG_*` dirs and `--pure`, so personal settings, hooks and plugins don't leak into the capture.

Before committing:
1. Grep the new files for key prefixes, local paths and 64-hex hashes.
2. Update the table above.
3. Keep the old version directory until the extractor's version range drops it.
