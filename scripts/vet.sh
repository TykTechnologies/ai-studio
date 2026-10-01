#!/usr/bin/env bash
# Runs `go vet` over AI Studio's own code: the root, microgateway and
# enterprise modules, in the Community and Enterprise editions.
#
# third_party/ is left out. third_party/gorm.io is a vendored copy of
# upstream gorm that is never edited by hand, and upstream trips vet's
# lostcancel check (callbacks.go, finisher_api.go); third_party/langchaingo
# is Tyk's fork, checked by its own tests. See third_party/README.md.
#
#   scripts/vet.sh            # both editions (make vet)
#   scripts/vet.sh ce         # Community Edition: root and microgateway
#   scripts/vet.sh ent        # Enterprise: root, microgateway and enterprise
#
# CE_ONLY=1 vets the Community Edition only, for checkouts without the
# private enterprise submodule (CI on pull requests from forks).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

edition=${1:-all}
if [ "${CE_ONLY:-}" = 1 ]; then
  if [ "$edition" = ent ]; then
    echo "vet: CE_ONLY=1, not vetting the enterprise edition" >&2
    exit 0
  fi
  edition=ce
fi

case "$edition" in
  ce) runs=(":." ":microgateway") ;;
  ent) runs=("enterprise:." "enterprise:microgateway" "enterprise:enterprise") ;;
  all) runs=(":." ":microgateway" "enterprise:." "enterprise:microgateway" "enterprise:enterprise") ;;
  *) echo "usage: $0 [ce|ent|all]" >&2; exit 2 ;;
esac

status=0
for run in "${runs[@]}"; do
  tags=${run%%:*}
  mod=${run#*:}
  if [ ! -f "$mod/go.mod" ]; then
    echo "vet: $mod/go.mod missing (is the enterprise submodule checked out?)" >&2
    exit 1
  fi
  echo "vet: module $mod${tags:+ (-tags $tags)}"
  # go list runs on its own so that a failure stops the check rather than
  # vetting nothing.
  pkgs=$(cd "$mod" && go list ${tags:+-tags "$tags"} ./... | grep -v '/third_party/')
  # shellcheck disable=SC2086 # one argument per package
  if ! (cd "$mod" && go vet ${tags:+-tags "$tags"} $pkgs); then
    status=1
  fi
done
exit $status
