#!/usr/bin/env bash
# Regenerates the schema snapshot goldens (models/testdata/schema and
# microgateway/internal/database/testdata/schema) on SQLite and on a
# throwaway postgres:16 container, the version CI checks them against.
# Run it after a deliberate model change and review the golden diff.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

name=schema-golden-pg-$$
docker run -d --rm --name "$name" -e POSTGRES_PASSWORD=golden -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
trap 'docker stop "$name" >/dev/null' EXIT
for _ in $(seq 1 60); do
  docker exec "$name" pg_isready -U postgres >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$name" psql -U postgres -qc 'CREATE DATABASE mgw' >/dev/null
port=$(docker port "$name" 5432/tcp | head -1 | sed 's/.*://')
dsn="postgres://postgres:golden@127.0.0.1:$port"

UPDATE_SCHEMA_GOLDEN=1 DATABASE_URL="$dsn/postgres?sslmode=disable" \
  go test -count=1 -run 'TestStudioSchemaSnapshot' ./models/
(cd microgateway && UPDATE_SCHEMA_GOLDEN=1 MGW_TEST_POSTGRES_DSN="$dsn/mgw?sslmode=disable" \
  go test -count=1 -run 'TestGatewaySchemaSnapshot' ./internal/database/)

git status --short -- models/testdata/schema microgateway/internal/database/testdata/schema
