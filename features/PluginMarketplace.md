# Plugin Marketplace: installs, update detection and upgrades

This spec covers how an installed plugin is tied back to the marketplace entry it came from, how AI Studio
detects that a newer version exists, and how a plugin is upgraded in place without losing its configuration.

Browsing and managing marketplace sources is out of scope here; see `docs/site/docs/configuration.md`
(enabling the marketplace) and `docs/site/docs/plugins-publishing.md` (publishing to it).

## Prerequisites

The marketplace service only runs when `MARKETPLACE_ENABLED=true` (default) **and** `AI_STUDIO_OCI_CACHE_DIR`
is set. Without it there is no index to compare against, so no plugin is linked and no updates are reported.

## How an install is linked to the marketplace

A marketplace install goes through the ordinary plugin creation wizard (`POST /plugins` →
`validate-and-load` → `approve-scopes` → `PATCH`) with the marketplace entry's OCI reference as the command.
Nothing is recorded at install time. Instead the link is **derived from the command**:

`MarketplaceService.ReconcileInstalledVersions` (`services/marketplace_installed_versions.go`) is the single
writer of `installed_plugin_versions`. For every plugin whose command starts with `oci://` it:

1. Parses the command and matches **registry + repository** against the synced `marketplace_plugins` rows.
   That gives the marketplace plugin ID. Matching on the repository, not on the plugin ID alone, means a
   second marketplace source that reuses a plugin ID cannot offer its own artifact as an upgrade.
2. Resolves the installed version: the digest is authoritative; a tag counts only when exactly one version
   carries it; after that the manifest the plugin reported (`plugins.manifest.version`, then
   `registered_plugins.manifest_version`). The fallbacks cover versions that were republished under a new
   digest or removed from the index. When nothing resolves, the version is unknown and no update is claimed.
3. Records the latest upgrade candidate: highest semver, not deprecated, and not enterprise-only on the
   Community edition. `update_available` is a semver comparison (`models.IsNewerVersion`).
4. Removes rows for plugins that no longer match anything (deleted, or moved to a non-marketplace command).

Because of this, installs that predate the tracking are picked up by the first sync after an upgrade of AI
Studio itself; nobody has to reinstall anything.

Reconcile runs after every marketplace sync (hourly by default, or "Sync Marketplace"), and for the single
plugin concerned after a plugin is created, has its command changed by `PATCH`, or is upgraded.

## Where updates are shown

- `GET /api/v1/plugins` and `GET /api/v1/plugins/:id` carry `version` and a `marketplace` block
  (`marketplace_id`, `installed_version`, `available_version`, `update_available`) next to `attributes`.
  The list also carries `updates_available`, the count across all pages. This lives on the plugin API so a
  user with `plugins:read` sees updates without needing `marketplace:read`.
- `GET /api/v1/marketplace/plugins` carries an `installed` map (marketplace plugin ID → installed copies),
  so marketplace cards can show "Installed" / "Update available" and offer **Upgrade** instead of a second
  install.
- `GET /api/v1/marketplace/updates` lists every plugin with an update.
- Admin UI: the Plugins list has a Version column, an "Update: vX" chip and a banner; the plugin detail page
  shows the version and an upgrade prompt. Starting an upgrade needs `plugins:write`.

## Upgrading

Two routes, both `plugins:write` (`api/plugin_upgrade_handlers.go`, `services/plugin_upgrade.go`):

| Route | Purpose |
|-------|---------|
| `POST /api/v1/plugins/:id/upgrade/preview` | Body `{version?}` (default: latest candidate). Pulls and probes the target, reports what would change. Writes nothing. It is a write permission because it pulls and starts new code. |
| `POST /api/v1/plugins/:id/upgrade` | Body `{version, approved_scopes, allow_downgrade}`. Applies the upgrade. |

### Why in place

An upgrade updates the **same plugin row**. A delete and reinstall would change the numeric plugin ID, and
about fourteen tables key on it: the plugin's KV data, agent configs, App and group bindings to plugin
resources, per-LLM config overrides, schedules and their history, RBAC resources, and edge payload routing.
The ID is also passed to the plugin in its init config. The upgrade owns exactly these columns: `command`,
`oci_reference`, `checksum`, `manifest`, `service_scopes`, `service_access_authorized`, `hook_type`,
`hook_types`, `hook_types_customized`. **`config` is never touched.**

### What the preview reports

Installed → target version, the version list for the picker, scope and hook differences, the target's config
schema with the existing config validated against it, deprecation, whether edge gateways run the plugin, and
the version's `CHANGELOG.md`. The changelog is fetched from the directory of the entry's `manifest_url` (where
`tools/publish-plugin.sh` writes it), only when that URL is on the same host as the marketplace index, with
redirects held to that host and the body capped at 256 KiB. It is best effort: no changelog never blocks.

Config schema mismatches are **warnings**. Plugin schemas are of uneven quality, so the real test is the
plugin's own `Initialize` (below).

### What the upgrade enforces

1. The plan is rebuilt server-side; a client-held preview is never trusted.
2. The target must come from the same OCI repository and marketplace ID, and its manifest `id` must equal the
   installed one. An older target needs `allow_downgrade: true`. Enterprise-only targets are refused on CE.
3. **Scopes fail closed.** Every scope the target adds over the currently *approved* set must be listed in
   `approved_scopes`, else `409` with `missing_scopes`. Scopes the target no longer declares are dropped.
   A plugin that was authorized stays authorized throughout: there is no window where a running plugin
   loses service access.
4. **Health check with rollback.** If AI Studio runs the plugin (it is loaded, or it has a Studio-side hook),
   it is unloaded and loaded on the new artifact, which calls `Initialize` with the existing configuration.
   If that fails, the previous columns are restored, the previous version is started again (its artifact is
   still in the digest-addressed OCI cache) and the call returns `422` with `rolled_back: true`. Plugins
   AI Studio does not run (gateway-only hooks, inactive plugins) were already started once by the probe.
5. On success: RBAC resources of the new manifest are synced, the previous command's cached config schema is
   dropped if no other plugin uses it, the tracking row is reconciled, and `system.plugin.updated` is emitted.

Related fix: `RegisterPluginUI` now refreshes `plugins.manifest` when the manifest **version** changes, not
only when its ID does. Without that a reloaded newer version kept the old manifest on the row, and its new
RBAC resources were never registered.

### Edge gateways

`system.plugin.updated` changes the namespace snapshot checksum, so edges go **Pending** until the
configuration is pushed. An edge reloads a plugin when its `checksum` or config differs from what it has
loaded. The config is deliberately unchanged by an upgrade, so the upgrade writes the target's OCI digest to
`plugins.checksum`; that is what makes an edge reload the plugin and pull the new artifact. For OCI plugins
`checksum` therefore holds the **artifact digest**, not a hash of the binary. This works with edges that are
already deployed.

## Limits and follow-ups

- Only OCI plugins linked to a marketplace entry can be upgraded this way.
- `min_studio_version` is reported, not enforced (see `plugins-publishing.md`).
- Auto-update (`installed_plugin_versions.auto_update`) is not implemented.
- In a multi-node AI Studio deployment only the node that served the request restarts the plugin process,
  the same limitation as reloading a plugin today.
- The edge reconcile does not compare `command`, so a *hand-edited* command that leaves checksum and config
  alone is not reloaded on edges. Upgrades are unaffected.
- No notification is raised when a sync finds new updates; the Plugins page is where they show.
