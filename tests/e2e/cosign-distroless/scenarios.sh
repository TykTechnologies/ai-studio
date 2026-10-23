#!/usr/bin/env bash
# Scenario helpers for the c1g.15 e2e (sourced after lib.sh).

# studio_oci NAME REF: create an OCI plugin in Studio from a reference
# (Studio pulls and verifies it); prints the HTTP status and the reply.
studio_oci() {
  api POST /api/v1/plugins/oci "{\"name\":\"$1\",\"oci_reference\":\"$2\",\"hook_type\":\"post_auth\",\"is_active\":true}" | cut -c1-1500
}

# create_llm: an LLM the edge receives in its snapshot; prints its ID.
create_llm() {
  api POST /api/v1/llms '{"data":{"type":"LLM","attributes":{"name":"c1g-e2e-llm","vendor":"openai","api_key":"sk-e2e-unused","api_endpoint":"https://api.openai.invalid/v1","privacy_score":50,"active":true,"default_model":"gpt-4o-mini","namespace":""}}}' \
    | sed 's/^[0-9]* //' | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])'
}

# create_plugin NAME COMMAND: a plugin row (POST /api/v1/plugins); prints
# "status id" (id empty on refusal) and the reply on stderr.
create_plugin() {
  local out; out=$(api POST /api/v1/plugins "{\"name\":\"$1\",\"command\":\"$2\",\"hook_type\":\"post_auth\",\"is_active\":true}")
  echo "${out:0:400}" >&2
  local status=${out%% *}
  local id; id=$(sed 's/^[0-9]* //' <<<"$out" | python3 -I -c 'import json,sys
try: print(json.load(sys.stdin)["data"]["id"])
except Exception: print("")')
  echo "$status $id"
}

# attach LLM_ID PLUGIN_ID...: set the LLM's plugins.
attach() {
  local llm=$1; shift
  local ids; ids=$(IFS=,; echo "$*")
  api PUT "/api/v1/llms/$llm/plugins" "{\"plugin_ids\":[${ids}]}" | cut -c1-200
}

push_edges() { api POST /api/v1/edges/reload-all | cut -c1-200; }

# wait_gateway_log PATTERN [SECONDS]: wait until the gateway log matches.
wait_gateway_log() {
  local pattern=$1 secs=${2:-90}
  for _ in $(seq 1 "$secs"); do
    gateway_logs | sed 's/\x1b\[[0-9;]*m//g' | grep -qE "$pattern" && return 0
    sleep 1
  done
  return 1
}
