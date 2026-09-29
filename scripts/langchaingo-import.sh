#!/usr/bin/env bash
# Copies the langchaingo packages AI Studio uses from a langchaingo module
# into a directory, rewriting their imports to the in-tree path. It made
# third_party/langchaingo (see third_party/README.md). The copy is edited in
# place afterwards, so to take upstream changes, import into a scratch
# directory and merge the diff by hand:
#
#   scripts/langchaingo-import.sh github.com/tmc/langchaingo@v0.1.14 /tmp/lcg
#   diff -ru /tmp/lcg third_party/langchaingo
#
# Only non-test files of the packages in third_party/langchaingo/PACKAGES are
# copied (plus files they //go:embed and the LICENSE): most of langchaingo's
# tests pull in testcontainers and friends, which would land in every host's
# module graph. The packages in TESTED (the LLM clients Tyk patches) keep
# their tests and testdata; those need only testify, go-cmp and the
# internal/httprr and testing/llmtest helpers.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
MODULE=${1:?usage: $0 <module@version> <destination dir>}
DEST=${2:?usage: $0 <module@version> <destination dir>}
PACKAGES="$ROOT/third_party/langchaingo/PACKAGES"
TESTED="$ROOT/third_party/langchaingo/TESTED"
NEWPATH=github.com/TykTechnologies/midsommar/v2/third_party/langchaingo

if [ -e "$DEST" ] && [ -n "$(ls -A "$DEST" 2>/dev/null | grep -v -e '^PACKAGES$' -e '^TESTED$' -e '^VERSION$' -e '^README.md$')" ]; then
  echo "langchaingo-import: $DEST is not empty; import into a scratch directory and diff" >&2
  exit 1
fi

SRC=$(cd "$ROOT" && GOFLAGS=-mod=mod go mod download -json "$MODULE" | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')
[ -d "$SRC" ] || { echo "langchaingo-import: could not download $MODULE" >&2; exit 1; }

mkdir -p "$DEST"
DEST=$(cd "$DEST" && pwd)
cp "$SRC/LICENSE" "$DEST/LICENSE"

while read -r pkg; do
  [ -z "$pkg" ] && continue
  mkdir -p "$DEST/$pkg"
  for f in "$SRC/$pkg"/*.go; do
    case "$f" in *_test.go) continue ;; esac
    cp "$f" "$DEST/$pkg/"
    # Files a package embeds, e.g. chains' prompt templates.
    sed -n 's|^//go:embed \(.*\)$|\1|p' "$f" | tr ' ' '\n' | while read -r pattern; do
      [ -z "$pattern" ] && continue
      (cd "$SRC/$pkg" && for m in $pattern; do
        mkdir -p "$DEST/$pkg/$(dirname "$m")"
        cp -R "$m" "$DEST/$pkg/$(dirname "$m")/"
      done)
    done
  done
done < "$PACKAGES"

while read -r pkg; do
  [ -z "$pkg" ] && continue
  cp "$SRC/$pkg"/*_test.go "$DEST/$pkg/"
  if [ -d "$SRC/$pkg/testdata" ]; then
    cp -R "$SRC/$pkg/testdata" "$DEST/$pkg/"
  fi
done < "$TESTED"

chmod -R u+w "$DEST"
find "$DEST" -name '*.go' -exec perl -pi -e "s#\"github\\.com/tmc/langchaingo/#\"$NEWPATH/#g" {} +
# The rewritten paths sort differently; gofmt puts the imports back in order.
gofmt -w "$DEST"

# Every langchaingo package the copy imports must itself be in the copy.
missing=$(grep -rhoE "\"$NEWPATH/[^\"]+\"" "$DEST" --include='*.go' | tr -d '"' | sed "s#^$NEWPATH/##" | sort -u | while read -r p; do
  [ -d "$DEST/$p" ] || echo "$p"
done)
if [ -n "$missing" ]; then
  echo "langchaingo-import: add these to PACKAGES, they are imported by the copy:" >&2
  echo "$missing" >&2
  exit 1
fi
echo "Imported $(wc -l < "$PACKAGES" | tr -d ' ') packages of $MODULE into $DEST"
