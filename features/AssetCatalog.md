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
| Platform integration | Every asset type is registered as a plugin resource type at runtime, so assets appear in the Teams plugin-resource sections and in the portal's unified catalog. Assets are not offered in App forms and are not shipped to gateways in the config snapshot; access to gated fields comes only from the catalog's own access requests (see Platform under Status). |
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

`approved`, `production` and `deprecated` are reserved stages by default (per-type policy): entering one needs `assets:publish` or the per-type `assets-<type>:publish`, and publish holders may move any asset of the type between stages (send back). Only `approved`/`production` assets are offered for App binding; non-owners only see `approved`, `production` and `deprecated` assets.

## Rules

- **Visibility**: listings visible to all portal users (visible stages only); owners and `assets(-<type>):read` holders see everything including drafts and inactive assets.
- **Gating**: gated fields visible to owners, asset managers (`assets(-<type>):write`) and grantees. No approval requirement → everyone sees everything. `assets:read` alone never unmasks gated values.
- **Editing**: owners (any role) or asset managers; content edits require change notes; privacy score and approval flag need the write row.
- **Relationships**: `depends_on`, `uses`, `recommended_with`, `derived_from`, `supersedes`; `depends_on` rejects cycles; deprecating/deleting an asset with inbound `depends_on` needs `force` from an asset manager and emits `asset.dependency_warning`.
- **Deletion**: `assets(-<type>):delete`, soft by default; hard delete only with no inbound relationships.
- **Schema evolution**: additive edits any time; removing a property still in use needs `force`; assets are re-validated on their next edit.
- **Submissions**: approval creates the asset owned by the submitter at lifecycle `approved`; idempotent on `submission_id`.

## Permissions (RBAC)

The plugin is the reference adoption of plugin-declared RBAC (see `docs/site/docs/plugins-manifests.md`, "Permissions (RBAC) Block"). Rows in the role editor, all under *Plugins → Asset Catalog*:

| Row | Meaning |
|---|---|
| base `plugin:com.tyk.enterprise.asset-catalog` | **read**: open the pages, list types, call the asset methods (every catalog role starts here; without an `assets` read row the admin Assets page shows only the publicly visible stages, like the portal); **write**: catalog administrator (every row below on every type, re-seed, plugin config). |
| `asset-types` (r/w/d) | write: define, edit, deactivate, reactivate types. |
| `assets` (r/w/d/publish) | read: see every asset incl. drafts/inactive; write: create and edit any asset regardless of ownership, transfer ownership, revoke grants, privacy score, approval flag, `force`, see gated values and grants; delete: deactivate/hard delete; publish: enter reserved stages and move assets between stages as a reviewer. |
| `assets-<type>` (r/w/d/publish, registered at runtime per active type via `RegisterPermissionResources`) | the same, narrowed to one type. |
| `access-requests` (r/w) | read: list/inspect every request; write: approve, deny, cancel any pending request. |

Enforcement is layered: Studio gates `POST /plugins/:id/rpc/:method` on the manifest's `rbac.rpc_methods`; the router (`rpc/router.go`) checks the same table; the catalog (`catalog/types.go` `Actor.Can*` helpers, `catalog/catalog.go`) applies the row that actually matters per asset. Because Studio can only name one fixed permission per method and the per-type rows are dynamic, the asset methods (`admin_list_assets`, `admin_create_asset`, `admin_delete_asset`, the aliases `admin_transition` / `admin_revoke_grant`) and `admin_list_types` are gated on the base read and decided in the catalog. `Actor.IsAdmin` (full administrator, or base write on the admin route) and base write on any route are the umbrella; the catalog never consults `IsAdmin` directly. The admin pages hide controls the caller cannot use (`pluginAPI.can(...)` plus the server-computed `can_edit` / `can_manage` / `can_delete` / `allowed_transitions` on `AssetView`), and the portal page relies on the server flags only. Portal RPC is not governed by roles (portal users hold none). Role recipes and their guarantees are asserted in `catalog/rbac_test.go` (`TestRoleRecipes`), `rpc/router_rbac_test.go` and `main_rbac_test.go`.

## Platform changes made for this feature (generic, reusable by any ResourceProvider plugin)

1. **Community submissions for plugin resource types** (`services/submission_service.go`, `services/submission_versioning_service.go`, `api/submission_handlers.go`, `api/plugin_resource_handlers.go`): `CreatePluginSubmission`, schema validation against `PluginResourceType.SubmissionSchema`, approval calls the plugin's `CreateResourceInstance` with a `plugin_sdk.SubmissionEnvelope`, `Submission.PluginInstanceID`, `GET /common/plugin-resource-types`, dynamic portal Submission form and admin review views.
2. **`CreateNotification` management RPC** (`notifications.write` scope) and `NotificationService.NotifyDirect` for template-less notifications. Also fixes submission notifications, which previously failed silently because `Notify` requires a template path.
3. **`RegisterResourceTypes` management RPC** (`resource-types.manage` scope) and `plugin_sdk.SyncResourceTypes` for runtime-defined resource types; resource types are deactivated when a plugin unloads or is deleted (loaded or not), and at startup `LoadAllUIAndAgentPlugins` deactivates any left active by a plugin that no longer exists (`models.DeactivateOrphanedPluginResourceTypes`).
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
- Platform: assets appear in the Teams plugin-resource sections and in the portal's unified catalog; plugin types appear in the Submission form. The plugin declares no `access_granted_via_app`, so the platform resolves its types to false (it has no `custom_endpoint` hook): assets are not offered in the App forms, show no Build app action, and are not shipped to gateways. Attaching an asset to an App grants nothing, and access to gated fields comes only from the catalog's own access requests. Every asset type registers `portal_detail_path` `/portal/plugins/asset-catalog#/assets/{id}` (`resource.PortalDetailPath`, since 1.1.2), so an asset card in the unified catalog opens the plugin's asset page directly (schema fields, gated values, the access request form) rather than the built-in detail page, which can show none of that. The path before `#` must stay a registered portal route; `TestManifestIsValidAndComplete` guards it. Deriving a per-asset `access_granted_via_app` override from dependencies on proxied resources is still a follow-up; such assets would keep the built-in page with Build app and link to the asset page from there.

## Configuration

`seed_examples`, `default_requires_approval`, `notify_admins`, `event_topic_prefix`, `require_license_feature` (see `config.schema.json`). License: any valid enterprise license; optionally the `feature_asset_catalog` entitlement (`services/licensing/types.go`).

## Testing

- Unit: `cd enterprise/plugins/asset-catalog && go test -tags enterprise ./...` (catalog rules, router gating, RBAC role recipes, events, resource bridge, governance write/rollback/delete paths against `catalog.MemoryGovernance`, and the Studio adapter's error classification).
- E2E: `go test -tags "e2e enterprise" ./tests/e2e/...` (boots the binary through `pkg/testinfra/plugintest`).
- Browser: `tests/ui/tests/asset-catalog-rbac.spec.ts` (Playwright, Enterprise dev stack with the plugin binary in `/app/bin/plugins`): registers the plugin if needed, creates one role and user per recipe, and walks the admin pages as each user.
- Core: `services/submission_plugin_test.go`, `services/grpc/plugin_extension_rpc_test.go`, `pkg/plugin_sdk/rpc_user_context_test.go`, `api/plugin_resource_types_portal_test.go`.

## Future work

- Group-scoped visibility of the catalog (today the whole catalog is visible; access is gated per asset).
- Update proposals for plugin resources through the submission system (owners edit directly today).
- Grant expiry (`duration_days` is captured in the request form but not enforced).
- Bulk actions and CSV export in the admin UI.
