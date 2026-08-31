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

## Testing a script

The filter editor's **Test** panel runs the script server-side against composed
input, in a sandbox, with no traffic and no spend. Note that `messages` are
derived from `raw_input` when it parses as JSON, so pasting a real request body
behaves the way the gateway does.
