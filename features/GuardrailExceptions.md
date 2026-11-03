# Guardrail exceptions and app capability

Status: app guardrail capability implemented (model, suggestion, API, App page), 2026-11-03 (midsommar-clean-9u1j.2); exceptions model, floors, service and API, 2026-11-03 (.3). Exceptions at runtime and on edges, their UI, the alert action and the LLM second opinion follow; the epic midsommar-clean-9u1j holds the full design.

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

## Floors

A fixed table (`guardrails/exceptions.go`, not configurable): the softest action an exception may reach, by the app's effective capability and the channel of the flagged text. The hub checks it when an exception is saved or approved, and the runtime (hub and edge) again when it applies one, since the capability can change after approval.

| Capability | Own/system text (`text_user`, `text_system`) | Tool results (`tool_result`) | Tool calls / definitions (`tool_call`, `tool_definition`) |
|---|---|---|---|
| `read_only` | down to `log` | down to `log` | down to `log` |
| `read_tools` | down to `log` | down to `quarantine` | down to `log` |
| `acting` / unattested | down to `quarantine` | no downgrade | no downgrade |

What an exception may soften to also depends on what the runtime can do on the channel (`guardrails.ExceptionActions`): text and tool results `log` or `quarantine` (the flagged segment replaced), a tool call `log` only, a tool definition `log` or `remove`. `guardrails.CheckException` explains a refusal ("an app whose capability is not attested (treated as acting) cannot soften verdicts on tool_result; ..."). `guardrails.SoftenedAction` is what the runtime applies: the softer of the configured action and the exception's, or no change when the exception is outside the floor now.

## Exceptions

`guardrail_exceptions` (`models.GuardrailException`): `app_id`, `filter_id` (a guardrail filter), `detector` (`""` = any), `channel` (`text_user`, `text_system`, `tool_result`, `tool_call`, `tool_definition`), `tool_name` (tool channels only, `""` = any tool), `action` (the softened action), `status` (`proposed`, `approved`, `retired`), `justification` (required, one line, at most 500 bytes), `proposed_by`, `approved_by`, `approved_at`, `expires_after_days` (default 30), `last_hit_at`, `hit_count`, `source_event` (the compliance event it was created from; must be the app's). Never keyed on content, path or phrase.

Rules (`services/guardrail_exceptions.go`):

- Anyone with apps write proposes; the administrators are notified (in-app and email, link to the app page). A full administrator's own exception is approved as it is created (proposer = approver).
- Approving takes the `content_hash` of the exception as reviewed (filter, detector, channel, tool, action, justification, expiry); an exception changed since is refused with 409 "Guardrail Exception Changed". The floor is checked again at approval.
- One proposed or approved exception per scope (app, filter, detector, channel, tool): a second is a conflict. At most 20 approved per app.
- A change to an approved exception returns it to proposed, unless a full administrator makes it. The filter and source event cannot change; a retired exception cannot change.
- An approved exception that goes `expires_after_days` without a hit (counted from approval when it never hit) lapses back to proposed when the runtime next reads the app's exceptions (`ApprovedGuardrailExceptions`). One outside the floor for the app's current capability is skipped but stays approved, so attesting the capability back restores it.
- Hits are counted in memory and written in batches (`GuardrailExceptionHits`: an exception's pending count at most once a minute, or on `Flush`).
- Every change emits `system.app.guardrail_exceptions.changed`, which recomputes the edge snapshot checksums (edges become pending once exceptions are in the snapshot).

### API

| Route | Permission |
|---|---|
| `GET /api/v1/apps/{id}/guardrail-exceptions[?status=]` (meta: `max_approved`, actions per channel, floors) | apps read |
| `POST /api/v1/apps/{id}/guardrail-exceptions` | apps write |
| `GET /api/v1/apps/{id}/guardrail-exceptions/{eid}` | apps read |
| `PATCH /api/v1/apps/{id}/guardrail-exceptions/{eid}` | apps write |
| `DELETE /api/v1/apps/{id}/guardrail-exceptions/{eid}` | apps delete |
| `POST /api/v1/apps/{id}/guardrail-exceptions/{eid}/approve` (`{"content_hash": ...}`) | full administrator |
| `POST /api/v1/apps/{id}/guardrail-exceptions/{eid}/retire` | apps write |
| `GET /api/v1/filters/{id}/exceptions[?status=]` | filters read |

These routes are the only way to manage exceptions: no plugin SDK service, gRPC management call (`TestGuardrailExceptions_NotOnManagementSurfaces`) or agent scope reaches them.
