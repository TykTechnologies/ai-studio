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

Only the operations on a tool's whitelist count. A chat-only tool (REST and MCP access off, or a client tool) gives the app nothing and is skipped. Plugin resources bound to the app (ResourceProvider plugins) are something Studio cannot classify: any binding makes the suggestion `acting`, naming the resource and its type (`Service.SuggestAppCapabilityFor` reads them).

### Capability drift

The attestation describes the app's bindings. When anyone other than a full administrator changes an attested app's tools, datasources, MCP servers or plugin resources, `apps.guardrail_capability` is cleared (unattested, so acting) until a full administrator attests again. A full administrator's own changes keep it. It is enforced in the service (`guardCapabilityDrift`, around `UpdateApp`, `UpdateAppWithResources`, `AddToolToApp`, `RemoveToolFromApp`, `AddDatasourceToApp`, `RemoveDatasourceFromApp`, `SetAppMCPServers`, `SetAppPluginResources`), so every caller is covered: the admin API names its caller with `WithCapabilityActor`, and a caller that names none (the management gRPC `UpdateApp` plugins use) counts as not a full administrator. A change that leaves the bindings as they were (a rename) keeps it.

- **Visible** when a guardrail filter is attached to one of the app's LLMs (any mode): `guardrail_capability_reset_by` (null when no user is known, such as a plugin) and `guardrail_capability_reset_at` are recorded, the full administrators are notified to re-attest (link to the app), and the response to the change reports it: `guardrail_capability_reset: true` in the app attributes of `PATCH /apps/{id}` and `POST /apps/{id}/tools/{tool_id}`, and `DELETE /apps/{id}/tools/{tool_id}` answers 200 with `{"data": {"guardrail_capability_reset": true}}` instead of 204.
- **Quiet** otherwise: the attestation is cleared with nothing recorded, notified or reported. It has no effect without guardrails, and an app that gains a guardrail later starts at the strictest tier and shows the suggestion.

The reset fails closed: if clearing the attestation cannot be written (after one retry), the call returns an error (`ErrGuardrailCapabilityReset`, a 500 on the API) even though the resource change itself may have been saved. Attesting again (`PUT .../guardrail-capability`) clears the reset record. Known limitation (out of scope): an attested `read_tools` app can still gain acting ability through an edit to a bound tool's OpenAPI spec or an MCP server's primitives, which do not change the app's bindings.

The capability columns (`AppGuardedColumns`) are written only by the capability service and the reset, never by a save of the whole app, which could write back a value loaded before an administrator changed it.

### API

- `GET /api/v1/apps/{id}/guardrail-capability` (apps read) returns `{value, effective, suggestion, reasons, guardrails_apply, reset_by, reset_at}`; `guardrails_apply` says whether a resource change would reset visibly (the app editor warns before saving only then).
- `PUT /api/v1/apps/{id}/guardrail-capability` with `{"value": "read_only"}` (full administrators only; `""` clears it). Recorded by the audit trail as "Update App Guardrail Capability" and emitted as `system.app.updated`.
- The app response carries `guardrail_capability` read-only; the app create and update APIs never set it.

### UI

The App page's App Information shows a capability chip ("Not attested (treated as acting)" until set) with the suggestion. A full administrator accepts the suggestion in one click, or opens **Change**, which shows the suggestion pre-selected with its reasons. After a visible reset it says when (and by whom) the attestation was cleared.
