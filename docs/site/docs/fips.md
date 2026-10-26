# FIPS 140-3

AI Studio and the microgateway are available as FIPS 140-3 builds, like Tyk Gateway, Dashboard, Pump and MDCB. They are **FIPS 140-3 compliant**: the approved algorithms they use come from a validated module, and the few non-approved read paths left are listed under [Known exceptions](#known-exceptions). They are not themselves certified products.

## What is covered

- The FIPS builds link the **Go Cryptographic Module v1.0.0** ([CMVP certificate #5247](https://csrc.nist.gov/projects/cryptographic-module-validation-program/certificate/5247)), selected at build time with `GOFIPS140=v1.0.0`.
- They start in FIPS mode (`fips140=on` by default): TLS, hashing, HMAC, key derivation and encryption use the module's approved algorithms.
- Standard builds are unchanged. A database moves between standard and FIPS builds in both directions (see [Data formats](#data-formats)).
- FIPS packages are published for both editions; the container images are built from the Enterprise edition, as Tyk's other `-fips` images are. Ask your Tyk account team about licensing FIPS builds.

## Packages and images

| Component | Package (deb/rpm) | Container image |
|---|---|---|
| AI Studio | `tyk-ai-studio-fips` (CE), `tyk-ai-studio-fips-ee` (Enterprise) | `tykio/tyk-ai-studio-fips` |
| Microgateway (server and `mgw` CLI) | `tyk-microgateway-fips` (CE), `tyk-microgateway-fips-ee` (Enterprise) | `tykio/tyk-microgateway-fips` |

- Packages are built for amd64, arm64 and s390x. Images are built for amd64 and arm64, from the Enterprise edition, on Tyk's FIPS base image (`tykio/dhi-busybox:1.37-fips`), with provenance and an SBOM attestation.
- To use the FIPS images with Helm or Docker Compose, override the image: `image.repository: tykio/tyk-ai-studio-fips` (or `tykio/tyk-microgateway-fips`) in the Helm values shown in [Kubernetes / Helm](./deployment-helm-k8s.md), or `image: tykio/tyk-ai-studio-fips:<version>` in the `compose.yml` of [Docker Compose](./deployment-docker.md). Paths, ports and settings are the same as the standard images; the images run as uid 65532.
- From source: `make build-fips` (root and `microgateway/`) builds FIPS binaries and checks them with `scripts/fips-verify.sh`.

## Verifying a deployment

- **Build metadata.** `go version -m <binary>` shows `GOFIPS140=v1.0.0-…`, `DefaultGODEBUG=fips140=on` and the `fips` build tag. `scripts/fips-verify.sh --fips <binary>` checks all three.
- **Startup log.** Both binaries log a `FIPS 140-3` line with `enabled`, `version` and `enforced`.
- **Self-report endpoint.** `GET /health` on AI Studio and on a microgateway returns:

  ```json
  {"status": "ok", "fips": {"enabled": true, "version": "v1.0.0", "enforced": false}}
  ```

- **Edges.** Edge gateways report their status in heartbeats; the Edge Gateways page shows it for each edge, and the edge API returns it as `attributes.fips` (see [Edge Gateways](./edge-gateways.md#fips-140-3-status)). The status is the edge's own report.
- **`GODEBUG=fips140=off` turns FIPS mode off at run time, even in a FIPS build.** The self-report then shows `"enabled": false`: check it, not the package or image name. `GODEBUG=fips140=only` additionally refuses every non-approved algorithm (`"enforced": true`); it is meant for testing.

## Data formats

FIPS-mode processes write approved formats; every build reads both the old and the new ones.

| Data | Standard build writes | FIPS build writes |
|---|---|---|
| Secrets at rest (`TYK_AI_SECRET_KEY`) | `v2:` (scrypt + AES-GCM) | `v3:` (PBKDF2-HMAC-SHA256 + AES-GCM) |
| User passwords, OAuth client secrets, microgateway credential secrets | bcrypt | PBKDF2-HMAC-SHA256 |
| Hub-to-edge snapshot secrets | AES-GCM, random nonce | AES-GCM, random nonce |

- **Secrets.** At startup, in the background, a FIPS-mode AI Studio re-encrypts stored secrets from older formats to `v3`: the Secrets page, and the encrypted values in other tables (LLM API keys and credential metadata, LLM resource signing secrets, webhook headers and signing secrets, Tyk connection tokens and gateway definitions, submission credentials, MCP credential tokens). Until it has run, older values are still read; a standard build does not re-encrypt.
- **Passwords.** A user's bcrypt hash is replaced with a PBKDF2 one the next time they log in on a FIPS build; an OAuth client secret on its next successful use.
- **Schema version.** These formats raise the database schema version to 26, so an older AI Studio refuses a database a newer one has migrated instead of misreading it.
- **Legacy secrets.** Under `fips140=only`, values encrypted before the `v2` format (AES-CFB) are read only by the startup re-encryption: until it has run, a read of one returns an error. Start AI Studio once in FIPS mode before enforcing strict mode, and let the re-encryption finish (it logs how many values it re-encrypted).

## Operational requirements

- **TLS.** FIPS mode accepts TLS 1.2 and 1.3 with approved cipher suites and curves only. Peers that offer only ChaCha20-Poly1305, or only plain X25519 key exchange, fail to connect; the hybrid post-quantum X25519MLKEM768 key exchange of TLS 1.3 is allowed. This applies to LLM vendors, the hub-edge gRPC link, plugins over gRPC, and your own TLS endpoints.
- **TLS certificates** must use RSA keys of 2048 bits or more, ECDSA P-256/P-384/P-521, or Ed25519 in FIPS mode; others are refused (with `fips140=on` and `fips140=only` alike).
- **Signing keys outside TLS** (cosign plugin-signing keys, SSO identity providers' JWKS): use RSA 2048+ or ECDSA. FIPS mode on (`fips140=on`) still accepts smaller RSA keys there; strict mode does not.
- **Postgres.** Use SCRAM-SHA-256 password authentication, not `md5`.
- **Skipping TLS verification** (`InsecureSkipVerify`-style options) is outside the scope of FIPS compliance; do not use it in a FIPS deployment.
- **Resource signing.** LLM resource IDs (native API extensions) are signed with a key from the hub's signing secret, or, without one, from the vendor credential. Set a hub signing secret for every LLM: a key derived from a short or placeholder credential (such as Ollama's `none`) is guessable in any build. Short-credential IDs signed in standard mode do not verify in FIPS mode, and the other way round.
- **Plugins.** Every plugin is a separate process with its own cryptography (TLS to vendors and to the gateway). The deployment is only FIPS compliant if every plugin is a FIPS build too: build plugins with `GOFIPS140=v1.0.0` (see [Publishing Plugins](./plugins-publishing.md)).

## Known exceptions

These uses are not approved in FIPS mode. The first three are read paths for data written by standard builds (or older releases), reached until that data is rewritten in an approved format; the last is MongoDB driver code that AI Studio never reaches:

| Use | Where | Notes |
|---|---|---|
| bcrypt verification | passwords and secrets stored by standard builds | replaced on next login or use |
| scrypt key derivation | `v2:` secrets written by standard builds | re-encrypted to `v3` at startup in FIPS mode |
| AES-CFB decryption | secrets written before `v2:` | read only by the startup re-encryption |
| MD5 / SHA-1 | the vendored MongoDB driver used by SSO's identity broker | only when talking to MongoDB, which AI Studio never does |
