# Asset Catalog (Enterprise plugin)

## Overview

The Asset Catalog is an enterprise-only plugin (`enterprise/plugins/asset-catalog`, manifest id `com.tyk.enterprise.asset-catalog`) that lets an organisation register and govern AI assets the gateway does not proxy: agents, prompts, skills, agent configurations, hooks, guardrails, external AI services. Assets are metadata records with ownership, versioning and lineage, relationships, lifecycle tags, community submissions and an access-request workflow. Every change is published on the internal event bus so other plugins (webhooks, email, ticketing) can automate follow-on work.

The plugin bundles two example asset types, **Agent** and **Prompt**, plus two sample assets.

## Problem

- Governance stops at the gateway: LLMs, Tools and Datasources are tracked, but the agents, prompts and guardrails built on top of them live in wikis and repositories with no ownership, lifecycle or approval trail.
- Shadow AI: teams cannot discover what already exists, or who is accountable for it.

## Design decisions

| Decision | Choice |
|---|---|
| Packaging | Enterprise plugin using ResourceProvider + UIProvider + PortalUIProvider + ConfigProvider; no core tables. |
| Storage | Plugin KV (document per record + index keys) with an in-memory cache hydrated at session start. |
| Creation path | Portal users create assets only through the platform Submission form; admins create directly; owners edit and version their own assets. |
| Access model | Assets (or their type) may require approval. Approval records a grant and unlocks the asset's gated fields for that user. Visibility of the catalog itself is open to all portal users. |
| Platform integration | Every asset type is registered as a plugin resource type at runtime, so assets can be attached to Apps, assigned to groups, appear in the portal sidebar and reach gateways in the config snapshot. |
| Events | Published locally on the bus as `asset_catalog.<kind>` with a full payload (never gated values). Exact topic matching. |
| Governance | Assets of governed types (default: all) carry the platform's Governed Metadata. Governance fields are never duplicated into the type schema; Studio validates and stores them keyed by asset ID (`plugin_resource:<plugin id>:<slug>`), an enforcing schema blocks the save, and the assets appear in the compliance report. Community submissions create the asset without a record. |

## Data model

### AssetType
`slug`, `name`, `description`, `icon`, `builtin`, `schema` (JSON Schema for metadata; properties may carry `x-gated: true`), `requires_approval`, `access_request_schema`, `lifecycle_policy { initial, admin_only_stages }`, `relationship_kinds`, `has_privacy_score`, `is_active`, `created_by`, `schema_version`.

### Asset
`id`, `type_slug`, `name`, `description`, `tags`, `owners { created_by, responsible, accountable }` (denormalised user refs), `lifecycle` + `lifecycle_history`, `metadata` (validated) + `extra` (free-form), `current_version`, `lineage { derived_from_asset_id, derived_from_version }`, `relationships[] { kind, target_asset_id, note }`, `requires_approval` (optional override), `grants[]`, `community_submitted`, `submission_id`, `privacy_score`, `is_active`.

### AssetVersion
Full snapshot per version, `change_notes`, `changed_by`, `parent_version`. Content edits create versions; tag, ownership and lifecycle edits only append history. Owners and admins can restore an earlier version (`restore_version`): the restore appends a new version carrying the old content, so history stays linear and auditable.

### AccessRequest
`asset_id`, `requester`, `requester_groups`, `form`, `status` (`pending | approved | denied | cancelled`), `reviewer`, `decision_note`, timestamps.

## Lifecycle

```
draft → experimental → in_review → approved → production → deprecated
   ↑          ↓            ↓                        ↓
   └──────────┴────────────┘ (send back)   deprecated → draft (revive)
```

`approved`, `production` and `deprecated` are admin-only by default (per-type policy). Only `approved`/`production` assets are offered for App binding; non-owners only see `approved`, `production` and `deprecated` assets.

## Rules

- **Visibility**: listings visible to all portal users (visible stages only); owners/admins see everything including drafts and inactive assets.
- **Gating**: gated fields visible to owners, admins and grantees. No approval requirement → everyone sees everything.
- **Editing**: owners (any role) or admins; content edits require change notes; privacy score and approval flag are admin-only.
- **Relationships**: `depends_on`, `uses`, `recommended_with`, `derived_from`, `supersedes`; `depends_on` rejects cycles; deprecating/deleting an asset with inbound `depends_on` needs an admin `force` and emits `asset.dependency_warning`.
- **Deletion**: admin only, soft by default; hard delete only with no inbound relationships.
- **Schema evolution**: additive edits any time; removing a property still in use needs `force`; assets are re-validated on their next edit.
- **Submissions**: approval creates the asset owned by the submitter at lifecycle `approved`; idempotent on `submission_id`.

## Platform changes made for this feature (generic, reusable by any ResourceProvider plugin)

1. **Community submissions for plugin resource types** (`services/submission_service.go`, `services/submission_versioning_service.go`, `api/submission_handlers.go`, `api/plugin_resource_handlers.go`): `CreatePluginSubmission`, schema validation against `PluginResourceType.SubmissionSchema`, approval calls the plugin's `CreateResourceInstance` with a `plugin_sdk.SubmissionEnvelope`, `Submission.PluginInstanceID`, `GET /common/plugin-resource-types`, dynamic portal Submission form and admin review views.
2. **`CreateNotification` management RPC** (`notifications.write` scope) and `NotificationService.NotifyDirect` for template-less notifications. Also fixes submission notifications, which previously failed silently because `Notify` requires a template path.
3. **`RegisterResourceTypes` management RPC** (`resource-types.manage` scope) and `plugin_sdk.SyncResourceTypes` for runtime-defined resource types; resource types are deactivated when a plugin unloads.
4. **Caller identity on admin RPC**: `CallRequest.user_context` and the optional `plugin_sdk.UserAwareRPCHandler` interface.
5. **Governed metadata test fakes** (`pkg/testinfra/plugintest/test_service_broker_metadata.go`): `TestManagementServer` implements `GetObjectMetadata`, `SetObjectMetadata`, `DeleteObjectMetadata`, `GetResolvedMetadataSchema` and `ValidateObjectMetadata` with a per-object-type schema (`SetMetadataSchema`), `plugin_resource:self:` resolution, an Enterprise gate (`SetMetadataEnterprise`) and record inspection, so any resource-provider plugin can test its governance integration end to end.

## Governance metadata integration

The plugin follows the resource-provider recipe in `docs/site/docs/governed-metadata.md`:

- `AssetType.Governed` (optional, default true) drives `ResourceTypeRegistration.SupportsMetadata`; the type becomes the object type `plugin_resource:<plugin id>:<slug>` (addressed as `plugin_resource:self:<slug>` on the Go side).
- `catalog.Governance` is the seam: `createAsset` / `UpdateAsset` call `Set` before persisting and roll back on a failed persist; `DeleteAsset` and `DeactivateType` call `Delete` first; `AttachGovernance` fills `governance_display` (portal audience) for readers and `governance` (all values) for editors on single-asset reads. `governance/studio.go` adapts `plugin_sdk.StudioServices` and maps FailedPrecondition, Unimplemented, scope denials and unknown object types to `ErrGovernanceUnavailable`, which the catalog treats as "no governance" (saves proceed, nothing rendered).
- Rejections surface as RPC code `governance_invalid` with the validation result in `details`; the UIs pass `errors[]` to `<governed-metadata-fields>.setErrors`. The admin form uses the element with `object-type` (admin API, live validation); the portal form feeds it the payload of `get_governance_schema` and validates on save. User-picker fields are hidden in the portal and their stored values preserved.

## Events

Topics (`asset_catalog.` prefix, configurable): `asset.created|updated|version_created|lifecycle_changed|relationships_changed|ownership_changed|deleted|dependency_warning`, `access_request.created|approved|denied|cancelled`, `type.created|updated|deactivated`. Payload: `event`, `occurred_at`, `plugin_id`, `actor`, `asset` (summary without gated values), `access_request`, `version`, `type`, `extra`, `links`. Types live in `enterprise/plugins/asset-catalog/events` for consumers.

## Notifications

- New access request → admins (in-app + email when SMTP is configured) with the form answers and a link to the admin queue.
- Decision → requester.
- New submissions → platform's own submission notifications.

## UI

- Portal: `/portal/plugins/asset-catalog` (browse, detail with hash sub-route `#/assets/{id}`, versions, relationships, request access, owner editing) and `/portal/plugins/asset-catalog/mine` (my assets, my requests, my access).
- Admin: `/admin/enterprise/asset-catalog/{overview,types,assets,requests}`.
- Platform: assets appear in the App form and Teams plugin-resource sections and in the portal's auto-generated resource pages; plugin types appear in the Submission form.

## Configuration

`seed_examples`, `default_requires_approval`, `notify_admins`, `event_topic_prefix`, `require_license_feature` (see `config.schema.json`). License: any valid enterprise license; optionally the `feature_asset_catalog` entitlement (`services/licensing/types.go`).

## Testing

- Unit: `cd enterprise/plugins/asset-catalog && go test -tags enterprise ./...` (catalog rules, router gating, events, resource bridge, governance write/rollback/delete paths against `catalog.MemoryGovernance`, and the Studio adapter's error classification).
- E2E: `go test -tags "e2e enterprise" ./tests/e2e/...` (boots the binary through `pkg/testinfra/plugintest`).
- Core: `services/submission_plugin_test.go`, `services/grpc/plugin_extension_rpc_test.go`, `pkg/plugin_sdk/rpc_user_context_test.go`, `api/plugin_resource_types_portal_test.go`.

## Future work

- Group-scoped visibility of the catalog (today the whole catalog is visible; access is gated per asset).
- Update proposals for plugin resources through the submission system (owners edit directly today).
- Grant expiry (`duration_days` is captured in the request form but not enforced).
- Bulk actions and CSV export in the admin UI.
