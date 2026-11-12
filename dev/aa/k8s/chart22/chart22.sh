#!/usr/bin/env bash
# Bead .22 chart check on aa-epic-23: two Studio replicas from helm/ (the
# Studio chart) on a PostgreSQL of their own, in namespace aa-chart22.
#
#   chart22.sh apply      namespace, Postgres, Secrets (random), render and apply the chart
#   chart22.sh render     render only (no cluster); OFFLINE=1 chart22.sh check checks it
#   chart22.sh check      rendered-object checks, pods, both replicas in /api/v1/cluster/status
#   chart22.sh status     a probe administrator, then GET /api/v1/cluster/status (two live replicas, label site-a, one leader)
#   chart22.sh teardown   delete the namespace and rendered/
#
# Needs STUDIO_IMAGE and MICROGATEWAY_IMAGE (repository:tag). The chart's
# Secret carries the generated values, so rendered/ is gitignored and removed
# by teardown.
set -euo pipefail
CTX=${AA_K8S_CONTEXT:-aa-epic-23}
NS=aa-chart22
HERE=$(cd "$(dirname "$0")" && pwd)
CHART=$(cd "$HERE/../../../../helm" && pwd)
k() { kubectl --context "$CTX" -n "$NS" "$@"; }
rand() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

cmd_apply() {
	: "${STUDIO_IMAGE:?}" "${MICROGATEWAY_IMAGE:?}"
	kubectl --context "$CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$CTX" apply -f -
	if ! k get secret chart22-pg >/dev/null 2>&1; then
		local pw; pw=$(rand 16)
		k create secret generic chart22-pg --from-literal=POSTGRES_PASSWORD="$pw" >/dev/null
		k create secret generic studio-database \
			--from-literal=DATABASE_URL="postgres://postgres:$pw@pg:5432/studio?sslmode=disable&connect_timeout=3" >/dev/null
	fi
	k apply -f "$HERE/00-postgres.yaml"
	k rollout status statefulset pg --timeout=180s
	cmd_render
	k apply -f "$HERE/rendered/studio.yaml"
}

cmd_render() {
	: "${STUDIO_IMAGE:?}" "${MICROGATEWAY_IMAGE:?}"
	mkdir -p "$HERE/rendered"
	local grpc enc
	grpc=$(rand 16); enc=$(rand 16)
	sed -e "s|STUDIO_IMAGE_REPO|${STUDIO_IMAGE%:*}|" -e "s|STUDIO_IMAGE_TAG|${STUDIO_IMAGE##*:}|" \
		-e "s|MICROGATEWAY_IMAGE_REPO|${MICROGATEWAY_IMAGE%:*}|" -e "s|MICROGATEWAY_IMAGE_TAG|${MICROGATEWAY_IMAGE##*:}|" \
		-e "s|GENERATED_CSRF_KEY|$(rand 32)|" -e "s|GENERATED_SECRET_KEY|$(rand 16)|" \
		-e "s|GENERATED_GRPC_TOKEN|$grpc|g" -e "s|GENERATED_ENC_KEY|$enc|g" "$HERE/values.yaml" > "$HERE/rendered/values.yaml"
	docker run --rm -v "$CHART:/chart:ro" -v "$HERE/rendered/values.yaml:/values.yaml:ro" alpine/helm:3.16.2 \
		template studio /chart --namespace "$NS" -f /values.yaml > "$HERE/rendered/studio.yaml"
	echo "rendered/studio.yaml: $(grep -c '^kind:' "$HERE/rendered/studio.yaml") objects"
}

cmd_check() {
	local r="$HERE/rendered/studio.yaml"
	python3 - "$r" <<'PY'
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
dep = next(d for d in docs if d["kind"] == "Deployment" and "microgateway" not in d["metadata"]["name"])
spec = dep["spec"]; pod = spec["template"]["spec"]; c = pod["containers"][0]
env = {e["name"]: e for e in c.get("env", [])}
cms = [d for d in docs if d["kind"] == "ConfigMap"]
secrets = [d for d in docs if d["kind"] == "Secret"]
checks = {
  "replicas 2": spec.get("replicas") == 2,
  "RollingUpdate maxUnavailable 0": spec.get("strategy", {}).get("type") == "RollingUpdate" and spec["strategy"].get("rollingUpdate", {}).get("maxUnavailable") in (0, "0"),
  "PodDisruptionBudget": any(d["kind"] == "PodDisruptionBudget" for d in docs),
  "two topologySpreadConstraints": len(pod.get("topologySpreadConstraints", [])) == 2,
  "startupProbe /health 5s x 36": (c.get("startupProbe", {}).get("httpGet", {}).get("path") == "/health" and c["startupProbe"].get("periodSeconds") == 5 and c["startupProbe"].get("failureThreshold") == 36),
  "readinessProbe /ready": c.get("readinessProbe", {}).get("httpGet", {}).get("path") == "/ready",
  "DATABASE_URL from the Secret": env.get("DATABASE_URL", {}).get("valueFrom", {}).get("secretKeyRef", {}).get("name") == "studio-database",
  "DATABASE_URL not in a ConfigMap": not any("DATABASE_URL" in (d.get("data") or {}) for d in cms),
  "CSRF_KEY in the chart Secret": any("CSRF_KEY" in (d.get("data") or {}) or "CSRF_KEY" in (d.get("stringData") or {}) for d in secrets),
  "CLUSTER_NODE_LABEL site-a": any((d.get("data") or {}).get("CLUSTER_NODE_LABEL") == "site-a" for d in cms) or env.get("CLUSTER_NODE_LABEL", {}).get("value") == "site-a",
}
print("rendered Deployment:", dep["metadata"]["name"])
for name, ok in checks.items():
    print(("PASS " if ok else "FAIL ") + name)
PY
	[ "${OFFLINE:-}" = 1 ] || k get pods -o wide
}

# A probe administrator of the check's own (never a person's account), then
# GET /api/v1/cluster/status through a port-forward to the Studio Service.
cmd_status() {
	local pw jar token pf
	pw="Probe-$(rand 8)!x"
	k port-forward svc/studio-midsommar 18280:8080 >/dev/null 2>&1 &
	pf=$!
	trap 'kill $pf 2>/dev/null' RETURN
	sleep 3
	jar=$(mktemp)
	csrf() { curl -s -D - -o /dev/null -b "$jar" -c "$jar" -H "Origin: http://localhost:18280" http://localhost:18280/csrf-token | tr -d '\r' | awk 'tolower($1)=="x-csrf-token:"{print $2}'; }
	token=$(csrf)
	curl -s -o /dev/null -w "register: HTTP %{http_code}\n" -b "$jar" -c "$jar" -H "X-CSRF-Token: $token" -H "Origin: http://localhost:18280" -H "Referer: http://localhost:18280/" \
		-H 'Content-Type: application/json' -d "{\"data\":{\"type\":\"user\",\"attributes\":{\"email\":\"aa-probe-chart22@tyk.io\",\"name\":\"aa-probe-chart22\",\"password\":\"$pw\"}}}" http://localhost:18280/auth/register
	k exec pg-0 -- psql -U postgres -d studio -qAtc "UPDATE users SET email_verified = true, is_admin = true WHERE email = 'aa-probe-chart22@tyk.io'"
	token=$(csrf)
	curl -s -o /dev/null -w "login: HTTP %{http_code}\n" -b "$jar" -c "$jar" -H "X-CSRF-Token: $token" -H "Origin: http://localhost:18280" -H "Referer: http://localhost:18280/" \
		-H 'Content-Type: application/json' -d "{\"data\":{\"type\":\"user\",\"attributes\":{\"email\":\"aa-probe-chart22@tyk.io\",\"password\":\"$pw\"}}}" http://localhost:18280/auth/login
	curl -s -b "$jar" -H "Origin: http://localhost:18280" http://localhost:18280/api/v1/cluster/status | python3 -m json.tool
	rm -f "$jar"
}

case "${1:-}" in
status) cmd_status ;;
apply) cmd_apply ;;
render) cmd_render ;;
check) cmd_check ;;
teardown) kubectl --context "$CTX" delete namespace "$NS" --ignore-not-found; rm -rf "$HERE/rendered" ;;
*) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
