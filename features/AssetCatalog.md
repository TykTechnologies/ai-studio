# Asset Catalog (Enterprise plugin)

## Overview

The Asset Catalog is an enterprise-only plugin (`enterprise/plugins/asset-catalog`, manifest id `com.tyk.enterprise.asset-catalog`) that lets an organisation register and govern AI assets the gateway does not proxy: agents, prompts, skills, agent configurations, hooks, guardrails, external AI services. Assets are metadata records with ownership, versioning and lineage, relationships, lifecycle tags, community submissions and an access-request workflow. Every change is published on the internal event bus so other plugins (webhooks, email, ticketing) can automate follow-on work.

Since 1.2 the catalog is first an **internal governance inventory** and only second a portal catalogue: assets are internal until published, each asset's dependency graph spans catalog assets and AI Studio objects (LLMs, tools, data sources, MCP servers, model and semantic routers, Apps), the graph carries a risk rating, approvals snapshot the composition, and the graph exports as CycloneDX. An agent asset can be linked to the App it runs as, which makes the catalog the governance record of that App (lifecycle, periodic review, ownership).

The plugin bundles four asset types, **Agent**, **Prompt**, **Business Application** and **Integration**, plus two sample assets.

## Problem

- Governance stops at the gateway: LLMs, Tools and Datasources are tracked, but the agents, prompts and guardrails built on top of them live in wikis and repositories with no ownership, lifecycle or approval trail.
- Shadow AI: teams cannot discover what already exists, or who is accountable for it.
- Governance frameworks ask for an inventory, not a storefront: NIST AI RMF GOVERN 1.6 (inventory) and MAP 4.1 (risks of all components, third-party included), ISO/IEC 42001 A.4 (resources) and A.6.2.8 (event logs), EU AI Act Annex IV (interactions with other systems, third-party components, lifecycle changes). Comparable products (Entra Agent ID / Agent 365, ServiceNow AI Control Tower, watsonx.governance) keep the registry separate from what is published to users.

## Design decisions

| Decision | Choice |
|---|---|
| Packaging | Enterprise plugin using ResourceProvider + UIProvider + PortalUIProvider + ConfigProvider; no core tables. |
| Storage | Plugin KV (document per record + index keys) with an in-memory cache hydrated at session start. |
| Creation path | Portal users create assets only through the platform Submission form; admins create directly; owners edit and version their own assets. |
| Access model | Assets (or their type) may require approval. Approval records a grant and unlocks the asset's gated fields for that user. |
| Visibility | Internal by default. A publisher publishes an approved or production asset to everyone (the Default team) or to chosen teams. The team list is not stored in the plugin: core's `group_plugin_resources` rows are the single source of truth, written through `SetResourceInstanceGroups`, so the Teams page and the catalog agree. The asset types register with `default_access: explicit`, so core does not auto-grant new assets to Default (CE would force `auto`; the plugin is Enterprise-only). Upgrades grandfather assets in a visible stage as published to everyone, granting Default before the types switch to explicit. |
| Dependencies | Typed edges (`target_kind` asset or a core kind, `target_ref`, `target_version` pin for assets). Apps and routers expand into their bindings through a `ComponentResolver` over the governance read RPCs; an unreadable component resolves to "missing", never an error. |
| Risk | High-water mark over propagating edges (`depends_on`, `uses`, bindings), as in FIPS 199, not a weighted average, so one critical component cannot vanish among benign ones; agentic amplification (autonomous, irreversible tool actions, external exposure: one step each, after OWASP AIVSS); unrated components make the rating `incomplete`. Overrides keep computed and accepted tiers. |
| Re-approval | A composition change to an approved or production asset by a non-publisher creates a version, moves the asset to `in_review` and unpublishes it; a publisher's change keeps the stage and is logged. A linked App's binding change counts as a non-publisher change. |
| Concurrency | Every catalog mutation holds `Catalog.wmu`. Store getters return copies, and RPCs, Studio events and the review schedule run concurrently, so without it a read-modify-write silently overwrites another (seen live: the resync triggered by the plugin's own App flag undid an owner hand-over). |
| Platform integration | Every asset type is registered as a plugin resource type at runtime, so assets appear in the Teams plugin-resource sections and in the portal's unified catalog. Assets are not offered in App forms and are not shipped to gateways in the config snapshot; access to gated fields comes only from the catalog's own access requests (see Platform under Status). |
| Events | Published locally on the bus as `asset_catalog.<kind>` with a full payload (never gated values). Exact topic matching. |
| Governance | Assets of governed types (default: all) carry the platform's Governed Metadata. Governance fields are never duplicated into the type schema; Studio validates and stores them keyed by asset ID (`plugin_resource:<plugin id>:<slug>`), an enforcing schema blocks the save, and the assets appear in the compliance report. Community submissions create the asset without a record. |

## Data model

### AssetType
`slug`, `name`, `description`, `icon`, `builtin`, `schema` (JSON Schema for metadata; properties may carry `x-gated: true`), `requires_approval`, `access_request_schema`, `lifecycle_policy { initial, admin_only_stages }`, `relationship_kinds`, `has_privacy_score`, `links_app` (default true for Agent and Business Application), `is_active`, `created_by`, `schema_version`.

### Asset
`id`, `type_slug`, `name`, `description`, `tags`, `owners { created_by, responsible, accountable }` (denormalised user refs), `lifecycle` + `lifecycle_history`, `metadata` (validated) + `extra` (free-form), `current_version`, `lineage { derived_from_asset_id, derived_from_version }`, `relationships[] { kind, target_kind, target_asset_id | target_ref, target_version, note, managed }` (`managed: "app"` marks edges derived from the linked App; users cannot add or remove them), `requires_approval` (optional override), `grants[]`, `community_submitted`, `submission_id`, `privacy_score`, `is_active`, `publication { state, audience, by, at, note }`, `risk_profile { declared_tier, autonomy, tool_action_class, exposure }`, `risk_override`, `linked_app { app_id, app_name, linked_by, linked_at, synced_at, missing }`, `review { due_at, interval_days, last_reviewed_at/by, notified_for, lapsed, suspended_app }`, `owner_pending { since, departed_owner, held_by }`.

### Activity log and snapshots
Append-only per asset in KV: `asset:<id>:log:<n>` chunks of 200 entries `{ at, actor, kind, summary, details }` plus a head document, written for every mutation (content, tags, extra, ownership, lifecycle, publication, relationships, grants, approval flag, privacy score, governance, risk override, App control); `asset:<id>:snap:<n>` immutable approval snapshots (resolved components with versions and tiers, risk, approver, stage). `meta.layout` (currently 2) drives idempotent migrations at hydrate.

### AssetVersion
Full snapshot per version, `change_notes`, `changed_by`, `parent_version`. Content edits create versions; tag, ownership and lifecycle edits only append history. Owners and admins can restore an earlier version (`restore_version`): the restore appends a new version carrying the old content, so history stays linear and auditable.

### AccessRequest
`asset_id`, `requester`, `requester_groups`, `form`, `status` (`pending | approved | denied | cancelled`), `reviewer`, `decision_note`, timestamps.

## Lifecycle

```
draft → experimental → in_review → approved → production → deprecated → retired
   ↑          ↓            ↓                        ↓
   └──────────┴────────────┘ (send back)   deprecated → draft (revive)
```

`approved`, `production` and `deprecated` are reserved stages by default (per-type policy): entering one needs `assets:publish` or the per-type `assets-<type>:publish`, and publish holders may move any asset of the type between stages (send back). `retired` is terminal, always publish-only, read-only (no edits, resync or risk override; ownership can still change, and an admin can unlink the App to free it) and never visible in the portal; retired assets cannot be added as new components (existing edges are kept). Leaving the usable stages, other than to `deprecated`, unpublishes the asset. Entering `approved` or `production` takes a snapshot and schedules the next review.

## Rules

- **Visibility**: a portal user sees an asset when it is active, in a visible stage, published, and their team holds a grant (`ListAccessibleResourceInstances`, once per request; `groups:write` holders see all). Fallback on a Studio without the RPC: published means visible to all. Owners always see their own assets; `assets(-<type>):read` holders see everything including drafts and inactive assets. The resource provider reports an instance active only when it is published, so internal assets never reach the unified catalog even if a grant row exists. Inbound and related lists in portal views drop assets the caller cannot see, and the graph collapses them into a hidden count.
- **Gating**: gated fields visible to owners, asset managers (`assets(-<type>):write`) and grantees. No approval requirement → everyone sees everything. `assets:read` alone never unmasks gated values.
- **Editing**: owners (any role) or asset managers; content edits require change notes; privacy score and approval flag need the write row.
- **Relationships**: `depends_on`, `uses`, `recommended_with`, `derived_from`, `supersedes`; `depends_on` rejects cycles; deprecating/deleting an asset with inbound `depends_on` needs `force` from an asset manager and emits `asset.dependency_warning`.
- **Deletion**: `assets(-<type>):delete`, soft by default; hard delete only with no inbound relationships.
- **Schema evolution**: additive edits any time; removing a property still in use needs `force`; assets are re-validated on their next edit.
- **Submissions**: approval creates the asset owned by the submitter at lifecycle `approved`, internal; idempotent on `submission_id`.
- **Composition**: `compose_asset` validates schema, governance, relationships and cycles first and then creates the asset or writes one version with its components. Portal users must own the asset (or be creating it), may add only catalog assets they can see, and keep existing core components; new drafts start in the type's initial stage and `submit` moves them to review. `set_relationships` applies the same rules. `preview_risk` is the dry run.
- **Publishing gate**: approved or production, an accountable owner, accepted tier ≤ `risk_model.max_publish_tier` (or a publisher's override with a note).

## Dependency graph, risk and export

- `GetGraph(id, direction down|up|both, depth)` BFS over typed edges with App and router expansion; nodes carry kind, version and pin, lifecycle, publication, drift (`newer_version`, `inactive`, `missing`, `changed_since_approval`) and, in the down direction, each node's tier from the asset's assessment (`rateGraph`).
- Inherent tier per node: declared tier → governed-metadata `risk_model.tier_field` → type field `risk_level` → privacy-score bands. Data classification is the high-water mark of `risk_model.classification_field`. Drivers name the paths that set the tier.
- Timeline (`get_timeline`, admin audience): the activity log and versions, `ListObjectMetadataAudit` for the asset and its core components, `ListAuditRecords` (mutations only, which includes plugin `RPC` records) for core components, and component assets' activity logs, sorted by time with a source label.
- Export (`admin_export_graph`): CycloneDX 1.6 JSON (`metadata.component` the root; agents/applications/integrations and routers as `application`, prompts and data sources as `data`, LLMs as `machine-learning-model` with a minimal model card, tools and MCP servers as `services`; `dependencies[]`; `bom-ref` `tyk:<kind>:<id>[@v]`; `tyk:` properties for lifecycle, publication, owners, risk and portal-visible governance values) or catalog JSON; current or from a snapshot. Validated in tests against the vendored CycloneDX 1.6 schema (`catalog/testdata/cyclonedx`).

## App governance chain

- **Link** (`admin_link_app`, asset managers; one asset per App): the App's bindings become managed edges, resynced on `system.app.updated` and on demand. A change in reach runs as a non-publisher, so the asset returns to review and owners are notified; a binding the owner had already declared just becomes managed (no new version). Retired assets are not resynced. The App carries the flag `governed_by_asset`.
- **Lifecycle**: deprecated → flag `asset_deprecated` and notify owners; retired → deactivate the App (refused if that fails, unless forced, which records the failure).
- **Review**: due date by accepted tier (`review_model.intervals`) or a governed-metadata date (`due_field`). The manifest schedule `review-check` (daily 06:00) notifies once within `notice_days`, notifies owners and admins when overdue, and after `grace_days` suspends the App and flags `review_lapsed`. `admin_record_review` resets the due date and reactivates an App the catalog suspended.
- **Owner departure**: on `system.user.updated` (disabled) or `system.user.deleted`, assets whose accountable owner left pass to `fallback_owner` (or stay with them, flagged), get `owner_pending`, and their App is flagged `ownerless`; a departed responsible owner is replaced by the accountable one. `take_ownership` (the fallback owner or an asset manager) clears it.
- App control goes through core's `SetAppGovernanceState` (`apps.lifecycle`): active state plus flags under `App.Metadata.governance_flags`, audited as the plugin. The plugin's own changes come back as `system.app.updated`, so every handler is idempotent.

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
6. **`default_access: auto | explicit` on plugin resource types** (`models.PluginResourceType.DefaultAccess`, schema version 3, `services/default_catalogue_policy.go` `autoGrantsDefaultGroup`): explicit types are not auto-granted to the Default team; CE always behaves as auto. `instance_changed` now grants new active auto instances to Default immediately (previously only at plugin load, in both editions).
7. **Governance RPCs** (`services/grpc/governance_read_server.go`): `ListObjectMetadataAudit` (`metadata.read`), `ListAuditRecords` (`audit.read`, no bodies), MCP server and router reads (`mcp-servers.read`, `routers.read`; MCP info never carries upstream or auth details), team grants for a plugin's own types (`ListGroups`, `Get/SetResourceInstanceGroups`, `ListAccessibleResourceInstances`, `resource-access.manage`), and `AppInfo` bindings for MCP servers, routers and plugin resources. SDK wrappers in `pkg/ai_studio_sdk/governance_api.go` and `pkg/plugin_sdk/governance.go`; fakes in `pkg/testinfra/plugintest/test_service_broker_governance.go`.
8. **`SetAppGovernanceState`** (`apps.lifecycle`, `services/app_governance.go`): suspend or reactivate an App and set governance flags, never its access; audited as `Plugin Update App Governance State`. `governance_flags` is a reserved App metadata key: `UpdateApp` keeps the stored flags whatever it is sent, create paths drop it, `PatchAppMetadata` refuses it; the admin App form hides it and the App details page shows the flags read-only (`AppGovernanceFlags`). On SQLite the transaction takes the write lock first (a read-then-write transaction fails instantly under a concurrent writer).
9. **Hidden manifest routes** (`UISlotItem.Hidden`): mounted and reachable by URL, not listed in the sidebar (the admin asset workspace and the composers).
10. **`TestEventService`** fans events out per subscription stream (it dropped events when several streams shared one channel).

## Governance metadata integration

The plugin follows the resource-provider recipe in `docs/site/docs/governed-metadata.md`:

- `AssetType.Governed` (optional, default true) drives `ResourceTypeRegistration.SupportsMetadata`; the type becomes the object type `plugin_resource:<plugin id>:<slug>` (addressed as `plugin_resource:self:<slug>` on the Go side).
- `catalog.Governance` is the seam: `createAsset` / `UpdateAsset` call `Set` before persisting and roll back on a failed persist; `DeleteAsset` and `DeactivateType` call `Delete` first; `AttachGovernance` fills `governance_display` (portal audience) for readers and `governance` (all values) for editors on single-asset reads. `governance/studio.go` adapts `plugin_sdk.StudioServices` and maps FailedPrecondition, Unimplemented, scope denials and unknown object types to `ErrGovernanceUnavailable`, which the catalog treats as "no governance" (saves proceed, nothing rendered).
- Rejections surface as RPC code `governance_invalid` with the validation result in `details`; the UIs pass `errors[]` to `<governed-metadata-fields>.setErrors`. The admin form uses the element with `object-type` (admin API, live validation); the portal form feeds it the payload of `get_governance_schema` and validates on save. User-picker fields are hidden in the portal and their stored values preserved.

## Events

Topics (`asset_catalog.` prefix, configurable): `asset.created|updated|version_created|lifecycle_changed|relationships_changed|ownership_changed|deleted|dependency_warning|publication_changed|app_linked|review_required|deprecated|review_due|review_overdue|review_lapsed|reviewed|owner_departed`, `access_request.created|approved|denied|cancelled`, `type.created|updated|deactivated`. Payload: `event`, `occurred_at`, `plugin_id`, `actor`, `asset` (summary without gated values), `access_request`, `version`, `type`, `extra`, `links`. Types live in `enterprise/plugins/asset-catalog/events` for consumers.

## Notifications

- New access request → admins (in-app + email when SMTP is configured) with the form answers and a link to the admin queue.
- Decision → requester.
- New submissions → platform's own submission notifications.
- App governance chain: review required (linked App changed), deprecation, review due / overdue / lapsed → owners (admins too for overdue and lapsed); owner departure → admins and the fallback owner.

## UI

- Portal: `/portal/plugins/asset-catalog` (browse, detail with hash sub-route `#/assets/{id}`, versions, relationships, request access, owner editing) and `/portal/plugins/asset-catalog/mine` (my assets, my requests, my access).
- Admin: `/admin/enterprise/asset-catalog/{overview,types,assets,requests}`, plus hidden routes `/admin/enterprise/asset-catalog/asset#/assets/{id}[/tab]` (workspace: overview, components, risk, activity, approvals, export) and `/admin/enterprise/asset-catalog/compose#/new?type=…` / `#/assets/{id}` (composer).
- Portal composer: hidden route `/portal/plugins/asset-catalog/compose`.
- Shared `ui/src/graph.js`: dependency-free layered SVG (columns by depth, tier colour bars, same-column bindings drawn as arcs) with an accessible table fallback; node tests in `ui/test`. The composer lays out by its own width (container query), not the viewport's.
- Platform: assets appear in the Teams plugin-resource sections and in the portal's unified catalog; plugin types appear in the Submission form. The plugin declares no `access_granted_via_app`, so the platform resolves its types to false (it has no `custom_endpoint` hook): assets are not offered in the App forms, show no Build app action, and are not shipped to gateways. Attaching an asset to an App grants nothing, and access to gated fields comes only from the catalog's own access requests. Every asset type registers `portal_detail_path` `/portal/plugins/asset-catalog#/assets/{id}` (`resource.PortalDetailPath`, since 1.1.2), so an asset card in the unified catalog opens the plugin's asset page directly (schema fields, gated values, the access request form) rather than the built-in detail page, which can show none of that. The path before `#` must stay a registered portal route; `TestManifestIsValidAndComplete` guards it. Deriving a per-asset `access_granted_via_app` override from dependencies on proxied resources is still a follow-up; such assets would keep the built-in page with Build app and link to the asset page from there.

## Configuration

`seed_examples`, `default_requires_approval`, `notify_admins`, `event_topic_prefix`, `require_license_feature`, `risk_model { tier_field, tier_values, classification_field, classification_order, privacy_bands, max_publish_tier }`, `review_model { intervals, notice_days, grace_days, due_field, disabled }`, `fallback_owner` (see `config.schema.json`). License: any valid enterprise license; optionally the `feature_asset_catalog` entitlement (`services/licensing/types.go`).

## Testing

- Unit: `cd enterprise/plugins/asset-catalog && go test -tags enterprise ./...` (catalog rules, router gating, RBAC role recipes, events, resource bridge, governance write/rollback/delete paths against `catalog.MemoryGovernance`, and the Studio adapter's error classification).
- E2E: `go test -tags "e2e enterprise" ./tests/e2e/...` (boots the binary through `pkg/testinfra/plugintest`), including compose → approve → snapshot → publish → export and the App governance chain driven by injected `system.app.*` / `system.user.*` events.
- UI: `cd ui && npm test` (graph layout and rendering).
- Browser: `tests/ui/tests/asset-catalog-rbac.spec.ts` (Playwright, Enterprise dev stack with the plugin binary in `/app/bin/plugins`): registers the plugin if needed, creates one role and user per recipe, and walks the admin pages as each user.
- Core: `services/submission_plugin_test.go`, `services/grpc/plugin_extension_rpc_test.go`, `pkg/plugin_sdk/rpc_user_context_test.go`, `api/plugin_resource_types_portal_test.go`.

## Future work

- Portal composer access to core objects the user can reach (needs an "accessible core resources for user" RPC).
- Recertification by EU AI Act role and tier (via governed metadata); model cards and dataset sheets.
- Matching gateway traffic to the inventory to find shadow assets.
- Signed attestations (CycloneDX CDXA) and SPDX 3.0 export.
- Notifying dependents when a core component changes (subscribe to `system.llm.updated` and friends).
- Moving other resource plugins (MCP registry) to `default_access: explicit`.
- A SQL-backed store for large catalogs.
- Update proposals for plugin resources through the submission system (owners edit directly today).
- Grant expiry (`duration_days` is captured in the request form but not enforced).
- Bulk actions and CSV export in the admin UI.
