# Plugin Security Model

This page describes what the plugin system does and does not protect against. Read it before you install a plugin, and before you set the plugin environment variables for a production deployment.

## Trust model in one paragraph

A plugin is a program you choose to run next to AI Studio or the Microgateway. Each plugin runs as a **separate operating-system process** and talks to its host over gRPC ([HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)). That separation means a plugin that crashes or hangs does not take the host down with it, and a plugin can reach the host's data only through the [Service API](./plugins-service-api.md), within the scopes its manifest declares and an administrator approved. It is **not** a sandbox: plugins run as separate processes with the host's privileges. Install only plugins you trust, and treat installing one like deploying code to the host.

## What process isolation gives you

| Property | Provided | Notes |
|---|---|---|
| Crash isolation | Yes | A plugin panic or exit ends the plugin process only; the host logs it and carries on. |
| Scoped access to Studio data | Yes | Service API calls (KV, apps, LLMs, users, and so on) are checked against the scopes in the plugin's manifest. See [Manifests & Permissions](./plugins-manifests.md). |
| Restricted environment | Yes | A plugin receives an allowlist of host environment variables, not the host's whole environment. See [Plugin process environment](#plugin-process-environment). |
| CPU and memory limits | No | Apply them at the container or OS level if you need them. |
| Filesystem isolation | No | A plugin can read and write whatever the host's user can. |
| Network egress filtering | No | See [Network egress](#network-egress). |
| Secret redaction in plugin config | Partial: `$SECRET/` references are resolved for the plugin (in AI Studio, and encrypted in transit to edges), and the stored config keeps the reference; literal values are not redacted | See [Plugin configuration and secrets](#plugin-configuration-and-secrets). |

## Network egress

The platform does not filter a plugin's outbound network traffic. The SDK's `DefaultHTTPClient` is a convenience, not a control: a plugin can create its own HTTP client or open raw sockets, and can reach any address the host can, including internal services and cloud metadata endpoints.

If you need to restrict egress:

- **Kubernetes**: run AI Studio and the Microgateway with a `NetworkPolicy` that allows only the destinations your LLMs, tools and plugins need. Plugins share the pod's network namespace, so the policy applies to them too.
- **Docker / VMs**: use host firewall rules or an egress proxy for the container or VM.
- **Choose plugins carefully**: prefer plugins whose source you can review, or that are signed by a publisher you trust (see [OCI plugins](#oci-plugins-registries-digests-and-signatures)).

The internal-address checks described under [Command validation](#command-validation) apply to the **plugin command** (where the host looks for the plugin). They do not restrict what a running plugin connects to.

## Plugin process environment

AI Studio and the Microgateway start every `file://` and `oci://` plugin with an explicit environment instead of their own. A plugin receives:

| Variables | Purpose |
|---|---|
| `PATH`, `HOME`, `USER`, `TMPDIR`, `TZ`, `LANG`, `LC_*` | Process basics and locale |
| `SSL_CERT_FILE`, `SSL_CERT_DIR` | Custom CA bundles |
| `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (and the lower-case forms) | Outbound proxy settings |
| `GOMAXPROCS`, `GOMEMLIMIT`, `GOGC`, `GODEBUG`, `GOTRACEBACK` | Go runtime tuning |
| `PLUGIN_RUNTIME`, `GATEWAY_MODE`, `MICROGATEWAY_MODE`, `EDGE_ID` | Runtime detection and edge identity, read by the SDK and shipped plugins |
| `PLUGIN_GRPC_MAX_MSG_BYTES` | The largest gRPC message the plugin accepts (see [Plugin message size](./plugins-deployment.md#plugin-message-size)) |

go-plugin adds its own handshake variables on top. Anything else in the host's environment (database settings, encryption keys, provider keys, the license and so on) is not passed to plugins.

To give plugins more, list the variables in `PLUGIN_ENV_PASSTHROUGH` on the host, comma-separated. An entry ending in `*` matches a prefix, and one starting with `*` matches a suffix:

```bash
PLUGIN_ENV_PASSTHROUGH="GITHUB_RAG_*,SSH_KNOWN_HOSTS"
```

- The pass-through applies to **every** plugin the host starts, so list only what your plugins need.
- A bare `*` is ignored, so the whole environment can't be passed by accident.
- The host logs a warning when the pass-through lets one of these through: `ENCRYPTION_KEY`, `MICROGATEWAY_ENCRYPTION_KEY`, `DATABASE_URL`, `TYK_AI_SECRET_KEY`, `TYK_AI_LICENSE`, `EDGE_AUTH_TOKEN`, `GRPC_AUTH_TOKEN`, `JWT_SECRET`, or any name ending in `_API_KEY`, `_AI_KEY`, `_SECRET`, `_TOKEN` or `_PASSWORD`. The variable is still passed; the warning is there so the choice is deliberate.
- `ALLOW_INTERNAL_NETWORK_ACCESS` is not passed by default. A plugin that honours it for its own checks only sees it if you add it to the pass-through, for local development.
- Proxy variables are passed by default, so credentials embedded in a proxy URL are visible to plugins.

`grpc://` plugins run outside the host, so their environment is whatever you give their own process.

## Plugin configuration and secrets

Plugin configuration is stored in AI Studio's database and sent to the plugin process in plaintext when the plugin starts (`Initialize`, `InitRequest.Config`). Every value is passed as a string; arrays and objects are JSON-encoded.

**Secret references.** A top-level string value that is exactly `$SECRET/NAME` is resolved from AI Studio's secret store before it is passed to a plugin running in AI Studio (since 2.3). The database and the configuration snapshot keep the reference. A reference inside a longer string, or inside an array or object, is not resolved, and a reference that cannot be resolved is passed through unchanged (and logged).

Because the plugin receives the secret's value, adding a `$SECRET/` reference to a plugin's configuration needs the right to read secrets (`secrets` read; administrators hold it). Plugins themselves cannot add references through the management API (`UpdatePluginConfig` refuses them), and a plugin that saves back the resolved value of a reference it was given keeps the reference rather than storing the secret.

`$ENV/NAME` is **not** resolved by AI Studio: its own environment holds platform secrets such as the encryption key and the database URL. A plugin may resolve `$ENV/NAME` itself, against the environment it is given (the default allowlist and `PLUGIN_ENV_PASSTHROUGH`).

**Edges receive plugin configuration.** The configuration snapshot that AI Studio sends to every edge gateway includes the stored configuration of every active plugin, Studio-only plugins included. A literal credential therefore reaches every edge.

**Secret references on edges.** For a plugin that runs on edges (it has a gateway hook: `pre_auth`, `auth`, `post_auth`, `on_response`, `data_collection` or `custom_endpoint`), AI Studio also resolves each top-level `$SECRET/NAME` value and sends it encrypted with the edge encryption key (`MICROGATEWAY_ENCRYPTION_KEY`), the way LLM API keys travel. The plain configuration in the snapshot keeps the reference. The edge stores the value encrypted, decrypts it only for the plugin's `Initialize` configuration, and never returns it from its admin API. The rules:

- Only top-level string values that are exactly `$SECRET/NAME` are resolved, as in AI Studio. A nested reference reaches the plugin unresolved.
- A reference that cannot be resolved reaches the plugin as `refused-credential-reference`, never as the reference.
- `$ENV/NAME` is not resolved for edges: nothing from AI Studio's environment is sent.
- Studio-only plugins (UI, agent, object hooks) get no values on edges.
- A reference set in an LLM's per-plugin configuration override is not resolved, since writing an override does not need the right to read secrets. Put the reference in the plugin's own configuration.
- Changing a referenced secret changes the edge configuration checksum, so edges show as pending until the change is pushed; on the push the edge reloads the plugin with the new value.
- Edges released before this change ignore the encrypted values and keep receiving the reference unresolved.

What this means in practice:

- Anyone who can read the plugin's configuration in AI Studio (an administrator with plugin read access, or the database) can read a credential stored in it literally. Store a `$SECRET/` reference instead.
- The plugin process receives the credential in clear text, resolved or not. Only give a plugin a credential you would be comfortable giving to the plugin's author.
- **Plugin authors**: do not log configuration values, and do not echo them in errors or UI responses. If your plugin needs a credential, document which config field holds it, keep the field optional where you can, and consider letting operators supply it from a file mounted only into your deployment.

Optional hardening, such as marking config fields as secret and redacting them in transit, is tracked in [#445](https://github.com/TykTechnologies/ai-studio/issues/445).

## Command validation

A plugin's **command** says where the host gets the plugin: a local executable (`file://` or a plain path), an OCI artifact (`oci://`), or a remote gRPC server (`grpc://`). Commands are checked in two places, and the two behave differently.

### Enforced checks (always on)

These run when a plugin is created or updated through the API, and again when AI Studio loads a plugin, so a command that reached the database another way is checked before a process starts. A command that fails is **refused**. The internal-hosts check runs on more paths still; see the table.

| Check | Rule |
|---|---|
| Length | At most 1024 characters. |
| Path traversal | No `../` or `..\`. |
| Shell metacharacters | No `;`, `\|`, `&`, `` ` ``, `$(`, newlines or NUL. |
| Executable location | Local executables must be under `/usr/bin/`, `/bin/`, `/usr/local/bin/` or `/opt/plugins/`, or a relative path under `plugins/`. Add directories with `AI_STUDIO_PLUGIN_ALLOWED_DIRS` (a path list, `:`-separated on Linux). The Microgateway allows `/opt/microgateway/plugins` and `./plugins`, refuses symbolic links, and requires the file to be executable. |
| URL scheme | Only `grpc`, `oci`, `http` and `https` URLs. |
| Internal hosts | The host of a URL in the command must not be internal: a `grpc://` plugin's host and an `oci://` plugin's registry alike. The host is resolved, and a name that does not resolve is treated as internal (fail closed). Internal means loopback, RFC 1918 private ranges, shared address space (`100.64.0.0/10`, including the `100.100.100.200` cloud metadata endpoint), link-local (including `169.254.169.254` cloud metadata), `0.0.0.0/8`, the unspecified address (`::`), IPv6 unique-local and `localhost`. Community and Enterprise editions classify hosts the same way. The check runs on every path, in AI Studio and the Microgateway: create and update (API, management gRPC, marketplace install, upgrade), loading a stored plugin (even from the OCI cache), pre-warm and manifest reads, plugins an edge receives from AI Studio, and every connection to an OCI registry (checked on the address being dialled, so a DNS answer that changes later cannot reach an internal address). The error names the host and both settings. To allow one internal host or registry, name it in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (comma-separated; `.corp.internal` matches the domain and its subdomains); this is the preferred route. `ALLOW_INTERNAL_NETWORK_ACCESS=true` is platform-wide: it allows every internal plugin host and OCI registry, on every path in AI Studio and the Microgateway; it turns off `LLM_UPSTREAM_BLOCK_INTERNAL`, so LLM upstreams, guardrail provider calls (Enterprise) and script HTTP calls (Enterprise) may reach internal addresses again; and it skips the internal-address check on submitted embed URLs. |

### Advisory checks (warning-only by default)

AI Studio's plugin service, and the Microgateway's, also run these checks. By default they **only log a warning** (`PLUGIN SECURITY WARNING`) and let the command through:

| Check | Default | How to make it stricter |
|---|---|---|
| Absolute path outside `/usr/bin/`, `/bin/`, `/usr/local/bin/` | Warn | Rely on the enforced executable-location check above, which already refuses paths outside the allowed directories. |
| Internal address anywhere in the command: URL hosts, `host:port` arguments, `--flag=host:port` values, bare IP literals. Hosts are parsed, not substring-matched, so IPv4-mapped IPv6 and decimal, hex or octal IPv4 spellings in URLs are caught. Shared address space (`100.64.0.0/10`) also counts as internal here. | Warn | `PLUGIN_BLOCK_INTERNAL_URLS=true` refuses such commands. **Deprecated**: URL hosts are refused by the enforced internal-hosts check anyway; the variable is still accepted and only ever adds refusals (for `host:port` arguments, or a literal that `ALLOW_INTERNAL_NETWORK_ACCESS` or `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` would allow), never loosens the rule. |
| Command not in `PLUGIN_COMMAND_ALLOWLIST` | Warn (an info message when the variable is unset) | None today: the allowlist is advisory. Entries are plain substrings (no wildcards), comma-separated. Use it to make unexpected commands visible in the logs. |

Making the advisory checks blocking by default is a product decision tracked in [#427](https://github.com/TykTechnologies/ai-studio/issues/427).

### Edge gateways

A Microgateway in edge mode has no management API: it receives plugins from AI Studio through configuration sync, so a plugin reaches an edge only after it has passed AI Studio's API validation. A standalone Microgateway (no AI Studio) has its own management API, which runs the same enforced checks. In both modes the Microgateway validates local executable paths again when it loads a plugin, and applies the internal-hosts check to `grpc://` hosts and OCI registries from its own network position (a name can resolve differently there than on AI Studio). A refused plugin is marked failed in plugin health with the reason.

## OCI plugins: registries, digests and signatures

For production, OCI is the recommended way to distribute plugins.

- **Internal registries are refused**: a registry that is, or resolves to, an internal address, or does not resolve, is refused before any request (see the internal-hosts check under [Command validation](#command-validation)). This includes plugins already installed and cached. Registry, signature and token-server connections are also checked on the address being dialled, whatever `LLM_UPSTREAM_BLOCK_INTERNAL` is set to. To use a registry inside your network, name it in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` on AI Studio and on every Microgateway that loads its plugins. `ALLOW_INTERNAL_NETWORK_ACCESS=true` also works, but opens much more (see above).
- **Restrict registries.** Set `AI_STUDIO_OCI_ALLOWED_REGISTRIES` (AI Studio) and `OCI_PLUGINS_ALLOWED_REGISTRIES` (Microgateway) to a comma-separated list of registry hosts. An `oci://` command for any other registry is refused. With the variable unset, any registry is allowed.
- **Pin by digest.** `oci://registry.example.com/plugins/my-plugin@sha256:…` always loads the same bytes. A tag (`:v1.2.3`) can be moved to different content by anyone who can push to the repository.
- **Require signatures (Enterprise Edition).** AI Studio and the Microgateway require signatures by default (`AI_STUDIO_OCI_REQUIRE_SIGNATURE`, `OCI_PLUGINS_REQUIRE_SIGNATURE`, both default `true`; AI Studio's default was `false` before 2.3.0). Signatures are verified in-process, without the `cosign` binary, against the digest that was pulled: cosign v2 signatures (the `.sig` tag) and cosign v3 Sigstore bundles (OCI referrers) are both accepted, and an attestation signed with the same key is not. The Tyk signing key is always trusted. Add your own keys as `OCI_PLUGINS_PUBKEY_1`, `OCI_PLUGINS_PUBKEY_2`, … or `OCI_PLUGINS_PUBKEY_<NAME>` (key content), or `OCI_PLUGINS_PUBKEY_FILE_<NAME>` (a file path), and select one per plugin with `?pubkey=` on the command (`?pubkey=2`, `?pubkey=CI`).
  - **Transparency log.** By default a signature also needs a valid Rekor entry for the signing key (`AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` = `required`), checked offline against an embedded Sigstore trusted root (`*_TRUSTED_ROOT_FILE` replaces it). Keep `required`: `optional` does not protect against someone who can write to the registry and remove the entry. Use `disabled` only for air-gapped signing (`--tlog-upload=false`).
  - With signatures required and no verifier, a plugin is refused, never loaded unverified.
  - In **Community Edition** signatures are not verified, whatever these variables say: OCI plugins load recorded as unverified, and a warning is logged once at startup.
- **Protect the cache.** Pulled binaries are cached (`AI_STUDIO_OCI_CACHE_DIR`, `OCI_PLUGINS_CACHE_DIR`). Each cached binary's SHA-256 is checked against the value recorded when it was fetched, and a binary that changed on disk is fetched and verified again. Someone who can write to the cache directory can replace both the binary and its record, so keep the directory writable only by the service user.

See [Deployment Options](./plugins-deployment.md) for registry authentication and packaging.

## Hardening: restrict plugin sources

A hardened installation can refuse whole plugin deployment types. Each process enforces its own setting:

| Setting | Process | Values | Default |
|---|---|---|---|
| `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` | AI Studio | comma-separated `oci`, `file`, `grpc` (case and spaces ignored) | empty: all three |
| `PLUGINS_ALLOWED_SOURCES` | Microgateway | the same | empty: all three |

An unknown value (`ocii`) or a list that names no source (`,`) stops the process at startup, so a typo never opens everything up or locks everything out. A restricted policy is logged at startup.

How a command is classified: `oci://…` is `oci`, `grpc://…` is `grpc`, and every other command (a path, `file://…`, even `http://…`) is run as a local executable and counts as `file`. The prefixes are case-sensitive, exactly as the plugin is run.

What is refused when a source is not allowed:

- **AI Studio:** creating or updating a plugin with that command (API `400`, also marketplace installs and upgrades), fetching an OCI plugin or its manifest (no registry request is made), and loading the plugin. The console offers only the allowed types and disables marketplace installs when `oci` is not allowed.
- **Microgateway:** its management API (`400`), every plugin load (plugins the hub pushes, global data-collection plugins from `PLUGINS_CONFIG_PATH`, local configuration) and OCI pre-fetching and pre-warming. **An edge enforces its own setting whatever AI Studio allows**: a plugin the hub pushes with a refused source is not loaded, and the refusal shows in the edge's plugin health and logs (with the plugin ID).
- Existing plugins with a source that is now refused are kept, so you can read, edit or delete them, but they do not load and cannot be saved with that command.

Example profiles:

```bash
# OCI only, signed (Enterprise Edition): the most locked-down profile
AI_STUDIO_PLUGIN_ALLOWED_SOURCES=oci
AI_STUDIO_OCI_ALLOWED_REGISTRIES=registry.example.com
AI_STUDIO_OCI_REQUIRE_SIGNATURE=true     # the default
PLUGINS_ALLOWED_SOURCES=oci              # on every Microgateway
OCI_PLUGINS_ALLOWED_REGISTRIES=registry.example.com

# Local binaries and OCI, but no remote gRPC plugin processes
AI_STUDIO_PLUGIN_ALLOWED_SOURCES=oci,file
AI_STUDIO_PLUGIN_ALLOWED_DIRS=/opt/plugins
PLUGINS_ALLOWED_SOURCES=oci,file
```

The source policy decides which deployment types are accepted at all. Within an allowed type the existing controls still apply: `AI_STUDIO_PLUGIN_ALLOWED_DIRS` limits where `file` plugins may live, `*_OCI_ALLOWED_REGISTRIES` limits which registries `oci` plugins come from, signatures are checked for `oci` plugins, and internal-network blocking applies to `grpc` hosts.

## Production checklist

1. Install only plugins you trust; prefer OCI artifacts pinned by digest from an allowlisted registry, with signatures required (Enterprise Edition).
2. Keep `ALLOW_INTERNAL_NETWORK_ACCESS` unset (or `false`).
3. Name internal registries or plugin hosts you need in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` rather than setting `ALLOW_INTERNAL_NETWORK_ACCESS`. (`PLUGIN_BLOCK_INTERNAL_URLS=true` is deprecated; it still adds refusals for internal `host:port` arguments.)
4. Keep `AI_STUDIO_PLUGIN_ALLOWED_DIRS` to the directories you actually deploy plugins into, and allow only the plugin sources you use (`AI_STUDIO_PLUGIN_ALLOWED_SOURCES`, `PLUGINS_ALLOWED_SOURCES`; see [Hardening: restrict plugin sources](#hardening-restrict-plugin-sources)).
5. Review the scopes a plugin's manifest requests before approving them.
6. Do not store credentials in plugin configuration unless you would hand them to the plugin's author, and keep `PLUGIN_ENV_PASSTHROUGH` to the variables your plugins need (act on any warning it logs).
7. Restrict egress at the network layer if plugins must not reach internal services.
8. Run the host (and so its plugins) as a non-root user with a read-only filesystem where possible, and keep the OCI cache directory writable only by that user.
