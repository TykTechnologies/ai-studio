#!/bin/sh
# Entrypoint of a PostgreSQL node of the two-site stack on Kubernetes
# (dev/aa/k8s). The same idea as dev/aa/pg-node.sh: nothing fails over by
# itself, aa-k8s.sh plays the failover manager. Differences:
#
#   - Which node is the primary is written in the ConfigMap aa-pg-roles
#     (mounted at /aa-roles/primary, updated in place by Kubernetes).
#     aa-k8s.sh changes it when it promotes. A node that is not the named
#     primary comes up as its standby, rebuilding its data directory from it
#     if it was a primary before: that is how a lost site rejoins.
#   - No address is trusted. Pods have VPC addresses, so "the pod network" is
#     the VPC: every TCP connection, replication included, needs the password
#     from the Secret aa-shared. Only the local socket (kubectl exec) does not.
set -eu

: "${AA_NODE:?}" "${AA_PEER:?}" "${POSTGRES_PASSWORD:?}"
PGUSER=${POSTGRES_USER:-postgres}
PGDB=${POSTGRES_DB:-midsommar}
export PGUSER
PRIMARY=$(cat /aa-roles/primary)

log() { echo "[$AA_NODE] $*"; }

write_hba() {
	cat >"$PGDATA/pg_hba.conf" <<CONF
# Written by pg-node-k8s.sh at every start; do not edit.
local all all trust
host all all all scram-sha-256
host replication all all scram-sha-256
CONF
}

base_backup() { # $1: host to copy from
	log "waiting for $1 to accept connections as a primary"
	until [ "$(PGPASSWORD=$POSTGRES_PASSWORD psql -h "$1" -d postgres -qAtc 'SELECT NOT pg_is_in_recovery()' 2>/dev/null)" = t ]; do
		sleep 2
	done
	log "copying the data directory from $1"
	mkdir -p "$PGDATA"
	find "$PGDATA" -mindepth 1 -delete
	PGPASSWORD=$POSTGRES_PASSWORD pg_basebackup -h "$1" -D "$PGDATA" -X stream -c fast
	chmod 700 "$PGDATA"
	touch "$PGDATA/standby.signal"
	cat >"$PGDATA/aa.conf" <<CONF
primary_conninfo = 'host=$1 port=5432 user=$PGUSER password=$POSTGRES_PASSWORD application_name=$AA_NODE'
CONF
}

if [ "$AA_NODE" = "$PRIMARY" ]; then
	if [ ! -s "$PGDATA/PG_VERSION" ]; then
		# Never initialise next to a peer that already has data.
		if PGPASSWORD=$POSTGRES_PASSWORD psql -h "$AA_PEER" -d postgres -qAtc 'SELECT 1' >/dev/null 2>&1; then
			log "named primary with an empty data directory, but $AA_PEER is up: refusing to initialise"
			exit 1
		fi
		log "initialising as the primary"
		mkdir -p "$PGDATA"
		printf '%s\n' "$POSTGRES_PASSWORD" >/tmp/pwfile
		initdb -D "$PGDATA" -U "$PGUSER" --auth-local=trust --auth-host=scram-sha-256 --pwfile=/tmp/pwfile -E UTF8 >/dev/null
		cat >>"$PGDATA/postgresql.conf" <<CONF
listen_addresses = '*'
max_connections = 300
wal_level = replica
hot_standby = on
max_wal_senders = 10
wal_keep_size = '2GB'
# Sized for the analytics ingest of a load run (6 GiB container).
shared_buffers = '1500MB'
max_wal_size = '4GB'
include_if_exists = 'aa.conf'
CONF
		echo "synchronous_standby_names = '*'" >"$PGDATA/aa.conf"
		pg_ctl -D "$PGDATA" -w -o "-c listen_addresses='' -c synchronous_standby_names=''" start >/dev/null
		createdb "$PGDB"
		pg_ctl -D "$PGDATA" -w -m fast stop >/dev/null
	fi
elif [ ! -s "$PGDATA/PG_VERSION" ] || [ ! -f "$PGDATA/standby.signal" ]; then
	# Empty, or a former primary: become a standby of the named primary.
	base_backup "$PRIMARY"
fi

write_hba
if [ -f "$PGDATA/standby.signal" ]; then
	log "starting as a hot standby (primary: $PRIMARY)"
else
	log "starting as the primary"
fi
exec postgres -D "$PGDATA"
