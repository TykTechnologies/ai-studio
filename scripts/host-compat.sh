#!/usr/bin/env bash
# Checks AI Studio against a host's go.mod (by default the Tyk Dashboard's,
# fetched from its private repository; never commit a copy of it here).
#
#   scripts/host-compat.sh [--build] [path/to/host/go.mod]
#
# 1. Version floor (no network): tools/hostcompat fails if Studio's go.mod
#    or the enterprise module's requires any module above the host's
#    version, since Go's version selection would silently raise it in the
#    host, unless scripts/host-compat-allow.txt names it with a reason.
# 2. With --build: builds pkg/studio inside the host's module graph (its
#    requirements and replaces) with CGO_ENABLED=0, for the Community and
#    Enterprise editions, and lists every module the result selects above
#    the host's go.mod. Needs read access to the host's private
#    dependencies (GOPRIVATE=github.com/TykTechnologies plus git credentials).
#
# Without a path it fetches TykTechnologies/tyk-analytics' go.mod with gh
# (GH_TOKEN). See features/Embedding.md, "Host version floor".
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
BUILD=false
if [ "${1:-}" = --build ]; then BUILD=true; shift; fi

WORK=$(mktemp -d)
trap 'chmod -R u+w "$WORK" 2>/dev/null; rm -rf "$WORK"' EXIT

HOST_MOD=${1:-}
if [ -z "$HOST_MOD" ]; then
  HOST_MOD="$WORK/host.go.mod"
  gh api repos/TykTechnologies/tyk-analytics/contents/go.mod -H "Accept: application/vnd.github.raw" > "$HOST_MOD"
fi
HOST_MOD=$(cd "$(dirname "$HOST_MOD")" && pwd)/$(basename "$HOST_MOD")

studio_mods=(-studio "$ROOT/go.mod")
if [ -f "$ROOT/enterprise/go.mod" ]; then
  studio_mods+=(-studio "$ROOT/enterprise/go.mod")
fi
(cd "$ROOT/tools/hostcompat" && go run . -host "$HOST_MOD" "${studio_mods[@]}" -allow "$ROOT/scripts/host-compat-allow.txt")

$BUILD || exit 0

build() { # edition
  local edition=$1 dir="$WORK/host-$1" tags=studio_noui
  mkdir -p "$dir"
  sed -e 's#^module .*#module example.com/studiohostcompat#' "$HOST_MOD" > "$dir/go.mod"
  {
    echo
    echo "require github.com/TykTechnologies/midsommar/v2 v2.0.0-00010101000000-000000000000"
    echo "replace github.com/TykTechnologies/midsommar/v2 => $ROOT"
  } >> "$dir/go.mod"
  local imports="\"github.com/TykTechnologies/midsommar/v2/pkg/studio\""
  if [ "$edition" = ent ]; then
    tags="studio_noui enterprise"
    {
      echo "require github.com/TykTechnologies/ai-studio-enterprise/v2 v2.0.0-00010101000000-000000000000"
      echo "replace github.com/TykTechnologies/ai-studio-enterprise/v2 => $ROOT/enterprise"
    } >> "$dir/go.mod"
    imports="$imports
	_ \"github.com/TykTechnologies/ai-studio-enterprise/v2/all\""
  fi
  printf 'package main\n\nimport (\n\t"fmt"\n\n\t%s\n)\n\nfunc main() { fmt.Println(studio.ErrAlreadyRunning) }\n' "$imports" > "$dir/main.go"
  (
    cd "$dir"
    export GOFLAGS=-mod=mod GOWORK=off CGO_ENABLED=0 GOPRIVATE=${GOPRIVATE:-github.com/TykTechnologies}
    go build -tags "$tags" -o /dev/null .
    echo "host-compat: pkg/studio ($edition) builds in the host's module graph with CGO_ENABLED=0."
    # Listing the whole graph resolves every module in it, including the
    # enterprise module Studio's go.mod names at a placeholder version. A host
    # that builds the enterprise edition (the Dashboard always does) requires
    # a real version instead, so only the ent build can list it.
    if [ "$edition" = ent ]; then
      go list -m -f '{{.Path}} {{.Version}}' all > selected.txt
    fi
  )
  [ "$edition" = ent ] || return 0
  python3 - "$HOST_MOD" "$dir/selected.txt" "$edition" <<'PY'
import re, sys
host, sel, ed = sys.argv[1:]
want = {m.group(1): m.group(2) for m in re.finditer(r'^\s*(?:require\s+)?(\S+)\s+(v\S+)', open(host).read(), re.M)}
def key(v):
    v = v.lstrip('v').split('+')[0]; base, _, pre = v.partition('-')
    return [int(x) for x in base.split('.')], (pre == '', pre)
up = [(p, want[p], v) for p, v in (l.split() for l in open(sel) if len(l.split()) == 2)
      if p in want and v != want[p] and key(v) > key(want[p])]
print(f"host-compat: {len(up)} module(s) end up above the host's go.mod in the {ed} build" + (":" if up else "."))
for p, h, v in up: print(f"  {p} {h} -> {v}")
PY
}
build ce
if [ -f "$ROOT/enterprise/go.mod" ]; then build ent; fi
