#!/bin/bash
# pprof-watch.sh OUTDIR [CPU_AT_SECONDS]
#
# Runs on the loadgen VM beside a gwbench run, against an edge deployed with
# BENCH_ENABLE_PROFILING=true (pprof on <gateway private ip>:6060). Polls the
# gateway's /metrics once a second and saves into OUTDIR:
#
#   - on a stall (go_goroutines above STALL_GOROUTINES, default 3000): a full
#     goroutine dump (debug=2) plus mutex and block profiles, at most
#     MAX_DUMPS (3) times, at least 20 s apart;
#   - CPU_AT_SECONDS after it starts: one 30 s CPU profile (0: none);
#   - when stopped (SIGTERM/SIGINT): mutex and block profiles for the run.
#
# Profiling costs a little CPU and a goroutine dump briefly stops the world,
# so label runs taken with it.
cd /opt/gwbench || exit 1
set -a; . ./loadgen.env; set +a
out=${1:?usage: pprof-watch.sh OUTDIR [CPU_AT_SECONDS]}
cpu_at=${2:-0}
thresh=${STALL_GOROUTINES:-3000}
max=${MAX_DUMPS:-3}
mkdir -p "$out"
host=$(echo "$GWBENCH_GATEWAY_URL" | sed -E 's#^https?://([^:/]+).*#\1#')
pp="http://$host:6060/debug/pprof"
curl -s -m 5 -o /dev/null -w '%{http_code}' "$pp/" | grep -q 200 || { echo "no pprof at $pp" >&2; exit 1; }

finish() {
  curl -s -m 30 "$pp/mutex" > "$out/mutex-end.pb.gz"
  curl -s -m 30 "$pp/block" > "$out/block-end.pb.gz"
  wait
  echo "$(date -u +%T) pprof-watch: done"
  exit 0
}
trap finish TERM INT

start=$(date +%s) dumps=0 last=0 cpu_done=0
echo "$(date -u +%T) pprof-watch: $pp, stall > $thresh goroutines, cpu at +${cpu_at}s"
while :; do
  now=$(date +%s)
  g=$(curl -s -m 2 -H "Authorization: Bearer $GWBENCH_METRICS_TOKEN" "$GWBENCH_GATEWAY_URL/metrics" |
    awk '$1 == "go_goroutines" { print int($2) }')
  if [ -n "$g" ] && [ "$g" -gt "$thresh" ] && [ "$dumps" -lt "$max" ] && [ $((now - last)) -ge 20 ]; then
    dumps=$((dumps + 1)) last=$now ts=$(date -u +%H%M%S)
    echo "$(date -u +%T) stall: goroutines=$g, dump $dumps"
    curl -s -m 60 "$pp/goroutine?debug=2" > "$out/goroutines-$ts-g$g.txt" &
    curl -s -m 60 "$pp/mutex" > "$out/mutex-$ts.pb.gz" &
    curl -s -m 60 "$pp/block" > "$out/block-$ts.pb.gz" &
  fi
  if [ "$cpu_at" -gt 0 ] && [ "$cpu_done" -eq 0 ] && [ $((now - start)) -ge "$cpu_at" ]; then
    cpu_done=1
    echo "$(date -u +%T) cpu profile, 30 s (goroutines=$g)"
    curl -s -m 90 "$pp/profile?seconds=30" > "$out/cpu-$(date -u +%H%M%S).pb.gz" &
  fi
  sleep 1
done
