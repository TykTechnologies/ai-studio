#!/usr/bin/env bash
# The two-site stack on Kubernetes, for failover under load (bead
# midsommar-clean-4x5y.29). kubectl only, one context, one namespace; this
# script is the failover manager, as dev/aa/aa.sh is for the compose stack.
#
#   aa-k8s.sh secrets                 create the Secrets with throwaway random values (once)
#   aa-k8s.sh apply                   namespace, PostgreSQL, Studio, control proxies, mock upstream
#                                     (needs STUDIO_IMAGE and TOOLS_IMAGE; edges are the Helm chart, see README)
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
wait_sql() { local i; for i in $(seq 1 90); do [ "$(sql "$1" "$2" 2>/dev/null)" = t ] && return 0; sleep 1; done; die "pg-$1 did not $3 within 90 s"; }
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
		--from-literal=GRPC_AUTH_TOKEN="$token" --from-literal=CSRF_KEY="$(rand)" ${TYK_AI_LICENSE:+--from-literal=TYK_AI_LICENSE="$TYK_AI_LICENSE"}
	k create secret generic aa-edge --from-literal=EDGE_AUTH_TOKEN="$token" --from-literal=ENCRYPTION_KEY="$enc" ${TYK_AI_LICENSE:+--from-literal=TYK_AI_LICENSE="$TYK_AI_LICENSE"}
	k create secret generic aa-load --from-literal=PROBE_PASSWORD="Probe-$(rand | cut -c1-16)!x" --from-literal=APP_KEY=unset
}

cmd_apply() {
	: "${STUDIO_IMAGE:?set STUDIO_IMAGE to the Studio image in the stack registry}" "${TOOLS_IMAGE:?set TOOLS_IMAGE to the benchmark tools image (gwbench, mockllm)}"
	kubectl --context "$CTX" apply -f "$HERE/00-namespace.yaml"
	k get secret aa-shared >/dev/null 2>&1 || die "run 'aa-k8s.sh secrets' first"
	k create configmap aa-pg-node --from-file=pg-node-k8s.sh="$HERE/pg-node-k8s.sh" --dry-run=client -o yaml | k apply -f -
	k create configmap aa-scenarios --from-file="$HERE/scenarios" --dry-run=client -o yaml | k apply -f -
	k create configmap aa-adminloop --from-file=adminloop.py="$HERE/adminloop.py" --dry-run=client -o yaml | k apply -f -
	# The roles ConfigMap is the failover state: never reset it on a re-apply.
	k get configmap aa-pg-roles >/dev/null 2>&1 || k apply -f "$HERE/05-pg-roles.yaml"
	k apply -f "$HERE/10-postgres.yaml"
	sed "s|STUDIO_IMAGE|$STUDIO_IMAGE|" "$HERE/20-studio.yaml" | k apply -f -
	k apply -f "$HERE/25-edges-service.yaml"
	sed "s|TOOLS_IMAGE|$TOOLS_IMAGE|" "$HERE/30-tools.yaml" | k apply -f -
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
	k exec "deploy/gwbench-$1" -- sh -c "
		sed 's/RUN_DURATION/$4/' /bench/scenarios/$3.yaml > /tmp/$2-$1.yaml &&
		nohup gwbench run -no-analytics-check -rate-scale $5 -label '$2 ($3 x$5)' \
			-metrics http://edge-$6-microgateway:8080/metrics /tmp/$2-$1.yaml > /bench/results/$2-$1.log 2>&1 &"
}

cmd_run() {
	local name=${1:?run name} duration=${2:-12m}
	k scale deployment adminloop --replicas=1 >/dev/null
	k rollout status deployment adminloop --timeout=60s >/dev/null
	# Two processes, because gwbench runs one rate at a time; the analytics
	# check is done once for both afterwards (they share the App).
	start_half short "$name" "${SHORT_SCENARIO:-aa-short}" "$duration" "${SHORT_SCALE:-1}" a
	start_half stream "$name" aa-stream "$duration" "${STREAM_SCALE:-1}" b
	stamp "run $name started for $duration (short x${SHORT_SCALE:-1}, stream x${STREAM_SCALE:-1})"
}

cmd_fetch() {
	local name=${1:?run name} out="$HERE/results/${1}" half pod
	mkdir -p "$out"
	for half in short stream; do
		pod=$(k get pod -l "app=gwbench,half=$half" -o jsonpath='{.items[0].metadata.name}')
		kubectl --context "$CTX" cp "$NS/$pod:/bench/results" "$out/$half" >/dev/null
	done
	k logs deploy/adminloop --tail=-1 > "$out/adminloop.jsonl"
	k scale deployment adminloop --replicas=0 >/dev/null
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
		k exec "pg-$s-0" -c postgres -- sh -c "sed -i '/^synchronous_standby_names/d' \"\$PGDATA/aa.conf\""
		k exec "pg-$s-0" -c postgres -- sh -c 'pg_ctl -D "$PGDATA" -w promote' >/dev/null
		wait_sql "$s" 'SELECT NOT pg_is_in_recovery()' "leave recovery"
		name_primary "$s"
		stamp "pg-$s is the primary"
		return
	done
	die "no running standby to promote"
}

cmd_lose_db() {
	local p
	p=$(primary) || die "no primary"
	stamp "fault: pg-$p killed and held at zero"
	k scale statefulset "pg-$p" --replicas=0
	k delete pod "pg-$p-0" --grace-period=0 --force >/dev/null 2>&1 || true
	k wait --for=delete "pod/pg-$p-0" --timeout=60s >/dev/null 2>&1 || true
	cmd_promote
}

cmd_rejoin() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	[ "$(role "$o")" = primary ] || die "pg-$o is $(role "$o"), not a primary"
	[ "$(k get configmap aa-pg-roles -o jsonpath='{.data.primary}')" = "pg-$o" ] || die "the named primary is not pg-$o"
	# The pod's entrypoint sees it is not the named primary and rebuilds
	# its data directory from pg-$o.
	k scale statefulset "pg-$s" --replicas=1
	wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE state = 'streaming'" "see pg-$s streaming"
	set_sync "$o" on
	wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE sync_state = 'sync'" "get a synchronous standby"
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
	k scale statefulset "edge-$s-microgateway" "pg-$s" --replicas=0
	k delete pod "pg-$s-0" "edge-$s-microgateway-0" --grace-period=0 --force >/dev/null 2>&1 || true
	if [ "$was" = primary ]; then k wait --for=delete "pod/pg-$s-0" --timeout=60s >/dev/null 2>&1 || true; cmd_promote; fi
}

cmd_return_site() {
	local s; s=$(site "${1:-}")
	stamp "site-$s returns"
	cmd_rejoin "$s"
	k scale deployment "studio-$s" "cp-$s" --replicas=1
	k rollout status deployment "studio-$s" --timeout=180s
	k scale statefulset "edge-$s-microgateway" --replicas=1
	k rollout status statefulset "edge-$s-microgateway" --timeout=180s
}

cmd_cp() { # 0|1 a|b|all
	local which=${2:-}
	case "$which" in a | b) set -- "$1" "cp-$which" ;; all) set -- "$1" cp-a cp-b ;; *) die "which control plane? a, b or all" ;; esac
	local n=$1; shift
	stamp "control proxies $* to $n"
	k scale deployment "$@" --replicas="$n"
}

cmd_kill_edge() { local s; s=$(site "${1:-}"); stamp "fault: edge-$s pod deleted"; k delete pod "edge-$s-microgateway-0" --grace-period=0 --force; }

cmd_teardown() {
	[ "${1:-}" = "$NS" ] || die "to delete the namespace and everything in it, run: aa-k8s.sh teardown $NS"
	kubectl --context "$CTX" delete namespace "$NS"
}

case "${1:-}" in
secrets) cmd_secrets ;;
apply) cmd_apply ;;
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
*) sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
