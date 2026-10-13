#!/bin/bash
# Start the tool-result guardrail E2E stack from a clean state: a recording
# mock upstream, an Enterprise Studio (hub, embedded gateway) and a
# microgateway edge, all local binaries, all on SQLite.
#
#   E2E_SECRETS_FILE=/path/to/dev/.env.secrets BIN=/path/to/bin [E2E_DIR=dir] tests/guardrail_tool_results/start.sh
#
# E2E_SECRETS_FILE holds TYK_AI_LICENSE. Without E2E_DIR a private directory
# is made with mktemp -d and printed; pass it to live_e2e.py. Everything the
# stack writes there (env files with the license, databases, logs) is 0600.
#
# BIN must hold `studio` (go build -tags enterprise .) and `mgw`
# (cd microgateway && go build -tags enterprise ./cmd/microgateway). Build the
# frontend first (ui/admin-frontend: npm run build) if the UI is needed.
set -u
umask 077
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
: "${E2E_SECRETS_FILE:?set E2E_SECRETS_FILE to a dotenv file holding TYK_AI_LICENSE}"
E="${E2E_DIR:-$(mktemp -d)}"
BIN="${BIN:?set BIN to the directory holding studio and mgw}"
MOCK_PORT="${E2E_MOCK_PORT:-14031}"
export E2E_DIR="$E"

for p in "${E2E_STUDIO_PORT:-18080}" "${E2E_PROXY_PORT:-19090}" "${E2E_GRPC_PORT:-50071}" "${E2E_EDGE_PORT:-18081}" "$MOCK_PORT"; do
  lsof -ti :"$p" -sTCP:LISTEN | xargs kill 2>/dev/null  # listeners only, never clients
done
sleep 1
mkdir -p "$E"
rm -f "$E"/studio.db* "$E"/edge.db* "$E"/requests.jsonl "$E"/*.log
python3 "$HERE/make_env.py" || exit 1

python3 "$HERE/mockupstream.py" "$MOCK_PORT" "$E/requests.jsonl" > "$E/mock.log" 2>&1 &

# Studio must run from the repo root (templates/).
cd "$ROOT"
"$BIN/studio" -env "$E/studio.env" > "$E/studio.log" 2>&1 &
for i in $(seq 1 90); do curl -sf "http://127.0.0.1:${E2E_STUDIO_PORT:-18080}/health" >/dev/null 2>&1 && break; sleep 1; done
echo "studio up after ${i}s"

cd "$ROOT/microgateway"
set -a; . "$E/gateway.env"; set +a
"$BIN/mgw" > "$E/gateway.log" 2>&1 &
for i in $(seq 1 60); do curl -sf "http://127.0.0.1:${E2E_EDGE_PORT:-18081}/health" >/dev/null 2>&1 && break; sleep 1; done
echo "edge up after ${i}s"
echo "stack directory (env files, databases, logs): E2E_DIR=$E"
