#!/usr/bin/env bash
# Links the enterprise tests of core packages into their packages.
#
# Tests that exercise core packages with enterprise features registered live
# in the private enterprise repository, under enterprise/_coretests/<package
# dir>/*_entlink_test.go, not in this repository: `go mod tidy` in a program
# that imports Studio reads the tests of every package it imports, under
# every build tag, so a committed test importing the enterprise module would
# make each Community Edition consumer try to fetch the private repository.
#
# This script symlinks each of them into the core package directory it
# belongs to (they are gitignored there), and removes links whose source is
# gone. `make init-enterprise`, the test targets and the enterprise CI jobs
# run it; without the submodule it does nothing. Edit the files in place:
# the links point at the enterprise repository.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
SRC=enterprise/_coretests

# Drop existing links (and stray copies) so deleted or renamed tests go too.
find . -name '*_entlink_test.go' \
  -not -path './enterprise/*' -not -path './.claude/*' -not -path '*/node_modules/*' \
  -exec rm -f {} +

if [ ! -d "$SRC" ]; then
  echo "enterprise-link-tests: no $SRC (Community Edition checkout); nothing to link"
  exit 0
fi

n=0
while IFS= read -r src; do
  rel=${src#"$SRC"/}
  dir=$(dirname "$rel")
  if [ ! -d "$dir" ]; then
    echo "enterprise-link-tests: $src belongs to $dir, which does not exist in core" >&2
    exit 1
  fi
  up=$(printf '%s' "$dir" | sed -e 's#[^/][^/]*#..#g')
  ln -s "$up/$src" "$dir/$(basename "$src")"
  n=$((n + 1))
done < <(find "$SRC" -name '*_entlink_test.go' | sort)
echo "Linked $n enterprise tests of core packages."
