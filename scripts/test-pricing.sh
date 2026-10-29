#!/usr/bin/env bash
# Runs the full pricing regression matrix (proxy TestPricingRegression*, 180
# days of traffic, 180k rows recalculated) and the hub, edge and enterprise
# budget regressions on SQLite and on a throwaway postgres:16 container. The
# normal test run covers the same tests at a smaller scale, and CI's Go
# Postgres Tests job runs them on Postgres.
#
#   make test-pricing
#
# Set DATABASE_URL to use a Postgres you already run instead of a container.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if [ -z "${DATABASE_URL:-}" ]; then
  name=test-pricing-pg-$$
  docker run -d --rm --name "$name" -e POSTGRES_PASSWORD=pricing -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
  trap 'docker stop "$name" >/dev/null' EXIT
  for _ in $(seq 1 60); do
    docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break
    sleep 1
  done
  port=$(docker port "$name" 5432/tcp | head -1 | sed 's/.*://')
  DATABASE_URL="postgres://postgres:pricing@127.0.0.1:$port/postgres?sslmode=disable"
fi

export DATABASE_URL
filter() { grep -E '^(\s*--- |ok|FAIL|PASS)|pricing_regression_test'; }
# Every suite runs; the script fails at the end if any did.
status=0
PRICING_REGRESSION_FULL=1 go test -count=1 -v -run 'TestPricingRegression' ./proxy/ ./grpc/ ./api/ | filter || status=1
(cd microgateway && MGW_TEST_POSTGRES_DSN="$DATABASE_URL" go test -count=1 -v -tags enterprise -run 'TestPricingRegression' ./internal/services/ | filter) || status=1
if [ -f enterprise/features/budget/pricing_regression_test.go ]; then
  (cd enterprise && go test -count=1 -v -tags enterprise -run 'TestPricingRegression' ./features/budget/ | filter) || status=1
fi
exit $status
