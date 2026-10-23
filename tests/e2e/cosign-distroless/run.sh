#!/usr/bin/env bash
# c1g.15 end-to-end scenarios on the distroless images built by build.sh.
#
# Needs: the c1g-e2e-registry container (registry:2 on localhost:15000) with
# request_enricher published by tools/publish-plugin.sh under the tags
# 1.0.0-v2 (cosign v2 .sig), 1.0.0-v3 (cosign v3 bundle), 1.0.0-plain
# (unsigned), 1.0.0-wrongkey and 1.0.0-platform (only the linux/amd64
# manifest signed), all signed with --tlog-upload=false by the key in
# $KEYS_DIR/e2e-good.key (e2e-wrong.key for 1.0.0-wrongkey); TYK_AI_LICENSE
# for the Enterprise images; network access to docker.tyk.io and GitHub for
# the marketplace scenario (MARKETPLACE=0 skips it).
#
# Prints PASS/FAIL per check and exits non-zero if any check failed.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"
source "$HERE/scenarios.sh"
set +euo pipefail # checks below handle failures themselves
export KEYS_DIR="${KEYS_DIR:?set KEYS_DIR to the directory with e2e-good.pub}"
FAILS=0
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; FAILS=$((FAILS + 1)); }
expect() { # <description> <command...>: PASS when the command succeeds
  local what=$1; shift
  if "$@" >/dev/null 2>&1; then pass "$what"; else fail "$what"; fi
}
glog() { gateway_logs | sed 's/\x1b\[[0-9;]*m//g'; }
slog() { studio_logs | sed 's/\x1b\[[0-9;]*m//g'; }
in_gateway() { glog | grep -qE "$1"; }
in_studio() { slog | grep -qE "$1"; }
status_of() { cut -d' ' -f1; }

start() { # env assignments for compose, then start and wait for the edge
  down
  up
  wait_studio || return 1
  for _ in $(seq 1 60); do in_gateway 'Initial configuration synced' && break; sleep 2; done
  login
}

# push_to_edge NAME=COMMAND...: plugin rows on one LLM, pushed to the edge.
push_to_edge() {
  local ids=() pair r
  for pair in "$@"; do
    r=$(create_plugin "${pair%%=*}" "${pair#*=}" 2>/dev/null)
    ids+=("${r#* }")
  done
  attach "$(create_llm)" "${ids[@]}" >/dev/null
  push_edges >/dev/null
  sleep 15
}

V2=$(digest 1.0.0-v2) V3=$(digest 1.0.0-v3) PLAIN=$(digest 1.0.0-plain) WRONG=$(digest 1.0.0-wrongkey)
studio_ref() { echo "oci://$REPO@$1?pubkey=E2E&arch=linux/amd64"; }

log "1. images: no cosign, no shell"
expect "ENT images contain no cosign binary and no shell" bash "$HERE/check_images.sh"

log "3-6. signatures, tlog optional (the signatures have no tlog entry)"
export EDITION=ent STUDIO_TLOG_POLICY=optional GATEWAY_TLOG_POLICY=optional STUDIO_ALLOWED_SOURCES= GATEWAY_ALLOWED_SOURCES= MARKETPLACE_ENABLED=false
start
[ "$(studio_oci s-v2 "$(studio_ref "$V2")" | status_of)" = 201 ] && pass "Studio: cosign v2 signed, by digest" || fail "Studio: cosign v2 signed, by digest"
[ "$(studio_oci s-v2tag "oci://$REPO:1.0.0-v2?pubkey=E2E&arch=linux/amd64" | status_of)" = 201 ] && pass "Studio: cosign v2 signed, by tag on a multi-arch index" || fail "Studio: v2 by tag"
[ "$(studio_oci s-v3 "$(studio_ref "$V3")" | status_of)" = 201 ] && pass "Studio: cosign v3 bundle" || fail "Studio: cosign v3 bundle"
[ "$(studio_oci s-plat "oci://$REPO:1.0.0-platform?pubkey=E2E&arch=linux/amd64" | status_of)" = 201 ] && pass "Studio: platform-manifest signature (fallback)" || fail "Studio: platform fallback"
r=$(studio_oci s-plain "$(studio_ref "$PLAIN")"); [[ $r != 201* && $r == *"no signatures found"* ]] && pass "Studio: unsigned rejected (no signatures found)" || fail "Studio: unsigned rejected: ${r:0:200}"
r=$(studio_oci s-wrong "$(studio_ref "$WRONG")"); [[ $r != 201* && $r == *"does not verify with the key"* ]] && pass "Studio: wrong key rejected" || fail "Studio: wrong key: ${r:0:200}"
push_to_edge "e-v2=oci://$REPO@$V2?pubkey=E2E" "e-v2tag=oci://$REPO:1.0.0-v2?pubkey=E2E" "e-v3=oci://$REPO@$V3?pubkey=E2E" \
  "e-plat=oci://$REPO:1.0.0-platform?pubkey=E2E" "e-plain=oci://$REPO@$PLAIN?pubkey=E2E" "e-wrong=oci://$REPO@$WRONG?pubkey=E2E"
for p in e-v2 e-v2tag e-v3 e-plat; do
  expect "edge: $p verified and loaded" in_gateway "Plugin pre-warmed successfully .*plugin_name=$p\$"
done
expect "edge: e-plat verified on the platform manifest" in_gateway 'signature check passed on the platform manifest'
expect "edge: unsigned rejected with a clear reason" in_gateway "Failed to pre-warm plugin .*no signatures found.*plugin_name=e-plain"
expect "edge: wrong key rejected with a clear reason" in_gateway "Failed to pre-warm plugin .*signature does not verify with the key.*plugin_name=e-wrong"

log "7. tlog policy required: the same signatures (no tlog entry) are refused"
export STUDIO_TLOG_POLICY=required GATEWAY_TLOG_POLICY=required MARKETPLACE_ENABLED=true
start
r=$(studio_oci s-v2 "$(studio_ref "$V2")"); [[ $r == *"no transparency-log entry"* ]] && pass "Studio: no tlog entry refused under required" || fail "Studio required: ${r:0:200}"
push_to_edge "e-v2=oci://$REPO@$V2?pubkey=E2E" "e-v3=oci://$REPO@$V3?pubkey=E2E"
expect "edge: no tlog entry refused under required" in_gateway "Failed to pre-warm plugin .*no transparency-log entry.*plugin_name=e-v2"

if [ "${MARKETPLACE:-1}" = 1 ]; then
  log "8. marketplace plugin (published, Tyk key, real Rekor entry), tlog required"
  REF="oci://docker.tyk.io/studio-plugins/asset-catalog@sha256:bd2e24de558d1be9778c7aea5eb0d809e22c9f7b5324259c0c689823d4d1f6e3"
  out=$(api POST /api/v1/plugins/oci "{\"name\":\"asset-catalog\",\"oci_reference\":\"$REF\",\"hook_type\":\"resource_provider\",\"is_active\":true}")
  [ "${out%% *}" = 201 ] && pass "Studio: published marketplace plugin verified (embedded key, tlog required)" || fail "marketplace plugin: ${out:0:200}"
  id=$(sed 's/^[0-9]* //' <<<"$out" | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])' 2>/dev/null)
  [ "$(api POST "/api/v1/plugins/$id/validate-and-load" '{}' | status_of)" = 200 ] && pass "Studio: marketplace plugin loads (plugin manager)" || fail "validate-and-load"
  [ "$(api POST "/api/v1/plugins/$id/manifest/parse" '{}' | status_of)" = 200 ] && pass "Studio: manifest parsed (manifest service client)" || fail "manifest/parse"
fi

log "9a. gateway PLUGINS_ALLOWED_SOURCES=oci; Studio allows all"
export STUDIO_TLOG_POLICY=optional GATEWAY_TLOG_POLICY=optional GATEWAY_ALLOWED_SOURCES=oci MARKETPLACE_ENABLED=false
start
push_to_edge "e-file=/opt/plugins/request_enricher" "e-grpc=grpc://plugin-host:50051" "e-oci=oci://$REPO@$V2?pubkey=E2E"
expect "Studio accepted the file plugin (its own policy allows all)" in_studio '\| 201 \|.*POST +"/api/v1/plugins"'
expect "edge refuses the hub-pushed file plugin" in_gateway 'not allowed by PLUGINS_ALLOWED_SOURCES plugin_id=[0-9]+ plugin_name=e-file source=file'
expect "edge refuses the hub-pushed grpc plugin" in_gateway 'not allowed by PLUGINS_ALLOWED_SOURCES plugin_id=[0-9]+ plugin_name=e-grpc source=grpc'
expect "edge loads the OCI plugin" in_gateway 'Plugin pre-warmed successfully .*plugin_name=e-oci$'

log "9b. Studio AI_STUDIO_PLUGIN_ALLOWED_SOURCES=oci"
export GATEWAY_ALLOWED_SOURCES= STUDIO_ALLOWED_SOURCES=oci
start
pub=$(curl -s "$STUDIO/auth/config")
[[ $pub != *plugin_allowed_sources* && $pub != *oci_require_signature* ]] && pass "public /auth/config carries no plugin policy" || fail "public /auth/config leaks the policy"
[[ $(api GET /api/v1/plugin-policy) == 200*'"oci"'* ]] && pass "authenticated plugin policy lists oci only" || fail "plugin-policy"
r=$(api POST /api/v1/plugins '{"name":"f","command":"/opt/plugins/x","hook_type":"post_auth","is_active":true}')
[[ $r == 400*AI_STUDIO_PLUGIN_ALLOWED_SOURCES* ]] && pass "API: file path refused with 400" || fail "API file: ${r:0:200}"
if [ -n "${NODE_PATH:-}" ]; then
  expect "UI: wizard offers only oci:// and refuses a file path" node "$HERE/ui_lockdown.js" "${UI_SHOT:-}"
fi

log "9c. invalid policy values stop startup"
down
export STUDIO_ALLOWED_SOURCES=ocii
up studio; sleep 8
expect "Studio refuses AI_STUDIO_PLUGIN_ALLOWED_SOURCES=ocii" in_studio 'invalid AI_STUDIO_PLUGIN_ALLOWED_SOURCES: unknown plugin source \\"ocii\\"'
down
export STUDIO_ALLOWED_SOURCES=
out=$(docker run --rm -e GATEWAY_MODE=standalone -e ENCRYPTION_KEY=c1ge2eencryptionkey0123456789abc -e PLUGINS_ALLOWED_SOURCES=ocii "c1g-e2e/microgateway:$EDITION" 2>&1)
[[ $? -ne 0 && $out == *'invalid PLUGINS_ALLOWED_SOURCES'* ]] && pass "gateway refuses PLUGINS_ALLOWED_SOURCES=ocii" || fail "gateway ocii"

log "9d. a hub-pushed grpc:// plugin on an internal address is refused by the edge (c1g.31)"
export GATEWAY_ALLOW_INTERNAL=false
start
push_to_edge "e-grpc-internal=grpc://10.0.0.5:50051"
expect "edge refuses grpc://10.0.0.5 pushed by the hub" in_gateway 'Failed to pre-warm plugin .*(internal|blocked).*plugin_name=e-grpc-internal'
export GATEWAY_ALLOW_INTERNAL=

log "10. Community Edition: loads unverified, warns once"
export EDITION=ce STUDIO_TLOG_POLICY=required GATEWAY_TLOG_POLICY=required
start
[ "$(studio_oci c-plain "$(studio_ref "$PLAIN")" | status_of)" = 201 ] && pass "CE Studio loads an unsigned plugin (unverified)" || fail "CE Studio unsigned"
push_to_edge "c-plain=oci://$REPO@$PLAIN?pubkey=E2E"
expect "CE edge loads an unsigned plugin" in_gateway 'Plugin pre-warmed successfully .*plugin_name=c-plain$'
[ "$(slog | grep -c 'SECURITY WARNING: OCI RequireSignature is enabled')" = 1 ] && pass "CE Studio warns once" || fail "CE Studio warning count"
[ "$(glog | grep -c 'SECURITY WARNING: OCI RequireSignature is enabled')" = 1 ] && pass "CE gateway warns once" || fail "CE gateway warning count"
expect "CE records the plugin as NOT verified" in_gateway 'signature NOT verified: signature verification is not available in this edition'

down
echo
[ "$FAILS" = 0 ] && echo "ALL CHECKS PASSED" || echo "$FAILS CHECK(S) FAILED"
exit "$FAILS"
