# Streamed LLM responses

These fixtures test streaming capture (bead `midsommar-clean-jyx.15`, `proxy/stream_capture_test.go`). They are hand-built in each vendor's documented stream format.

- `<name>.sse` is the stream exactly as the gateway relays it.
- `<name>.json`, where one exists, is the same response as a non-streamed body.

The capture of the stream must equal `extract.ParseResponse` on the `.json`: the same tool call ids, names and argument fingerprints, response id, finish reasons and server tools.

| Fixture | What it covers |
|---|---|
| `openai_chat_parallel_stream` | Chat Completions, 3 parallel `tool_calls` assembled from `delta.tool_calls[index]`. Fragments of different calls interleave, and the arguments are formatted differently from the non-streamed body, so canonicalisation must make them equal. |
| `anthropic_messages_stream_parallel` | Messages with a thinking block, a text block, 2 `tool_use` blocks whose `input_json_delta`s interleave, and a `server_tool_use` (a provider tool, kept apart). |
| `openai_responses_stream` | Responses: a reasoning item, 2 `function_call` items (deltas, then `output_item.done`), and a `web_search_call` server item. |
| `stream_abort` | Chat Completions cut mid-arguments, with no `[DONE]`. The second call keeps its id and name, its arguments are unavailable, and the stream is incomplete with finish reason `error`. |

The correlation goldens (bead `jyx.24`) may reuse these.
