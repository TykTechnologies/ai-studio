#!/usr/bin/env bash
# Runtime smoke test of FIPS 140-3 builds: starts a Studio hub and an edge
# with TLS on every listener, plus the mock LLM vendor, and runs
# tests/fipssmoke (FIPS self-report on /health, TLS posture of each
# listener, hub->edge sync over TLS, one proxied LLM call).
#
# Usage:
#   tests/fipssmoke/run.sh image STUDIO_IMAGE GATEWAY_IMAGE
#       the -fips container images (tykio/tyk-ai-studio-fips,
#       tykio/tyk-microgateway-fips, or a local build of their Dockerfiles)
#   tests/fipssmoke/run.sh deb DEB_SMOKE_IMAGE
#       an image of tests/packaging/Dockerfile.deb-smoke built with
#       --build-arg PACKAGES=fips: the -fips .deb packages installed on
#       Debian, both services run by supervisord
#
# FIPS_SMOKE_EXPECT=std runs the same checks against standard builds: the
# /health checks then expect FIPS mode off, and the posture checks only FIPS
# mode makes (ChaCha20, X25519) must fail. That is how the checks are shown
# to catch a regression; it is not a CI mode.
#
# Needs docker (host networking, so Linux), openssl, curl and go (the test
# and the mock vendor are built from this tree). The ports used on
# 127.0.0.1 are 18080 (Studio HTTPS), 15051 (control gRPC), 18081 (edge
# HTTPS), 18989/19090 (Studio docs and proxy ports, unused) and 19999 (mock
# vendor). TYK_AI_LICENSE is passed through for Enterprise builds.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"

mode=${1:-}
case "$mode" in
  image) [ $# -eq 3 ] || { echo "usage: $0 image STUDIO_IMAGE GATEWAY_IMAGE" >&2; exit 2; } ;;
  deb) [ $# -eq 2 ] || { echo "usage: $0 deb DEB_SMOKE_IMAGE" >&2; exit 2; } ;;
  *) echo "usage: $0 image STUDIO_IMAGE GATEWAY_IMAGE | $0 deb DEB_SMOKE_IMAGE" >&2; exit 2 ;;
esac

work=$(mktemp -d)
run_id=fips-smoke-$$
containers=()
mock_pid=
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    for c in "${containers[@]}"; do
      echo "--- logs of $c (last 80 lines)" >&2
      docker logs "$c" 2>&1 | tail -n 80 >&2 || true
      if [ "$mode" = deb ]; then
        docker exec "$c" sh -c 'tail -n 80 /var/log/tyk-*.log' >&2 || true
      fi
    done
  fi
  for c in "${containers[@]}"; do docker rm -f "$c" >/dev/null 2>&1 || true; done
  [ -z "$mock_pid" ] || kill "$mock_pid" 2>/dev/null || true
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT

# A CA and a P-256 server certificate for localhost, used by every listener.
mkdir -p "$work/tls"
(
  cd "$work/tls"
  openssl ecparam -name prime256v1 -genkey -noout -out ca.key
  openssl req -x509 -new -key ca.key -sha256 -days 2 -subj "/CN=fips-smoke-ca" -out ca.crt
  openssl ecparam -name prime256v1 -genkey -noout -out server.key
  openssl req -new -key server.key -subj "/CN=localhost" -out server.csr
  printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' > ext.cnf
  openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 2 -sha256 \
    -extfile ext.cnf -out server.crt 2>/dev/null
  chmod 644 ./*.key # read by the services' non-root users
)

# The mock vendor, on the host (the containers share the host network).
go build -o "$work/mockllm" ./benchmarks/gateway/cmd/mockllm
"$work/mockllm" -addr 127.0.0.1:19999 -profile tokens=8 > "$work/mockllm.log" 2>&1 &
mock_pid=$!

# Settings shared by both modes, as KEY=value lines (docker --env-file, and
# /etc/default files the deb image's supervisord sources): no quotes, no
# shell metacharacters.
grpc_token=fips-smoke-grpc-token-0123456789abcdef
edge_key=fipssmokeencryptabcdef0123456789
write_env() { # write_env <data dir> <tls dir>
  local data=$1 tlsdir=$2
  cat > "$work/studio.env" <<EOF
SERVER_PORT=18080
DOCS_PORT=18989
PROXY_PORT=19090
SITE_URL=https://localhost:18080
CERT_FILE=$tlsdir/server.crt
KEY_FILE=$tlsdir/server.key
DATABASE_TYPE=sqlite
DATABASE_URL=$data/studio.db
TYK_AI_SECRET_KEY=fips-smoke-secret-key-0123456789abcdef
ALLOW_REGISTRATIONS=true
ALLOW_INTERNAL_NETWORK_ACCESS=true
GATEWAY_MODE=control
GRPC_PORT=15051
GRPC_HOST=127.0.0.1
GRPC_TLS_CERT_PATH=$tlsdir/server.crt
GRPC_TLS_KEY_PATH=$tlsdir/server.key
GRPC_AUTH_TOKEN=$grpc_token
MICROGATEWAY_ENCRYPTION_KEY=$edge_key
EOF
  cat > "$work/gateway.env" <<EOF
PORT=18081
HOST=127.0.0.1
TLS_ENABLED=true
TLS_CERT_PATH=$tlsdir/server.crt
TLS_KEY_PATH=$tlsdir/server.key
DATABASE_TYPE=sqlite
DATABASE_DSN=$data/microgateway.db
DB_AUTO_MIGRATE=true
ENCRYPTION_KEY=$edge_key
PLUGINS_CONFIG_PATH=
GATEWAY_MODE=edge
CONTROL_ENDPOINT=127.0.0.1:15051
EDGE_ID=fips-smoke-edge
EDGE_NAMESPACE=default
EDGE_AUTH_TOKEN=$grpc_token
EDGE_TLS_ENABLED=true
EDGE_TLS_CA_PATH=$tlsdir/ca.crt
EDGE_HEARTBEAT_INTERVAL=2s
EDGE_RECONNECT_INTERVAL=2s
ALLOW_INTERNAL_NETWORK_ACCESS=true
EOF
}

wait_health() { # wait_health <url>
  for _ in $(seq 1 60); do
    if curl -sf --cacert "$work/tls/ca.crt" "$1/health" >/dev/null; then return 0; fi
    sleep 2
  done
  echo "fips-smoke: $1/health never answered" >&2
  return 1
}

license=()
[ -z "${TYK_AI_LICENSE:-}" ] || license=(-e TYK_AI_LICENSE)

case "$mode" in
  image)
    write_env /data /tls
    containers=("$run_id-studio" "$run_id-gateway")
    docker run -d --name "$run_id-studio" --network host --tmpfs /data:uid=65532 \
      -v "$work/tls:/tls:ro" --env-file "$work/studio.env" "${license[@]}" "$2" >/dev/null
    # An edge that cannot reach its hub at start-up exits: start it second.
    wait_health https://127.0.0.1:18080
    docker run -d --name "$run_id-gateway" --network host --tmpfs /data:uid=65532 \
      -v "$work/tls:/tls:ro" --env-file "$work/gateway.env" "${license[@]}" "$3" >/dev/null
    ;;
  deb)
    # Both services run as tyk; the image gives it /opt/tyk-ai-studio/data.
    write_env /opt/tyk-ai-studio/data /etc/fips-smoke-tls
    containers=("$run_id-deb")
    docker run -d --name "$run_id-deb" --network host \
      -v "$work/tls:/etc/fips-smoke-tls:ro" \
      -v "$work/studio.env:/etc/default/tyk-ai-studio:ro" \
      -v "$work/gateway.env:/etc/default/tyk-microgateway:ro" \
      -e TYK_AI_LICENSE="${TYK_AI_LICENSE:-}" "$2" >/dev/null # supervisord needs it set
    ;;
esac

wait_health https://127.0.0.1:18080
wait_health https://127.0.0.1:18081

FIPS_SMOKE_STUDIO_URL=https://127.0.0.1:18080 \
FIPS_SMOKE_GATEWAY_URL=https://127.0.0.1:18081 \
FIPS_SMOKE_TLS_TARGETS=studio-https=127.0.0.1:18080,control-grpc=127.0.0.1:15051,gateway-https=127.0.0.1:18081 \
FIPS_SMOKE_CA="$work/tls/ca.crt" \
FIPS_SMOKE_CERT="$work/tls/server.crt" \
FIPS_SMOKE_KEY="$work/tls/server.key" \
FIPS_SMOKE_MOCK_URL=http://127.0.0.1:19999 \
  go test -count=1 -v -run TestFIPSSmoke ./tests/fipssmoke/
