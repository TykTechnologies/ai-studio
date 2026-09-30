#!/usr/bin/env bash
# Checks AI Studio against the go.mod of each Tyk product that embeds it: by
# default the Tyk Dashboard (tyk-analytics) and MDCB (tyk-sink), fetched from
# their private repositories (never commit a copy of them here).
#
#   scripts/host-compat.sh [--build] [path/to/host/go.mod]
#
# 1. Version floor (no network): tools/hostcompat fails if Studio's go.mod
#    or the enterprise module's requires any module above the host's
#    version, since Go's version selection would silently raise it in the
#    host, unless the host's allowlist (scripts/host-compat-allow.<repo>.txt)
#    names it with a reason.
# 2. With --build: builds pkg/studio inside the host's module graph (its
#    requirements and replaces) with CGO_ENABLED=0, for the Community and
#    Enterprise editions, and lists every module the result selects above
#    the host's go.mod. Needs read access to the host's private
#    dependencies (GOPRIVATE=github.com/TykTechnologies plus git credentials).
#
# Without a path it fetches the go.mod of every repository in HOST_REPOS
# (default "tyk-analytics tyk-sink") under TykTechnologies with gh (GH_TOKEN),
# checks each in turn and fails if any fails. With a path it checks that one
# go.mod against HOST_ALLOW (default the Dashboard's allowlist). See
# features/Embedding.md, "Host version floor".
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
BUILD=false
if [ "${1:-}" = --build ]; then BUILD=true; shift; fi

WORK=$(mktemp -d)
trap 'chmod -R u+w "$WORK" 2>/dev/null; rm -rf "$WORK"' EXIT

studio_mods=(-studio "$ROOT/go.mod")
if [ -f "$ROOT/enterprise/go.mod" ]; then
  studio_mods+=(-studio "$ROOT/enterprise/go.mod")
fi

build() { # host-go.mod work-dir edition
  local host_mod=$1 dir="$2/host-$3" edition=$3 tags=studio_noui
  mkdir -p "$dir"
  sed -e 's#^module .*#module example.com/studiohostcompat#' "$host_mod" > "$dir/go.mod"
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
  python3 - "$host_mod" "$dir/selected.txt" "$edition" <<'PY'
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

check() { # name host-go.mod allowlist
  local name=$1 host_mod=$2 allow=$3 work="$WORK/$1"
  mkdir -p "$work"
  host_mod=$(cd "$(dirname "$host_mod")" && pwd)/$(basename "$host_mod")
  (cd "$ROOT/tools/hostcompat" && go run . -host "$host_mod" "${studio_mods[@]}" -allow "$allow")
  $BUILD || return 0
  build "$host_mod" "$work" ce
  if [ -f "$ROOT/enterprise/go.mod" ]; then build "$host_mod" "$work" ent; fi
}

# run checks one host in a subshell of its own, so a failure (set -e) ends
# that host's check and the next host is still checked.
failed=()
run() { # name host-go.mod allowlist
  echo "=== host-compat: $1 ==="
  set +e
  (set -e; check "$@")
  local rc=$?
  set -e
  if [ $rc -ne 0 ]; then failed+=("$1"); echo "host-compat: $1 FAILED"; fi
  echo
}

if [ -n "${1:-}" ]; then
  run "$(basename "$1")" "$1" "${HOST_ALLOW:-$ROOT/scripts/host-compat-allow.tyk-analytics.txt}"
else
  for repo in ${HOST_REPOS:-tyk-analytics tyk-sink}; do
    mod="$WORK/$repo.go.mod"
    if ! gh api "repos/TykTechnologies/$repo/contents/go.mod" -H "Accept: application/vnd.github.raw" > "$mod"; then
      echo "=== host-compat: $repo ==="
      echo "host-compat: cannot read TykTechnologies/$repo's go.mod (GH_TOKEN needs read access)"
      failed+=("$repo")
      continue
    fi
    run "$repo" "$mod" "$ROOT/scripts/host-compat-allow.$repo.txt"
  done
fi

if [ ${#failed[@]} -gt 0 ]; then
  echo "host-compat: failed for ${failed[*]}" >&2
  exit 1
fi
