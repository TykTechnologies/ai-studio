#!/usr/bin/env bash
# Runs the Go test suites in FIPS 140-3 strict mode: built against the Go
# Cryptographic Module v1.0.0 (GOFIPS140=v1.0.0) and run with
# GODEBUG=fips140=only, under which any non-approved algorithm returns an
# error or panics. This is the exit criterion of FIPS Phase 2 (see the
# midsommar-clean-8dy7 epic and docs/site/docs/fips.md).
#
# Usage: scripts/test-fips.sh [ce|ent]     (make test-fips [EDITION=ent])
#
# Modules: the root module and the microgateway module, ./... with -short
# (the unit packages test-ci runs; the Postgres-gated tests skip without
# DATABASE_URL), plus, in the ent edition, the enterprise module. Not -race:
# the race detector does not change which algorithms run, and halves the
# time.
#
# Each module runs the strict-mode canary, TestFIPSStrictCanary
# (pkg/fipstest), with FIPS_STRICT_CANARY=1. The run fails if a canary did
# not pass, since a run that did not enforce strict mode proves nothing.
#
# Output: go test -json streams, stderr and a Markdown summary (failed
# packages and tests, the end of the output of a package that failed
# without a failing test) in FIPS_TEST_OUT (default: a temporary
# directory). The summary is printed, and appended to $GITHUB_STEP_SUMMARY
# when set. Exits 1 if anything failed.
#
# The ent edition fails at once unless enterprise/go.mod is the real module
# (not the CE stub), and needs make ent-link first, which make test-fips
# does.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

edition=${1:-${TEST_EDITION:-ce}}
case "$edition" in
  ce) tags=() ;;
  ent) tags=(-tags enterprise) ;;
  *) echo "usage: $0 [ce|ent]" >&2; exit 2 ;;
esac
if [ "$edition" = ent ] && ! grep -q '^require' enterprise/go.mod 2>/dev/null; then
  echo "test-fips: the ent edition needs the enterprise module, but enterprise/go.mod is missing or a stub" >&2
  echo "(git submodule update --init enterprise)" >&2
  exit 1
fi

out=${FIPS_TEST_OUT:-$(mktemp -d)}
mkdir -p "$out"
timeout=${FIPS_TEST_TIMEOUT:-25m}

export GOFIPS140=v1.0.0
export GODEBUG=fips140=only
export FIPS_STRICT_CANARY=1

canary=github.com/TykTechnologies/midsommar/v2/pkg/fipstest

# run <module> <dir> <package pattern ...>: go test -json into $out/<module>-<edition>.json.
# A failing go test is recorded, not fatal: every module runs.
summary_args=()
run() {
  local name=$1 dir=$2 file="$out/$1-$edition"
  shift 2
  echo ">>> $name ($edition): GOFIPS140=$GOFIPS140 GODEBUG=$GODEBUG go test ${tags[*]} -short -count=1 $* (in $dir)" >&2
  (cd "$dir" && go test "${tags[@]}" -json -short -count=1 -timeout "$timeout" "$@") \
    > "$file.json" 2> "$file.stderr" || true
  if [ -s "$file.stderr" ]; then
    tail -n 20 "$file.stderr" >&2
  fi
  summary_args+=("$name ($edition)=$file.json")
}

run root . ./...
run microgateway microgateway ./...
if [ "$edition" = ent ]; then
  # The enterprise module has no canary of its own: run the root one built
  # in the enterprise module.
  run enterprise enterprise ./... "$canary"
fi

status=0
python3 -I "$ROOT/scripts/test-fips-summary.py" "${summary_args[@]}" > "$out/summary.md" || status=$?
cat "$out/summary.md"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  cat "$out/summary.md" >> "$GITHUB_STEP_SUMMARY"
fi
echo "test-fips: results in $out" >&2
exit "$status"
