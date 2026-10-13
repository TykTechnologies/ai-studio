# toolfixtures

Vendor request bodies with tool use, for tests of tool-result guardrails
(epic `midsommar-clean-4uc`). Import from any test:

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"

for _, f := range toolfixtures.ByFormat("anthropic", "openai", "google_ai", "vertex") {
    segs := mySegmenter(f.Format, f.Body)
    // compare with f.Segments, f.LastUserSegments(), f.ToolSegments() ...
    out := rewrite(f.Body, quarantine(f.InjectionSegments()))
    toolfixtures.AssertOnlyChanged(t, f.Body, out, toolfixtures.Paths(f.InjectionSegments())...)
    toolfixtures.AssertPreserved(t, f, out)
}
```

## Markers

- Every fixture plants an injection canary, `[canary:<id>]`, only in tool
  output. Each canary sentence matches the built-in `injection.indirect_marker`
  and `injection.instruction_override` patterns; nothing else in any body trips
  the built-in injection category (checked by
  `scripting/tool_fixtures_canary_adversarial_test.go`, enterprise tag). A
  detection therefore proves the guardrail read the tool output.
- Segments flagged `pii` contain `jane.doe@example.com`, for redact tests.
- `[marker:<id>]` tags locate PII-only segments without being a canary.

## Manifest fields (`fixtures/manifest.json`)

| Field | Meaning |
|---|---|
| `format` | `anthropic`, `openai`, `ollama`, `google_ai`, `vertex`, `openai_responses` (placeholder name, bead .9), `anthropic_batch` (bead .9), `bedrock` (control) |
| `status` | `in_scope`; `open_question` (server-side tool results inside an assistant turn; the epic does not decide them yet); `control` (no segmenter; the raw-body fallback must still scan it) |
| `segments` | Expected segments in document order: `role` (system, user, assistant, tool), `kind` (text, tool_result), `path` (RFC 6901 pointer to the string leaf), `tool_name`, `tool_call_id` (empty where the format has no id), `contains`, and flags `injection`, `pii`, `is_error`, `last_user` (selected by scope last_user: everything after the last assistant message), `proposed` (expectation not settled by the epic) |
| `optional_segments` | String leaves a segmenter may or may not report (titles, URLs, empty outputs) |
| `not_segments` | String leaves that must never be reported: image data, signatures, encrypted reasoning, model-generated arguments. Three of them trip `injection.base64_blob` if scanned |
| `preserve` | Pointers whose values must be unchanged after a rewrite of flagged segments: tool ids, `is_error`, `cache_control`, images, unknown fields, a 2^53+1 integer (`/seed`, `row_id`) that float64 re-marshalling corrupts |

## Helpers

- `DiffPaths(before, after)`: pointers at which two bodies differ; key order,
  whitespace, HTML escaping and `1.50` vs `1.5` are not differences, lost
  integer precision is.
- `AssertOnlyChanged(t, before, after, allowed...)`, `AssertPreserved(t, f, after)`.
- `Lookup`, `LookupString`, `Decode` (numbers kept as `json.Number`).

Bodies are embedded; `All()` returns copies, so a test may modify them.
