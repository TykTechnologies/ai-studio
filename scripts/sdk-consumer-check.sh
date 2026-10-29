#!/usr/bin/env bash
# Checks that a module importing the public plugin SDK resolves its whole
# module graph from the public Go proxy, as a third-party plugin author's
# does: `go mod tidy`, `go list -m all` (what govulncheck, Dependabot and
# Renovate walk) and a build.
#
# The root module must never require the private enterprise module: Go
# ignores the root's `replace` in a consumer, so any such requirement makes
# `go list -m all` fail with a 404 for every plugin author. Enterprise builds
# resolve the module through enterprise.work instead.
#
# The consumer replaces midsommar/v2 with this checkout, so the check covers
# the go.mod about to be released. Linked enterprise files are ignored: a
# consumer only reads the packages it imports.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

cat >"$WORK/go.mod" <<EOF
module example.com/sdk-consumer

go $(sed -n 's/^go //p' "$ROOT/go.mod")

require github.com/TykTechnologies/midsommar/v2 v2.0.0

replace github.com/TykTechnologies/midsommar/v2 => $ROOT
EOF
cat >"$WORK/main.go" <<'EOF'
package main

import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type plugin struct{ plugin_sdk.BasePlugin }

func main() {
	plugin_sdk.Serve(&plugin{BasePlugin: plugin_sdk.NewBasePlugin("sdk-consumer", "0.0.1", "SDK consumer check")})
}
EOF

# The public proxy only, as an outside author sees it: no direct fetches,
# no private-module exemptions, no workspace.
export GOWORK=off GOFLAGS=-mod=mod GOPROXY=https://proxy.golang.org GOPRIVATE= GONOPROXY= GONOSUMDB=

go -C "$WORK" mod tidy
go -C "$WORK" list -m all >/dev/null
go -C "$WORK" build -o /dev/null .
echo "SDK consumer check passed: the plugin SDK's module graph resolves from the public proxy."
