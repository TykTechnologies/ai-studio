# Governed Metadata (Enterprise)

Governed Metadata lets administrators define **which governance information every published object must carry** — business, technical and application owners, lifecycle state, risk tier, data classification, regulatory applicability, approved consumers, support contact, expiration date — and have AI Studio validate and enforce it. It applies to LLMs, Tools and Data Sources, and to plugin-provided resource types that opt in.

Available from **v2.2.0** in the Enterprise Edition. In the Community Edition the admin pages show an upgrade notice and the API returns `403`.

---

## Why

Objects published to the AI Portal or pushed to edge gateways are consumed by people and systems that need to know who is accountable for them and what data they may touch. Free-text descriptions do not answer audit questions. Governed Metadata turns those answers into typed, validated fields with a standard validation result, an audit trail, and a compliance report.

## Building blocks

### Vocabularies

A vocabulary is a controlled list of terms (`value`, `label`, optional description, `deprecated` flag). Fields of type *Vocabulary* or *Vocabulary (multiple values)* only accept those terms; deprecated terms remain valid but produce a warning.

**Admin → Governance → Vocabularies**

### Schemas

A schema is a named set of fields applied to one or more object types (or *All object types*). Each field has:

| Setting | Meaning |
|---|---|
| Key | Stored identifier (`^[a-z][a-z0-9_]*$`), unique across active schemas that overlap in object types |
| Type | Text, multi-line text, number, yes/no, date, email, URL, user, vocabulary, multi-value vocabulary, list of text values |
| Required + severity | *Error* blocks when the schema enforces; *Warning* only reports |
| Constraints | Pattern, max length, min/max, vocabulary |
| Warn when in the past | For dates such as an expiration date |
| Visible in portal | Included (with labels resolved) in portal catalogue responses |
| Sent to gateways | Included in the edge configuration snapshot and exposed to gateway plugins |

Several active schemas may apply to the same object type; they are merged. A schema also has an **enforcement** level:

- **Advisory** — validation issues are recorded on the object and shown in the compliance report, but never block saving.
- **Enforce** — creating or editing an LLM, Tool or Data Source in the admin UI/API fails with `422` until every hard-required field is valid. Switch this on only after the compliance report is clean.

**Admin → Governance → Metadata schemas**

### The seeded "Governance Core" schema

On first start the Enterprise Edition seeds four vocabularies (`lifecycle_state`, `risk_tier`, `data_classification`, `regulatory_framework`) and an advisory schema `governance-core` with the ten fields listed above. Edit it freely, or add schemas alongside it.

## Entering metadata

The LLM, Tool and Data Source forms gain a **Governance Metadata** section (hidden when no schema applies). Values are validated live as you type; an enforced schema keeps the section open and, on save, highlights the failing fields. The detail pages show the stored values and the validation status.

Metadata can also be set directly:

```http
PUT /api/v1/metadata/objects/llm/12
{ "values": { "lifecycle_state": "active", "risk_tier": "high", "data_classification": "confidential" }, "merge": true }
```

or as part of the object itself:

```http
PATCH /api/v1/llms/12
{ "data": { "type": "LLM", "attributes": { "governed_metadata": { "risk_tier": "high" } } } }
```

Omitting `governed_metadata` leaves stored values untouched; sending `{}` clears them.

## Compliance report

**Admin → Governance → Metadata compliance** re-validates every LLM, Tool and Data Source against the current schemas and lists objects that are *missing*, *invalid*, *expired* (a warn-when-past date has passed), carry *warnings*, or are *valid*, with a link to fix each one.

```http
GET /api/v1/metadata/compliance?object_type=llm&status=missing
```

## Where the values go

| Destination | What is sent |
|---|---|
| Admin API | All values plus `governed_metadata_status` on each object |
| AI Portal | Only fields marked *Visible in portal*, as `Label: value` badges with user names and vocabulary labels resolved |
| Edge gateways | Only fields marked *Sent to gateways*, in the configuration snapshot (`governed_metadata` on the LLM, tool and datasource configs). Gateway plugins read them from the plugin context: `ctx.Metadata["governed_metadata"]` (JSON) or `ctx.Metadata["governed_metadata.data_classification"]`. |
| Events | `system.governed_metadata.updated` and `.deleted` |

Because only gateway-visible fields enter the snapshot, changing an owner or a support contact never triggers an edge resync; changing a classification does.

## Plugins

Plugins can take part in four ways:

1. **Read and write metadata** through the management API (`GetObjectMetadata`, `SetObjectMetadata`, `DeleteObjectMetadata`, `GetResolvedMetadataSchema`, `ValidateObjectMetadata`; scopes `metadata.read`, `metadata.write`). See [Plugin Service APIs](plugins-service-api.md#governed-metadata-enterprise).
2. **Validate or react to changes** with object hooks on the `governed_metadata` object type (`before_update`, `after_update`, `before_delete`, `after_delete`). A rejection blocks the save; a modification replaces the values and is re-validated.
3. **Contribute schemas and vocabularies** from `manifest.json` (`metadata.vocabularies`, `metadata.schemas`). They are created inactive and advisory; administrators activate them. See [Plugin Manifests](plugins-manifests.md#governed-metadata-contributions-enterprise).
4. **Opt a resource type in** with `"supports_metadata": true` so instances of `plugin_resource:<plugin_id>:<slug>` can carry metadata.

### Governing objects a plugin owns (resource providers)

A plugin that publishes its own objects to the portal — an asset catalogue of prompts, agents or guardrails, an MCP server registry, and so on — keeps those objects in its own storage, so Studio cannot validate them on its behalf. The plugin opts in and calls the subsystem at the right moments; everything else (schemas, vocabularies, enforcement level, compliance report, audit trail) is shared with LLMs, Tools and Data Sources.

**1. Opt the resource type in.** Set `supports_metadata: true` on the resource type — in `manifest.json` for static types, or on `ResourceTypeRegistration` for types registered at runtime. The object type becomes `plugin_resource:<plugin_id>:<slug>` and appears in the *Applies to* list of the schema form and in the compliance report. Since a plugin does not know its numeric ID, it may always write `plugin_resource:self:<slug>` instead (`plugin_sdk.SelfResourceObjectType(slug)`); Studio resolves it on every management API call and in manifest `applies_to` entries.

**2. Validate and store on save.** In the plugin's create/update path call `SetObjectMetadata` with the governance values the user entered. When an enforcing schema rejects them the call returns `ok=false` with the structured result instead of an error — surface those field errors in your form and refuse the save, exactly as the built-in forms do. `ValidateObjectMetadata` gives the same result without storing, for live feedback. Plugin writes are audited as `plugin:<id>`.

```go
studio := ctx.Services.Studio()
objectType := plugin_sdk.SelfResourceObjectType("prompts")

ok, _, resultJSON, err := studio.SetObjectMetadata(ctx, objectType, asset.ID, governanceJSON, false)
if err != nil { return err }
if !ok {
    return catalog.ValidationFailed(resultJSON) // {valid:false, enforced:true, errors:[{field,code,message}]}
}
```

**3. Render the form.** `GetResolvedMetadataSchema` returns everything a form needs: the ordered field definitions, a draft-07 JSON Schema, the vocabulary terms (with labels) the fields reference, and the enforcement level. Either feed that to your own renderer, or drop the host-provided Web Component into your admin UI and let it render the standard inputs, live validation and error display:

```html
<governed-metadata-fields object-type="plugin_resource:12:prompts"></governed-metadata-fields>
```

```js
const el = this.shadowRoot.querySelector('governed-metadata-fields');
el.value = asset.governance;                          // pre-fill
el.addEventListener('change', (e) => { draft.governance = e.detail.values; });
// on a failed save: el.setErrors({ risk_tier: 'Risk tier is required' })
```

With `object-type` set the element loads the schema and validates live through the admin API (admin session). Where that API is not reachable — portal pages — pass the payload from `GetResolvedMetadataSchema` as `el.schema = {fields, vocabularies, enforcement}` and route the save through your plugin RPC. The element renders into its own shadow root with scoped styles, so it works inside a plugin's shadow DOM. Methods: `getValues()`, `validate()`, `setErrors(obj)`; events: `ready`, `change`, `error`.

**4. Show it to portal users.** `GetObjectMetadata` with visibility `portal` returns only the fields flagged *portal visible*, plus a display-ready list (`[{key, label, type, value}]`, labels and user names resolved). Render it yourself or hand it to `<governed-metadata-badges>` (`el.items = displayList`). The built-in portal pages for plugin resources already show these badges. Visibility `gateway` returns the gateway-visible subset for anything you push to edges.

**5. Clean up.** Call `DeleteObjectMetadata` when the plugin deletes the object so no orphaned record (or compliance row) remains.

**Compliance.** Instances of opted-in resource types are listed in the compliance report by asking the plugin for its instances, so an unhealthy plugin simply drops out of the report instead of failing it. Studio never blocks a plugin's own create path — enforcement for plugin objects is the plugin honouring `ok=false` in step 2.

#### Integration checklist

| Step | Where | What |
|---|---|---|
| Declare scopes | `manifest.json` → `permissions.services` | add `metadata.read` and `metadata.write` |
| Opt types in | manifest `resource_types[].supports_metadata`, or `ResourceTypeRegistration.SupportsMetadata` for types registered at runtime | `true` for every type that should carry governance metadata. Types registered through a runtime registration RPC must forward this flag end to end, otherwise they can never opt in |
| Optional: ship a schema | manifest `metadata.schemas[].applies_to` | `plugin_resource:self:<slug>`; created inactive/advisory, admins switch it on |
| Create / update | plugin save path | `SetObjectMetadata(ctx, SelfResourceObjectType(slug), id, valuesJSON, merge)`; refuse the save when `ok == false` and show `resultJSON.errors[]` per field |
| Admin form | plugin admin web component | `<governed-metadata-fields object-type="plugin_resource:<id>:<slug>">`, or `el.schema = GetResolvedMetadataSchema(...)` payload when the admin API is not reachable |
| Portal detail | plugin portal RPC + web component | `GetObjectMetadataForAudience(..., MetadataVisibilityPortal)` → `displayJSON` → `<governed-metadata-badges>` (`el.items`) |
| Delete | plugin delete path | `DeleteObjectMetadata(ctx, objectType, id)` |
| Verify | Studio admin UI | the type appears under *Applies to* in the schema form and its instances in **Governance → Metadata compliance** |

## Validation reference

Every validation returns the same envelope:

```json
{
  "valid": false,
  "enforced": true,
  "errors":   [{ "field": "risk_tier", "code": "required", "message": "Risk tier is required" }],
  "warnings": [{ "field": "expiration_date", "code": "expired", "message": "Expiration date is in the past (2025-01-01)" }]
}
```

Codes: `required`, `unknown_field`, `invalid_type`, `invalid_format`, `invalid_enum`, `pattern`, `range`, `length`, `expired`, `deprecated_term`, `user_not_found`, `hook_rejected`. A `422` from an object create/update carries one entry per hard error with `source.pointer` set to `/data/attributes/governed_metadata/<field>`.

## API summary

All endpoints are under `/api/v1/metadata` and require an administrator.

| Method | Path | Purpose |
|---|---|---|
| GET | `/available` | `{ "available": true }` in Enterprise |
| GET | `/object-types` | Built-in and opted-in plugin object types |
| GET, POST | `/schemas` | List / create (JSON:API body) |
| GET, PATCH, DELETE | `/schemas/:id` | Read / update / delete |
| GET | `/schemas/resolve?object_type=` | Merged fields, JSON Schema and enforcement |
| GET, POST | `/vocabularies` | List / create |
| GET, PATCH, DELETE | `/vocabularies/:id` | Read / update / delete (409 when referenced) |
| POST | `/validate` | Validate without saving |
| GET, PUT, DELETE | `/objects/:object_type/:object_id` | Read / set (`{values, merge}`) / clear |
| GET | `/objects/:object_type/:object_id/audit` | Change history |
| GET | `/compliance` | Compliance report |
