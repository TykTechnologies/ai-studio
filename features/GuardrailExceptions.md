# Guardrail exceptions and app capability

Status: app guardrail capability implemented (model, suggestion, API, App page), 2026-11-03 (midsommar-clean-9u1j.2). Exceptions, the alert action and the LLM second opinion follow; the epic midsommar-clean-9u1j holds the full design.

## Problem

A guardrail filter that scores legitimate traffic as an attack (a code-review bot whose pull requests contain injection-like test vectors) can today only be turned to log or shadow as a whole, which drops protection for every app on that LLM. Exceptions let an operator change the **consequence** of a verdict for one app, filter, detector and channel, never the detection. How far an exception may soften a verdict is bounded by what the app can do: its **guardrail capability**.

## App guardrail capability

`apps.guardrail_capability` (`guardrails.Capability`):

| Value | Meaning |
|---|---|
| `""` | Not attested. Treated as `acting`, the strictest (`Capability.Effective`). Every existing app starts here. |
| `read_only` | The app only reads model output: no tools act for it. |
| `read_tools` | The app's tools only read (GET requests, read-only MCP tools, datasources). |
| `acting` | The app's tools can change things. |

The value is **attested** by a full administrator, not derived: the gateway cannot see everything an app can do (a CI job's own GitHub token, say).

### Suggestion

`services.SuggestAppCapability(app)` suggests a value with its reasons (not stored):

- `read_only` when the app has no tools, MCP servers or datasources ("No tools, datasources or MCP servers attached").
- `read_tools` when every tool operation the app can call is read-only, every MCP tool primitive is marked read-only, and datasources (retrieval) are present. Tool operations use the same MCP annotations as the tool MCP endpoint (`pkg/mcpannotations`): GET/HEAD/OPTIONS are read-only, other methods are not, and `x-mcp-read-only` on the operation overrides the method. MCP primitives are read-only when a tool primitive carries `readOnlyHint: true` (the MCP default is false); resources and prompts only read.
- `acting` otherwise, including what Studio cannot tell: a spec it cannot parse, an operation missing from the spec, an MCP server whose primitives are not known. The reasons name each deciding tool operation or MCP primitive.

Only the operations on a tool's whitelist count. A chat-only tool (REST and MCP access off, or a client tool) gives the app nothing and is skipped.

### API

- `GET /api/v1/apps/{id}/guardrail-capability` (apps read) returns `{value, effective, suggestion, reasons}`.
- `PUT /api/v1/apps/{id}/guardrail-capability` with `{"value": "read_only"}` (full administrators only; `""` clears it). Recorded by the audit trail as "Update App Guardrail Capability" and emitted as `system.app.updated`.
- The app response carries `guardrail_capability` read-only; the app create and update APIs never set it.

### UI

The App page's App Information shows a capability chip ("Not attested (treated as acting)" until set) with the suggestion. A full administrator accepts the suggestion in one click, or opens **Change**, which shows the suggestion pre-selected with its reasons.
