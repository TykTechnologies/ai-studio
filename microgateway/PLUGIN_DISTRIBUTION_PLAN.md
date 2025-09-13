# OCI-based Plugin Distribution for Microgateway — MVP Implementation Guide

**Date:** 13 Sep 2025
**Audience:** Gateway team, plugin authors, DevOps

---

## Executive summary

We will ship **hashicorp/go-plugin** binaries to edge gateways using **OCI artifacts** (the same protocol/container registries use) and **cosign** signatures. This gives us: content-addressed packages (digests), standard registries (Nexus/Harbor/etc.), simple signing (file keys), and no container runtime on the edge. Gateways pull **by digest**, verify the signature with a trusted public key, cache by digest, `chmod +x`, and execute the plugin process via go-plugin (gRPC).

**Why this over ZIP + manifest?** We gain ubiquitous distribution infra, built‑in content addressing, first‑class signatures/SBOMs (now or later), and repeatable updates/rollbacks with minimal new tooling.

---

## Goals & non‑goals

**Goals**

* Minimal, **end‑user friendly** distribution path for custom plugins.
* Works with **in‑house** registries (Nexus/Harbor/Artifactory/Quay/GHCR/ECR).
* No dependency on Docker/containerd on the edge.
* Basic security: **sign with a file key** in CI; **verify** at the gateway.

**Non‑goals (for MVP)**

* Keyless/OIDC signing, attestations, SBOM enforcement, or TUF. (We can add later.)
* Sandboxing beyond standard OS process isolation. (We will note options.)

---

## Roles

* **Plugin author (internal or customer):** Compiles a Go (or other) plugin binary and publishes it as an OCI artifact.
* **Registry operator:** Runs/uses a registry (Nexus/Harbor/etc.) with basic auth.
* **Gateway operator:** Configures the gateway with registry location, **digest**, and **trusted public key(s)**.

---

## High‑level architecture

```
[Plugin Author CI]
  ├─ build plugin (linux/amd64, arm64)
  ├─ oras push   (artifact = raw binary)
  └─ cosign sign (file key)
        │
        ▼
[OCI Registry: Nexus/Harbor]
        │  (HTTPS + basic auth)
        ▼
[Edge Gateway]
  1) pull by digest
  2) verify cosign signature
  3) materialize to disk, chmod +x
  4) launch via go-plugin (gRPC)
```

---

## Artifact format (what we store in the registry)

**Artifact type (manifest `config.mediaType`):**

* `application/vnd.tyk.plugin.binary.v1` (example; pick your own stable type)

**Layers:**

* **Layer\[0] = the plugin binary file**, media type `application/vnd.tyk.plugin.layer.v1` (or `application/octet-stream`).
* (Optional) Additional layers later for readme/license, etc.

**Optional config blob:** `application/vnd.tyk.plugin.config.v1+json`

```json
{
  "name": "ner",
  "version": "1.2.3",
  "plugin_api": "2",            // go-plugin protocol/ABI level for host
  "os": "linux",
  "arch": "amd64",
  "libc": "glibc",              // or "musl" if relevant
  "host_min_version": "0.23.0",
  "capabilities": ["network", "fs:read"],
  "notes": "optional, free-form"
}
```

> **Rule of thumb:** keep the binary as the **first/only layer** for the MVP so the client logic is trivial.

**Multi‑arch (later):** publish an **OCI index** if you want one ref to cover both `linux/amd64` and `linux/arm64`. The client selects the matching platform; for the MVP we can publish separate repos or tags per arch.

---

## Publishing workflow (for plugin authors)

Assume a single, static Go binary `ner-linux-amd64`.

### 0) One‑time: generate a cosign keypair (file‑key mode)

```
cosign generate-key-pair --output-key-prefix ./plugin-ci
# produces plugin-ci.key (private) and plugin-ci.pub (public)
```

* Store the **private key** in CI secrets (or KMS). Distribute the **public key** with the gateway.

### 1) Push the binary as an OCI artifact (ORAS)

```
# login (basic auth is fine)
oras login nexus.example.com -u plugin-ci -p '***'

# (optional) add a small config json if you want
printf '{"name":"ner","plugin_api":"2","os":"linux","arch":"amd64"}' > plugin.json

# push: artifact type names the kind of thing this is
oras push nexus.example.com/plugins/ner:1.2.3 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
  ./ner-linux-amd64:application/vnd.tyk.plugin.layer.v1
```

> ORAS returns the **digest** (`sha256:…`) of the pushed manifest. Treat that digest as the immutable ID of this release.

### 2) Sign the artifact **after** pushing

```
# Best: sign by digest
DGST=sha256:...   # from oras output
cosign sign --key ./plugin-ci.key nexus.example.com/plugins/ner@$DGST

# Acceptable: sign by tag (cosign resolves tag→digest at sign time)
cosign sign --key ./plugin-ci.key nexus.example.com/plugins/ner:1.2.3
```

* On registries that fully support **OCI referrers** (e.g., Harbor), the signature is stored **alongside** the artifact. On others, cosign uses a fallback tag scheme—verification still works transparently.

### 3) (Optional) Publish a human tag

* Push a `stable`/`latest` tag for convenience in docs, but **gateways should use the digest** in production.

---

## Gateway consumption workflow (runtime)

At startup (or when a plugin is first referenced), the gateway should:

1. **Resolve & pull by digest** from the configured registry/repo.
2. **Verify** the cosign signature using a trusted public key set.
3. **Materialize** the binary to disk from the pulled layer.
4. `chmod +x` and **launch** via hashicorp/go-plugin with your usual handshake/config.
5. Cache by **digest**; use a **name → digest** symlink for the active version.

### Minimal gateway config (example)

```yaml
plugins:
  - name: ner
    registry: nexus.example.com
    repository: plugins/ner
    digest: "sha256:0123deadbeef..."     # pin in prod
    arch: linux/amd64
    cache_dir: /var/lib/tyk/plugins
    cosign_pubkeys:
      - /etc/tyk/plugins/trusted/pubkey1.pem
      - /etc/tyk/plugins/trusted/pubkey-rotated.pem
    auth:
      username: plugin-reader
      password_env: PLUGIN_READER_PASSWORD
    allow_registries:
      - nexus.example.com
```

### Implementation sketch (Option A: call CLI tools)

This is the **fastest to harden** (no dependency on cosign/ORAS Go APIs). The process shells out to `oras` to pull and `cosign` to verify.

```bash
# pull + verify + write to disk (one-liner style)
oras pull nexus.example.com/plugins/ner@sha256:... -o /var/lib/tyk/plugins/cas
cosign verify --key /etc/tyk/plugins/trusted/pubkey1.pem \
  nexus.example.com/plugins/ner@sha256:...
# materialize the first layer from the CAS directory to an executable path
```

**Pros:** simple, well‑tested CLIs; easier to keep verification semantics aligned with cosign upstream.
**Cons:** requires shipping two small binaries (`oras`, `cosign`) with the gateway.

### Implementation sketch (Option B: embed in Go)

Embed **oras-go** for pulling and use **cosign** either via CLI or library for verification. Pseudocode below favors clarity:

```go
// PSEUDOCODE: Fetch, verify, materialize, exec the plugin by digest.
// For production, add retries, timeouts, sandboxing, and robust error handling.

package pluginfetch

import (
  "context"
  "encoding/json"
  "fmt"
  "io"
  "os"
  "os/exec"
  "path/filepath"

  ocispec "github.com/opencontainers/image-spec/specs-go/v1"
  "oras.land/oras-go/v2"
  "oras.land/oras-go/v2/content"
  "oras.land/oras-go/v2/content/file"
  "oras.land/oras-go/v2/registry/remote"
)

type Ref struct {
  Registry string // e.g. "nexus.example.com"
  Repo     string // e.g. "plugins/ner"
  Digest   string // e.g. "sha256:..."
}

func FetchVerifyExec(ctx context.Context, r Ref, pubKeyPath, cacheDir string, args ...string) error {
  // 1) Pull to local content-addressed store
  repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", r.Registry, r.Repo))
  if err != nil { return err }
  store, err := file.New(cacheDir)
  if err != nil { return err }
  defer store.Close()

  // Copy manifest + blobs for the digest from repo -> local store
  // Using digest as the reference keeps it immutable
  desc, err := oras.Copy(ctx, repo, r.Digest, store, r.Digest, oras.DefaultCopyOptions)
  if err != nil { return err }

  // 2) Verify signature (shell out to cosign for minimal drift)
  ref := fmt.Sprintf("%s/%s@%s", r.Registry, r.Repo, desc.Digest.String())
  out, err := exec.CommandContext(ctx, "cosign", "verify", "--key", pubKeyPath, ref).CombinedOutput()
  if err != nil { return fmt.Errorf("cosign verify failed: %s", string(out)) }

  // 3) Read manifest to locate the single layer (our binary)
  maniBytes, err := content.FetchAll(ctx, store, desc)
  if err != nil { return err }
  var mani ocispec.Manifest
  if err := json.Unmarshal(maniBytes, &mani); err != nil { return err }
  if len(mani.Layers) == 0 { return fmt.Errorf("no layers in artifact") }
  layer := mani.Layers[0] // MVP rule: first/only layer is the binary

  // 4) Materialize to an executable file path (content addressed)
  rc, err := store.Fetch(ctx, layer)
  if err != nil { return err }
  defer rc.Close()
  bin := filepath.Join(cacheDir, "bin-"+layer.Digest.Encoded())
  f, err := os.Create(bin)
  if err != nil { return err }
  if _, err := io.Copy(f, rc); err != nil { return err }
  if err := f.Close(); err != nil { return err }
  if err := os.Chmod(bin, 0o755); err != nil { return err }

  // 5) Exec (or hand off to your go-plugin launcher)
  cmd := exec.CommandContext(ctx, bin, args...)
  return cmd.Start()
}
```

> **Tip:** keep an on‑disk structure like:
>
> ```
> /var/lib/tyk/plugins/
>  cas/                   # ORAS file store (by digest)
>  bin-<blob-digest>      # materialized executables
>  active/ner -> ../bin-<blob-digest>
> ```
>
> Swap the `active/ner` symlink atomically on upgrade.

---

## Updates, rollback, and caching

* **Pin by digest** in gateway config. To upgrade, change the digest; the gateway pulls & verifies the new artifact, then atomically switches the symlink.
* **Rollback** = restore the previous digest; the old binary remains in cache.
* **Garbage collect** old blobs on a schedule (keep last N).
* **Network failures**: use exponential backoff and retain last known good executable.

---

## End‑user distribution (in‑house)

Most customers will publish to their own registry. Provide a short checklist:

1. **Stand up a registry** (Nexus/Harbor/etc.) with HTTPS and basic auth.
2. **Install ORAS & cosign** on their CI or dev machine.
3. **Generate cosign keys** (file mode) and store the private key safely; hand the **public key** to the gateway admin.
4. **oras push** their plugin binary, then **cosign sign**.
5. Provide the gateway admin the **registry**, **repository**, and **digest**.

### Bare‑minimum Nexus steps (example)

* Enable the Docker/OCI **hosted** repository type (e.g., `plugins`).
* Create a **service account** with read permissions.
* Publish with ORAS as shown above.

### Air‑gapped/offline (optional)

* Mirror artifacts into an **OCI layout** on disk (USB/NAS). The gateway can pull from `oci:/path/to/layout` using ORAS, or you can copy the CAS directory onto the box and skip the network step.

---

## Security policy (MVP)

* **Trust root:** a small set of **cosign public keys** shipped with the gateway (and rotatable). Verification requires at least one key to validate.
* **Registry allow‑list:** only fetch from approved registries/repos.
* **Digest pinning:** the config must specify a digest (not a tag) in production.
* **Permissions:** run plugins as a non‑root user; consider a dedicated UID/GID per plugin.
* **Sandboxing (next steps):**

  * Linux: seccomp/profile, `no_new_privs`, `chroot`/namespaces.
  * Limit plugin IPC/network via OS firewalls or a tiny sidecar proxy if needed.

---

## Operational notes

* **Logging:** record the exact ref (`repo@digest`) and signer key ID on load.
* **Metrics:** time to pull, bytes downloaded, verification time, cache hit/miss.
* **Key rotation:** allow multiple pubkeys; deprecate old keys on a schedule.
* **Multi‑arch:** either separate repos (`plugins/ner-amd64`, `plugins/ner-arm64`) or move to **OCI index** later.
* **SBOM/provenance (later):** attach as referrers; policy engine can require their presence.

---

## “Plugin author quickstart” (copy‑paste)

```bash
# 0) one-time
cosign generate-key-pair --output-key-prefix ./plugin-ci

# 1) build your binary (example for linux/amd64)
GOOS=linux GOARCH=amd64 go build -o ner-linux-amd64 ./cmd/ner

# 2) push
oras login nexus.example.com -u plugin-ci -p '***'
printf '{"name":"ner","plugin_api":"2","os":"linux","arch":"amd64"}' > plugin.json
oras push nexus.example.com/plugins/ner:1.2.3 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
  ./ner-linux-amd64:application/vnd.tyk.plugin.layer.v1

# capture digest (from output) and sign it
DGST=sha256:...
cosign sign --key ./plugin-ci.key nexus.example.com/plugins/ner@$DGST

echo "Publish complete. Share repo = plugins/ner, digest = $DGST, and your pubkey with the gateway admin."
```

---

## “Gateway quickstart” (copy‑paste)

```bash
# Place trusted public keys
install -D -m 0644 pubkey1.pem /etc/tyk/plugins/trusted/pubkey1.pem

# Minimal config entry
cat >> /etc/tyk/plugins.yaml <<'YAML'
- name: ner
  registry: nexus.example.com
  repository: plugins/ner
  digest: "sha256:0123deadbeef..."
  cache_dir: /var/lib/tyk/plugins
  cosign_pubkeys:
    - /etc/tyk/plugins/trusted/pubkey1.pem
  auth:
    username: plugin-reader
    password_env: PLUGIN_READER_PASSWORD
YAML

# Run a one-off prefetch (CLI path)
oras pull nexus.example.com/plugins/ner@sha256:0123deadbeef... -o /var/lib/tyk/plugins/cas
cosign verify --key /etc/tyk/plugins/trusted/pubkey1.pem \
  nexus.example.com/plugins/ner@sha256:0123deadbeef...
```

---

## Future enhancements (drop‑in later)

* **Keyless/OIDC** signing (cosign) → policy by issuer/subject.
* **SBOM + SLSA provenance** (in‑toto) → attach as referrers and enforce.
* **TUF** on top for threshold trust, expiry/rotation, compromise‑resilience.
* **Policy engine** in the gateway for signer allow‑list, required attestations, etc.
* **OCI index** for multi‑arch, plus platform negotiation at fetch time.

---

## Appendix A — go-plugin launch reminder

```go
// Pseudocode example of launching a verified binary with go-plugin
client := plugin.NewClient(&plugin.ClientConfig{
  HandshakeConfig: Handshake,        // your handshake
  Plugins:         PluginMap,        // your interface map
  Cmd:             exec.Command(bin),
  AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
})
rpcClient, err := client.Client()
// ... dispense your interface and use it
```

---

## Appendix B — troubleshooting checklist

* **401 from registry:** verify service-account creds and repo path.
* **cosign verify fails:** wrong public key, or you signed a different digest; double‑check you signed **after** pushing.
* **Digest mismatch:** never rewrite binaries in place; every rebuild → new digest.
* **Exec permission denied:** ensure `chmod +x` after materializing.
* **Wrong arch/libc:** publish per‑arch artifacts for now; consider OCI index later.
* **Air‑gapped:** use OCI layout export or pre‑seed the CAS directory.

---

## One‑page TL;DR for the team

1. **Authors:** `oras push` your binary → get **digest** → `cosign sign` it.
2. **Gateway:** pull **by digest**, `cosign verify`, write executable, run via go-plugin.
3. **Config:** pin digest, ship pubkeys, allow‑list registries.
4. **Upgrades:** swap digest; atomic symlink switch; rollback by restoring old digest.
