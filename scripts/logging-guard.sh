#!/usr/bin/env bash
# Checks that the control plane logs only through the logger package.
#
# An embedding host (the Dashboard, MDCB) passes its own logger in
# (studio.Options.Logger, which calls logger.Use), and expects every line
# from these packages on it. zerolog's global logger, log/slog and the
# standard library's log all bypass it: their lines go to the process's
# stderr in their own format, or nowhere.
#
# The packages checked are the ones the edge control plane runs: the gRPC
# control server, the cluster registry, event log and relay, the Postgres
# listener, edge pushes, analytics recording, secrets, panic recovery and
# pkg/studio itself. Tests are not checked.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

PACKAGES=(grpc pkg/cluster pkg/pglisten pkg/safe services/pushes analytics secrets pkg/studio)

# An import line (in a block or on its own, optionally named) of zerolog's
# global logger, log/slog or the standard library's log.
PATTERN='^(import[[:space:]]+)?[[:space:]]*([A-Za-z_.]+[[:space:]]+)?"(log|log/slog|github\.com/rs/zerolog/log)"[[:space:]]*$'

if matches=$(git grep -nE "$PATTERN" -- "${PACKAGES[@]}" | grep -E '^[^:]+\.go:' | grep -vE '^[^:]+_test\.go:'); then
  echo "These control-plane files log around the logger package; use logger.Log (or logger.Infof etc.)" >&2
  echo "so an embedding host's logger gets their lines:" >&2
  echo "$matches" >&2
  exit 1
fi
echo "logging-guard: the control-plane packages log only through the logger package"
