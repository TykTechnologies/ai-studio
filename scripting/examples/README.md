# Filter script examples

> [!WARNING]
> **These examples target an older, incompatible script contract and will not
> run as written.** They use `payload` as the input variable and assign
> `result` as the output, and some call `tyk.llm` with three arguments. The
> current contract reads an `input` object and assigns an `output` map.
>
> Copying one of these into the filter editor produces a script that does not
> work, with nothing explaining why. They are kept for reference while they are
> ported; treat them as pseudocode for the redaction logic, not as templates.

## The current contract

A filter script reads a global `input` and assigns a global `output`.

### `input`

| field | type | notes |
|---|---|---|
| `raw_input` | string | the full request or response JSON body |
| `messages` | array | normalised messages extracted from the body |
| `vendor_name` | string | e.g. `"openai"`, `"anthropic"` |
| `model_name` | string | e.g. `"gpt-5"` |
| `context` | map | `llm_id`, `app_id`, `request_id` — **there is no `user_id`** |
| `is_chat` | bool | hardcoded `false` on both proxy filter paths |
| `is_response` | bool | true on the response side |
| `is_chunk` | bool | true for a streaming chunk |
| `chunk_index` | int | chunk number when streaming |
| `current_buffer` | string | accumulated response text when streaming |
| `status_code` | int | HTTP status from the LLM |

### `output`

| field | type | notes |
|---|---|---|
| `block` | bool | stops the request/response chain |
| `payload` | string | modified content; empty means no modification |
| `messages` | array | modified messages, as an alternative to `payload` |
| `message` | string | blocking reason or log message |
| `compliance_events` | array | `{event_type, severity, description, metadata}` (enterprise) |

### Minimal example

```tengo
text := import("text")

redacted := text.re_replace(`[\w\.-]+@[\w\.-]+\.\w+`, input.raw_input, "[REDACTED EMAIL]")

output := {
    block: false,
    payload: redacted,
    message: "",
    compliance_events: [
        {
            event_type: "pii_redacted",
            severity: "info",
            description: "Redacted email addresses from the request body"
        }
    ]
}
```

## Tool filters

A filter attached to a **Tool** rather than an LLM uses the same `input` /
`output` contract, with two differences.

**Direction** comes from the filter's `response_filter` flag: `false` runs the
script on the arguments heading *to* the tool, `true` on the body it returned.
`input.is_response` tells the script which side it is on.

**`raw_input`** on the input side is the tool call envelope, identical across
the REST endpoint, the MCP transports and chat tool calls:

```json
{
  "operation_id": "createTicket",
  "parameters": {"project": ["ACME"]},
  "payload": {"summary": "card 4111 1111 1111 1111 was declined"},
  "headers": {}
}
```

On the output side it is the tool's raw response body.

`input.context` carries `tool_id`, `tool_name`, `app_id` and `user_id`, plus
`session_id` and `call_id` when the call came from a chat session.

Returning a `payload` rewrites the call. On the input side only `parameters`,
`payload` and `headers` are taken back - `operation_id` is fixed, so a filter
cannot redirect the call to a different operation.

```tengo
text := import("text")

// Redact card numbers before they leave for the downstream tool.
output := {
    block: false,
    payload: text.re_replace(`\b(?:\d[ -]*?){13,16}\b`, input.raw_input, "[REDACTED CARD]"),
    compliance_events: [
        {event_type: "pii_redacted", severity: "warning", description: "redacted a card number from tool arguments"}
    ]
}
```

Two behaviours differ from LLM filters:

- **Tool filters fail closed.** A script that errors or panics blocks the call,
  in both directions. (An LLM *response* filter fails open.)
- **The caller never sees `message`.** A blocked tool call returns a generic
  `blocked by policy`; the reason reaches the logs and the compliance event
  only. Every block records an event even if the script reported none.

## Testing a script

The filter editor's **Test** panel runs the script server-side against composed
input, in a sandbox, with no traffic and no spend. Note that `messages` are
derived from `raw_input` when it parses as JSON, so pasting a real request body
behaves the way the gateway does.
