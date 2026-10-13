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
| Secret redaction in plugin config | No | See [Plugin configuration and secrets](#plugin-configuration-and-secrets). |

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

**Edges receive plugin configuration.** The configuration snapshot that AI Studio sends to every edge gateway includes the stored configuration of every active plugin, Studio-only plugins included. A literal credential therefore reaches every edge; a reference does not. Plugins running on edges receive references unresolved.

What this means in practice:

- Anyone who can read the plugin's configuration in AI Studio (an administrator with plugin read access, or the database) can read a credential stored in it literally. Store a `$SECRET/` reference instead.
- The plugin process receives the credential in clear text, resolved or not. Only give a plugin a credential you would be comfortable giving to the plugin's author.
- **Plugin authors**: do not log configuration values, and do not echo them in errors or UI responses. If your plugin needs a credential, document which config field holds it, keep the field optional where you can, and consider letting operators supply it from a file mounted only into your deployment.

Optional hardening, such as marking config fields as secret and redacting them in transit, is tracked in [#445](https://github.com/TykTechnologies/ai-studio/issues/445).

## Command validation

A plugin's **command** says where the host gets the plugin: a local executable (`file://` or a plain path), an OCI artifact (`oci://`), or a remote gRPC server (`grpc://`). Commands are checked in two places, and the two behave differently.

### Enforced checks (always on)

These run when a plugin is created or updated through the API, and again when AI Studio loads a plugin, so a command that reached the database another way is checked before a process starts. A command that fails is **refused**.

| Check | Rule |
|---|---|
| Length | At most 1024 characters. |
| Path traversal | No `../` or `..\`. |
| Shell metacharacters | No `;`, `\|`, `&`, `` ` ``, `$(`, newlines or NUL. |
| Executable location | Local executables must be under `/usr/bin/`, `/bin/`, `/usr/local/bin/` or `/opt/plugins/`, or a relative path under `plugins/`. Add directories with `AI_STUDIO_PLUGIN_ALLOWED_DIRS` (a path list, `:`-separated on Linux). The Microgateway allows `/opt/microgateway/plugins` and `./plugins`, refuses symbolic links, and requires the file to be executable. |
| URL scheme | Only `grpc`, `oci`, `http` and `https` URLs. |
| Internal hosts | The host of a URL in the command must not be internal. The host is resolved, and a name that does not resolve is treated as internal (fail closed). Internal means loopback, RFC 1918 private ranges, link-local (including `169.254.169.254` cloud metadata), IPv6 unique-local and `localhost`. Set `ALLOW_INTERNAL_NETWORK_ACCESS=true` to allow internal hosts, for local development only. |

### Advisory checks (warning-only by default)

AI Studio's plugin service, and the Microgateway's, also run these checks. By default they **only log a warning** (`PLUGIN SECURITY WARNING`) and let the command through:

| Check | Default | How to make it stricter |
|---|---|---|
| Absolute path outside `/usr/bin/`, `/bin/`, `/usr/local/bin/` | Warn | Rely on the enforced executable-location check above, which already refuses paths outside the allowed directories. |
| Internal address anywhere in the command: URL hosts, `host:port` arguments, `--flag=host:port` values, bare IP literals. Hosts are parsed, not substring-matched, so IPv4-mapped IPv6 and decimal, hex or octal IPv4 spellings in URLs are caught. Shared address space (`100.64.0.0/10`) also counts as internal here. | Warn | Set `PLUGIN_BLOCK_INTERNAL_URLS=true` to refuse such commands. This check does not resolve hostnames; the enforced check above does. |
| Command not in `PLUGIN_COMMAND_ALLOWLIST` | Warn (an info message when the variable is unset) | None today: the allowlist is advisory. Entries are plain substrings (no wildcards), comma-separated. Use it to make unexpected commands visible in the logs. |

Making the advisory checks blocking by default is a product decision tracked in [#427](https://github.com/TykTechnologies/ai-studio/issues/427).

### Edge gateways

A Microgateway in edge mode has no management API: it receives plugins from AI Studio through configuration sync, so a plugin reaches an edge only after it has passed AI Studio's API validation. A standalone Microgateway (no AI Studio) has its own management API, which runs the same enforced checks. In both modes the Microgateway validates local executable paths again when it loads a plugin.

## OCI plugins: registries, digests and signatures

For production, OCI is the recommended way to distribute plugins.

- **Restrict registries.** Set `AI_STUDIO_OCI_ALLOWED_REGISTRIES` (AI Studio) and `OCI_PLUGINS_ALLOWED_REGISTRIES` (Microgateway) to a comma-separated list of registry hosts. An `oci://` command for any other registry is refused. With the variable unset, any registry is allowed.
- **Pin by digest.** `oci://registry.example.com/plugins/my-plugin@sha256:…` always loads the same bytes. A tag (`:v1.2.3`) can be moved to different content by anyone who can push to the repository.
- **Require signatures (Enterprise Edition).** Set `AI_STUDIO_OCI_REQUIRE_SIGNATURE=true` on AI Studio. The Microgateway requires signatures by default (`OCI_PLUGINS_REQUIRE_SIGNATURE`, default `true`). Signatures are checked with `cosign` against the digest that was pulled. The Tyk signing key is always trusted. Add your own keys as `OCI_PLUGINS_PUBKEY_1`, `OCI_PLUGINS_PUBKEY_2`, … or `OCI_PLUGINS_PUBKEY_<NAME>` (key content), or `OCI_PLUGINS_PUBKEY_FILE_<NAME>` (a file path), and select one per plugin with `?pubkey=` on the command (`?pubkey=2`, `?pubkey=CI`).
  - In **Community Edition** signatures are not verified, whatever these variables say; a warning is logged at startup if you set them.
- **Protect the cache.** Pulled binaries are cached (`AI_STUDIO_OCI_CACHE_DIR`, `OCI_PLUGINS_CACHE_DIR`). Each cached binary's SHA-256 is checked against the value recorded when it was fetched, and a binary that changed on disk is fetched and verified again. Someone who can write to the cache directory can replace both the binary and its record, so keep the directory writable only by the service user.

See [Deployment Options](./plugins-deployment.md) for registry authentication and packaging.

## Production checklist

1. Install only plugins you trust; prefer OCI artifacts pinned by digest from an allowlisted registry, with signatures required (Enterprise Edition).
2. Keep `ALLOW_INTERNAL_NETWORK_ACCESS` unset (or `false`).
3. Set `PLUGIN_BLOCK_INTERNAL_URLS=true`.
4. Keep `AI_STUDIO_PLUGIN_ALLOWED_DIRS` to the directories you actually deploy plugins into.
5. Review the scopes a plugin's manifest requests before approving them.
6. Do not store credentials in plugin configuration unless you would hand them to the plugin's author, and keep `PLUGIN_ENV_PASSTHROUGH` to the variables your plugins need (act on any warning it logs).
7. Restrict egress at the network layer if plugins must not reach internal services.
8. Run the host (and so its plugins) as a non-root user with a read-only filesystem where possible, and keep the OCI cache directory writable only by that user.
