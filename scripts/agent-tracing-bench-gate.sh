#!/usr/bin/env bash
# Agent tracing performance gate (spec §10, bead jyx.34).
#
# ci (blocking PR check; deterministic, a few minutes):
#   - TestAgentTraceAllocGate: tracing off allocates nothing on any capture
#     hook; tracing on stays within per-request allocation and byte budgets.
#   - Enterprise with DATABASE_URL: a short assembler soak gating write
#     amplification (<= 3 rows per span) and round trips (<= 0.05
#     statements per span on Postgres); its spans/s is reported.
#   - Gateway overhead (tracing on vs off in one process) is reported, not
#     gated: shared runners vary too much for timing budgets.
# soak (manual or nightly, make bench-agent-tracing-soak):
#   - TestAssemblerSoak for AGENT_TRACE_SOAK (default 1h) at GOMAXPROCS=2
#     with the >= 2,000 spans/s gate; run Postgres limited to 2 CPUs.
# overhead (local, quiet machine):
#   - TestAgentTraceOverheadGate gating the added p50 (and p99 with
#     AGENT_TRACE_BENCH_STRICT=1), and the per-request capture benchmark.
#
# Usage: scripts/agent-tracing-bench-gate.sh ci|soak|overhead [ce|enterprise]
set -euo pipefail
mode=${1:-ci}
edition=${2:-ce}
tags=agenttracebench
if [ "$edition" = enterprise ]; then
  tags="agenttracebench enterprise"
fi

soak() {
  (cd enterprise && GOMAXPROCS=2 go test -tags 'enterprise agenttracebench' -count=1 -run TestAssemblerSoak -timeout 3h -v \
    ./features/agenttracing/assembler/ | grep -E 'soak_bench_test|^(--- |ok|FAIL)')
}

case "$mode" in
ci)
  echo "== allocation gate ($edition)"
  go test -tags "$tags" -count=1 -run TestAgentTraceAllocGate -v ./proxy/ | grep -E 'alloc_gate_test|^(--- |ok|FAIL)'
  if [ "$edition" = enterprise ] && [ -n "${DATABASE_URL:-}" ]; then
    echo "== assembler write amplification (short soak, GOMAXPROCS=2)"
    AGENT_TRACE_SOAK=${AGENT_TRACE_SOAK:-90s} soak
  fi
  echo "== gateway overhead (reported, not gated)"
  AGENT_TRACE_BENCH_REPORT=1 AGENT_TRACE_BENCH_N=${AGENT_TRACE_BENCH_N:-1500} \
    go test -tags "$tags" -count=1 -run TestAgentTraceOverheadGate -v ./proxy/ | grep -E 'agent_trace_overhead_test|^(--- |ok|FAIL)'
  ;;
soak)
  : "${DATABASE_URL:?soak needs DATABASE_URL (Postgres)}"
  AGENT_TRACE_SOAK=${AGENT_TRACE_SOAK:-1h} AGENT_TRACE_SOAK_STRICT=1 soak
  ;;
overhead)
  go test -tags "$tags" -count=1 -run TestAgentTraceOverheadGate -v ./proxy/ | grep -E 'agent_trace_overhead_test|^(--- |ok|FAIL)'
  go test -tags agenttracebench -run xxx -bench BenchmarkAgentTrace -benchtime 3000x ./proxy/ | grep -E '^Benchmark|^ok'
  ;;
*)
  echo "usage: $0 ci|soak|overhead [ce|enterprise]" >&2
  exit 2
  ;;
esac
