#!/usr/bin/env bash
# Checks that AI Studio builds only with its own copy of langchaingo
# (third_party/langchaingo, see third_party/README.md): no package of the
# root, microgateway or enterprise module, in either edition, may import
# github.com/tmc/langchaingo. A replace directive is not an option, because Go
# ignores it in a module that imports Studio, which would then build with
# upstream langchaingo and without Tyk's fixes.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

status=0
if matches=$(git grep -n --recurse-submodules '"github\.com/tmc/langchaingo' -- '*.go' ':!third_party/'); then
  echo "Import langchaingo from github.com/TykTechnologies/midsommar/v2/third_party/langchaingo, not github.com/tmc/langchaingo:" >&2
  echo "$matches" >&2
  status=1
fi

for mod in . microgateway enterprise; do
  if [ ! -f "$mod/go.mod" ]; then
    echo "langchaingo-verify: $mod/go.mod missing (is the enterprise submodule checked out?)" >&2
    exit 1
  fi
  if grep -q 'github.com/tmc/langchaingo' "$mod/go.mod"; then
    echo "langchaingo-verify: $mod/go.mod still names github.com/tmc/langchaingo" >&2
    status=1
  fi
  for tags in "" enterprise; do
    deps=$(cd "$mod" && go list -deps -test ${tags:+-tags "$tags"} ./...)
    if found=$(grep '^github\.com/tmc/langchaingo' <<< "$deps"); then
      echo "langchaingo-verify: module $mod${tags:+ (-tags $tags)} builds with upstream langchaingo:" >&2
      echo "$found" >&2
      status=1
    fi
  done
done
[ "$status" -eq 0 ] || exit "$status"
echo "No github.com/tmc/langchaingo packages in the root, microgateway or enterprise build graphs."
