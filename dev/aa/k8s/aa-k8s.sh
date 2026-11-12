#!/usr/bin/env bash
# The two-site stack on Kubernetes, for failover under load (bead
# midsommar-clean-4x5y.29). kubectl only, one context, one namespace; this
# script is the failover manager, as dev/aa/aa.sh is for the compose stack.
#
#   aa-k8s.sh secrets                 create the Secrets with throwaway random values (once)
#   aa-k8s.sh license                 Enterprise only: TYK_AI_LICENSE from AA_LICENSE_ENV_FILE into its own Secret
#   aa-k8s.sh apply                   namespace, PostgreSQL, Studio, control proxies, mock upstream
#                                     (needs STUDIO_IMAGE and TOOLS_IMAGE; edges are the Helm chart, see README)
#   aa-k8s.sh render-edges            helm template of the two edges into rendered/ (needs MICROGATEWAY_IMAGE)
#   aa-k8s.sh status                  pods, roles, replication, replicas, edges
#   aa-k8s.sh users                   register the test's own users; make the seeder and the two writers administrators
#   aa-k8s.sh seed                    gwbench seed: LLMs on the mock, App and key, prices, push to both edges
#   aa-k8s.sh run NAME [DURATION]     start one run: both gwbench halves and the admin loop
#                                     (SHORT_SCALE / STREAM_SCALE: --rate-scale, 1 = MEDIAN; SHORT_SCENARIO=aa-ramp-short for the ramp)
#   aa-k8s.sh fetch NAME              copy that run's results and the admin loop's log to results/NAME (gitignored)
#
#   aa-k8s.sh lose-cp a|b             fault 1: that site's Studio held at zero       undo: return-cp
#   aa-k8s.sh lose-db                 fault 2: the primary's pod killed and held at zero, then promote
#   aa-k8s.sh promote                 promote the standby and name it the primary
#   aa-k8s.sh rejoin a|b              bring that site's PostgreSQL back as the synchronous standby
#   aa-k8s.sh lose-site a|b           fault 3: Studio, control proxy, PostgreSQL and edge of that site at zero
#   aa-k8s.sh return-site a|b         undo: rejoin, then Studio, proxy and edge
#   aa-k8s.sh cut-cp a|b|all          fault 4, first half: control proxies at zero   undo: restore-cp
#   aa-k8s.sh kill-edge a|b           fault 4, second half: delete that edge's pod
#   aa-k8s.sh switchover              fault 5: planned move of the primary role
#                                     (SWITCHOVER_HOLD=N: N more seconds without a primary before the promote)
#   aa-k8s.sh pulse [FILE]            each edge's dropped-pulse-records counters and pod start (run/fetch record them)
#   aa-k8s.sh zone-b up|down          fault 7: mock and generator in zone b, observers on, mock Service zone-local
#   aa-k8s.sh run-partition NAME [DURATION]  fault 7: zone a's generators on edge-a, zone b's on edge-b, half the
#                                     part A level each (fetch NAME collects both zones)
#   aa-k8s.sh drop-sync-at EPOCH      fault 7: the primary's own pod drops the synchronous requirement at that time
#   aa-k8s.sh teardown                delete the namespace (asks for the namespace name)
set -euo pipefail

CTX=${AA_K8S_CONTEXT:-aa-epic-23}
NS=aa-load
HERE=$(cd "$(dirname "$0")" && pwd)
k() { kubectl --context "$CTX" -n "$NS" "$@"; }
die() { echo "aa-k8s.sh: $*" >&2; exit 1; }
site() { case "${1:-}" in a | b) echo "$1" ;; *) die "which site? a or b" ;; esac; }
other() { [ "$1" = a ] && echo b || echo a; }
rand() { head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'; }

sql() { k exec "pg-$1-0" -c postgres -- psql -U postgres -d "${3:-postgres}" -X -qAt -c "$2"; }
role() {
	[ "$(k get pod "pg-$1-0" -o jsonpath='{.status.phase}' 2>/dev/null)" = Running ] || { echo stopped; return; }
	case "$(sql "$1" 'SELECT pg_is_in_recovery()' 2>/dev/null)" in f) echo primary ;; t) echo standby ;; *) echo starting ;; esac
}
primary() { local s; for s in a b; do [ "$(role "$s")" = primary ] && { echo "$s"; return; }; done; return 1; }
wait_sql() { local i n=${WAIT_TRIES:-90}; for i in $(seq 1 "$n"); do [ "$(sql "$1" "$2" 2>/dev/null)" = t ] && return 0; sleep 1; done; die "pg-$1 did not $3 within $n tries"; }
set_sync() { # <site> on|off
	local names=''; [ "$2" = on ] && names='*'
	k exec "pg-$1-0" -c postgres -- sh -c "sed -i '/^synchronous_standby_names/d' \"\$PGDATA/aa.conf\"; echo \"synchronous_standby_names = '$names'\" >> \"\$PGDATA/aa.conf\""
	sql "$1" 'SELECT pg_reload_conf()' >/dev/null
	wait_sql "$1" "SELECT current_setting('synchronous_standby_names') = '$names'" "apply synchronous_standby_names"
}
name_primary() { k patch configmap aa-pg-roles --type merge -p "{\"data\":{\"primary\":\"pg-$1\"}}" >/dev/null; }
stamp() { echo "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ) $*"; }

cmd_secrets() {
	k get secret aa-shared >/dev/null 2>&1 && die "the Secrets exist already; delete the namespace to start over"
	local pg key
	pg=$(rand); key=$(rand)
	k create secret generic aa-shared --from-literal=POSTGRES_PASSWORD="$pg" \
		--from-literal=DATABASE_URL="postgres://postgres:$pg@pg-a:5432,pg-b:5432/midsommar?sslmode=disable&target_session_attrs=read-write&connect_timeout=3"
	# The edges need the same token and encryption key as Studio.
	local token enc
	token=$(rand); enc=$(rand | cut -c1-32)
	k create secret generic aa-studio --from-literal=TYK_AI_SECRET_KEY="$key" --from-literal=MICROGATEWAY_ENCRYPTION_KEY="$enc" \
		--from-literal=GRPC_AUTH_TOKEN="$token" --from-literal=CSRF_KEY="$(rand)"
	k create secret generic aa-edge --from-literal=EDGE_AUTH_TOKEN="$token" --from-literal=ENCRYPTION_KEY="$enc"
	k create secret generic aa-load --from-literal=PROBE_PASSWORD="Probe-$(rand | cut -c1-16)!x" --from-literal=APP_KEY=unset
}

# The Enterprise license, for Enterprise images only: read in place from a
# gitignored env file (AA_LICENSE_ENV_FILE, a line TYK_AI_LICENSE=...) straight
# into a Secret of its own. It is never printed, never an argument, and in no
# manifest; it goes when the namespace is deleted.
cmd_license() {
	: "${AA_LICENSE_ENV_FILE:?set AA_LICENSE_ENV_FILE to the env file holding TYK_AI_LICENSE}"
	grep -q '^TYK_AI_LICENSE=.' "$AA_LICENSE_ENV_FILE" || die "no TYK_AI_LICENSE in that file"
	k create secret generic aa-license --from-env-file=<(grep '^TYK_AI_LICENSE=' "$AA_LICENSE_ENV_FILE") >/dev/null
	echo "secret aa-license created ($(k get secret aa-license -o jsonpath='{.data.TYK_AI_LICENSE}' | wc -c) base64 characters)"
}

cmd_apply() {
	: "${STUDIO_IMAGE:?set STUDIO_IMAGE to the Studio image in the stack registry}" "${TOOLS_IMAGE:?set TOOLS_IMAGE to the benchmark tools image (gwbench, mockllm)}"
	kubectl --context "$CTX" apply -f "$HERE/00-namespace.yaml"
	k get secret aa-shared >/dev/null 2>&1 || die "run 'aa-k8s.sh secrets' first"
	k create configmap aa-pg-node --from-file=pg-node-k8s.sh="$HERE/pg-node-k8s.sh" --dry-run=client -o yaml | k apply -f -
	# This stack's scenarios, and the benchmark's A/A calibration as it is.
	k create configmap aa-scenarios --from-file="$HERE/scenarios" \
		--from-file="$HERE/../../../benchmarks/gateway/scenarios/s0-aa-calibration.yaml" --dry-run=client -o yaml | k apply -f -
	k create configmap aa-adminloop --from-file=adminloop.py="$HERE/adminloop.py" --dry-run=client -o yaml | k apply -f -
	k create configmap aa-observer --from-file=observer.py="$HERE/observer.py" --from-file=observer-pg.sh="$HERE/observer-pg.sh" --dry-run=client -o yaml | k apply -f -
	k apply -f "$HERE/50-observers.yaml"
	# The roles ConfigMap is the failover state: never reset it on a re-apply.
	k get configmap aa-pg-roles >/dev/null 2>&1 || k apply -f "$HERE/05-pg-roles.yaml"
	k apply -f "$HERE/10-postgres.yaml"
	sed "s|STUDIO_IMAGE|$STUDIO_IMAGE|" "$HERE/20-studio.yaml" | k apply -f -
	# The edges, if they have been rendered (aa-k8s.sh render-edges).
	if [ -f "$HERE/rendered/edge-a.yaml" ] && [ "${AA_HUB_ONLY:-}" != 1 ]; then
		k apply -f "$HERE/rendered/edge-a.yaml" -f "$HERE/rendered/edge-b.yaml"
	fi
	# AA_HUB_ONLY=1: the hub side alone (the edge and tools nodes may not be up yet).
	[ "${AA_HUB_ONLY:-}" = 1 ] && return 0
	k apply -f "$HERE/25-edges-service.yaml"
	sed "s|TOOLS_IMAGE|$TOOLS_IMAGE|" "$HERE/30-tools.yaml" | k apply -f -
}

# helm template with the local alpine/helm image; nothing touches the cluster.
# AA_CHART_DIR: the chart to render (default helm/charts/microgateway).
cmd_render_edges() {
	: "${MICROGATEWAY_IMAGE:?set MICROGATEWAY_IMAGE to repository:tag of the microgateway image}"
	local chart=${AA_CHART_DIR:-$HERE/../../../helm/charts/microgateway} s
	mkdir -p "$HERE/rendered"
	for s in a b; do
		sed -e "s|MICROGATEWAY_REPOSITORY|${MICROGATEWAY_IMAGE%:*}|" -e "s|MICROGATEWAY_TAG|${MICROGATEWAY_IMAGE##*:}|" "$HERE/values-edge-$s.yaml" > "$HERE/rendered/values-edge-$s.yaml"
		docker run --rm -v "$(cd "$chart" && pwd):/chart:ro" -v "$HERE/rendered/values-edge-$s.yaml:/values.yaml:ro" alpine/helm:3.16.2 \
			template "edge-$s" /chart --namespace "$NS" -f /values.yaml > "$HERE/rendered/edge-$s.yaml"
		echo "rendered/edge-$s.yaml: $(grep -c '^kind:' "$HERE/rendered/edge-$s.yaml") objects"
	done
}

cmd_status() {
	k get pods -o wide
	local s p
	for s in a b; do echo "site-$s: postgres $(role "$s")"; done
	echo "named primary: $(k get configmap aa-pg-roles -o jsonpath='{.data.primary}')"
	if p=$(primary); then
		echo "replication (on pg-$p): $(sql "$p" "SELECT COALESCE(string_agg(application_name || ' ' || state || ' ' || sync_state, ', '), 'no standby attached') FROM pg_stat_replication")"
		sql "$p" "SELECT 'replica ' || label || ' ' || node_id || ' last seen ' || round(EXTRACT(EPOCH FROM now() - last_seen)) || ' s ago' FROM cluster_nodes ORDER BY label" midsommar 2>/dev/null || true
		sql "$p" "SELECT 'edge ' || e.edge_id || ' ' || e.status || ' held by ' || COALESCE(n.label, 'no live replica') FROM edge_instances e LEFT JOIN cluster_nodes n ON n.node_id = e.owner_node_id WHERE e.deleted_at IS NULL ORDER BY e.edge_id" midsommar 2>/dev/null || true
	else
		echo "no primary"
	fi
}

cmd_users() {
	local p pw u
	p=$(primary) || die "no primary"
	pw=$(k get secret aa-load -o jsonpath='{.data.PROBE_PASSWORD}' | base64 -d)
	k port-forward svc/studio-a 18180:8080 >/dev/null 2>&1 &
	local pf=$!
	trap 'kill $pf 2>/dev/null' RETURN
	sleep 3
	for u in aa-probe-seed aa-probe-write-a aa-probe-write-b aa-probe-login-a aa-probe-login-b; do
		local jar token
		jar=$(mktemp)
		token=$(curl -s -D - -o /dev/null -c "$jar" -H "Origin: http://studio.aa-load.test" http://localhost:18180/csrf-token | tr -d '\r' | awk 'tolower($1)=="x-csrf-token:"{print $2}')
		curl -s -o /dev/null -w "$u@tyk.io register: HTTP %{http_code}\n" -b "$jar" -H "X-CSRF-Token: $token" -H "Origin: http://studio.aa-load.test" -H "Referer: http://studio.aa-load.test/" \
			-H 'Content-Type: application/json' -d "{\"data\":{\"type\":\"user\",\"attributes\":{\"email\":\"$u@tyk.io\",\"name\":\"$u\",\"password\":\"$pw\",\"with_portal\":true,\"with_chat\":true}}}" http://localhost:18180/auth/register
		rm -f "$jar"
	done
	sql "$p" "UPDATE users SET email_verified = true WHERE email LIKE 'aa-probe-%@tyk.io'; UPDATE users SET is_admin = true WHERE email LIKE 'aa-probe-write-%@tyk.io' OR email = 'aa-probe-seed@tyk.io'" midsommar
}

cmd_seed() {
	local pw
	pw=$(k get secret aa-load -o jsonpath='{.data.PROBE_PASSWORD}' | base64 -d)
	# In the short-request generator's pod: it reaches Studio, the mock and
	# the edges by their Service names. The state file holds the App key.
	k exec deploy/gwbench-short -- env GWBENCH_ADMIN_PASSWORD="$pw" gwbench seed -email aa-probe-seed@tyk.io \
		-mock-upstream http://mockllm:9999 -min-edges 2 -state /tmp/state.json
	k exec deploy/gwbench-short -- cat /tmp/state.json |
		k create secret generic aa-gwbench-state --from-file=state.json=/dev/stdin --dry-run=client -o yaml | k apply -f -
	k rollout restart deployment gwbench-short gwbench-stream
	k rollout status deployment gwbench-short --timeout=120s
	k rollout status deployment gwbench-stream --timeout=120s
}

# start_half <short|stream> <run name> <scenario> <duration> <rate scale> <edge whose /metrics is sampled>
start_half() {
	k exec "deploy/${GEN_DEPLOY:-gwbench-$1}" -- sh -c "
		sed 's/RUN_DURATION/$4/' /bench/scenarios/$3.yaml > /tmp/$2-$1.yaml &&
		mkdir -p /bench/results/$2 &&
		${GATEWAY_URL:+GWBENCH_GATEWAY_URL=$GATEWAY_URL} nohup gwbench run -no-analytics-check -rate-scale $5 -label '$2 ($3 x$5)' -out /bench/results/$2 \
			-metrics http://edge-$6:8080/metrics /tmp/$2-$1.yaml > /bench/results/$2/$1.log 2>&1 &"
}

cmd_run() {
	local name=${1:?run name} duration=${2:-12m}
	k scale deployment adminloop --replicas=1 >/dev/null
	k rollout status deployment adminloop --timeout=60s >/dev/null
	# Two processes, because gwbench runs one rate at a time; the analytics
	# check is done once for both afterwards (they share the App).
	mkdir -p "$HERE/results/$name"
	log_rows > "$HERE/results/$name/proxy-log-rows.before"
	cmd_pulse "$HERE/results/$name/pulse-dropped.before"
	start_half short "$name" "${SHORT_SCENARIO:-aa-short}" "$duration" "${SHORT_SCALE:-1}" a
	# NO_STREAM=1: the short half alone (the ramp, with the stream generator at zero).
	[ "${NO_STREAM:-}" = 1 ] || start_half stream "$name" aa-stream "$duration" "${STREAM_SCALE:-1}" b
	stamp "run $name started for $duration (short x${SHORT_SCALE:-1}, stream x${STREAM_SCALE:-1})"
}

# Fault 7: each zone's load goes to its own zone's edge, so the cut never
# stands between a generator and its edge. Half the part A level per zone
# (0.25 of the MEDIAN scale each): 1,500 short + 94 streams/s.
cmd_run_partition() {
	local name=${1:?run name} duration=${2:-12m} sc=${ZONE_SCALE:-0.25}
	k get deploy gwbench-zb >/dev/null 2>&1 || die "run 'aa-k8s.sh zone-b up' first"
	k scale deployment adminloop --replicas=1 >/dev/null
	k rollout status deployment adminloop --timeout=60s >/dev/null
	mkdir -p "$HERE/results/$name"
	log_rows > "$HERE/results/$name/proxy-log-rows.before"
	cmd_pulse "$HERE/results/$name/pulse-dropped.before"
	GATEWAY_URL=http://edge-a:8080 start_half short "$name" aa-short "$duration" "$sc" a
	GATEWAY_URL=http://edge-a:8080 start_half stream "$name" aa-stream "$duration" "$sc" a
	GEN_DEPLOY=gwbench-zb start_half short "$name-zb-short" aa-short "$duration" "$sc" b
	GEN_DEPLOY=gwbench-zb start_half stream "$name-zb-stream" aa-stream "$duration" "$sc" b
	stamp "partition run $name started for $duration (each zone: short x$sc, stream x$sc on its own edge)"
}

# Rows in Studio's proxy log, in all and by response code. gwbench's own
# completeness check is off (two generators share the App), so a run compares
# this before and after with what both generators' requests were answered.
log_rows() {
	local p
	p=$(primary) || die "no primary"
	sql "$p" "SELECT count(*) FROM proxy_logs" midsommar
}

cmd_fetch() {
	local name=${1:?run name} out="$HERE/results/${1}" half pod
	mkdir -p "$out"
	for half in short stream; do
		pod=$(k get pod -l "app=gwbench,half=$half" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) || continue
		[ -n "$pod" ] && k exec "$pod" -- test -d "/bench/results/$name" 2>/dev/null || continue
		# Every request is a line of results.jsonl (millions per run): compress first.
		k exec "$pod" -- sh -c "cd /bench/results/$name && tar czf /tmp/results.tgz ."
		kubectl --context "$CTX" cp "$NS/$pod:/tmp/results.tgz" "$out/$half.tgz" >/dev/null
		mkdir -p "$out/$half" && tar xzf "$out/$half.tgz" -C "$out/$half"
		# Note: the generators write to the node's own disk (about 1.5 GB a
		# run) and nothing here clears it; a dozen runs put the node under disk
		# pressure and the pod is evicted. Restart the generators between passes.
	done
	# Fault 7: the zone-b generator's two halves.
	pod=$(k get pod -l app=gwbench-zb -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) || pod=
	for half in short stream; do
		[ -n "$pod" ] && k exec "$pod" -- test -d "/bench/results/$name-zb-$half" 2>/dev/null || continue
		k exec "$pod" -- sh -c "cd /bench/results/$name-zb-$half && tar czf /tmp/results-$half.tgz ."
		kubectl --context "$CTX" cp "$NS/$pod:/tmp/results-$half.tgz" "$out/zb-$half.tgz" >/dev/null
		mkdir -p "$out/zb-$half" && tar xzf "$out/zb-$half.tgz" -C "$out/zb-$half"
	done
	k logs deploy/adminloop --tail=-1 > "$out/adminloop.jsonl"
	k scale deployment adminloop --replicas=0 >/dev/null
	# Analytics reach Studio on the edges' pulse: wait until the count has
	# stood still for 30 s (5 minutes at most), then compare.
	local rows last=-1 still=0 i
	for i in $(seq 1 60); do
		rows=$(log_rows)
		if [ "$rows" = "$last" ]; then still=$((still + 5)); else still=0; fi
		[ "$still" -ge 30 ] && break
		last=$rows; sleep 5
	done
	echo "$rows" > "$out/proxy-log-rows.after"
	cmd_pulse "$out/pulse-dropped.after"
	local p; p=$(primary)
	sql "$p" "SELECT response_code || ' ' || count(*) FROM proxy_logs GROUP BY response_code ORDER BY response_code" midsommar > "$out/proxy-log-rows.by-code"
	python3 "$HERE/analytics-check.py" "$out"
	stamp "results of $name in $out"
}

cmd_lose_cp() { local s; s=$(site "${1:-}"); stamp "fault: studio-$s to zero"; k scale deployment "studio-$s" --replicas=0; }
cmd_return_cp() { local s; s=$(site "${1:-}"); stamp "studio-$s back"; k scale deployment "studio-$s" --replicas=1; k rollout status deployment "studio-$s" --timeout=180s; }

cmd_promote() {
	local s o
	for s in a b; do
		[ "$(role "$s")" = standby ] || continue
		o=$(other "$s")
		[ "$(role "$o")" = stopped ] || die "pg-$o is $(role "$o"): it must be stopped first (lose-db, lose-site), or use switchover"
		# One exec: the promoted node starts without a standby, and pg_ctl -w
		# returns once it has left recovery.
		k exec "pg-$s-0" -c postgres -- sh -c 'sed -i "/^synchronous_standby_names/d" "$PGDATA/aa.conf"; pg_ctl -D "$PGDATA" -w promote >/dev/null; psql -U postgres -d postgres -qAtc "SELECT NOT pg_is_in_recovery()"' | grep -qx t ||
			die "pg-$s did not leave recovery"
		stamp "pg-$s is the primary"
		name_primary "$s"
		return
	done
	die "no running standby to promote"
}

cmd_lose_db() {
	local p
	p=$(primary) || die "no primary"
	kill_primary "$p"
	cmd_promote
}

# kill_primary <site>: the primary's pod is killed at once and held at zero,
# then PROMOTE_DELAY seconds pass before anything is promoted. The delay
# stands in for a failover manager noticing a dead primary (default 10;
# 0 shows the floor, which is this script's own overhead). It is counted
# from the kill, so the time without a primary is the delay plus the promote.
kill_primary() {
	local t0
	stamp "fault: pg-$1 killed and held at zero (PROMOTE_DELAY=${PROMOTE_DELAY:-10}s)"
	t0=$(date +%s.%N)
	k scale statefulset "pg-$1" --replicas=0 >/dev/null
	k delete pod "pg-$1-0" --grace-period=0 --force >/dev/null 2>&1 || true
	while [ "$(role "$1")" != stopped ]; do sleep 0.2; done
	python3 -c "import time; time.sleep(max(0, ${PROMOTE_DELAY:-10} - (time.time() - $t0)))"
}

cmd_rejoin() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	[ "$(role "$o")" = primary ] || die "pg-$o is $(role "$o"), not a primary"
	[ "$(k get configmap aa-pg-roles -o jsonpath='{.data.primary}')" = "pg-$o" ] || die "the named primary is not pg-$o"
	# The pod's entrypoint sees it is not the named primary and rebuilds
	# its data directory from pg-$o.
	k scale statefulset "pg-$s" --replicas=1
	# By name: while the node copies its data directory, pg_basebackup's own
	# WAL stream is in pg_stat_replication too, and must not be taken for the
	# standby (it once became the synchronous one and held every commit).
	# The copy takes minutes once the database has grown, hence the long wait.
	WAIT_TRIES=900 wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE application_name = 'pg-$s' AND state = 'streaming'" "see pg-$s streaming"
	# Synchronous only once the standby has caught up: a commit waits for a
	# synchronous standby to have everything before it.
	WAIT_TRIES=900 wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE application_name = 'pg-$s' AND pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) < 8 * 1024 * 1024" "see pg-$s caught up"
	set_sync "$o" on
	wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE application_name = 'pg-$s' AND sync_state = 'sync'" "get a synchronous standby"
	stamp "pg-$s is the synchronous standby of pg-$o"
}

cmd_switchover() {
	local p o
	p=$(primary) || die "no primary"; o=$(other "$p")
	[ "$(role "$o")" = standby ] || die "pg-$o is $(role "$o"), not a standby"
	stamp "switchover: pg-$p to pg-$o"
	set_sync "$p" off
	k scale statefulset "pg-$p" --replicas=0 # SIGTERM: a clean shutdown, the standby has everything
	k wait --for=delete "pod/pg-$p-0" --timeout=60s >/dev/null
	# SWITCHOVER_HOLD: a slower switchover, longer without a writable database.
	[ "${SWITCHOVER_HOLD:-0}" = 0 ] || { stamp "holding ${SWITCHOVER_HOLD}s without a primary"; sleep "$SWITCHOVER_HOLD"; }
	cmd_promote
	cmd_rejoin "$p"
}

cmd_lose_site() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	stamp "fault: site-$s lost"
	if [ "$(role "$s")" = standby ] && [ "$(role "$o")" = primary ]; then set_sync "$o" off; fi
	local was; was=$(role "$s")
	k scale deployment "studio-$s" "cp-$s" --replicas=0
	k scale statefulset "edge-$s-microgateway" --replicas=0
	k delete pod "edge-$s-microgateway-0" --grace-period=0 --force >/dev/null 2>&1 || true
	if [ "$was" = primary ]; then
		kill_primary "$s"
		cmd_promote
	else
		k scale statefulset "pg-$s" --replicas=0
		k delete pod "pg-$s-0" --grace-period=0 --force >/dev/null 2>&1 || true
	fi
}

cmd_return_site() {
	local s; s=$(site "${1:-}")
	stamp "site-$s returns"
	cmd_rejoin "$s"
	# The edge is started whatever Studio does: a slow Studio start must not
	# leave the site without its edge (seen under load, when the rejoining
	# standby held commits up for a minute).
	k scale deployment "studio-$s" "cp-$s" --replicas=1
	k scale statefulset "edge-$s-microgateway" --replicas=1
	k rollout status deployment "studio-$s" --timeout=600s || stamp "studio-$s is not Ready after 600 s"
	k rollout status statefulset "edge-$s-microgateway" --timeout=300s || stamp "edge-$s is not Ready after 300 s"
}

cmd_cp() { # 0|1 a|b|all
	local which=${2:-}
	case "$which" in a | b) set -- "$1" "cp-$which" ;; all) set -- "$1" cp-a cp-b ;; *) die "which control plane? a, b or all" ;; esac
	local n=$1; shift
	stamp "control proxies $* to $n"
	k scale deployment "$@" --replicas="$n"
}

cmd_kill_edge() { local s; s=$(site "${1:-}"); stamp "fault: edge-$s pod deleted"; k delete pod "edge-$s-microgateway-0" --grace-period=0 --force; }

# Each edge's microgateway_analytics_pulse_dropped_records_total{reason}
# (images with the .37 fix), read through the API server's Service proxy so
# no tool is needed in any image; with the pod's start time and restarts,
# because a restarted edge starts its counters at zero.
cmd_pulse() {
	local out=${1:-/dev/stdout} s m
	{
		stamp "pulse counters"
		for s in a b; do
			k get pod "edge-$s-microgateway-0" -o jsonpath="{'edge-$s pod started '}{.status.startTime}{' restarts '}{.status.containerStatuses[0].restartCount}{'\n'}" 2>/dev/null || echo "edge-$s pod absent"
			if m=$(kubectl --context "$CTX" get --raw "/api/v1/namespaces/$NS/services/edge-$s:8080/proxy/metrics" 2>/dev/null); then
				echo "$m" | grep -e '^microgateway_analytics_pulse_dropped_records_total' | sed "s/^/edge-$s /" || echo "edge-$s no pulse-dropped counter yet"
			else
				echo "edge-$s /metrics unreachable"
			fi
		done
	} > "$out"
}

# Fault 7 only. up: a second mock and a generator on the zone-b tools node,
# the mock Service made zone-local (each edge keeps to its own zone's mock
# through the cut), the observers at 1. down: undo.
cmd_zone_b() {
	: "${TOOLS_IMAGE:?set TOOLS_IMAGE}"
	case "${1:-}" in
	up)
		sed "s|TOOLS_IMAGE|$TOOLS_IMAGE|" "$HERE/35-zone-b.yaml" | k apply -f -
		k patch service mockllm --type merge -p '{"spec":{"trafficDistribution":"PreferClose"}}'
		k scale deployment observer-a observer-b --replicas=1
		k rollout status deployment mockllm-b --timeout=180s
		k rollout status deployment gwbench-zb --timeout=180s
		;;
	down)
		k scale deployment observer-a observer-b --replicas=0
		k patch service mockllm --type json -p '[{"op":"remove","path":"/spec/trafficDistribution"}]' || true
		sed "s|TOOLS_IMAGE|$TOOLS_IMAGE|" "$HERE/35-zone-b.yaml" | k delete -f - --ignore-not-found
		;;
	*) die "zone-b up|down" ;;
	esac
}

# Fault 7, run (ii): kubectl may not reach the pods during the cut, so the
# drop is staged beforehand inside the primary's own pod, on a timer.
cmd_drop_sync_at() {
	local at=${1:?epoch seconds} p
	p=$(primary) || die "no primary"
	k exec -i "pg-$p-0" -c postgres -- sh -c 'cat > /tmp/drop-sync.sh && setsid nohup sh /tmp/drop-sync.sh </dev/null >/dev/null 2>&1 &' <<EOS
while [ \$(date +%s) -lt $at ]; do sleep 0.2; done
sed -i '/^synchronous_standby_names/d' "\$PGDATA/aa.conf"
echo "synchronous_standby_names = ''" >> "\$PGDATA/aa.conf"
psql -U postgres -qAtc 'SELECT pg_reload_conf()'
date -u +%FT%TZ > /tmp/sync-dropped
EOS
	stamp "pg-$p drops the synchronous requirement at $(date -u -d "@$at" +%FT%TZ) (marker /tmp/sync-dropped in its pod)"
}

cmd_teardown() {
	[ "${1:-}" = "$NS" ] || die "to delete the namespace and everything in it, run: aa-k8s.sh teardown $NS"
	kubectl --context "$CTX" delete namespace "$NS"
}

case "${1:-}" in
secrets) cmd_secrets ;;
license) cmd_license ;;
apply) cmd_apply ;;
render-edges) cmd_render_edges ;;
status) cmd_status ;;
users) cmd_users ;;
seed) cmd_seed ;;
run) cmd_run "${2:-}" "${3:-}" ;;
fetch) cmd_fetch "${2:-}" ;;
lose-cp) cmd_lose_cp "${2:-}" ;;
return-cp) cmd_return_cp "${2:-}" ;;
lose-db) cmd_lose_db ;;
promote) cmd_promote ;;
rejoin) cmd_rejoin "${2:-}" ;;
switchover) cmd_switchover ;;
lose-site) cmd_lose_site "${2:-}" ;;
return-site) cmd_return_site "${2:-}" ;;
cut-cp) cmd_cp 0 "${2:-}" ;;
restore-cp) cmd_cp 1 "${2:-}" ;;
kill-edge) cmd_kill_edge "${2:-}" ;;
teardown) cmd_teardown "${2:-}" ;;
pulse) cmd_pulse "${2:-/dev/stdout}" ;;
zone-b) cmd_zone_b "${2:-}" ;;
drop-sync-at) cmd_drop_sync_at "${2:-}" ;;
run-partition) cmd_run_partition "${2:-}" "${3:-}" ;;
*) sed -n '2,33p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
