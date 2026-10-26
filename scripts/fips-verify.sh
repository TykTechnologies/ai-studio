#!/usr/bin/env bash
# Checks the build metadata of FIPS and standard binaries, so a binary sold as
# FIPS really links the Go Cryptographic Module v1.0.0 (CMVP #5247) and a
# standard one does not (the Phase 0 bug: "-fips" packages built without
# GOFIPS140). See the FIPS epic, midsommar-clean-8dy7.
#
# A FIPS binary must show, in `go version -m`:
#   - GOFIPS140=v1.0.0 (or v1.0.0-<hash>): the frozen, certified module;
#   - DefaultGODEBUG including fips140=on: FIPS mode by default;
#   - the "fips" build tag (Tyk's marker for FIPS builds);
# and `go tool nm` must show no _goboringcrypto symbols (BoringCrypto is a
# different module).
# A standard binary must show no GOFIPS140 setting and no fips140 default.
# With --edition ent, every binary must also carry the enterprise build tag.
#
# Usage:
#   scripts/fips-verify.sh [--edition ce|ent] --fips BIN...
#   scripts/fips-verify.sh [--edition ce|ent] --std BIN...
#   scripts/fips-verify.sh [--edition ce|ent] --dist DIR
#
# --dist checks every Go binary under DIR (goreleaser's dist/): one in a
# directory whose name contains "fips" is checked as FIPS, any other as
# standard. It fails if DIR holds no FIPS binary or no standard binary.
#
# Exit status: 0 when every binary passes, 1 otherwise, 2 on a usage error.
set -euo pipefail

usage() {
  sed -n '/^# Usage:/,/^# Exit status/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

edition=""
mode=""
args=()
while [ $# -gt 0 ]; do
  case "$1" in
    --edition) edition=${2:-}; shift 2 ;;
    --fips|--std|--dist) [ -z "$mode" ] || usage; mode=${1#--}; shift ;;
    -h|--help) usage ;;
    *) args+=("$1"); shift ;;
  esac
done
[ -n "$mode" ] && [ ${#args[@]} -gt 0 ] || usage
case "$edition" in ""|ce|ent) ;; *) usage ;; esac

failures=0
fail() {
  echo "fips-verify: FAIL $1: $2" >&2
  failures=$((failures + 1))
}

# build_setting BUILDINFO KEY prints the value of "build KEY=..." or nothing.
build_setting() {
  printf '%s\n' "$1" | awk -v k="$2" -F'\t' '$2 == "build" && index($3, k "=") == 1 { print substr($3, length(k) + 2); exit }'
}

has_tag() { # has_tag TAGS TAG
  case ",$1," in *",$2,"*) return 0 ;; *) return 1 ;; esac
}

check() { # check fips|std BIN
  local kind=$1 bin=$2 info gofips godebug tags before=$failures
  if ! info=$(go version -m "$bin" 2>/dev/null); then
    fail "$bin" "not a Go binary"
    return
  fi
  gofips=$(build_setting "$info" GOFIPS140)
  godebug=$(build_setting "$info" DefaultGODEBUG)
  tags=$(build_setting "$info" -tags)

  if [ "$kind" = fips ]; then
    case "$gofips" in
      v1.0.0|v1.0.0-*) ;;
      "") fail "$bin" "no GOFIPS140 (not built against the Go Cryptographic Module)" ;;
      *) fail "$bin" "GOFIPS140=$gofips, want v1.0.0 (the CMVP-validated module)" ;;
    esac
    has_tag "$godebug" fips140=on || fail "$bin" "DefaultGODEBUG='$godebug' lacks fips140=on"
    has_tag "$tags" fips || fail "$bin" "build tags '$tags' lack the fips marker"
    if go tool nm "$bin" 2>/dev/null | grep -q _goboringcrypto; then
      fail "$bin" "links BoringCrypto (_goboringcrypto symbols)"
    fi
  else
    [ -z "$gofips" ] || fail "$bin" "standard binary built with GOFIPS140=$gofips"
    case ",$godebug," in *",fips140="*) fail "$bin" "standard binary defaults to $godebug" ;; esac
    ! has_tag "$tags" fips || fail "$bin" "standard binary carries the fips tag"
  fi
  if [ "$edition" = ent ]; then
    has_tag "$tags" enterprise || fail "$bin" "ENT build without the enterprise tag (tags '$tags')"
  fi
  [ "$failures" -eq "$before" ] && echo "fips-verify: ok   $kind ${edition:+$edition }$bin"
  return 0
}

if [ "$mode" = dist ]; then
  [ ${#args[@]} -eq 1 ] && [ -d "${args[0]}" ] || usage
  nfips=0 nstd=0
  while IFS= read -r -d '' f; do
    go version -m "$f" >/dev/null 2>&1 || continue
    case "$(basename "$(dirname "$f")")" in
      *fips*) check fips "$f"; nfips=$((nfips + 1)) ;;
      *) check std "$f"; nstd=$((nstd + 1)) ;;
    esac
  done < <(find "${args[0]}" -mindepth 2 -maxdepth 2 -type f -perm -u+x -print0 | sort -z)
  [ "$nfips" -gt 0 ] || fail "${args[0]}" "no FIPS binaries found"
  [ "$nstd" -gt 0 ] || fail "${args[0]}" "no standard binaries found"
else
  for bin in "${args[@]}"; do
    check "$mode" "$bin"
  done
fi

if [ "$failures" -gt 0 ]; then
  echo "fips-verify: $failures failure(s)" >&2
  exit 1
fi
