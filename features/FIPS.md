# FIPS 140-3 builds

Internal spec. Customer docs: `docs/site/docs/fips.md`. Plan and history: beads epic `midsommar-clean-8dy7` (its design field).

## Builds

- goreleaser (`ci/goreleaser/goreleaser.yml`, `goreleaser-microgateway.yml`): `fips-{amd64,arm64,s390x}`, `mgw-fips-*`, `mgw-cli-fips-*` build with `GOFIPS140=v1.0.0`, `-trimpath` and `-tags=fips` (plus `enterprise` for ENT: command-line `-tags` replace the `GOFLAGS` ones). Packages: `tyk-ai-studio-fips[-ee]`, `tyk-microgateway-fips[-ee]`.
- Every goreleaser build runs `scripts/fips-verify.sh` as a post hook: FIPS binaries need `GOFIPS140=v1.0.0*`, `DefaultGODEBUG` with `fips140=on`, the `fips` tag and no BoringCrypto; standard ones none of these; `--edition ent` needs the `enterprise` tag. `release.yml` and `release-gate.yml` re-check `dist/`.
- `make build-fips` (root and `microgateway/`), `build-linux-{amd64,arm64}-fips`.
- Images: `ci/Dockerfile.fips.distroless`, `ci/Dockerfile.microgateway.fips.distroless` on `tykio/dhi-busybox:1.37-fips` (Debian 13 glibc, uid 65532), pushed as `tykio/tyk-ai-studio-fips` / `tykio/tyk-microgateway-fips` from the ENT jobs of `release.yml`, after fips-verify on the image binary and a Trivy scan.

## Runtime

- `pkg/fipsinfo`: `Status{Enabled, Version, Enforced}` from `crypto/fips140`. Logged at startup, in `GET /health` (`"fips"`), sent by edges in registration and heartbeat `health.metrics` (`fips.enabled`, `fips.version`, `fips.enforced`), stored per edge in `edge_instances.fips` (nullable: unknown), shown in the Edge Gateways UI.

## Rule: gate writers on `fips140.Enabled()`

Every build reads old and new formats; only FIPS-mode processes write the approved ones, so a database moves between standard and FIPS builds both ways. Schema version 26 marks the formats (older code misreads them).

| Data | Code | Standard | FIPS mode |
|---|---|---|---|
| Secrets at rest | `secrets/models.go` | `v2:` scrypt + GCM | `v3:<iter>:` PBKDF2-HMAC-SHA256 (210k) + GCM; the startup sweep moves legacy and `v2` values to `v3`: Secret rows (`ReencryptLegacySecrets`) and the `$ENC/` values of other tables (`models.ReencryptStoredCredentials`, list in `encryptedColumns`; FIPS mode only, compare-and-swap) |
| Passwords, OAuth client secrets, mgw secrets | `pkg/pwhash` | bcrypt | `$pbkdf2-sha256$<iter>$<salt>$<key>` (600k); bcrypt upgraded on successful use (compare-and-swap) |
| LLM resource signing keys | `models.HKDFKeyMaterial` | HKDF over the material | material under 14 bytes SHA-256'd first |
| Tiktoken rank-file cache | `pkg/tiktokenbpe` | tiktoken-go's loader (SHA-1 names) | SHA-256 names; SHA-1 names read from a precomputed table |

All builds: AES-GCM uses `cipher.NewGCMWithRandomNonce` (secrets, microgateway crypto service, agent tracing, hub→edge snapshots). The snapshot sync checksum is computed over keyed digests of the secrets (`grpc/snapshot_secret_digests.go`), not their ciphertexts; edges report the hub's checksum and never recompute it.

## Strict mode (`GODEBUG=fips140=only`)

- `make test-fips [EDITION=ent]` runs the root, microgateway and enterprise modules in strict mode (`scripts/test-fips.sh`), with canaries; nightly in `fips-nightly.yml`.
- `scripts/fips-guard.sh` (`make fips-guard`, CI) fails on new non-approved crypto imports and `cipher.NewGCM*` / `NewCFB*` outside `scripts/fips-guard.allowlist`; the list only shrinks.
- `fips140.WithoutEnforcement` is used only by the secrets sweep's legacy AES-CFB read; other legacy reads error under strict mode.
- `third_party/mgo.v2`: a fork of the MongoDB driver (bson machine id without MD5), wired by `replace` in the root, microgateway and enterprise `go.mod`s, so binaries get past init.
- gorm hashes generated names over 64 characters with SHA-1: `TestSchemaIdentifiersNeedNoSHA1Shortening` keeps every schema identifier shorter.
