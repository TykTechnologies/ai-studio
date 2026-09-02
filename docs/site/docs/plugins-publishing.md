# Publishing Plugins

Releasing a plugin means four things happening together: a cross-compiled
binary per platform, a multi-platform OCI index in the registry, a cosign
signature over that index, and a version entry in the marketplace repo whose
CI rebuilds `index.yaml`.

`make plugin-publish` does all four in one run. Use it rather than the
per-plugin `Makefile`s, which are kept only for one-off local pushes.

## Quick start

```bash
# One-time setup
oras login docker.tyk.io -u <user>          # see Prerequisites below

# Release a plugin. The version comes from its manifest.json.
make plugin-publish NAME=llm-cache SIGN_KEY=~/keys/plugin-ci.key

# See exactly what would happen, without pushing anything
make plugin-publish NAME=llm-cache DRY_RUN=1
```

To cut a release you bump `version` in the plugin's `manifest.json` and run the
command. If that version is already published the command refuses to run, so a
forgotten bump fails loudly instead of overwriting a release.

## What it does

1. **Resolves the plugin** under `community/plugins/`, `enterprise/plugins/`,
   `tyk-internal/plugins/` or `examples/plugins/`. It picks up the `enterprise`
   build tag, a `server/` submodule layout, `plugin.manifest.json` vs
   `manifest.json`, and a `ui/` bundle on its own.
2. **Builds the UI** (`npm ci && npm run build`) if the plugin has one.
3. **Cross-compiles** for `linux/amd64` and `darwin/arm64` with `CGO_ENABLED=0`.
4. **Pushes each binary** as an OCI artifact to a version-scoped platform tag
   (`1.0.2-linux_amd64`), then assembles the multi-platform index at `1.0.2`.
5. **Signs the index by digest** with cosign, then verifies the signature it
   just made. A signature that did not take fails the command.
6. **Writes the marketplace entry**: pulls the marketplace repo, generates
   `plugins/<name>/<version>/manifest.yaml`, carries the README, icon and
   changelog across, commits and pushes to `main` so the index CI fires.

Two properties are deliberate, because both were sources of broken releases:

- **The version is the tag.** It comes from `manifest.json` and is used verbatim
  as the OCI tag, so the marketplace entry, the registry tag and the binary
  cannot drift apart. Per-platform tags carry the version too, so a
  half-finished release can never leave the index pointing at a previous
  version's binary.
- **The digest is read back, never typed.** What lands in `oci.digest` is what
  the registry returned for the index that was just pushed.

## Variables

| Variable | Effect |
|----------|--------|
| `NAME` | Plugin to publish (required) |
| `VERSION` | Override the version from `manifest.json` |
| `SIGN_KEY` | Cosign private key |
| `PUB_KEY` | Public key for the read-back verify (defaults to `SIGN_KEY` with `.pub`) |
| `REG` | Registry host (defaults to whatever the last release used) |
| `PLATFORMS` | Platform list, e.g. `"linux/amd64 linux/arm64"` |
| `MIN_STUDIO_VERSION` | Override the compatibility floor (see below) |
| `DRY_RUN=1` | Print every step, push nothing |
| `YES=1` | Skip the confirmation prompt (CI) |
| `NO_PUSH=1` | Commit the marketplace entry but do not push it |
| `SKIP_MARKETPLACE=1` | Stop after signing |
| `SKIP_UI=1` | Skip the npm build |
| `SKIP_SIGN=1` | Push unsigned — test pushes only |
| `FORCE=1` | Overwrite a version directory that already exists |

The full flag list is in `./tools/publish-plugin.sh --help`; every variable
above has a matching long flag.

## Compatibility metadata

`requirements.min_studio_version` in the published entry comes from the
plugin's own `manifest.json`:

```json
{
  "compat": {
    "min_studio_version": "2.6.0",
    "min_gateway_version": "1.0.0"
  }
}
```

It is re-read on every release rather than carried forward, and the run warns
when the floor moves:

```
warning: min_studio_version moves from 2.0 to 2.6.0 in this release.
```

Override it for a single release with `MIN_STUDIO_VERSION=2.7.0`. If the plugin
declares no `compat` block, the previous entry's value is carried forward
unchanged.

Two caveats worth knowing:

- **`min_gateway_version` is not published.** The marketplace schema and index
  have no field for it (`pkg/marketplace/types.go`), so it stays a source-side
  declaration only.
- **`min_studio_version` is metadata, not a gate.** It is stored and surfaced
  through the marketplace API but nothing currently refuses an install on it.

Everything else in the entry — category, keywords, maturity, links,
`api_versions`, permissions, marketing copy — is curated in the marketplace repo
and carried forward from the previous version. Edit it there. On a plugin's
first publish those fields are generated as placeholders and the run tells you
to review them.

## Checking a release

```bash
make plugin-verify NAME=llm-cache                          # newest version
make plugin-verify NAME=llm-cache VERSION=1.0.1
make plugin-verify NAME=all PUB_KEY=~/keys/plugin-ci.pub   # audit everything
```

For each entry it checks that the recorded tag matches the manifest version,
that the tag still resolves to the recorded digest, that the index really is
multi-platform, and — given a public key — that the artifact is signed. It exits
non-zero if anything fails, so it works as a CI check.

## Repairing a bad entry

If the artifact in the registry is correct but the marketplace entry is not,
rewrite just the entry from the digest the registry actually returns:

```bash
./tools/publish-plugin.sh publish llm-cache --version 1.0.1 \
  --marketplace-only --force
```

This touches `manifest.yaml` only — the README, icon and changelog that shipped
with that version are left alone, and `created_at` is preserved.

If the *artifact* is wrong or missing — a version that was never pushed, or a
tag holding another version's binary — do a full republish instead:

```bash
make plugin-publish NAME=llm-cache FORCE=1
```

Note that re-pushing changes the index digest, which orphans the old signature.
That is fine here because signing happens in the same run, on the digest that
run produced.

## Prerequisites

- `oras` and `cosign` on `PATH` — install instructions are in
  `microgateway/docs/oci-plugin-workflow.md`.
- `oras login <registry>` for the target registry.
- The cosign private key. The matching public key ships in
  `keys/plugins/plugin-live.pub` and is embedded as `DefaultTykPublicKey` in
  `pkg/ociplugins/config.go`.
- Go and, for plugins with a UI, Node.

The marketplace repo is cloned automatically if it is missing
(`tyk-ai-studio-plugins`, or `tyk-internal-marketplace` for internal plugins).
Both are gitignored checkouts inside this repository.

## Running it in CI

```bash
make plugin-publish NAME=llm-cache YES=1 SIGN_KEY="$COSIGN_KEY_PATH"
```

`YES=1` skips the confirmation. Without it the command refuses to publish from a
non-interactive shell, so an unattended run cannot push by accident.

## See also

- `microgateway/docs/oci-plugin-workflow.md` — the OCI mechanics, registry
  setup, and the manual steps this command automates
- `docs/site/docs/plugins-deployment.md` — how gateways consume `oci://` plugins
- `docs/site/docs/plugins-manifests.md` — the manifest schema
