#!/bin/bash
# Restart parts of a running E2E stack, keeping the databases.
#   E2E_DIR=... BIN=... tests/guardrail_tool_results/restart_studio.sh [studio] [edge] [mock]
# With no argument, restarts the Studio only. Restart the edge after the
# Studio: its gRPC session to the hub does not survive a hub restart.
set -u
umask 077
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
E="${E2E_DIR:?set E2E_DIR to the stack directory start.sh printed}"
BIN="${BIN:-}"
STUDIO_PORT="${E2E_STUDIO_PORT:-18080}"
EDGE_PORT="${E2E_EDGE_PORT:-18081}"
MOCK_PORT="${E2E_MOCK_PORT:-14031}"

# Only listeners: a plain `lsof -ti :PORT` also lists every process with a
# client connection to the port (the gateway, the edge, other stacks).
stop() { lsof -ti :"$1" -sTCP:LISTEN | xargs kill 2>/dev/null; sleep 2; }
wait_up() { for i in $(seq 1 90); do curl -sf "$1" >/dev/null 2>&1 && break; sleep 1; done; echo "$2 up after ${i}s"; }

[ $# -eq 0 ] && set -- studio
for part in "$@"; do
  case "$part" in
  studio)
    : "${BIN:?set BIN}"
    stop "$STUDIO_PORT"
    cd "$ROOT"
    nohup "$BIN/studio" -env "$E/studio.env" >> "$E/studio.log" 2>&1 &
    wait_up "http://127.0.0.1:$STUDIO_PORT/health" studio
    ;;
  edge)
    : "${BIN:?set BIN}"
    stop "$EDGE_PORT"
    cd "$ROOT/microgateway"
    (set -a; . "$E/gateway.env"; set +a; nohup "$BIN/mgw" >> "$E/gateway.log" 2>&1 &)
    wait_up "http://127.0.0.1:$EDGE_PORT/health" edge
    ;;
  mock)
    stop "$MOCK_PORT"
    nohup python3 "$HERE/mockupstream.py" "$MOCK_PORT" "$E/requests.jsonl" "$E/sock/classifier.sock" >> "$E/mock.log" 2>&1 &
    wait_up "http://127.0.0.1:$MOCK_PORT/health" mock
    ;;
  esac
done
