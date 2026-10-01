#!/usr/bin/env bash
# Checks that AI Studio builds only with its own copy of gorm:
#   1. third_party/gorm.io is exactly what third_party/gorm-pin produces;
#   2. no package of the root, microgateway or enterprise module, in either
#      edition, imports gorm.io/... . gorm finds model hooks, column types
#      (GormDBDataType) and sentinel errors by type assertion at runtime,
#      so a stray import of the other gorm compiles and silently misbehaves.
#      A third-party library in ALLOWED_GORM_IMPORTERS may use its own
#      upstream gorm internally; it never touches Studio's models;
#   3. the copy's own tests pass.
# See third_party/README.md.
#
# CE_ONLY=1 checks the root and microgateway modules in the CE edition only,
# for checkouts without the private enterprise submodule (CI on pull requests
# from forks, which gets a stub enterprise/go.mod).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

modules=(. microgateway enterprise)
editions=("" enterprise)
if [ "${CE_ONLY:-}" = 1 ]; then
  modules=(. microgateway)
  editions=("")
  echo "gorm-verify: CE_ONLY=1, not checking the enterprise module or the enterprise edition" >&2
fi

"$ROOT/scripts/gorm-vendor.sh" --verify

status=0
if matches=$(git grep -n --recurse-submodules '"gorm\.io/' -- '*.go' ':!third_party/'); then
  echo "Import gorm from github.com/TykTechnologies/midsommar/v2/third_party/gorm.io, not gorm.io:" >&2
  echo "$matches" >&2
  status=1
fi

# Third-party packages allowed to import upstream gorm for their own use
# (an extended regexp of import-path prefixes). Never add a Studio path.
ALLOWED_GORM_IMPORTERS='github\.com/TykTechnologies/storage/'

for mod in "${modules[@]}"; do
  if [ ! -f "$mod/go.mod" ]; then
    echo "gorm-verify: $mod/go.mod missing (is the enterprise submodule checked out?)" >&2
    exit 1
  fi
  for tags in "${editions[@]}"; do
    # go list runs on its own so that a failure stops the check rather
    # than reading as "no gorm.io packages".
    deps=$(cd "$mod" && go list -deps -test ${tags:+-tags "$tags"} -f '{{.ImportPath}} {{join .Imports " "}}' ./...)
    # Who imports upstream gorm? Studio's own packages never may. A
    # third-party library may use its own gorm internally (it never sees
    # Studio's models): TykTechnologies/storage v1.5+, via TIB, has a
    # Postgres driver built on it. Such importers are allowlisted by prefix.
    if found=$(awk '$1 !~ /^gorm\.io\// { for (i = 2; i <= NF; i++) if ($i ~ /^gorm\.io\//) print $1 " imports " $i }' <<< "$deps" \
        | grep -v -E "^($ALLOWED_GORM_IMPORTERS)"); then
      echo "gorm-verify: module $mod${tags:+ (-tags $tags)} imports upstream gorm packages:" >&2
      echo "$found" >&2
      status=1
    fi
  done
done
[ "$status" -eq 0 ] || exit "$status"
echo "No Studio package imports upstream gorm (third-party importers allowed: $ALLOWED_GORM_IMPORTERS)."

go test -count=1 ./third_party/...
