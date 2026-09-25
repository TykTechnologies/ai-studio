#!/bin/bash
# The RUNBOOK-cloud.md sequence, run unattended on the loadgen VM. Started by
# `aws.sh suite`, which detaches it; follow it with `aws.sh logs`.
#
#   REPEATS   repeats of S0-S2 and S4-S5 (default 3, as the runbook asks)
#   VENDORS   1 = also run S3 against OpenAI and Anthropic (needs a
#             vendors-seeded stack and test-secrets/vendors.env)
#   SOAK      0 = skip the one-hour S7 soak
#   QUICK     1 = shrink every scenario (a smoke run; not publishable)
#
# S6 and S7 are sized from the first S4 run's sustained rate, as the runbook
# describes. The script keeps going when a scenario fails or is INVALID: the
# runbook keeps every run, so each is reported, not retried here.
set -uo pipefail
cd /opt/gwbench
set -a; . ./loadgen.env; set +a
ulimit -n 1048576

REPEATS=${REPEATS:-3}
VENDORS=${VENDORS:-0}
SOAK=${SOAK:-1}
QUICK=${QUICK:-0}
SC=benchmarks/gateway/scenarios
RESULTS=benchmarks/gateway/results
quick=()
[ "$QUICK" = 1 ] && quick=(-quick)

log() { echo "$(date -u +%FT%TZ) suite: $*"; }
status() { echo "$*" > suite.status; }
failures=0
gw() {
  log "gwbench $*"
  if ! ./bin/gwbench "$@"; then
    failures=$((failures + 1))
    log "FAILED: gwbench $*"
  fi
}

status "running"
log "start: repeats=$REPEATS vendors=$VENDORS soak=$SOAK quick=$QUICK label=$GWBENCH_LABEL"

first_s4=""
for i in $(seq 1 "$REPEATS"); do
  status "running: repeat $i/$REPEATS S0-S2"
  gw run "${quick[@]}" "$SC/s0-aa-calibration.yaml" "$SC/s1-overhead-floor.yaml" "$SC/s2-payload-size.yaml"
  status "running: repeat $i/$REPEATS S4-S5"
  gw run "${quick[@]}" "$SC/s4-capacity-ramp.yaml" "$SC/s5-throughput-ceiling.yaml"
  [ -z "$first_s4" ] && first_s4=$(ls -1d "$RESULTS"/*-s4-capacity-ramp 2>/dev/null | tail -1)
done

sustained=""
if [ -n "$first_s4" ] && [ -f "$first_s4/summary.json" ]; then
  sustained=$(jq -r '[.cells[].knee.sustained_rps // empty] | max // empty' "$first_s4/summary.json")
fi
if [ -z "$sustained" ] || [ "$sustained" = "0" ] || [ "$sustained" = "null" ]; then
  log "no sustained rate from S4 ($first_s4); skipping S6 and S7"
else
  scale=$(awk -v s="$sustained" 'BEGIN { printf "%.3f", s / 200 }')
  log "S4 sustained ${sustained} req/s -> rate scale ${scale}"
  echo "$sustained $scale" > suite.rate-scale
  status "running: S6 (scale $scale)"
  gw run "${quick[@]}" -rate-scale "$scale" "$SC/s6-spike.yaml"
  if [ "$SOAK" = 1 ]; then
    status "running: S7 soak (scale $scale)"
    gw run "${quick[@]}" -rate-scale "$scale" "$SC/s7-soak.yaml"
  fi
fi

if [ "$VENDORS" = 1 ]; then
  status "running: S3 real vendors"
  gw run "${quick[@]}" -vendors "$SC/s0-aa-calibration.yaml" "$SC/s3-real-vendors.yaml"
fi

log "done, $failures failed gwbench invocation(s)"
status "done: $failures failed"
