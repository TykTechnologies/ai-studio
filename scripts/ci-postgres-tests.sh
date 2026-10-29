#!/usr/bin/env bash
# Runs the Postgres-gated Go tests for CI (the Go Postgres Tests job).
#
# Every test package runs in a go test of its own, on a database of its
# own, PG_TEST_JOBS at a time. The packages used to share one database and
# so ran one after another (-p 1), which also serialised their builds; a
# database per package removes the reason for that. The enterprise packages
# are compiled once up front, before their tests start, so parallel runs do
# not compile the same packages side by side.
#
# Each package's output is printed whole when it finishes (failures without
# a collapsed group), and every run's go test -json stream is kept in
# $PG_TEST_OUT for the timings summary.
#
#   scripts/ci-postgres-tests.sh
#
# Environment:
#   PG_BASE_URL   server to create databases on
#                 (default postgres://postgres:postgres@localhost:5432)
#   PG_TEST_JOBS  go test runs at a time (default 4)
#   PG_TEST_OUT   directory for logs and JSON (default $RUNNER_TEMP/pg-tests)
#   FORK_PR=true  skip the enterprise suites (forks have only a stub)
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
BASE=${PG_BASE_URL:-postgres://postgres:postgres@localhost:5432}
JOBS=${PG_TEST_JOBS:-4}
OUT=${PG_TEST_OUT:-${RUNNER_TEMP:-/tmp}/pg-tests}
TEE="$ROOT/scripts/gotest-json.py"
mkdir -p "$OUT"

# Suites: label | module dir | build tags | -run regexp | DSN variables | packages
#
# Every run gets DATABASE_URL pointing at its own database. A DSN variable
# listed plainly points at the same database; VAR=own gets a second
# database (the budget tracker test drops and recreates its tables, and the
# edge schema has its own apps/llms tables).
SUITES=$(cat <<'EOF'
# Studio's Postgres-specific tests.
studio|.||PostgreSQL|_Postgres|InTransaction|EnsureInDefaultCatalogue||./models/ ./services/ ./pkg/studio/ ./analytics/
# The pricing regression suite: price changes against recorded history.
pricing|.||TestPricingRegression||./proxy/ ./api/
# The chat queue (LISTEN/NOTIFY), config and startup.
queue-config-startup|.||||./chat_session/ ./config/ ./startup/
# The cluster code runs on every Postgres deployment: the event log and
# node registry, the shared LISTEN connection, durable pushes and the
# leader lease. The push and lease tests run their SQLite and Postgres
# cases side by side.
cluster|.||||./pkg/cluster/ ./pkg/pglisten/ ./services/pushes/ ./services/scheduler/
# Pushes and the event-bus relay across several control-server replicas on
# one database, the budget usage tracker's queries and the pricing
# regression suite's hub budget periods.
grpc|.||TestPushCluster_|TestRelay_|_Postgres|TestPricingRegression|BUDGET_TEST_POSTGRES_DSN=own|./grpc/
# Analytics writer, budget ledger and budget sync against Postgres (ON
# CONFLICT, GREATEST, enforced foreign keys), and the edge schema snapshot.
mgw|microgateway||Postgres|TestPricingRegression|MGW_TEST_POSTGRES_DSN=own|./internal/services/ ./internal/database/
# Real edges against several Studio replicas sharing DATABASE_URL (a schema
# per test): pushes land whatever happens to the connections.
mgw-cluster|microgateway||||./tests/cluster/
# FIPS 140-3 cross-build migration (std -> FIPS -> std, on SQLite and
# Postgres, with fips140=on and fips140=only): each test builds its package
# as a standard and a GOFIPS140=v1.0.0 test binary and runs them in turn on
# one database (a schema per case). -short skips them in the unit jobs.
fips-migration|.||TestCrossBuildMigration||./tests/fipsmigration/
fips-migration-mgw|microgateway||TestCrossBuildMigration|MGW_TEST_POSTGRES_DSN|./internal/fipsmigration/
# Agent Call Tracing: the store and capture staging in core, and the
# enterprise assembler, correlation goldens, JSON v1 and content API
# (purge). Their gates read DATABASE_URL or AGENT_TRACE_TEST_POSTGRES_DSN,
# so both are set. A Postgres-only bug (jyx.126) must not pass by skipping.
agenttrace|.||AgentTrace|Capture|AGENT_TRACE_TEST_POSTGRES_DSN|./models/ ./analytics/
ent-agenttrace|enterprise|enterprise||AGENT_TRACE_TEST_POSTGRES_DSN|./features/agenttracing/...
ent-audit|enterprise|enterprise|||./features/audit/...
ent-webhooks|enterprise|enterprise|_Postgres||./features/webhooks/
# The pricing regression suite's budget side (spend read through the budget
# service after price and period edge cases).
ent-budget|enterprise|enterprise|TestPricingRegression||./features/budget/
EOF
)

# The -run regexps contain |, so a suite line is split on the fields that
# cannot: label, dir and tags first, packages and variables last.
parse() {
	local line=$1
	S_LABEL=${line%%|*}; line=${line#*|}
	S_DIR=${line%%|*}; line=${line#*|}
	S_TAGS=${line%%|*}; line=${line#*|}
	S_PKGS=${line##*|}; line=${line%|*}
	S_VARS=${line##*|}; line=${line%|*}
	S_RUN=$line
}

psql_admin() { psql "$BASE/postgres" -v ON_ERROR_STOP=1 -qAt -c "$1"; }

# --- Expand the suites into one run per package. ---------------------------
labels=() dirs=() tags=() runs=() vars=() pkgs=()
while IFS= read -r line; do
	case $line in "" | "#"*) continue ;; esac
	parse "$line"
	if [ "$S_DIR" = enterprise ] && [ "${FORK_PR:-false}" = true ]; then
		echo "Skipping $S_LABEL: fork pull request (enterprise stub)."
		continue
	fi
	tagflag=()
	[ -z "$S_TAGS" ] || tagflag=(-tags "$S_TAGS")
	# shellcheck disable=SC2086 # S_PKGS is a list of patterns
	list=$(cd "$S_DIR" && go list "${tagflag[@]}" -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' $S_PKGS) || {
		echo "::error::go list failed for suite $S_LABEL"
		exit 1
	}
	for p in $list; do
		labels+=("$S_LABEL ${p##*/}") dirs+=("$S_DIR") tags+=("$S_TAGS") runs+=("$S_RUN") vars+=("$S_VARS") pkgs+=("$p")
	done
done <<<"$SUITES"
n=${#pkgs[@]}
echo "$n test runs, $JOBS at a time"

# --- One database (or two) per run. ----------------------------------------
db_url() { echo "$BASE/$1?sslmode=disable"; }
for ((i = 0; i < n; i++)); do
	psql_admin "DROP DATABASE IF EXISTS pg_t$i" && psql_admin "CREATE DATABASE pg_t$i" || exit 1
	if [[ " ${vars[i]} " == *"=own "* ]]; then
		psql_admin "DROP DATABASE IF EXISTS pg_t${i}_own" && psql_admin "CREATE DATABASE pg_t${i}_own" || exit 1
	fi
done

# --- Run them. ----------------------------------------------------------------
run_one() {
	local i=$1 env=(DATABASE_URL="$(db_url "pg_t$i")") v tagflag=() runflag=()
	for v in ${vars[i]}; do
		case $v in
		*=own) env+=("${v%=own}=$(db_url "pg_t${i}_own")") ;;
		*) env+=("$v=$(db_url "pg_t$i")") ;;
		esac
	done
	[ -z "${tags[i]}" ] || tagflag=(-tags "${tags[i]}")
	[ -z "${runs[i]}" ] || runflag=(-run "${runs[i]}")
	cd "${dirs[i]}" &&
		env "${env[@]}" go test "${tagflag[@]}" -json -v -race -count=1 "${runflag[@]}" "${pkgs[i]}" 2>&1 |
		"$TEE" tee "$OUT/run$i.json" >"$OUT/run$i.log"
}

# The enterprise packages are not in the CE build cache this job restores:
# compile and link them once (-exec true runs nothing) before their tests.
ent_pkgs=()
for ((i = 0; i < n; i++)); do
	[ "${dirs[i]}" = enterprise ] && ent_pkgs+=("${pkgs[i]}")
done
build_pid=
if [ ${#ent_pkgs[@]} -gt 0 ]; then
	(cd enterprise && go test -tags enterprise -race -count=1 -exec /bin/true "${ent_pkgs[@]}" >"$OUT/ent-build.log" 2>&1) &
	build_pid=$!
fi
ent_ready=$([ -z "$build_pid" ] && echo true || echo false)

# The slowest packages start first, so the job does not end on one of them
# running alone (CI timings, longest first).
SLOW=(
	microgateway/tests/cluster
	ai-studio-enterprise/v2/features/agenttracing
	midsommar/v2/pkg/studio
	ai-studio-enterprise/v2/features/agenttracing/correlate
	midsommar/v2/services
	midsommar/v2/pkg/cluster
	ai-studio-enterprise/v2/features/agenttracing/assembler
	midsommar/v2/grpc
)
declare -A running=() started=() queued=()
failed=() queue=()
for s in "${SLOW[@]}"; do
	for ((i = 0; i < n; i++)); do
		if [[ ${pkgs[i]} == *"$s" ]] && [ -z "${queued[$i]+x}" ]; then
			queue+=("$i") queued[$i]=1
		fi
	done
done
for ((i = 0; i < n; i++)); do
	[ -n "${queued[$i]+x}" ] || queue+=("$i")
done

start_next() {
	local k i
	for k in "${!queue[@]}"; do
		i=${queue[k]}
		if [ "${dirs[i]}" = enterprise ] && [ "$ent_ready" != true ]; then
			continue
		fi
		unset 'queue[k]'
		run_one "$i" &
		running[$!]=$i
		started[$i]=$SECONDS
		return 0
	done
	return 1
}

finish() {
	local i=$1 status=$2 secs=$((SECONDS - started[$1]))
	local title="${labels[i]} (${pkgs[i]}, ${secs}s)"
	if [ "$status" -eq 0 ]; then
		echo "::group::ok   $title"
		cat "$OUT/run$i.log"
		echo "::endgroup::"
	else
		failed+=("$i")
		echo "::error::FAIL $title"
		cat "$OUT/run$i.log"
	fi
}

while :; do
	while [ ${#running[@]} -lt "$JOBS" ] && start_next; do :; done
	if [ ${#running[@]} -eq 0 ] && [ -z "$build_pid" ]; then
		break
	fi
	pid=
	wait -n -p pid
	status=$?
	if [ -n "$build_pid" ] && [ "$pid" = "$build_pid" ]; then
		echo "::group::Enterprise test build (exit $status)"
		cat "$OUT/ent-build.log"
		echo "::endgroup::"
		build_pid= ent_ready=true
		continue
	fi
	[ -n "$pid" ] && [ -n "${running[$pid]+x}" ] || continue
	i=${running[$pid]}
	unset "running[$pid]"
	finish "$i" "$status"
done

if [ ${#failed[@]} -gt 0 ]; then
	echo
	echo "Failed:"
	for i in "${failed[@]}"; do echo "  ${labels[i]} (${pkgs[i]})"; done
	exit 1
fi
echo "All $n Postgres test runs passed."
