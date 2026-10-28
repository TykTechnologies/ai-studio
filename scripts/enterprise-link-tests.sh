#!/usr/bin/env bash
# Links the enterprise files of core packages into their packages.
#
# Tests that exercise core packages with enterprise features registered live
# in the private enterprise repository, under enterprise/_coretests/<package
# dir>/*_entlink_test.go, not in this repository: `go mod tidy` in a program
# that imports Studio reads the tests of every package it imports, under
# every build tag, so a committed test importing the enterprise module would
# make each Community Edition consumer try to fetch the private repository.
#
# The same goes for the enterprise registrations of core main packages (the
# Studio binary, examples/embed-host): this module does not require the
# enterprise module at all, so that plugin authors' full module graphs
# (`go list -m all`, govulncheck) resolve, and the blank imports of
# enterprise/all live in enterprise/_coretests/<dir>/*_entlink.go. Enterprise
# builds resolve the module through enterprise.work (GOWORK).
#
# This script symlinks each of them into the core package directory it
# belongs to (they are gitignored there), and removes links whose source is
# gone. `make init-enterprise`, the build and test targets and the enterprise
# CI jobs run it; without the submodule it does nothing. Edit the files in
# place: the links point at the enterprise repository.
#
# --unlink removes the links and links nothing (`make tidy` runs `go mod
# tidy` between --unlink and a relink, since tidy does not use the workspace).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
SRC=enterprise/_coretests

# Drop existing links (and stray copies) so deleted or renamed files go too.
find . \( -name '*_entlink_test.go' -o -name '*_entlink.go' \) \
  -not -path './enterprise/*' -not -path './.claude/*' -not -path '*/node_modules/*' \
  -exec rm -f {} +

if [ "${1:-}" = "--unlink" ]; then
  echo "Unlinked the enterprise files of core packages."
  exit 0
fi

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
  if [ "$dir" = "." ]; then
    ln -s "$src" "$(basename "$src")"
  else
    up=$(printf '%s' "$dir" | sed -e 's#[^/][^/]*#..#g')
    ln -s "$up/$src" "$dir/$(basename "$src")"
  fi
  n=$((n + 1))
done < <(find "$SRC" \( -name '*_entlink_test.go' -o -name '*_entlink.go' \) | sort)
echo "Linked $n enterprise files of core packages."
