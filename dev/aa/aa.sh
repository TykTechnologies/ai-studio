#!/usr/bin/env bash
# The failover manager of the two-site dev stack, by hand.
#
# The stack (dev/docker-compose.active-active.yml, "make dev-aa") never fails
# over by itself; these commands do what Patroni, CloudNativePG or a managed
# database would do, one step at a time, so each step can be watched.
#
#   dev/aa/aa.sh status              roles, replication, replicas, edges
#   dev/aa/aa.sh stop-site a|b       stop that site's Studio, its control
#                                    endpoint and its PostgreSQL node
#   dev/aa/aa.sh start-site a|b      start a stopped site again as it was (no
#                                    failover happened in between)
#   dev/aa/aa.sh promote             promote the standby (the primary must be
#                                    stopped): the database failover
#   dev/aa/aa.sh rejoin a|b          bring a stopped site back: its PostgreSQL
#                                    node is rebuilt as the standby of the
#                                    current primary, then Studio is started
#   dev/aa/aa.sh switchover          planned move of the primary role to the
#                                    standby, both sites staying up
#   dev/aa/aa.sh cut-cp a|b          cut edges off from that site's control
#                                    plane (gRPC only; Studio keeps running)
#   dev/aa/aa.sh restore-cp a|b      undo cut-cp
#
# Every command can be undone with another one; none needs "make dev-aa-clean".
set -euo pipefail

SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
cd "$(dirname "$0")/.."
compose() { docker compose -f docker-compose.active-active.yml "$@"; }

die() { echo "aa.sh: $*" >&2; exit 1; }
site() { case "${1:-}" in a | b) echo "$1" ;; *) die "which site? a or b" ;; esac; }
other() { [ "$1" = a ] && echo b || echo a; }

running() { [ "$(docker inspect -f '{{.State.Running}}' "midsommar-aa-$1" 2>/dev/null)" = true ]; }
sql() { docker exec "midsommar-aa-pg-$1" psql -U postgres -d "${3:-postgres}" -X -qAt -c "$2"; }
# role <site>: primary, standby or stopped.
role() {
	running "pg-$1" || { echo stopped; return; }
	case "$(sql "$1" 'SELECT pg_is_in_recovery()' 2>/dev/null)" in
	f) echo primary ;; t) echo standby ;; *) echo starting ;;
	esac
}
primary() {
	local s
	for s in a b; do [ "$(role "$s")" = primary ] && { echo "$s"; return; }; done
	return 1
}
wait_sql() { # <site> <boolean query> <what>
	local i
	for i in $(seq 1 60); do
		[ "$(sql "$1" "$2" 2>/dev/null)" = t ] && return 0
		sleep 1
	done
	die "pg-$1 did not $3 within 60 s"
}
# The node's own settings live in aa.conf (see pg-node.sh).
set_sync() { # <site> on|off
	local names=''
	[ "$2" = on ] && names='*'
	docker exec "midsommar-aa-pg-$1" sh -c "sed -i '/^synchronous_standby_names/d' \"\$PGDATA/aa.conf\"; echo \"synchronous_standby_names = '$names'\" >> \"\$PGDATA/aa.conf\""
	sql "$1" 'SELECT pg_reload_conf()' >/dev/null
	wait_sql "$1" "SELECT current_setting('synchronous_standby_names') = '$names'" "apply synchronous_standby_names"
}

cmd_status() {
	local s p
	for s in a b; do
		echo "site-$s: postgres $(role "$s"), studio $(running "studio-$s" && echo up || echo stopped), control endpoint $(running "cp-$s" && echo open || echo cut)"
	done
	if p=$(primary); then
		echo "replication (on pg-$p): $(sql "$p" "SELECT COALESCE(string_agg(application_name || ' ' || state || ' ' || sync_state, ', '), 'no standby attached') FROM pg_stat_replication")"
		echo "synchronous_standby_names: '$(sql "$p" 'SHOW synchronous_standby_names')'"
		echo "replicas (cluster_nodes):"
		sql "$p" "SELECT '  ' || label || '  ' || node_id || '  last seen ' || round(EXTRACT(EPOCH FROM now() - last_seen)) || ' s ago' FROM cluster_nodes ORDER BY label" midsommar 2>/dev/null || echo "  (not available yet)"
		echo "edges (edge_instances):"
		sql "$p" "SELECT '  ' || e.edge_id || '  ' || e.status || '  held by ' || COALESCE(n.label, 'no live replica') || '  last heartbeat ' || round(EXTRACT(EPOCH FROM now() - e.last_heartbeat)) || ' s ago' FROM edge_instances e LEFT JOIN cluster_nodes n ON n.node_id = e.owner_node_id WHERE e.deleted_at IS NULL ORDER BY e.edge_id" midsommar 2>/dev/null || echo "  (not available yet)"
	else
		echo "no primary: writes fail until 'aa.sh promote' (or the primary's site is started again)"
	fi
}

cmd_stop_site() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	# Losing the standby's site: a failover manager drops the synchronous
	# requirement, or every commit on the primary would wait for it.
	if [ "$(role "$s")" = standby ] && [ "$(role "$o")" = primary ]; then
		echo "pg-$s is the synchronous standby: replication on pg-$o becomes asynchronous"
		set_sync "$o" off
	fi
	compose stop "studio-$s" "cp-$s" "pg-$s"
	echo "site-$s is stopped. $([ "$(role "$o")" = primary ] || echo "pg-$o is a standby: run 'aa.sh promote' to fail the database over.")"
}

cmd_start_site() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	if [ "$(role "$s")" = stopped ]; then
		# A node stopped as the primary must not come back as one next
		# to a promoted peer: that would be a split brain.
		if [ "$(role "$o")" = primary ] &&
			! compose run --rm --no-deps -T --entrypoint sh "pg-$s" -c 'test -f "$PGDATA/standby.signal"' 2>/dev/null; then
			die "pg-$o was promoted while site-$s was down: use 'aa.sh rejoin $s'"
		fi
		compose up -d --no-deps "pg-$s"
		wait_sql "$s" 'SELECT true' "accept connections"
	fi
	if [ "$(role "$s")" = standby ] && [ "$(role "$o")" = primary ]; then
		wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE state = 'streaming'" "see pg-$s streaming"
		set_sync "$o" on
	fi
	compose up -d --no-deps --wait "studio-$s" "cp-$s"
	echo "site-$s is started: postgres $(role "$s")."
}

cmd_promote() {
	local s o
	for s in a b; do
		[ "$(role "$s")" = standby ] || continue
		o=$(other "$s")
		[ "$(role "$o")" = stopped ] || die "pg-$o is $(role "$o"): stop its site first (stop-site $o), or use switchover"
		# The promoted node starts without a standby.
		docker exec "midsommar-aa-pg-$s" sh -c "sed -i '/^synchronous_standby_names/d' \"\$PGDATA/aa.conf\""
		docker exec "midsommar-aa-pg-$s" pg_ctl -D /var/lib/postgresql/data -w promote >/dev/null
		wait_sql "$s" 'SELECT NOT pg_is_in_recovery()' "leave recovery"
		echo "pg-$s is the primary. Bring the other site back with 'aa.sh rejoin $o'."
		return
	done
	die "no running standby to promote"
}

cmd_rejoin() {
	local s o
	s=$(site "${1:-}"); o=$(other "$s")
	[ "$(role "$o")" = primary ] || die "pg-$o is $(role "$o"), not a primary: nothing to rejoin to"
	[ "$(role "$s")" = primary ] && die "pg-$s is running as a primary too; stop its site first"
	if [ "$(role "$s")" = stopped ]; then
		# Its data may have diverged (it was the primary): rebuild it.
		compose run --rm --no-deps -T -e "AA_REBUILD_FROM=pg-$o" "pg-$s"
		compose up -d --no-deps "pg-$s"
	fi
	wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE state = 'streaming'" "see pg-$s streaming"
	set_sync "$o" on
	wait_sql "$o" "SELECT count(*) = 1 FROM pg_stat_replication WHERE sync_state = 'sync'" "get a synchronous standby"
	# --wait: Studio is healthy (migrations checked, API up) before this
	# returns, so the next command does not catch it mid-start.
	compose up -d --no-deps --wait "studio-$s" "cp-$s"
	echo "site-$s is back: pg-$s is the synchronous standby of pg-$o."
}

cmd_switchover() {
	local p o
	p=$(primary) || die "no primary"
	o=$(other "$p")
	[ "$(role "$o")" = standby ] || die "pg-$o is $(role "$o"), not a standby"
	echo "moving the primary role from pg-$p to pg-$o"
	set_sync "$p" off
	compose stop "pg-$p" # a clean shutdown: the standby has everything
	docker exec "midsommar-aa-pg-$o" sh -c "sed -i '/^synchronous_standby_names/d' \"\$PGDATA/aa.conf\""
	docker exec "midsommar-aa-pg-$o" pg_ctl -D /var/lib/postgresql/data -w promote >/dev/null
	wait_sql "$o" 'SELECT NOT pg_is_in_recovery()' "leave recovery"
	cmd_rejoin "$p"
}

cmd_cp() { # stop|start <site>
	local s
	s=$(site "${2:-}")
	compose "$1" "cp-$s"
	[ "$1" = stop ] && echo "edges can no longer reach site-$s's control plane (Studio itself is untouched)" ||
		echo "site-$s's control endpoint is open again"
}

case "${1:-}" in
status) cmd_status ;;
stop-site) cmd_stop_site "${2:-}" ;;
start-site) cmd_start_site "${2:-}" ;;
promote) cmd_promote ;;
rejoin) cmd_rejoin "${2:-}" ;;
switchover) cmd_switchover ;;
cut-cp) cmd_cp stop "${2:-}" ;;
restore-cp) cmd_cp start "${2:-}" ;;
*) sed -n '2,26p' "$SELF" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
