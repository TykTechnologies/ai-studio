#!/usr/bin/env bash
# Step 1: the e2e images contain no cosign binary and no shell.
set -uo pipefail
EDITION=${EDITION:-ent}
status=0
for img in "c1g-e2e/studio:$EDITION" "c1g-e2e/microgateway:$EDITION"; do
  echo "== $img"
  c=$(docker create "$img")
  files=$(docker export "$c" | tar -t)
  docker rm "$c" >/dev/null
  hits=$(grep -E '(^|/)(cosign[^/]*|sh|bash|busybox|ash|dash)$' <<<"$files" || true)
  if [ -n "$hits" ]; then
    echo "FOUND: $hits"; status=1
  else
    echo "no cosign binary, no shell ($(wc -l <<<"$files") paths)"
  fi
  # And nothing can start one.
  docker run --rm --entrypoint /bin/sh "$img" -c true 2>&1 | head -2
done
exit $status
