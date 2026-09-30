#!/usr/bin/env bash
# Builds a throwaway host module that imports Studio at VERSION, the way a
# program embedding it (the Tyk Dashboard) would: from a clean module cache,
# through the module proxy, with `go mod tidy` and a CGO_ENABLED=0 build.
#
#   scripts/release/consume-module.sh ce  <version>
#   scripts/release/consume-module.sh ent <version>   # needs read access to
#                                                      # ai-studio-enterprise
#
# ce runs with no credentials at all: a Community Edition host must never
# need the private enterprise module. ent requires the enterprise module at
# the same version, imports enterprise/all and builds with -tags enterprise;
# it fetches ai-studio-enterprise directly (GOPRIVATE) with whatever git
# credentials the environment has. <version> is a tag or a commit hash.
# The release workflow runs both after tagging; see features/Embedding.md.
set -euo pipefail

EDITION=${1:?usage: $0 ce|ent <version>}
VERSION=${2:?usage: $0 ce|ent <version>}
CORE=github.com/TykTechnologies/midsommar/v2
ENT=github.com/TykTechnologies/ai-studio-enterprise/v2

WORK=$(mktemp -d)
trap 'chmod -R u+w "$WORK" 2>/dev/null; rm -rf "$WORK"' EXIT
mkdir -p "$WORK/host" "$WORK/modcache"
cd "$WORK/host"

export GOMODCACHE="$WORK/modcache" GOFLAGS=-mod=mod GOPROXY=https://proxy.golang.org,direct
export GONOSUMDB= GONOPROXY= GOWORK=off CGO_ENABLED=0
case "$EDITION" in
  ce)
    # No credentials: the proxy can only serve public modules, and git must
    # not fall back to anything interactive or configured.
    export GOPRIVATE= GIT_TERMINAL_PROMPT=0 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
    TAGS=studio_noui
    ;;
  ent)
    export GOPRIVATE=github.com/TykTechnologies/ai-studio-enterprise GIT_TERMINAL_PROMPT=0
    TAGS="studio_noui enterprise"
    ;;
  *) echo "edition must be ce or ent" >&2; exit 2 ;;
esac

cat > go.mod <<GOMOD
module example.com/studiohost

go 1.26
GOMOD

{
  echo 'package main'
  echo
  echo 'import ('
  echo '	"fmt"'
  echo
  echo "	\"$CORE/pkg/studio\""
  [ "$EDITION" = ent ] && echo "	_ \"$ENT/all\""
  echo ')'
  echo
  echo 'func main() { fmt.Println(studio.ErrAlreadyRunning) }'
} > main.go

# A tag pushed seconds ago may not be on the proxy yet, and the proxy can
# take a while to see it (it caches "not found" for some minutes). Retry with
# a doubling backoff, capped at 5 minutes between attempts, for up to
# CONSUME_MODULE_DEADLINE seconds (30 minutes by default).
DEADLINE=$(( $(date +%s) + ${CONSUME_MODULE_DEADLINE:-1800} ))
get() {
  local delay=15 attempt=1
  while :; do
    if go get "$1@$VERSION"; then return 0; fi
    if [ $(( $(date +%s) + delay )) -gt "$DEADLINE" ]; then
      echo "consume-module: go get $1@$VERSION still failing after $attempt attempts; giving up" >&2
      return 1
    fi
    echo "consume-module: go get $1@$VERSION failed (attempt $attempt); retrying in ${delay}s" >&2
    sleep "$delay"
    attempt=$(( attempt + 1 ))
    delay=$(( delay * 2 > 300 ? 300 : delay * 2 ))
  done
}
get "$CORE"
[ "$EDITION" = ent ] && get "$ENT"

go mod tidy
go build -tags "$TAGS" -o studiohost .

if [ "$EDITION" = ce ] && grep -q 'ai-studio-enterprise' go.mod go.sum; then
  echo "consume-module: a Community Edition host's module graph names the enterprise module:" >&2
  grep -n 'ai-studio-enterprise' go.mod go.sum >&2
  exit 1
fi
echo "consume-module: $EDITION host built Studio $VERSION (tidy + CGO_ENABLED=0 build, tags: $TAGS)"
grep -E "^\s*(require )?($CORE|$ENT) " go.mod || true
