#!/usr/bin/env bash
# Checks that every in-repo plugin module is tidy against this tree.
#
#   scripts/plugins-mod-check.sh [--build] [root ...]
#
# Plugin modules replace github.com/TykTechnologies/midsommar/v2 with the
# checkout, so when the root go.mod raises a requirement their go.mod must
# follow, or `go build` stops at "updates to go.mod needed". That breaks
# `make plugins`, `make plugin-publish` (tools/publish-plugin.sh) and the dev
# plugin watcher, and no other check builds these modules.
#
# For each go.mod under the roots (default: examples, enterprise/plugins,
# community/plugins, tyk-internal/plugins; missing roots are skipped, so a
# checkout without the private submodules checks what it has) this runs
# `go mod tidy -diff`. A go.mod change fails. A go.sum change fails only
# where go.sum is tracked: enterprise plugin go.sum files are gitignored, so
# a fresh checkout has none. --build also runs `go build ./...` in each
# module (with -tags enterprise under enterprise/).
#
# Fix a failure with `go mod tidy` in the module named.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

BUILD=false
if [ "${1:-}" = --build ]; then BUILD=true; shift; fi

roots=("$@")
if [ ${#roots[@]} -eq 0 ]; then
  roots=(examples enterprise/plugins community/plugins tyk-internal/plugins)
fi

export GOWORK=off
export GOFLAGS="${GOFLAGS:-} -mod=readonly"
export GOPRIVATE=${GOPRIVATE:-github.com/TykTechnologies}

modules=()
for root in "${roots[@]}"; do
  [ -d "$root" ] || { echo "plugins-mod-check: skipping $root (not present)" >&2; continue; }
  while IFS= read -r mod; do
    modules+=("$(dirname "$mod")")
  done < <(find "$root" -name go.mod -not -path '*/node_modules/*' | sort)
done

if [ ${#modules[@]} -eq 0 ]; then
  echo "plugins-mod-check: no plugin modules found" >&2
  exit 0
fi

failed=()
for dir in "${modules[@]}"; do
  sum_tracked=false
  if (cd "$dir" && git ls-files --error-unmatch go.sum >/dev/null 2>&1); then
    sum_tracked=true
  fi

  status=ok
  if ! out=$(cd "$dir" && go mod tidy -diff 2>&1); then
    if grep -q '^+++ tidy/go.mod' <<<"$out"; then
      status="go.mod is not tidy"
    elif $sum_tracked && grep -q '^+++ tidy/go.sum' <<<"$out"; then
      status="go.sum is not tidy"
    elif ! grep -q '^+++ tidy/' <<<"$out"; then
      status="go mod tidy failed"
    fi
  fi

  if [ "$status" = ok ] && $BUILD; then
    tags=()
    case "$dir" in enterprise/*) tags=(-tags enterprise) ;; esac
    if ! out=$(cd "$dir" && go build ${tags[@]+"${tags[@]}"} -o /dev/null ./... 2>&1); then
      status="go build failed"
    fi
  fi

  if [ "$status" = ok ]; then
    echo "ok    $dir"
  else
    echo "FAIL  $dir: $status" >&2
    sed 's/^/      /' <<<"$out" | head -40 >&2
    failed+=("$dir")
  fi
done

if [ ${#failed[@]} -gt 0 ]; then
  echo >&2
  echo "plugins-mod-check: ${#failed[@]} module(s) failed (run \`go mod tidy\` in each):" >&2
  printf '  %s\n' "${failed[@]}" >&2
  exit 1
fi
echo "plugins-mod-check: ${#modules[@]} plugin module(s) tidy"
