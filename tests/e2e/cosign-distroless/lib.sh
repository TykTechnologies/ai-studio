#!/usr/bin/env bash
# Helpers for the c1g.15 e2e scenarios (sourced by run.sh).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE=(docker compose -f "$HERE/compose.yml")
STUDIO=http://127.0.0.1:18080
REPO=registry:5000/studio-plugins/request_enricher
ADMIN_EMAIL=e2e-admin@test.local
ADMIN_PASSWORD='E2eTest#2026-c1g'
API_KEY=""

log() { printf '\n### %s\n' "$*"; }

# up [service...]: start the stack (or services) with the current env and
# connect the registry container to the network as "registry".
up() {
  "${COMPOSE[@]}" up -d "$@" >/dev/null
  docker network connect --alias registry c1g-e2e c1g-e2e-registry 2>/dev/null || true
}

down() { "${COMPOSE[@]}" down -v >/dev/null 2>&1 || true; }

wait_studio() {
  for _ in $(seq 1 90); do
    curl -sf "$STUDIO/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  echo "studio did not become healthy"; "${COMPOSE[@]}" logs --tail 50 studio; return 1
}

# login: register the first user (admin) and roll an API key.
login() {
  local jar token
  jar=$(mktemp)
  # Browser-style session calls need the CSRF token (cookie + header).
  token=$(curl -s -c "$jar" -b "$jar" -D - -o /dev/null "$STUDIO/csrf-token" | tr -d '\r' | sed -n 's/^[Xx]-[Cc][Ss][Rr][Ff]-[Tt]oken: //p')
  local h=(-c "$jar" -b "$jar" -H 'Content-Type: application/json' -H "X-CSRF-Token: $token" -H "Origin: $STUDIO" -H "Referer: $STUDIO/")
  curl -s -o /dev/null "${h[@]}" -X POST "$STUDIO/auth/register" \
    -d "{\"data\":{\"type\":\"users\",\"attributes\":{\"email\":\"$ADMIN_EMAIL\",\"name\":\"E2E Admin\",\"password\":\"$ADMIN_PASSWORD\"}}}"
  curl -s -o /dev/null "${h[@]}" -X POST "$STUDIO/auth/login" \
    -d "{\"data\":{\"type\":\"login\",\"attributes\":{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}}}"
  API_KEY=$(curl -s "${h[@]}" -X POST "$STUDIO/common/me/api-key/roll" | python3 -I -c 'import json,sys; d=json.load(sys.stdin); print(d.get("data",{}).get("api_key",""))')
  rm -f "$jar"
  [ -n "$API_KEY" ] || { echo "could not get an API key"; return 1; }
}

# api METHOD PATH [JSON]: prints the HTTP status, then the body.
api() {
  local method=$1 path=$2 body=${3:-}
  local args=(-s -w '\n%{http_code}' -X "$method" "$STUDIO$path" -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json')
  [ -n "$body" ] && args+=(-d "$body")
  local out; out=$(curl "${args[@]}")
  printf '%s %s\n' "$(tail -n1 <<<"$out")" "$(sed '$d' <<<"$out")"
}

# digest TAG: the registry digest of a request_enricher tag.
digest() { oras resolve --plain-http "localhost:15000/studio-plugins/request_enricher:$1"; }

studio_logs() { "${COMPOSE[@]}" logs --no-color studio 2>&1; }
gateway_logs() { "${COMPOSE[@]}" logs --no-color gateway 2>&1; }
