#!/usr/bin/env bash
# Builds the e2e images from $SRC (an exported tree with enterprise/ and
# ui/admin-frontend/build): c1g-e2e/studio:{ent,ce} and
# c1g-e2e/microgateway:{ent,ce}.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="${SRC:?set SRC to the exported source tree}"
GO_VERSION="$(sed -n 's/^toolchain go//p' "$SRC/go.mod")"
GO_VERSION="${GO_VERSION:-$(sed -n 's/^go //p' "$SRC/go.mod")}"
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT

build() { # <edition> <tags>
  local edition=$1 tags=$2
  mkdir -p "$OUT/$edition"
  docker run --rm -u "$(id -u):$(id -g)" \
    -e HOME=/tmp -e GOCACHE=/cache/go-build -e GOMODCACHE=/go/pkg/mod -e CGO_ENABLED=1 \
    -v "$SRC:/src" -v "$OUT/$edition:/out" \
    -v "$HOME/go/pkg/mod:/go/pkg/mod" -v "$HOME/.cache/go-build:/cache/go-build" \
    -w /src "golang:$GO_VERSION-bookworm" bash -c "
      go build -tags '$tags' -ldflags '-X main.Version=e2e-$edition' -o /out/tyk-ai-studio . &&
      cd microgateway && go build -tags '$tags' -o /out/tyk-microgateway ./cmd/microgateway"
  docker build -q -f "$HERE/Dockerfile.studio" -t "c1g-e2e/studio:$edition" "$OUT/$edition"
  docker build -q -f "$HERE/Dockerfile.microgateway" -t "c1g-e2e/microgateway:$edition" "$OUT/$edition"
}

build ent enterprise
build ce ""
docker images 'c1g-e2e/*'
