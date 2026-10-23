# E2E: signature verification and plugin source lockdown on distroless images

The end-to-end check of epic midsommar-clean-c1g (bead c1g.15): Enterprise
Studio and microgateway on distroless images verify OCI plugin signatures
in-process (there is no cosign binary and no shell in the images), and each
process enforces its own `*_PLUGIN_ALLOWED_SOURCES` / `PLUGINS_ALLOWED_SOURCES`.

The images reuse the runtime stage of `ci/Dockerfile.distroless` and
`ci/Dockerfile.microgateway.distroless` (same distroless base, layout and
entrypoint). The binaries are built from a committed tree in a
`golang:<go.mod version>-bookworm` container (CGO, the distroless glibc),
rather than through goreleaser `.deb` packages, which would build every
architecture.

## Running it

```sh
# 1. Build images from a source tree (a clean export of a commit, with the
#    enterprise submodule and ui/admin-frontend/build in place)
SRC=/path/to/exported/tree tests/e2e/cosign-distroless/build.sh

# 2. A local registry and signed test plugins (tools/publish-plugin.sh)
docker run -d --name c1g-e2e-registry -p 127.0.0.1:15000:5000 registry:2
#    see publish.sh for the variants (v2, v3, unsigned, wrong key, platform)

# 3. Run the scenarios; needs TYK_AI_LICENSE in the environment (ENT)
tests/e2e/cosign-distroless/run.sh
```

Signing uses throwaway keys and `--tlog-upload=false`: nothing is uploaded to
the public Rekor log. The "required + transparency-log entry" case is covered
by the Enterprise unit test that verifies a published plugin's real Rekor
bundle offline (enterprise/features/plugin_security, TestVerify_RealCosignSignature).
