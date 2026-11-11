#!/bin/sh
# Entrypoint of a PostgreSQL node in the two-site dev stack
# (dev/docker-compose.active-active.yml). The stack has no failover manager:
# the scripts in this directory play that part, and this entrypoint only
# makes a node come up in the role its data directory says it has.
#
#   AA_NODE          this node's name (pg-a, pg-b); also its application_name
#   AA_PEER          the other node's host name
#   AA_INITIAL_ROLE  primary | standby: what an EMPTY data directory becomes
#   AA_REBUILD_FROM  set by "aa.sh rejoin" on a one-off run: wipe the data
#                    directory, copy it from that host and exit
#
# A data directory that already exists is started as it is: a standby if it
# holds standby.signal, a primary otherwise. Roles therefore survive
# "make dev-aa-down" and a restart of the stack.
#
# Who may connect (pg_hba.conf, rewritten at every start by write_hba):
# containers on the stack's own network without a password, which is how the
# replicas, the other node and aa.sh reach it; a connection through the
# published host port (bound to 127.0.0.1 in the compose file) needs the
# password; everything else is refused.
set -eu

: "${AA_NODE:?}" "${AA_PEER:?}" "${AA_INITIAL_ROLE:?}"
PGUSER=${POSTGRES_USER:-postgres}
PGDB=${POSTGRES_DB:-midsommar}
PGPASS=${POSTGRES_PASSWORD:-postgres}
export PGUSER

log() { echo "[$AA_NODE] $*"; }

# write_hba replaces this stack's lines in pg_hba.conf. It runs at every
# start, so a volume created by an earlier version of this script (which
# trusted every address) is corrected, and a copy taken from the other node
# gets this node's own lines.
write_hba() {
	# A connection through the published port arrives from the network's
	# gateway address, which is inside "samenet": it is matched first and
	# must authenticate.
	gateway=$(ip route | awk '/^default/ { print $3; exit }')
	[ -n "$gateway" ] || { log "cannot find the network gateway; refusing to write a pg_hba.conf that trusts it"; exit 1; }
	sed -i -e '/^# aa-begin/,/^# aa-end/d' \
		-e '/^host all all all trust$/d' -e '/^host replication all all trust$/d' "$PGDATA/pg_hba.conf"
	cat >>"$PGDATA/pg_hba.conf" <<CONF
# aa-begin (rewritten by pg-node.sh at every start; do not edit)
host all all $gateway/32 scram-sha-256
host replication all $gateway/32 reject
host all all samenet trust
host replication all samenet trust
# aa-end
CONF
}

base_backup() { # $1: host to copy from
	log "waiting for $1 to accept connections as a primary"
	until [ "$(psql -h "$1" -d postgres -qAtc 'SELECT NOT pg_is_in_recovery()' 2>/dev/null)" = t ]; do
		sleep 1
	done
	log "copying the data directory from $1"
	find "$PGDATA" -mindepth 1 -delete
	pg_basebackup -h "$1" -D "$PGDATA" -X stream -c fast
	chmod 700 "$PGDATA"
	touch "$PGDATA/standby.signal"
	# aa.conf belongs to the node, not to the data that was copied.
	cat >"$PGDATA/aa.conf" <<CONF
primary_conninfo = 'host=$1 port=5432 user=$PGUSER application_name=$AA_NODE'
CONF
}

if [ -n "${AA_REBUILD_FROM:-}" ]; then
	base_backup "$AA_REBUILD_FROM"
	write_hba
	log "rebuilt as a standby of $AA_REBUILD_FROM"
	exit 0
fi

if [ ! -s "$PGDATA/PG_VERSION" ]; then
	if [ "$AA_INITIAL_ROLE" = primary ]; then
		log "initialising as the primary"
		printf '%s\n' "$PGPASS" >/tmp/pwfile
		initdb -D "$PGDATA" -U "$PGUSER" --auth=trust --pwfile=/tmp/pwfile -E UTF8 >/dev/null
		cat >>"$PGDATA/postgresql.conf" <<CONF
listen_addresses = '*'
max_connections = 200
wal_level = replica
hot_standby = on
max_wal_senders = 10
wal_keep_size = '512MB'
include_if_exists = 'aa.conf'
CONF
		# Synchronous replication, as features/ActiveActive.md
		# recommends. "aa.sh stop-site" and "aa.sh promote" clear it
		# when the standby goes away, as a failover manager would.
		echo "synchronous_standby_names = '*'" >"$PGDATA/aa.conf"
		pg_ctl -D "$PGDATA" -w -o "-c listen_addresses='' -c synchronous_standby_names=''" start >/dev/null
		createdb "$PGDB"
		pg_ctl -D "$PGDATA" -w -m fast stop >/dev/null
	else
		base_backup "$AA_PEER"
	fi
fi

write_hba
if [ -f "$PGDATA/standby.signal" ]; then
	log "starting as a hot standby"
else
	log "starting as the primary"
fi
exec postgres -D "$PGDATA"
