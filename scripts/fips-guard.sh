#!/usr/bin/env bash
# Checks that first-party Go code uses only FIPS 140-3 approved cryptography
# (the "stdlib crypto only (FIPS rule)" of pkg/agenttrace, made a rule for the
# whole tree). It fails on:
#   - imports of crypto/md5, crypto/sha1, crypto/des, crypto/rc4, crypto/dsa
#     and anything under golang.org/x/crypto/. x/crypto is outside the Go
#     Cryptographic Module even where the algorithm is approved (pbkdf2,
#     hkdf, sha3), and much of it runs without error under fips140=only, so
#     only this guard catches it; use the standard library package
#     (crypto/pbkdf2, crypto/hkdf, crypto/sha3, crypto/mlkem, ...);
#   - uses (calls or method values) of cipher.NewGCM,
#     cipher.NewGCMWithNonceSize, cipher.NewGCMWithTagSize (caller-supplied
#     nonces, refused under GODEBUG=fips140=only; use
#     cipher.NewGCMWithRandomNonce) and cipher.NewCFBEncrypter /
#     cipher.NewCFBDecrypter;
#   - importing crypto/cipher under any name but cipher (rule
#     crypto/cipher-alias), which would hide the uses above from this check;
# unless the file and rule are listed in scripts/fips-guard.allowlist, each
# with a one-line reason. That list holds the known sites the FIPS epic
# (midsommar-clean-8dy7) remediates, and the dual-format readers that stay.
# It only shrinks: an entry that matches nothing fails the check too, so a
# remediation removes its entry in the same change.
#
# Scanned: every Go file of this repository, tracked or untracked but not
# ignored (tests included, since `make test-fips` runs them under
# fips140=only), and the enterprise submodule when it is checked out. Not
# scanned: the community/ and tyk-internal/ plugin submodules (their own
# repositories, built as plugins).
#
# This is a line-based text match, not a type checker. Comment lines are
# skipped; block comments are not understood, and a use split over two lines
# (cipher.<newline>NewGCM) is missed. gofmt keeps imports one per line.
# The allowlist works per file and rule, not per occurrence: a second
# cipher.NewGCM in a file already listed for it passes; review catches that.
#
# The enterprise submodule must be checked out (the check fails otherwise,
# like gorm-verify), unless CE_ONLY=1, which skips it and its allowlist
# entries (CI on pull requests from forks, which get a stub enterprise/).
#
# --self-test runs the guard against generated fixtures and checks it fails
# and passes where it should.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SCAN_ROOT=${FIPS_GUARD_ROOT:-$ROOT}
ALLOWLIST=${FIPS_GUARD_ALLOWLIST:-$ROOT/scripts/fips-guard.allowlist}

IMPORT_RE='^[[:space:]]*(import[[:space:]]+)?([A-Za-z_.][A-Za-z0-9_]*[[:space:]]+)?"(crypto/(md5|sha1|des|rc4|dsa)|golang\.org/x/crypto(/[^"]*)?)"[[:space:]]*(//.*)?$'
# The name, then anything that cannot continue an identifier, so method
# values match and cipher.NewGCMWithRandomNonce does not.
CALL_RE='cipher\.(NewGCM|NewGCMWithNonceSize|NewGCMWithTagSize|NewCFBEncrypter|NewCFBDecrypter)([^A-Za-z0-9_]|$)'
# crypto/cipher imported under a name; any name but cipher is a finding.
CIPHER_ALIAS_RE='^[[:space:]]*(import[[:space:]]+)?([A-Za-z_.][A-Za-z0-9_]*)[[:space:]]+"crypto/cipher"[[:space:]]*(//.*)?$'

self_test() {
  local dir out status=0
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  git -C "$dir" init -q
  mkdir -p "$dir/p"
  cat > "$dir/p/bad.go" <<'EOF'
package p

import (
	"crypto/aes"
	m "crypto/md5"
)

import "golang.org/x/crypto/bcrypt"

var notAnImport = []string{
	"crypto/sha1",
}

// cipher.NewGCM(block) in a comment is not a call.
func f(b cipher.Block) { _, _ = cipher.NewCFBDecrypter(b, nil), cipher.NewGCMWithRandomNonce(b) }

var newGCM = cipher.NewGCM

import (
	"golang.org/x/crypto/hkdf"
	c "crypto/cipher"
	"crypto/dsa"
)
EOF
  cat > "$dir/p/good.go" <<'EOF'
package p

import (
	cipher "crypto/cipher"
	"crypto/sha256"
)

import "crypto/cipher"

func g(b cipher.Block) { _, _ = cipher.NewGCMWithRandomNonce(b) }
EOF
  run() { CE_ONLY=1 FIPS_GUARD_ROOT="$dir" FIPS_GUARD_ALLOWLIST="$dir/allow" "$0" 2>&1; }
  expect() { # expect <0|1> <description> <grep -E pattern the output must match>
    local want=$1 what=$2 pattern=$3 got=0
    out=$(run) || got=$?
    if [ "$got" -ne "$want" ] || ! grep -qE -- "$pattern" <<< "$out"; then
      echo "fips-guard self-test FAILED: $what (exit $got, want $want). Output:" >&2
      echo "$out" >&2
      status=1
    fi
  }

  : > "$dir/allow"
  expect 1 "a non-allowlisted crypto/md5 import (aliased) fails" '^p/bad\.go:5: crypto/md5'
  expect 1 "a single-line import fails" '^p/bad\.go:8: golang\.org/x/crypto/bcrypt'
  expect 1 "a cipher.NewCFBDecrypter call fails" '^p/bad\.go:15: cipher\.NewCFBDecrypter'
  expect 1 "a cipher.NewGCM method value fails" '^p/bad\.go:17: cipher\.NewGCM$'
  expect 1 "any golang.org/x/crypto import fails" '^p/bad\.go:20: golang\.org/x/crypto/hkdf'
  expect 1 "an aliased crypto/cipher import fails" '^p/bad\.go:21: crypto/cipher-alias'
  expect 1 "a crypto/dsa import fails" '^p/bad\.go:22: crypto/dsa'
  if grep -qE '^p/(good\.go|bad\.go:(11|14)):|^p/.*: (crypto/sha1|cipher\.NewGCMWithRandomNonce)$' <<< "$out"; then
    echo "fips-guard self-test FAILED: a string literal, a comment, an approved call or a clean file was reported:" >&2
    echo "$out" >&2
    status=1
  fi

  printf '%s\n' '# comment' 'p/bad.go crypto/md5 fixture' 'p/bad.go golang.org/x/crypto/bcrypt fixture' \
    'p/bad.go cipher.NewCFBDecrypter fixture' 'p/bad.go cipher.NewGCM fixture' \
    'p/bad.go golang.org/x/crypto/hkdf fixture' 'p/bad.go crypto/cipher-alias fixture' \
    'p/bad.go crypto/dsa fixture' > "$dir/allow"
  expect 0 "allowlisted uses pass" 'fips-guard: OK'

  local got=0
  # (env -u: make fips-guard runs with CE_ONLY=1 on fork pull requests)
  out=$(env -u CE_ONLY FIPS_GUARD_ROOT="$dir" FIPS_GUARD_ALLOWLIST="$dir/allow" "$0" 2>&1) || got=$?
  if [ "$got" -eq 0 ] || ! grep -q 'enterprise submodule is not checked out' <<< "$out"; then
    echo "fips-guard self-test FAILED: a missing enterprise submodule without CE_ONLY=1 must fail (exit $got). Output:" >&2
    echo "$out" >&2
    status=1
  fi

  echo 'p/good.go crypto/md5 stale' >> "$dir/allow"
  expect 1 "an allowlist entry that matches nothing fails" 'p/good\.go crypto/md5'

  printf '%s\n' 'p/bad.go crypto/md5' 'p/bad.go golang.org/x/crypto/bcrypt fixture' \
    'p/bad.go cipher.NewCFBDecrypter fixture' > "$dir/allow"
  expect 1 "an allowlist entry without a reason fails" 'without a reason'

  [ "$status" -eq 0 ] && echo "fips-guard self-test: OK"
  return "$status"
}

if [ "${1:-}" = --self-test ]; then
  self_test
  exit
fi

[ -f "$ALLOWLIST" ] || { echo "fips-guard: allowlist $ALLOWLIST missing" >&2; exit 1; }

scan_enterprise=0
if [ "${CE_ONLY:-}" = 1 ]; then
  echo "fips-guard: CE_ONLY=1, not scanning the enterprise submodule" >&2
elif [ -e "$SCAN_ROOT/enterprise/.git" ]; then
  scan_enterprise=1
else
  echo "fips-guard: the enterprise submodule is not checked out (git submodule update --init enterprise," >&2
  echo "or CE_ONLY=1 to check without it)" >&2
  exit 1
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# grep_repo <dir> <path prefix>: "path:line:text" for each candidate line.
# git grep exits 1 for no match, 2+ for an error, which must stop the check.
grep_repo() {
  local rc=0
  (cd "$1" && git grep --untracked -n -I -E -e "$IMPORT_RE" -e "$CALL_RE" -e "$CIPHER_ALIAS_RE" -- '*.go') \
    | sed "s|^|$2|" || rc=$?
  [ "$rc" -le 1 ] || { echo "fips-guard: git grep failed in $1" >&2; exit 2; }
}
{
  grep_repo "$SCAN_ROOT" ""
  [ "$scan_enterprise" = 0 ] || grep_repo "$SCAN_ROOT/enterprise" "enterprise/"
} > "$work/lines"

# Findings, one per line: "<path> <rule> <path>:<line>".
while IFS= read -r l; do
  path=${l%%:*}; rest=${l#*:}; line=${rest%%:*}; text=${rest#*:}
  [[ "$text" =~ ^[[:space:]]*// ]] && continue
  if [[ "$text" =~ $IMPORT_RE ]]; then
    rule=$(sed -E 's/^[^"]*"([^"]+)".*$/\1/' <<< "$text")
    echo "$path $rule $path:$line"
  elif [[ "$text" =~ $CIPHER_ALIAS_RE ]]; then
    # (import "crypto/cipher" captures the keyword as the name)
    case "${BASH_REMATCH[2]}" in cipher | import) ;; *) false ;; esac || echo "$path crypto/cipher-alias $path:$line"
  else
    grep -oE "$CALL_RE" <<< "$text" | sed -E 's/[^A-Za-z0-9_]$//' | while IFS= read -r rule; do
      echo "$path $rule $path:$line"
    done
  fi
done < "$work/lines" | sort -u > "$work/findings"

# Allowlist entries: "<path> <rule>"; each needs a reason.
status=0
awk '!/^[[:space:]]*(#|$)/ { if (NF < 3) print }' "$ALLOWLIST" > "$work/noreason"
if [ -s "$work/noreason" ]; then
  echo "fips-guard: allowlist entries without a reason (want: <path> <rule> <reason>):" >&2
  cat "$work/noreason" >&2
  status=1
fi
awk '!/^[[:space:]]*(#|$)/ { print $1 " " $2 }' "$ALLOWLIST" | sort -u > "$work/allowed"
awk '{ print $1 " " $2 }' "$work/findings" | sort -u > "$work/found"

# (getline rather than NR == FNR, which breaks on an empty allowlist)
violations=$(awk -v allowed="$work/allowed" '
  BEGIN { while ((getline l < allowed) > 0) ok[l] = 1 }
  !(($1 " " $2) in ok) { print $3 ": " $2 }' "$work/findings")
if [ -n "$violations" ]; then
  echo "fips-guard: these files use cryptography that is not FIPS 140-3 approved, or that fails under" >&2
  echo "GODEBUG=fips140=only. Use the approved primitive (crypto/sha256, crypto/pbkdf2," >&2
  echo "cipher.NewGCMWithRandomNonce, ...). Only a dual-format reader of existing data may be" >&2
  echo "added to scripts/fips-guard.allowlist, with a reason:" >&2
  echo "$violations" >&2
  status=1
fi

stale=$(comm -23 "$work/allowed" "$work/found")
if [ "$scan_enterprise" = 0 ]; then
  stale=$(grep -v '^enterprise/' <<< "$stale" || true)
fi
if [ -n "$stale" ]; then
  echo "fips-guard: these allowlist entries match nothing; remove them from scripts/fips-guard.allowlist:" >&2
  echo "$stale" >&2
  status=1
fi

[ "$status" -eq 0 ] || exit "$status"
echo "fips-guard: OK ($(wc -l < "$work/findings" | tr -d ' ') allowlisted uses in $(wc -l < "$work/allowed" | tr -d ' ') entries; no new non-approved cryptography)"
