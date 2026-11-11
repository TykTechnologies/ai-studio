#!/bin/sh
# One site's view of both PostgreSQL nodes, once a second each (fault 7).
# A line is a timestamp, this side, the node asked, and either its answer
# (role, timeline, WAL position, synchronous setting, attached standbys,
# commits waiting for the synchronous standby, the replicas in cluster_nodes
# with the seconds since each was seen, the leader leases) or the error.
SQL="SELECT json_build_object(
  'in_recovery', pg_is_in_recovery(),
  'timeline', (SELECT timeline_id FROM pg_control_checkpoint()),
  'lsn', CASE WHEN pg_is_in_recovery() THEN pg_last_wal_replay_lsn()::text ELSE pg_current_wal_lsn()::text END,
  'sync_names', current_setting('synchronous_standby_names'),
  'standbys', (SELECT count(*) FROM pg_stat_replication),
  'waiting_for_sync', (SELECT count(*) FROM pg_stat_activity WHERE wait_event = 'SyncRep'),
  'replicas', (SELECT COALESCE(json_agg(label || ':' || round(EXTRACT(EPOCH FROM now() - last_seen))), '[]'::json) FROM cluster_nodes),
  'leases', (SELECT COALESCE(json_agg(to_jsonb(l)), '[]'::json) FROM cluster_leases l))"

watch() {
	while :; do
		t=$(date -u +%Y-%m-%dT%H:%M:%SZ)
		if out=$(timeout 3 psql -h "$1" -U postgres -d midsommar -X -qAt -c "$SQL" 2>&1); then
			echo "{\"t\":\"$t\",\"side\":\"$SIDE\",\"node\":\"$1\",\"answer\":$out}"
		else
			echo "{\"t\":\"$t\",\"side\":\"$SIDE\",\"node\":\"$1\",\"error\":\"$(echo "$out" | tr -d '"\\' | tr '\n' ' ' | cut -c1-140)\"}"
		fi
		sleep 1
	done
}
watch pg-a &
watch pg-b &
wait
