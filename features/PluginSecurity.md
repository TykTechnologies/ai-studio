# Plugin Security

**⚠️ ADVANCED FEATURES AVAILABLE IN ENTERPRISE EDITION ONLY**

Plugin security provides multiple layers of protection against malicious or misconfigured plugins. The system implements a **4-tier security model** with basic features in Community Edition and advanced features in Enterprise Edition.

---

## Table of Contents

- [Security Features Overview](#security-features-overview)
- [Community Edition Features](#community-edition-features)
- [Enterprise Edition Features](#enterprise-edition-features)
- [Architecture](#architecture)
- [Security Checks](#security-checks)
- [Configuration](#configuration)
- [API Reference](#api-reference)

---

## Security Features Overview

### Security Tiers

```
┌─────────────────────────────────────────────────────────────┐
│ Tier 1: Filesystem Security (CE + ENT)                      │
│ • Path Whitelisting                                          │
│ • Symlink Blocking                                           │
│ • Executable Validation                                      │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 2: Integrity Security (CE + ENT)                       │
│ • Checksum Validation (SHA256)                               │
│ • File Modification Detection                                │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 3: Network Security (ENT ONLY)                         │
│ • GRPC Host Whitelisting                                     │
│ • Internal IP Blocking (10.x, 192.168.x, 127.x, ::1)        │
│ • SSRF Prevention                                            │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 4: Supply Chain Security (ENT ONLY)                    │
│ • OCI Signature Verification (in-process, sigstore-go)      │
│ • cosign v2 (.sig tag) and v3 (Sigstore bundle) formats      │
│ • Transparency-log (Rekor) checks, keyless support           │
│ • Multiple Public Key Support                                │
└─────────────────────────────────────────────────────────────┘
```

---

## Community Edition Features

### 1. Path Whitelisting (Tier 1 - Filesystem Security)

**Purpose**: Prevents plugins from loading executables from dangerous locations

**Implementation**: [microgateway/plugins/manager.go:784-865](../microgateway/plugins/manager.go#L784-L865)

**Default Allowed Directories**:
```go
/opt/microgateway/plugins
./plugins
plugins/
```

**Security Checks**:
- ✅ Shell metacharacter detection (`; | & $( ) { } [ ] < > ? * ! ~`)
- ✅ Path traversal prevention (`../`, `..\\`)
- ✅ Absolute path resolution
- ✅ Symlink blocking
- ✅ Executable permission verification

**Example**:
```bash
# ✅ ALLOWED
file://./plugins/my-plugin
file:///opt/microgateway/plugins/validator

# ❌ BLOCKED
file://../../../etc/passwd
file:///usr/local/bin/malicious  # Outside allowed dirs
file://$(whoami)                  # Shell metacharacters
```

**Development Bypass**:
```bash
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=1  # Disables path restrictions
```

---

### 2. Checksum Validation (Tier 2 - Integrity Security)

**Purpose**: Ensures plugin files haven't been modified after deployment

**Implementation**:
- AI Studio: [services/plugin_service.go](../services/plugin_service.go)
- Microgateway: [microgateway/internal/services/plugin_service.go](../microgateway/internal/services/plugin_service.go#L307-L337)

**Database Schema**:
```sql
plugins (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  command TEXT NOT NULL,
  checksum TEXT,  -- SHA256 hash (optional)
  ...
)
```

**How It Works**:
1. Calculate SHA256 hash during plugin creation
2. Store hash in database
3. Validate hash before plugin loading (optional)
4. Detect file modifications

**Example**:
```go
// Calculate checksum
file, _ := os.Open(filePath)
hasher := sha256.New()
io.Copy(hasher, file)
calculatedChecksum := hex.EncodeToString(hasher.Sum(nil))

// Validate
if calculatedChecksum != plugin.Checksum {
    return fmt.Errorf("checksum mismatch: file has been modified")
}
```

**Tests**: [microgateway/internal/services/plugin_service_test.go:560-621](../microgateway/internal/services/plugin_service_test.go#L560-L621)

---

## Enterprise Edition Features

### 3. Internal plugin hosts and registries (Tier 3 - Network Security)

**Both editions** (midsommar-clean-c1g.32; before 2.3 only Community AI Studio resolved hostnames).

**Purpose**: Prevents plugins from targeting internal network addresses (SSRF protection), whether the plugin is a `grpc://` server or an `oci://` artifact on a registry.

**Rule** (`netguard.CheckPluginHost`; the security services' `ValidateGRPCHost` classify hosts the same way): unless `ALLOW_INTERNAL_NETWORK_ACCESS=true`, a host is refused when it is `localhost`/`*.localhost`, an internal IP literal (legacy IPv4 spellings such as `0x7f000001` included), a hostname that resolves to any internal address, or a hostname that does not resolve. Internal ranges (`netguard.IsInternalIP`): `10/8`, `172.16/12`, `192.168/16`, `100.64/10`, `127/8`, `169.254/16`, `0/8`, `::1`, `::`, `fc00::/7`, `fe80::/10`. Hostname lookups go through `netguard.LookupIP` (tests stub it with `netguardtest.StubLookupIP`).

**Where it applies** (Studio and Microgateway):
- API create/update (`plugin_security.ValidateCommand`, gateway `handlers/validation.go`), and the service layer (`plugin_security.CheckCommandHosts`: create, update, upgrade, management gRPC, marketplace install).
- Load of a stored plugin (`validateCommandForLoad`), and the gateway's `createPluginClient` for `grpc://` (the host and the address it resolved to, c1g.31).
- Every OCI client entry point: `ociplugins.CheckRegistryHost` in `FetchPlugin`, `GetPlugin` (cache hits included) and `NewRemoteRepository` (pulls, manifests, signature reads); gateway pre-fetch and pre-warm.
- At dial time for every OCI client connection (registry, signatures, token server) via `netguard.PluginHTTPTransport` / `PluginDialControl`, whatever `LLM_UPSTREAM_BLOCK_INTERNAL` says, against DNS rebinding. Behind a proxy the proxy endpoint is dialled unguarded and each request's target is name-checked (`CheckPluginHost`) in the transport's `Proxy` hook; no connect-time protection for the target then (c1g.33).
- Cache hits: `GetPlugin` loads a verified cache hit when the registry name only fails to resolve (`netguard.IsUnresolvedPluginHost`), with a warning; a positive internal resolution, a failed cache check or any fallthrough to `FetchPlugin` is refused. Studio's load-time `ValidateCommand` therefore skips the host check for `oci://` commands and leaves it to the OCI client (c1g.33).
- Marketplace indexes (2.3.0-rc5 re-test finding U2): `marketplace.NewFetcher` uses `netguard.PluginHTTPTransport`, so the index, manifest and changelog fetches (validate, scheduled and manual sync, the default source) are checked on the address dialled, redirects included. `marketplace.CheckIndexURL` applies `CheckPluginHost` when a source is added (the add handler), before anything is stored. Every fetch failure is the one `marketplace.ErrFetchFailed` (cause logged, never returned or stored as the sync error); an answer that is not YAML is `ErrInvalidDocument`, without the parser's quote of it. Answers are capped (32 MiB index, 1 MiB manifest). `POST /api/v1/admin/marketplaces/validate` needs `marketplace:write`.

**Opt-outs**: a host named in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (`netguard.PluginHostExempt`) is exempt from the check and dialled unguarded (preferred). `ALLOW_INTERNAL_NETWORK_ACCESS=true` (read at call time) is platform-wide: it allows every internal plugin host and OCI registry, on every path in AI Studio and the Microgateway; it turns off `LLM_UPSTREAM_BLOCK_INTERNAL`, so LLM upstreams, guardrail provider calls (Enterprise) and script HTTP calls (Enterprise) may reach internal addresses again; and it skips the internal-address check on submitted embed URLs. `PLUGIN_BLOCK_INTERNAL_URLS` is deprecated: still accepted, it only adds refusals for literals (including `host:port` arguments).

**Example**:
```bash
# ✅ ALLOWED (public addresses)
grpc://plugins.company.com:50051
oci://ghcr.io/org/plugin@sha256:...

# ❌ BLOCKED (internal addresses)
grpc://localhost:50051
grpc://10.0.0.5:8080
oci://registry.corp.internal/plugin:1   # resolves to 10.x
oci://localhost:5000/plugin:1
```

---

### 4. OCI Signature Verification (Tier 4 - Supply Chain Security)

**⚠️ ENTERPRISE EDITION ONLY**

**Purpose**: Verifies plugin authenticity using cryptographic signatures, in-process: no `cosign` binary is run or needed, so verification works in distroless images.

**Implementation**: [enterprise/features/plugin_security/signature_verifier.go](../enterprise/features/plugin_security/signature_verifier.go) (cosign v2), [bundle_verifier.go](../enterprise/features/plugin_security/bundle_verifier.go) (cosign v3, keyless), [trusted_root.go](../enterprise/features/plugin_security/trusted_root.go), built on sigstore-go and the core OCI client (`pkg/ociplugins`). The `sigstore-go` dependency is in the enterprise module only; Community binaries do not link it.

**Defaults**: `AI_STUDIO_OCI_REQUIRE_SIGNATURE` and `OCI_PLUGINS_REQUIRE_SIGNATURE` default to `true` (AI Studio's default was `false` before 2.3.0), as do the Helm charts.

**Signature formats** (a plugin verifies if either does; a failure reports both attempts):

| Format | Where it lives | Binding to the artifact |
|---|---|---|
| cosign v2 (what `tools/publish-plugin.sh` writes) | manifest tagged `sha256-<hex>.sig`; each `application/vnd.dev.cosign.simplesigning.v1+json` layer has the signature in `dev.cosignproject.cosign/signature` and an optional Rekor bundle in `dev.sigstore.cosign/bundle` | the signed payload's `critical.image.docker-manifest-digest` must equal the digest asked about (the `.sig` tag's location proves nothing); `critical.type` must be `cosign container image signature` |
| cosign v3 | a Sigstore bundle (`application/vnd.dev.sigstore.bundle.v0.3+json`) attached as an OCI referrer of the digest (referrers API, or the `sha256-<hex>` tag-schema fallback) | sigstore-go `WithArtifactDigest(sha256, digest)`; a DSSE bundle must be an in-toto statement with predicate `https://sigstore.dev/cosign/sign/v1` (attestations such as SLSA provenance or SBOMs are not signatures) |

**Which digest is verified**: the client (`pkg/ociplugins` `FetchPlugin`) resolves the reference once and passes the **top-level** digest it resolved to: the multi-arch index for a multi-arch plugin (what `publish-plugin.sh` signs), else the manifest. If the index has no valid signature, the **platform manifest that was pulled** (taken from the index content, never from a tag or second resolution) is verified with the same key; another platform's manifest is never consulted. A digest that is not `sha256:<64 hex>` fails closed. The cache records `ResolvedDigest`, `ManifestDigest` and `SignedDigest`; with signatures enforced a cache hit requires `ResolvedDigest` to be the requested digest and `SignedDigest` to be it or its platform manifest, so entries from earlier releases are verified once more.

**Transparency log** (`AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY`, `ociplugins.OCIConfig.EffectiveTlogPolicy()`):

| Policy | cosign v2 | cosign v3 |
|---|---|---|
| `required` (default) | the Rekor bundle's signed entry timestamp is checked offline against the trusted root's Rekor key for that log ID, within the key's validity window; the hashedrekord entry must record this payload's hash, this signature and **the verification key** | `WithTransparencyLog(1)` + `WithIntegratedTimestamps(1)` |
| `optional` | as `required` when the bundle annotation is present; accepted without one | as `required` when the bundle has tlog entries; key only otherwise |
| `disabled` | key only | key only (`WithCurrentTime`) |

Any other value fails closed. `optional` gives no protection against someone who can write to the registry and strip the entry; use it only in transition, and `disabled` only for air-gapped signing (`--tlog-upload=false`).

**Trusted root**: a snapshot of the Sigstore public-good `trusted_root.json` is embedded (`trusted_root.json`, refreshed with `refresh-trusted-root.sh` / `go generate`). `AI_STUDIO_OCI_TRUSTED_ROOT_FILE` / `OCI_PLUGINS_TRUSTED_ROOT_FILE` replace it (read with `ociplugins.ReadTrustedRootFile`: a regular file of at most 4 MiB); an invalid override fails closed and never falls back to the embedded root.

**Keyless** (`VerifyBundle(ctx, ref, issuer, subject)`): a cosign v3 bundle whose Fulcio certificate chains to the trusted root, with exact OIDC issuer and SAN, a tlog entry and an SCT. Nothing calls it today.

**Not supported**: cosign policy files. `VerifyWithPolicy` returns an error and is deprecated in both service interfaces.

**Fail closed**: with signatures required, an OCI client with no security service refuses every plugin (`no signature verifier configured`). Every OCI client in Studio and in both Microgateway service containers is built with the security service (`services.NewServiceWithOCI`, `Service.NewOCIPluginClient`, `plugins.NewPluginManagerWithOCI`, which requires it).

**Limits**: signature manifests, payloads and bundles are read with a 1 MiB cap; at most 32 simplesigning layers per `.sig` manifest and 16 bundle referrers are tried; key files must be regular files of at most 64 KiB.

**Public Key Resolution** (`ociplugins.ResolvePublicKey`, returns PEM bytes, no temp files):

```bash
# 1. Numbered keys
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
# Reference: "1" or ""  (uses first key)

# 2. Named keys
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_PROD="-----BEGIN PUBLIC KEY-----..."
# Reference: "CI" or "PROD"

# 3. File-based keys
OCI_PLUGINS_PUBKEY_FILE_COSIGN=/etc/ai-studio/cosign.pub
# Reference: "file:/etc/ai-studio/cosign.pub"

# 4. Direct file paths
# Reference: "/path/to/key.pub"

# 5. Environment variable reference
# Reference: "env:OCI_PLUGINS_PUBKEY_CI"

# 6. The embedded Tyk key: "embedded:default" (always the first default key)
```

**Which key is tried** (`OCIPluginClient.signatureKeys`): a command with `?pubkey=` is checked with that key only. A command without it (every marketplace install) is checked with each configured default key in turn (`DefaultPublicKeys`, from `LoadPublicKeysFromEnv`: the embedded Tyk key first, then `OCI_PLUGINS_PUBKEY_<n>`, named and file keys), for the index and then the platform-manifest fallback; the first key that verifies wins. Operators trust a third-party publisher by adding its key on Studio and every gateway, never by editing the install command; adding a key therefore lets it verify any unpinned plugin. The key that verified is recorded in the cache metadata (`SignatureKey`). A cache hit with `?pubkey=` needs that exact key recorded; one without needs the recorded key to still be configured, so removing a key revokes the plugins it verified (`""`, recorded by releases before this, is the default key and stays valid). The cache records the key ID, not the key's fingerprint, so replacing the PEM behind an existing name is not detected (midsommar-clean-0388).

**Core seams the Enterprise verifier uses** (`pkg/ociplugins`, documented in `doc.go`):
- `NewRemoteRepository(cfg, ref, authConfigName)`: the repository client plugin pulls use (insecure-registry list, registry auth, netguard SSRF guard).
- `WithAuthConfig` / `AuthConfigFrom`: the plugin reference's `?auth=` name, carried on the context to `VerifySignature`.
- `ResolvePublicKey`, `ParsePublicKey`, `PublicKeyPath`, `ReadTrustedRootFile`.
- `SignatureCapability`: a service that cannot verify (the Community one) reports it, and plugins load recorded as unverified.

**Community Edition Behavior**:
- ⚠️ Signatures are not verified; OCI plugins load and are recorded as unverified (`Verified` and `SignatureChecked` false), so an Enterprise build on the same cache verifies them again
- A warning is logged once per process when `RequireSignature` is set (the default)

---

## Plugin Source Lockdown (CE and ENT)

Hardened installations can limit which plugin deployment types they accept.
Each process enforces its own setting: a gateway refuses a source it does not
allow even when the hub pushes it.

| Process | Setting | Default |
|---------|---------|---------|
| AI Studio | `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` | empty = all sources |
| Microgateway | `PLUGINS_ALLOWED_SOURCES` | empty = all sources |

The value is a comma-separated list of `oci`, `file` and `grpc` (case and
spaces ignored, duplicates dropped), e.g. `PLUGINS_ALLOWED_SOURCES=oci`.
An unknown source (`ocii`) or a list naming none (`,`) stops startup with an
error naming the setting, so a typo never opens everything up or locks
everything out. A restricted policy is logged at INFO on startup.

**Classification** (`plugin_security.SourceOf`) matches how the plugin
managers dispatch a command, and both managers now dispatch through it:

| Command | Source |
|---------|--------|
| `oci://...` (case-sensitive prefix) | `oci` |
| `grpc://...` (case-sensitive prefix) | `grpc` |
| anything else: paths, `file://`, `http(s)://` | `file` (run as a local executable) |

Commands are not trimmed, lower-cased or parsed before classification, so
`OCI://x` or ` oci://x` are `file`, as they would be run.

**Types** ([services/plugin_security/sources.go](../services/plugin_security/sources.go)):
`ParseSourcePolicy(csv)`, `SourcePolicy.Check(command)` (returns
`*ErrPluginSourceNotAllowed`: `🔒 SECURITY: plugin source "file" is not
allowed by this installation (allowed: oci). Set <SETTING> to change this.`),
`Allowed()` (order oci, file, grpc), `AllowsAll()`. The zero `SourcePolicy`
allows all. The parsed policy travels on `plugin_security.Config.SourcePolicy`
(Studio: `AppConf.PluginSourcePolicy`; gateway: `PluginConfig.SourcePolicy`).

**Enforcement points (AI Studio)** — components read the policy from
configuration at check time (`services/plugin_source_policy.go`;
`SetSourcePolicy` overrides it), so one built without a setter or a security
service never fails open:

| Point | Where | Result |
|---|---|---|
| Plugin API create/update | `api/validation.go` `validatePluginCommand` | `400` |
| OCI create / refresh | `api/plugin_handlers.go` → `PluginService.CreateOCIPluginFromReference`, `RefreshOCIPlugin` | `400`, before any registry request |
| Service layer | `PluginService.validatePluginCommand` (create, marketplace upgrade plan), `UpdatePlugin` (command or OCI reference), `MarketplaceService.InstallFromMarketplace` | error |
| Manifest fetch | `PluginManifestService.ParsePluginManifest` | error, before any registry request |
| Load time | `AIStudioPluginManager.validateCommandForLoad` (load and config-only paths) | not spawned; WARN with plugin ID, name and source |

**Enforcement points (Microgateway)** — the policy is set on the plugin
manager by the shared `newPluginManager` builder both service containers use:

| Point | Where | Result |
|---|---|---|
| Management API | `internal/api/handlers/validation.go` (create/update), pre-warm handler | `400` |
| Every plugin load | `PluginManager.createPluginClient`, first statement (hub-pushed configs, global data-collection plugins, local config) | not loaded; WARN with plugin ID; `failed` in plugin health |
| OCI pre-fetch / pre-warm | `PreFetchOCIPlugin` (first statement), `PreWarmOCIPlugins` (skipped when `oci` is refused) | no registry request |

Existing plugin records with a refused source are kept (readable, editable,
deletable) but do not load and cannot be saved with that command.

**UI**: `GET /api/v1/plugin-policy` (plugins read permission) returns
`plugin_allowed_sources` and `oci_require_signature`; it is not on the public
`/auth/config`. The plugin wizard and form list only the allowed sources in
their help text, reject a refused command client-side with the backend's
wording, and note that OCI artifacts must be signed when they must. The
marketplace disables install and upgrade, with a tooltip naming
`AI_STUDIO_PLUGIN_ALLOWED_SOURCES`, when `oci` is refused. The client-side
`pluginSourceOf` (`ui/admin-frontend/src/admin/services/pluginService.js`)
uses the same rules as `SourceOf`.

**Helm**: `config.pluginAllowedSources` (AI Studio) and
`microgateway.config.pluginsAllowedSources` (gateways), empty by default.

---

## A Plugin That Cannot Be Loaded Refuses Traffic (Microgateway)

Every check above ends in "the plugin is not loaded". On an edge that used to
mean the LLM's requests ran without **any** of its pre_auth and post_auth
plugins: the hooks read a `GetPluginsForLLM` error as "no plugins"
(2.3.0-rc4 finding M1: an llm-firewall block bypassed by attaching a second,
unsigned plugin). Now:

- `PluginManager.GetPluginsForLLM` returns an error wrapping
  `*plugins.PluginLoadError` when a plugin of the requested hook type cannot be
  loaded, never a shorter list.
- The pre_auth and post_auth hooks (`microgateway/internal/api/auth_hooks.go`,
  `respondPluginsUnavailable`) refuse the request on any error from it: `503`,
  `Retry-After: 30`, `{"error":{"message","type":"api_error","code":"plugin_load_failed"}}`
  (`api.PluginLoadFailedCode`), in AWS's error shape on a Bedrock runtime
  call. The message names the LLM; the plugin and the cause are only logged.
  There is no setting that restores the old behavior.
- **Remembered failures** (`microgateway/plugins/load_failures.go`): a failed
  load is remembered for `PluginLoadRetryInterval` (30 s). Request-path loads
  (`loadPluginForRequest`, used by `GetPluginsForLLM` and `GetAuthPlugins`)
  return the remembered error without an OCI fetch, signature check, exec or
  connect, and concurrent requests for one plugin share one attempt
  (singleflight). Once the failure is older than the interval, the next
  request starts one attempt in the background and is refused with the
  remembered error, so no request waits on a retry. A request for a plugin
  with no remembered failure (its first load) waits for the load at most
  `pluginLoadWaitTimeout` (20 s). `LoadPlugin`, `ReloadPlugin` and
  `ReconcilePlugins` (a configuration sync) always try, and their outcome
  replaces what is remembered. Reconcile drops the entries of removed plugins.
- **Locking** (2.3.0-rc4 finding M2: one unreachable `grpc://` plugin stalled
  every request on the edge for minutes): `PluginManager.mu` guards the
  manager's maps and is never held across a load's slow steps (OCI fetch,
  process start, connect, `Initialize`) or an unload's RPCs (`CloseSession`,
  `Shutdown`). Loads and unloads of one plugin are serialized by its entry in
  `pluginLoadLocks`; lock order is the plugin's load lock, then `mu`.
  `UnloadPlugin` removes the plugin from the maps first and then stops it.
- **grpc:// connect bounds** (`connectWithRetry`): 3 attempts, 2 s apart, each
  a probe dial of the checked address with a 5 s timeout
  (`probeGRPCPlugin`; go-plugin's own reattach check is a `net.Dial` with no
  timeout) and a ping with a 5 s timeout: 19 s for a host that drops packets,
  34 s at most.
- **Logging**: `rememberLoadFailure` logs at ERROR with the plugin ID, name
  and cause when a failure starts or its cause changes, and
  `forgetLoadFailure` at INFO when the plugin loads again. Refused requests
  log at debug level.

- **Readiness** (2.3.0-rc5 re-test finding U3): a failed or loading plugin
  does not fail the edge's `/ready`. `ServiceContainer.Health()` checks the
  database only; `ReadinessCheck` answers `200` and lists such plugins in
  `plugins_not_ready` (`PluginManager.PluginsNotReady`: id, name and status,
  no command or error text, since `/ready` is unauthenticated). Failing
  readiness took every pod of an edge out of its Kubernetes Service for one
  LLM's plugin. `ServiceContainer.PluginsReady()` keeps the old check for
  the detailed health status (`GetDetailedHealthStatus`).

Not covered: auth plugins keep their rule (a `503` only when none of the
endpoint's auth plugins loads), a plugin that is loaded but unhealthy is still
skipped, and response hooks still fail open on a lookup error.

---

## Secret configuration fields: redacted on read (CE and ENT)

A plugin's configuration is free-form JSON described by the plugin's JSON Schema. The management API never returns the value of a secret field (midsommar-clean-6bg5.9).

- **Which fields.** With a cached schema (`plugin_config_schemas`, by command; read from the cache only, a read never calls the plugin, and an invalidated entry is still used): every position marked `format: password` or `writeOnly: true`, the two markers the edit form masks (`SchemaFormRenderer.isSecretProperty`). The walk follows the value through nested objects, array items (`items`, tuple items, `prefixItems`), `additionalProperties`, `patternProperties` (a pattern that does not compile counts as matching), local `$ref`s, `allOf`/`anyOf`/`oneOf` and `then`/`else`; a position is secret when any applicable branch marks it, and a position whose subschemas the walk could not all follow (more than 64) is treated as secret. Without a usable schema: string values under a key whose name contains `password`, `passwd`, `secret`, `token`, `apikey`/`api_key`, `private_key`, `credential` or `authorization`. Remote `$ref`s are not followed.
- **What is returned.** `services.REDACTED_VALUE` (`[redacted]`), the marker the other credential fields of the API use. An empty value and a `$SECRET/` or `$ENV/` reference are returned as they are. A non-string value at a secret position is replaced whole.
- **One place.** `services/plugin_config_redaction.go` (`PluginConfigSecrets.RedactPluginConfig`, `RestorePluginConfig`). In `api/`, `serializePlugin` takes a `pluginRedactor` argument, so no call site can serialise a plugin without one; the three hand-built `PluginResponse` blocks (plugin list, `serializeLLM`, `serializeLLMs`) and the per-LLM override routes use the same redactor. `SystemEventEmitter` redacts the plugin a `system.plugin.created/updated` event carries. `redactAgent` redacts the plugin embedded in an agent's response (list, get, create, update) and the agent's own configuration, which follows the same schema; agent create and update restore the marker like a plugin write. Rows served as stored that embed a plugin through a relationship are redacted in place before they are written out: the UI registry entries of `GET /plugins/ui-registry` and `GET /common/plugins/portal-ui-registry` (`redactRegistryPlugins`: the configuration is redacted for a caller who passes `mayReadPluginConfig`, i.e. holds `plugins:read`, and for everyone else, and always on the portal route, both the configuration (nil) and the command (empty) are left out, because schema-driven redaction cannot protect a secret under an unmarked key and a command line can hold a credential; found live in rc5 testing, the routes had answered 500 in the route sweep because the test service had no manifest service) and plugin schedules (`redactSchedulePlugin`: schedule detail, create and update). The route sweep in `api/plugin_config_redaction_test.go` now runs with a manifest service, seeds registry rows, a resource type and a schedule, and requires 200 from these routes. An LLM that leaves the process (`models.RedactLLMCredentials`: `system.llm.*` events, cluster_events, object hooks, an App's LLMs) lists its plugins **without** their configuration; a hook's result gets it back by plugin id (`RestoreLLMCredentials`). `api/plugin_config_redaction_test.go` calls every GET route under `/api/v1` and `/common` read from the router (streams and exports aside), as an authenticated administrator with permission checks on, and searches the bodies for the secrets; the routes known to carry a plugin's configuration (plugins, LLMs, the override, agents) must answer 200 and still carry its readable fields, so an empty 401 or 404 cannot pass as "no secret".
- **Writes** (`POST /plugins`, `POST /plugins/oci`, `PATCH /plugins/:id`, `PUT /llms/:id/plugins/:pluginId/config`) go through `restorePluginConfig` before the secret-reference check: the marker at a secret position becomes the value stored at that position; with nothing stored there the key is dropped (the marker is never stored); the marker at any other position is `400` with the field path and no value. Inside a list, an entry containing the marker must equal the redacted form of the stored entry at the same index, and a list of secret strings must keep its length, so deleting or reordering entries cannot hand one entry's secret to another.
- **Not changed.** The stored configuration, what the plugin process receives, the edge sync path and the config checksum. The plugin management gRPC API still returns a plugin its own configuration in clear (it already returns no other plugin's). A plugin schedule's own `config` is not covered.
- **Known limit.** A holder of plugin write can change a readable sibling (a host) while keeping the stored secret with the marker, and so send it to a server they control. Plugin write already allows changing the command, so this grants nothing new; lists are stricter (see above).

---

## Architecture

### Service Layer Pattern

```
┌─────────────────────────────────────────────────────────────┐
│ services/plugin_security/                                    │
│ ┌─────────────┐  ┌──────────┐  ┌──────────┐                │
│ │ interface.go│  │ types.go │  │ errors.go│  (Public)       │
│ └─────────────┘  └──────────┘  └──────────┘                │
│ ┌─────────────┐  ┌───────────────────┐                      │
│ │ factory.go  │  │ community.go      │  (Public)            │
│ └─────────────┘  └───────────────────┘                      │
└─────────────────────────────────────────────────────────────┘
                          ↓
        ┌─────────────────┴─────────────────┐
        │                                    │
┌───────▼────────┐                 ┌────────▼────────┐
│ Community Ed   │                 │ Enterprise Ed   │
│ (No-op stub)   │                 │ (Full security) │
└────────────────┘                 └─────────────────┘
     ⚠️ Allows                          🔒 Enforces
     all ops                            all checks
```

### Enterprise Implementation

```
enterprise/features/plugin_security/
├── service.go              # Main service implementation
├── init.go                 # Factory registration
├── grpc_validator.go       # GRPC host whitelisting
├── signature_verifier.go   # OCI signature verification
└── errors.go               # Enterprise error types
```

### Integration Points

**AI Studio Control Plane**:
```
main.go
  ↓ Creates auth.Config with OCIConfig
api/api.go
  ↓ Initializes plugin_security.Service
  ↓ Passes to validation functions
api/validation.go
  ↓ Uses securityService.ValidateGRPCHost()
api/plugin_handlers.go
  ↓ Calls validatePluginCommand(cmd, securityService)
```

**Microgateway Data Plane**:
```
cmd/microgateway/main.go
  ↓ Creates config.Config with OCIPlugins
internal/services/container.go
  ↓ Initializes plugin_security.Service
  ↓ Calls pluginManager.SetSecurityService()
plugins/manager.go
  ↓ Sets securityService on ociClient
pkg/ociplugins/client.go
  ↓ Uses securityService.VerifySignature()
```

---

## Security Checks

### Validation Flow

```
Plugin Command (e.g., "grpc://localhost:50051")
          ↓
┌─────────────────────────────────────────────┐
│ API Layer Validation                        │
│ (api/validation.go)                         │
│ - Command length check                      │
│ - Path traversal detection                  │
│ - Command injection detection               │
│ - URL parsing                                │
│ - Scheme validation                         │
│ - GRPC host validation (uses service) ← ENT │
└─────────────────────────────────────────────┘
          ↓
┌─────────────────────────────────────────────┐
│ Service Layer Validation                    │
│ (services/plugin_service.go)                │
│ - Plugin conflict check                     │
│ - Database validation                       │
└─────────────────────────────────────────────┘
          ↓
┌─────────────────────────────────────────────┐
│ Manager Layer Security                      │
│ (plugins/manager.go)                        │
│ - Path whitelisting ← CE                    │
│ - Checksum validation ← CE                  │
│ - OCI signature verification ← ENT          │
└─────────────────────────────────────────────┘
```

### Security Decision Matrix

| Plugin Command | CE Security | ENT Security |
|---------------|-------------|--------------|
| `file://./plugins/my-plugin` | ✅ Path check<br>✅ Checksum | ✅ Path check<br>✅ Checksum |
| `grpc://localhost:50051` | ⚠️ Allowed | ❌ Blocked (internal IP) |
| `grpc://plugins.company.com:50051` | ✅ Allowed | ✅ Allowed (public) |
| `oci://registry/plugin:v1` (unsigned) | ✅ Downloaded | ❌ Blocked (no signature) |
| `oci://registry/plugin:v1` (signed) | ✅ Downloaded | ✅ Verified + Downloaded |

---

## Community Edition Features

### Path Whitelisting

**File**: [microgateway/plugins/manager.go:784-865](../microgateway/plugins/manager.go#L784-L865)

**Validations Performed**:
1. Shell metacharacter detection
2. Path traversal prevention
3. Directory whitelist enforcement
4. Symlink rejection
5. Executable permission check

**Code**:
```go
func validatePluginPath(cmdPath string) error {
    // 1. Check for shell metacharacters
    if shellMetacharPattern.MatchString(cmdPath) {
        return fmt.Errorf("🔒 SECURITY: Plugin command contains shell metacharacters")
    }

    // 2. Resolve to absolute path
    absPath, err := filepath.Abs(cmdPath)

    // 3. Check if path is within allowed directories
    if !isPathInAllowedDirectories(absPath) {
        return fmt.Errorf("🔒 SECURITY: Plugin path not in allowed directories")
    }

    // 4. Reject symbolic links
    if fileInfo.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("🔒 SECURITY: Plugin cannot be a symbolic link")
    }

    // 5. Verify file is executable
    if fileInfo.Mode()&0111 == 0 {
        return fmt.Errorf("🔒 SECURITY: Plugin file is not executable")
    }
}
```

---

### Checksum Validation

**File**: [microgateway/internal/services/plugin_service.go:307-337](../microgateway/internal/services/plugin_service.go#L307-L337)

**Database Field**:
```go
type Plugin struct {
    Checksum string `json:"checksum"` // SHA256 hash
}
```

**Usage**:
```go
// Calculate checksum during creation
hasher := sha256.New()
io.Copy(hasher, file)
plugin.Checksum = hex.EncodeToString(hasher.Sum(nil))

// Validate before loading
if err := pluginService.ValidatePluginChecksum(pluginID, filePath); err != nil {
    return fmt.Errorf("plugin file has been modified: %w", err)
}
```

**Tests**: [microgateway/internal/services/plugin_service_test.go:560-621](../microgateway/internal/services/plugin_service_test.go#L560-L621)

---

## Enterprise Edition Features

### Internal plugin hosts and registries

**Files**: [pkg/netguard/plugin_hosts.go](../pkg/netguard/plugin_hosts.go), [pkg/ociplugins/fetcher.go](../pkg/ociplugins/fetcher.go) (`CheckRegistryHost`), [services/plugin_security/command_validation.go](../services/plugin_security/command_validation.go), [enterprise/features/plugin_security/grpc_validator.go](../enterprise/features/plugin_security/grpc_validator.go)

**Attack Prevention**:
- **SSRF (Server-Side Request Forgery)**: Prevents malicious plugins from accessing internal services
- **Cloud Metadata Attacks**: Blocks access to cloud provider metadata endpoints (169.254.169.254, 100.100.100.200)
- **Internal Service Discovery**: Prevents scanning of internal networks
- **DNS rebinding**: registry connections are checked on the address being dialled (not behind a proxy, where the proxy controls egress)

**Error Messages** (both editions; name the host and the setting):
```
plugin host "registry.corp.internal" is an internal address, resolves to one, or does not resolve; add it to LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS to allow this host, or set ALLOW_INTERNAL_NETWORK_ACCESS=true to allow every internal host
```

---

### OCI Signature Verification

See [Tier 4](#4-oci-signature-verification-tier-4---supply-chain-security) for the design. Failures are `*ociplugins.ErrSignatureVerificationFailed` with a reason naming each format tried and why it failed, never key material or tool output:

```
with key "1": cosign v3 bundle: no signatures found; cosign v2 signature (sha256-<hex>.sig): no valid signature: layer 0: signature is for sha256:<other>, not sha256:<hex>
```

---

## Configuration

### Environment Variables

**Community Edition**:
```bash
# Path whitelisting (CE)
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=0  # 0=enabled, 1=disabled (dev only)
```

**Enterprise Edition**:
```bash
# GRPC host whitelisting (ENT)
ALLOW_INTERNAL_NETWORK_ACCESS=false  # false=enforced, true=bypassed (dev only)

# OCI signature verification (ENT)
AI_STUDIO_OCI_REQUIRE_SIGNATURE=true      # AI Studio default (since 2.3; was false)
OCI_PLUGINS_REQUIRE_SIGNATURE=true        # Microgateway default (strict)

# Transparency-log (Rekor) policy (ENT): required | optional | disabled
AI_STUDIO_OCI_TLOG_POLICY=required        # AI Studio (default required)
OCI_PLUGINS_TLOG_POLICY=required          # Microgateway (default required)

# Sigstore trusted root override (ENT): a trusted_root.json replacing the
# embedded Sigstore public-good root (private Sigstore, Rekor key rotation)
AI_STUDIO_OCI_TRUSTED_ROOT_FILE=/etc/ai-studio/trusted_root.json
OCI_PLUGINS_TRUSTED_ROOT_FILE=/etc/microgateway/trusted_root.json

# Public keys (ENT)
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_FILE_COSIGN=/etc/keys/cosign.pub
```

**Transparency-log policy** (`ociplugins.OCIConfig.TlogPolicy`, read through
`EffectiveTlogPolicy()`, which treats empty as `required`):

| Policy | Meaning |
|--------|---------|
| `required` (default) | The signature must have a transparency-log (Rekor) entry, checked offline against the trusted root's Rekor key. Matches `cosign verify --key`. |
| `optional` | A transparency-log entry is checked when the signature has one; a signature without one is accepted. |
| `disabled` | Key-only verification, for air-gapped signing (`cosign sign --tlog-upload=false`). A WARN is logged once at startup. |

**Trusted root** (`TrustedRootFile`): empty uses the Sigstore public-good
trusted root embedded in the Enterprise verifier. When set, the file must be
readable.

Both settings are security settings: an unknown policy or an unreadable
trusted-root file refuses to start (Studio: `AppConf.ConfigErrors`, checked by
`studio.New`; gateway: `Config.Validate`), with an error naming the variable.
So does an invalid `AI_STUDIO_OCI_REQUIRE_SIGNATURE` / `OCI_PLUGINS_REQUIRE_SIGNATURE`.

### Code Configuration

**AI Studio** ([config/oci_config.go](../config/oci_config.go)):
```go
type OCIConfig struct {
    RequireSignature bool  `env:"AI_STUDIO_OCI_REQUIRE_SIGNATURE" envDefault:"true"`
    // ... other fields
}
```

**Microgateway** ([microgateway/internal/config/plugin_config.go](../microgateway/internal/config/plugin_config.go)):
```go
type OCIPluginConfig struct {
    RequireSignature bool  `env:"OCI_PLUGINS_REQUIRE_SIGNATURE" envDefault:"true"`
    // ... other fields
}
```

---

## API Reference

### Plugin Security Service Interface

**Location**: [services/plugin_security/interface.go](../services/plugin_security/interface.go)

```go
type Service interface {
    // GRPC host validation (ENT only)
    ValidateGRPCHost(host string) error
    IsInternalIP(host string) bool

    // Signature verification (ENT only)
    VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error
    VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error
    VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error

    // Public key management (ENT only)
    GetPublicKeyPath(pubKeyID string) (string, error)
    ValidatePublicKey(keyPath string) error
    LoadPublicKeysFromDirectory(dir string) ([]string, error)
}
```

### Community Edition Implementation

**Location**: [services/plugin_security/community.go](../services/plugin_security/community.go)

**Behavior**: All methods return nil (allow) or empty values

```go
// ValidateGRPCHost always returns nil in CE
func (s *communityService) ValidateGRPCHost(host string) error {
    s.logSecurityWarning("GRPC host whitelisting")
    return nil  // Allow all hosts
}

// VerifySignature always returns nil in CE
func (s *communityService) VerifySignature(ctx, ref, pubKeyID) error {
    s.logSecurityWarning("OCI signature verification")
    return nil  // Skip verification
}
```

### Enterprise Edition Implementation

**Location**: `enterprise/features/plugin_security/service.go`

**Behavior**: Full security enforcement with proper validation

```go
// ValidateGRPCHost blocks internal IPs in ENT
func (s *enterpriseService) ValidateGRPCHost(host string) error {
    return s.grpcValidator.ValidateHost(host)  // Returns error if internal
}

// VerifySignature verifies cosign v3 bundles and cosign v2 signatures
// in-process (sigstore-go) in ENT
func (s *enterpriseService) VerifySignature(ctx, ref, pubKeyID) error {
    return s.signatureVerifier.Verify(ctx, ref, pubKeyID)
}
```

---

## Edition Detection

**Check if enterprise security is available**:
```go
import "github.com/TykTechnologies/midsommar/v2/services/plugin_security"

if plugin_security.IsEnterpriseAvailable() {
    // Enterprise Edition
    log.Println("✅ Advanced plugin security enabled")
} else {
    // Community Edition
    log.Println("⚠️  Using basic plugin security - upgrade to Enterprise for advanced features")
}
```

---

## Security Best Practices

### Production Deployments

**Community Edition**:
1. ✅ Enable path whitelisting (default)
2. ✅ Enable checksum validation (recommended)
3. ⚠️  Understand GRPC host whitelisting is disabled
4. ⚠️  Understand OCI signature verification is disabled
5. 🔒 Consider upgrading to Enterprise for production

**Enterprise Edition**:
1. ✅ Enable all security features (defaults)
2. ✅ Configure public keys for OCI verification
3. ✅ Set `ALLOW_INTERNAL_NETWORK_ACCESS=false` (default)
4. ✅ Enable `RequireSignature=true` for OCI plugins
5. 🔒 Use policy-based verification for advanced control

### Development Environments

**Bypass Settings (Use with Caution)**:
```bash
# Bypass path restrictions (CE)
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=1

# Bypass GRPC host whitelisting (ENT)
ALLOW_INTERNAL_NETWORK_ACCESS=true

# Disable signature verification (ENT)
OCI_PLUGINS_REQUIRE_SIGNATURE=false
AI_STUDIO_OCI_REQUIRE_SIGNATURE=false
```

**⚠️ WARNING**: Never use bypass settings in production!

---

## Upgrade Path

### From CE to ENT

When upgrading from Community Edition to Enterprise Edition:

1. **No Database Migration Required** - Security settings are environment-based
2. **Configure Public Keys** - Set OCI_PLUGINS_PUBKEY_* variables
3. **Review Plugin Commands** - Ensure no internal IP targets
4. **Test Plugin Loading** - Verify signature verification works
5. **Update Monitoring** - Watch for security violation logs

### Backward Compatibility

- ✅ CE configurations work in ENT
- ✅ Plugin database models unchanged
- ✅ API endpoints remain consistent
- ✅ No breaking changes to plugin SDK

---

## Troubleshooting

### Common Issues

**CE: "Plugin Security: GRPC host whitelisting is disabled"**
- **Cause**: Using Community Edition
- **Solution**: Upgrade to Enterprise Edition or accept reduced security

**"plugin host ... is an internal address, resolves to one, or does not resolve"**
- **Cause**: The plugin's gRPC host or OCI registry is, or resolves to, an internal address, or does not resolve (both editions)
- **Solution**:
  - Use a host or registry with a public address
  - Name the host in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS`, or set `ALLOW_INTERNAL_NETWORK_ACCESS=true` (platform-wide)

**ENT: "signature verification failed ... no signatures found"**
- **Cause**: Plugin image not signed (neither a cosign v3 bundle nor a cosign v2 `.sig` tag)
- **Solution**:
  - Sign plugin: `cosign sign --key cosign.key registry/plugin@sha256:<digest>` (or `tools/publish-plugin.sh`)
  - Or disable verification: `AI_STUDIO_OCI_REQUIRE_SIGNATURE=false` / `OCI_PLUGINS_REQUIRE_SIGNATURE=false` (not recommended)

**ENT: "no transparency-log entry, which ..._TLOG_POLICY=required needs"**
- **Cause**: Signed with `--tlog-upload=false`
- **Solution**: Set `AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` to `disabled` (air-gapped signing)

**"no signature verifier configured"**
- **Cause**: An OCI client was built without the plugin security service while signatures are required (a wiring bug; every client should have it)

**ENT: "public key not found"**
- **Cause**: Public key reference not configured
- **Solution**: Set `OCI_PLUGINS_PUBKEY_1` or relevant key variable

---

## Related Documentation

- [Plugin System Overview](./README.md)
- [Plugin SDK](../pkg/plugin_sdk/README.md)
- [OCI Plugin Distribution](../microgateway/docs/extensibility/plugin-distribution.md)
- [Enterprise Framework](../ENTERPRISE_FRAMEWORK.md)

---

## Testing

### Community Edition Tests

**Location**: Tests are integrated into existing plugin tests

**Coverage**:
- ✅ Path whitelisting enforcement
- ✅ Checksum validation (valid, invalid, missing)
- ✅ Plugin loading with basic security

### Enterprise Edition Tests

**Location**: `enterprise/features/plugin_security/` (`-tags enterprise`)

**Coverage**:
- ✅ GRPC host validation (internal IPs, public IPs, localhost)
- ✅ cosign v2 and v3 signatures with generated keys against an in-process registry (`pkg/ociplugins/ocitest`), wrong key, wrong digest, tampered payload, attestations, size and count caps
- ✅ Transparency-log policies, Rekor SET and key binding (test trusted roots, sigstore-go's virtual Sigstore)
- ✅ Real cosign v2.4.1 and v3.1.4 signatures from `testdata/` (offline), a copied signature failing, and a network-gated check against a published plugin (`TYK_PLUGIN_SIGNATURE_NETWORK_TEST=1`)
- ✅ Keyless `VerifyBundle`; `VerifyWithPolicy` refused
- ✅ No `os/exec` in the package (guard test)

---

## Summary

Plugin Security provides **defense in depth** with multiple security layers:

**Community Edition** provides **basic security** sufficient for development and low-risk deployments:
- Filesystem protection via path whitelisting
- File integrity via checksum validation

**Enterprise Edition** adds **advanced security** required for production deployments:
- Network protection via GRPC host whitelisting (SSRF prevention)
- Supply chain security via OCI signature verification (trusted sources)

The security model ensures that even CE users have fundamental protections, while ENT users get enterprise-grade security controls for production environments.
