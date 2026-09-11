# Governed Metadata (Enterprise)

Governed Metadata attaches admin-defined, validated governance information to the objects AI Studio publishes to the portal and to edge gateways: LLMs, Tools, Datasources, and plugin resource types that opt in. It answers "who owns this, what state is it in, how risky is it, what data does it handle, who may use it, when does it expire" in a way that is enforceable rather than free text.

This is distinct from the plugin-owned `Metadata JSONMap` on the same models, which is an unvalidated bag namespaced per plugin (`plugin_<id>_<key>`).

## Concepts

| Concept | Model | Purpose |
|---|---|---|
| Schema | `models.MetadataSchema` | A named set of typed fields that applies to one or more object types (`applies_to`: `llm`, `tool`, `datasource`, `plugin_resource:<plugin_id>:<slug>`, or `*`). Has an enforcement level and an active flag. |
| Field definition | `models.MetadataFieldDef` (JSON inside the schema) | `key`, `label`, `type`, `required`, `severity` (`error`/`warning`), `required_on_publish`, `vocabulary_slug`, `pattern`, `min`/`max`, `max_length`, `warn_if_past`, `portal_visible`, `gateway_visible`, `order`. |
| Vocabulary | `models.MetadataVocabulary` | A shared controlled list of `{value, label, description, deprecated}` terms referenced by `vocabulary` / `multi_vocabulary` fields. |
| Object metadata | `models.ObjectMetadata` | One row per object (`object_type`, `object_id`) holding the `values` map, the last validation status/result, and who wrote it. |
| Audit | `models.ObjectMetadataAudit` | Before/after snapshot of every set/merge/delete with user, source and the plugins that ran. |

Field types: `string`, `text`, `number`, `boolean`, `date`, `email`, `url`, `user`, `vocabulary`, `multi_vocabulary`, `string_list`.

### Schema resolution

For an object type, every **active** schema whose `applies_to` contains the type or `*` contributes its fields, ordered by schema `order`, then field `order`. Field keys must be unique across schemas that overlap in `applies_to`; a collision is rejected when the schema is saved or activated. The resolved enforcement is `enforce` if any contributing schema enforces.

### Validation

Field definitions compile to a draft-07 JSON Schema (`additionalProperties: false`, `required` for hard-required fields, vocabularies as `enum`) validated with `xeipuuv/gojsonschema`, followed by a semantic pass: soft-required warnings, `warn_if_past` dates, deprecated vocabulary terms, and existence checks for `user` fields. The result is always the same envelope:

```json
{ "valid": false, "enforced": true,
  "errors":   [{ "field": "risk_tier", "code": "required", "message": "Risk tier is required" }],
  "warnings": [{ "field": "expiration_date", "code": "expired", "message": "..." }] }
```

Issue codes: `required`, `unknown_field`, `invalid_type`, `invalid_format`, `invalid_enum`, `pattern`, `range`, `length`, `expired`, `deprecated_term`, `user_not_found`, `hook_rejected`.

### Enforcement

- **advisory**: issues are reported and stored as the object's `validation_status`, nothing blocks.
- **enforce**: the admin REST create/update of an LLM, Tool or Datasource returns **422** when the supplied `governed_metadata` has hard errors. On create, an omitted attribute is treated as `{}` so required fields cannot be bypassed. Validation runs *before* the object write; the metadata row is written after it. Writes made through the plugin management gRPC API and UGC approvals are not blocked; they show up in the compliance report.

**Required to publish** (`required_on_publish`) is a third requiredness mode aimed at the submitter / reviewer workflow (see `features/RBAC.md`, "The publish rule"): the field may stay empty while the object is a draft, but the object cannot go live (LLM/Tool/Datasource `active` set to true through PATCH, create-live, or the dedicated `activate` route) until it is filled. The gate runs after the permission checks, in `api/authz_publish.go` → `publishGateOpen`, through `Service.ValidateForPublish(ctx, objectType, objectID, pending)`: stored values merged with the request's `governed_metadata`, validated with publishing semantics. Each missing field is a `422` entry with code `required_on_publish` and the usual `source.pointer`; the result is always enforced, so under an *advisory* schema only the publish fields block (ordinary hard errors are demoted to warnings for that check). Plugins get the same rule for their resource instances through `ValidateObjectMetadataForPublish` (`publishing: true` on the `ValidateObjectMetadata` RPC) before moving an instance into a live stage.

The seeded "Governance Core" schema (`governance-core`) is advisory so upgrades never block existing workflows. It defines: `business_owner`, `technical_owner`, `application_owner` (user), `lifecycle_state`, `risk_tier`, `data_classification` (vocabulary, required), `regulatory_applicability` (multi-vocabulary), `approved_consumers` (string list), `support_contact` (email), `expiration_date` (date, warns when past), plus vocabularies `lifecycle_state`, `risk_tier`, `data_classification`, `regulatory_framework`.

## Where metadata surfaces

| Surface | Shape |
|---|---|
| Admin REST (`/api/v1/llms`, `/tools`, `/datasources`) | `data.governed_metadata` (raw values map) and `data.governed_metadata_status` beside `attributes`; input accepts `attributes.governed_metadata` (absent = untouched, `{}` = clear). |
| Portal (`/common/...`) | `data[i].governed_metadata` as a display-ready `[{key, label, type, value}]` list of **portal-visible** fields with labels resolved (user names, vocabulary labels). `/common/accessible-plugin-resources` carries the same list per instance of opted-in resource types (rendered as badges by `PluginResourceListView`). |
| Edge snapshot | `LLMConfig`/`ToolConfig`/`DatasourceConfig.governed_metadata` JSON of **gateway-visible** fields only. The control server subscribes to `system.governed_metadata.*` events and recomputes the namespace checksum; edges are marked *pending* only when the checksum actually changed, so owner/contact edits never churn edges. Delivery follows the normal flow: edges show as pending in the sync status until an administrator pushes a reload (`POST /api/v1/edges/:id/reload`) or the edge reconnects. |
| Microgateway plugin context | `ctx.Metadata["governed_metadata"]` (JSON) and flattened `governed_metadata.<key>` strings on pre-auth, auth and post-auth for the LLM being called. |
| Events | `system.governed_metadata.updated` / `.deleted` with `{object_type, object_id, values, validation_status, source}`. |

Not yet surfaced: `on_response` hooks on the gateway, and tool/datasource metadata at gateway request time (the columns are synced but no plugin context is built for those requests).

## Admin API

All under `/api/v1/metadata` (admin only). CE answers **403** with `Enterprise Feature`.

```
GET    /available                        {available}
GET    /object-types                     built-ins + opted-in plugin resource types
GET|POST /schemas ; GET|PATCH|DELETE /schemas/:id     JSON:API {data:{attributes}}
GET    /schemas/resolve?object_type=     {fields, json_schema, enforcement, schema_slugs}
GET|POST /vocabularies ; GET|PATCH|DELETE /vocabularies/:id
POST   /validate                         {object_type, values} → ValidationResult
GET|PUT|DELETE /objects/:object_type/:object_id      PUT {values, merge}
GET    /objects/:object_type/:object_id/audit
GET    /compliance?object_type=&status=  {entries, counts}   status: missing|invalid|expired|warnings|valid
```

Errors: 400 invalid definition / unknown object type, 404, 409 key collision / vocabulary in use / plugin-owned schema, 422 validation or hook rejection (`errors[].source.pointer = /data/attributes/governed_metadata/<field>`).

## Plugin integration

- **Object hooks**: register `object_type: "governed_metadata"` with `before_update` / `after_update` / `before_delete` / `after_delete`. `object_json` is the `ObjectMetadata` record; a `modify` response may change `values` (re-validated), a rejection blocks the save (`hook_rejected`). `before_create` is never fired for this type.
- **Management gRPC** (`AIStudioManagementService`): `GetObjectMetadata` (optional `visibility` admin|portal|gateway; portal fills `display_json`), `SetObjectMetadata`, `DeleteObjectMetadata`, `GetResolvedMetadataSchema` (fields, JSON Schema, `vocabularies_json`, enforcement), `ValidateObjectMetadata`, gated by scopes `metadata.read` / `metadata.write`. Every call accepts `plugin_resource:self:<slug>` and resolves it to the calling plugin (`models.ResolveSelfObjectType`). SDK: `ctx.Services.Studio().GetObjectMetadata / GetObjectMetadataForAudience / SetObjectMetadata / DeleteObjectMetadata / GetResolvedMetadataSchema (→ *ResolvedMetadataSchema) / ValidateObjectMetadata`, helpers `plugin_sdk.SelfResourceObjectType`, `MetadataVisibility*`. Enforced rejections are returned in-band (`success=false` + `validation_result_json`).
- **Manifest contributions**: a `metadata` section with `vocabularies` and `schemas`; contributed schemas are created inactive and advisory with `source: plugin:<id>`, admins may only toggle `active`/`enforcement`. `applies_to` is validated (`isValidManifestAppliesTo`) and `plugin_resource:self:<slug>` is rewritten to the plugin's ID on load.
- **Resource types**: `supports_metadata: true` on a `resource_types` entry (or `ResourceTypeRegistration.SupportsMetadata` for runtime registrations) registers `plugin_resource:<plugin_id>:<slug>` as an object type; instance metadata is exposed on `GET /plugin-resource-types/:plugin_id/:slug/instances` (admin) and `/common/accessible-plugin-resources` (portal-visible, display-ready).
- **Compliance**: instances of opted-in resource types are enumerated through `Deps.ResourceInstances` (wired to `AIStudioPluginManager.ListResourceInstances`); a plugin that cannot be reached is skipped with a warning rather than failing the report. Studio does not block a plugin's own create path — the plugin honours `success=false` from `SetObjectMetadata`.
- **Host Web Components** (`ui/admin-frontend/src/admin/components/metadata/webc/governedMetadataElements.js`, registered at bootstrap): `<governed-metadata-fields>` (loads the schema via the admin API from `object-type`, or renders a plugin-provided `schema`; emits `ready`/`change`/`error`; `getValues()`, `validate()`, `setErrors()`) and `<governed-metadata-badges>` (`items` = portal display list). Both render into their own shadow root with a scoped Emotion cache so plugin UIs can embed them inside their shadow DOM.

## Key files

- `models/governed_metadata.go`, `models/plugin_manifest.go` (`ManifestMetadata`), `models/plugin_resource_type.go` (`SupportsMetadata`)
- `services/governed_metadata/` (interface, factory, CE stub); `enterprise/features/governed_metadata/` (service, validation, compliance, seed)
- `services/governed_metadata_hooks.go`, `services/hook_registry.go` (`ObjectTypeGovernedMetadata`)
- `api/governed_metadata_handlers.go`, `api/governed_metadata_helpers.go`, enforcement in `api/llm_handlers.go`, `tool_handlers.go`, `datasource_handlers.go`
- `grpc/control_server.go` (snapshot), `microgateway/internal/database/governed_metadata.go` (plugin context)
- `services/grpc/governed_metadata_server.go`, `pkg/ai_studio_sdk/service_api.go`, `pkg/plugin_sdk/context.go`
- UI: `ui/admin-frontend/src/admin/pages/Metadata*.js`, `components/metadata/` (incl. `webc/governedMetadataElements.js`), `services/governedMetadataService.js`, portal `components/GovernedMetadataBadges.js`, `components/PluginResourceListView.js`
