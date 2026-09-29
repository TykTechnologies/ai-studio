#!/usr/bin/env bash
# Builds third_party/gorm.io from the gorm modules pinned in
# third_party/gorm-pin/go.mod: each module's source as published (checked
# against gorm-pin/go.sum), without its go.mod/go.sum, with "gorm.io/..."
# imports rewritten to this repository's copy. See third_party/README.md.
#
#   scripts/gorm-vendor.sh           rewrite third_party/gorm.io
#   scripts/gorm-vendor.sh --verify  fail unless third_party/gorm.io is
#                                    exactly what the pins produce
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PIN=$ROOT/third_party/gorm-pin
DEST=$ROOT/third_party/gorm.io
PATCHES=$ROOT/third_party/patches
PREFIX=github.com/TykTechnologies/midsommar/v2/third_party/
MODULES=(gorm.io/gorm gorm.io/driver/postgres gorm.io/driver/sqlite)

verify=false
case "${1:-}" in
  "") ;;
  --verify) verify=true ;;
  *) echo "usage: $0 [--verify]" >&2; exit 2 ;;
esac

work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT
out=$work/gorm.io

# -mod=readonly: a pin whose hash is not in gorm-pin/go.sum fails here
# rather than being added.
(cd "$PIN" && GOFLAGS=-mod=readonly go mod download "${MODULES[@]}")

mkdir -p "$out"
: > "$out/VERSIONS"
for m in "${MODULES[@]}"; do
  read -r version dir < <(cd "$PIN" && GOFLAGS=-mod=readonly go list -m -f '{{.Version}} {{.Dir}}' "$m")
  if [ -z "${dir:-}" ] || [ ! -d "$dir" ]; then
    echo "gorm-vendor: $m is not in the module cache" >&2
    exit 1
  fi
  target=$out/${m#gorm.io/}
  mkdir -p "$target"
  cp -R "$dir"/. "$target"/
  chmod -R u+w "$target"
  # Without its go.mod the copy is part of the main module.
  rm -f "$target/go.mod" "$target/go.sum"
  echo "$m $version" >> "$out/VERSIONS"
done

# The only change to upstream source: imports of gorm.io/... name the copy.
find "$out" -name '*.go' -exec perl -pi -e 's#"gorm\.io/#"'"$PREFIX"'gorm.io/#g' {} +

# Local patches, applied in name order. Policy is to have none.
if [ -d "$PATCHES" ]; then
  for p in "$PATCHES"/*.patch; do
    [ -e "$p" ] || continue
    patch -s -p1 -d "$work" < "$p"
  done
fi

if $verify; then
  if ! diff -r "$out" "$DEST" > "$work/diff.txt"; then
    echo "gorm-vendor: third_party/gorm.io does not match the pins in third_party/gorm-pin/go.mod." >&2
    echo "Run scripts/gorm-vendor.sh and commit the result; never edit third_party/gorm.io by hand." >&2
    head -50 "$work/diff.txt" >&2
    exit 1
  fi
  echo "third_party/gorm.io matches:"
  cat "$out/VERSIONS"
  exit 0
fi

rm -rf "$DEST"
cp -R "$out" "$DEST"
echo "Wrote third_party/gorm.io:"
cat "$out/VERSIONS"
