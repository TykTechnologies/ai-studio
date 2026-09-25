#!/bin/bash
# pprof-watch.sh OUTDIR [CPU_AT_SECONDS]
#
# Runs on the loadgen VM beside a gwbench run, against an edge deployed with
# BENCH_ENABLE_PROFILING=true (pprof on <gateway private ip>:6060). Polls the
# gateway's /metrics once a second and saves into OUTDIR:
#
#   - on a sudden goroutine jump (go_goroutines above JUMP_FACTOR, default 2,
#     times the median of the last 30 samples, and above MIN_GOROUTINES,
#     default 500): a TRACE_SECONDS (3) runtime trace, trace-HHMMSS.out, at
#     most MAX_TRACES (3) times, at least TRACE_GAP (60) seconds apart. A
#     trace shows scheduler, GC (assist, STW) and syscall activity without the
#     stop-the-world pause of a full goroutine dump. DUMPS=1 also saves a
#     goroutine dump (debug=2) with each trace;
#   - CPU_AT_SECONDS after it starts: one 30 s CPU profile (0: none);
#   - when stopped (SIGTERM/SIGINT): mutex and block profiles for the run.
cd /opt/gwbench || exit 1
set -a; . ./loadgen.env; set +a
out=${1:?usage: pprof-watch.sh OUTDIR [CPU_AT_SECONDS]}
cpu_at=${2:-0}
factor=${JUMP_FACTOR:-2}
floor=${MIN_GOROUTINES:-500}
max=${MAX_TRACES:-3}
gap=${TRACE_GAP:-60}
tsec=${TRACE_SECONDS:-3}
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

start=$(date +%s) traces=0 last=0 cpu_done=0
hist=()
echo "$(date -u +%T) pprof-watch: $pp, trace ${tsec}s when goroutines > ${factor}x the 30 s median (min $floor), max $max, ${gap}s apart; cpu at +${cpu_at}s"
while :; do
  now=$(date +%s)
  g=$(curl -s -m 2 -H "Authorization: Bearer $GWBENCH_METRICS_TOKEN" "$GWBENCH_GATEWAY_URL/metrics" |
    awk '$1 == "go_goroutines" { print int($2) }')
  if [ -z "$g" ]; then
    echo "$(date -u +%T) metrics scrape failed or timed out"
  else
    med=0
    if [ ${#hist[@]} -ge 10 ]; then
      med=$(printf '%s\n' "${hist[@]}" | sort -n | awk '{ v[NR] = $1 } END { print v[int((NR + 1) / 2)] }')
    fi
    if [ "$med" -gt 0 ] && [ "$g" -gt "$floor" ] && [ "$g" -gt $((med * factor)) ] &&
      [ "$traces" -lt "$max" ] && [ $((now - last)) -ge "$gap" ]; then
      traces=$((traces + 1)) last=$now ts=$(date -u +%H%M%S)
      echo "$(date -u +%T) jump: goroutines=$g (30 s median $med), trace $traces"
      curl -s -m $((tsec + 30)) "$pp/trace?seconds=$tsec" > "$out/trace-$ts-g$g.out" &
      if [ "${DUMPS:-0}" = 1 ]; then
        curl -s -m 60 "$pp/goroutine?debug=2" > "$out/goroutines-$ts-g$g.txt" &
      fi
    fi
    hist+=("$g")
    [ ${#hist[@]} -gt 30 ] && hist=("${hist[@]:1}")
  fi
  if [ "$cpu_at" -gt 0 ] && [ "$cpu_done" -eq 0 ] && [ $((now - start)) -ge "$cpu_at" ]; then
    cpu_done=1
    echo "$(date -u +%T) cpu profile, 30 s (goroutines=$g)"
    curl -s -m 90 "$pp/profile?seconds=30" > "$out/cpu-$(date -u +%H%M%S).pb.gz" &
  fi
  sleep 1
done
