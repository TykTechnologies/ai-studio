#!/usr/bin/env bash
# Repeatable AWS environment for the gateway latency benchmarks, following
# RUNBOOK-cloud.md: four VMs (loadgen, gateway, mock, hub) in one cluster
# placement group, the released Studio and microgateway images, and the load
# generator and mock built from the git ref under test.
#
#   aws.sh up                 create key pair, security group, placement group, VMs
#   aws.sh build              build gwbench + mockllm (linux/amd64) from $BENCH_REF
#   aws.sh build-gateway [ref]  edge binary from a git ref, swapped into the
#                             released image (then BENCH_GATEWAY_IMAGE=... deploy)
#   aws.sh deploy             build tools, copy config + secrets, start services
#                             (the edge always starts with an empty database)
#   aws.sh seed [-vendors]    configure Studio + edge (from loadgen)
#   aws.sh smoke              quick S0/S1/S4 end-to-end check (not publishable)
#   aws.sh suite [VAR=val..]  full runbook sequence in the background on loadgen
#                             (REPEATS=3 VENDORS=0 SOAK=1 QUICK=0, see suite.sh)
#   aws.sh run <gwbench run args>   one gwbench run in the foreground
#   aws.sh logs | status      follow / summarise the background suite
#   aws.sh fetch              copy results to benchmarks/gateway/results/aws-<name>/
#   aws.sh ssh <role> [cmd]   shell on loadgen|gateway|mock|hub
#   aws.sh tunnel             Studio UI on http://localhost:18180 through SSH
#   aws.sh down               terminate everything tagged with this deployment
#
# Settings (environment):
#   BENCH_AWS_REGION   ap-southeast-2 (Sydney); ap-southeast-1 is Singapore
#   BENCH_AZ           <region>a
#   BENCH_NAME         gwbench (tags + state dir; run several side by side)
#   BENCH_REF          v2.2.0-rc10.1 (images tag, and the source of the tools)
#   BENCH_TOOLS_REF    $BENCH_REF (build gwbench/mockllm from another ref, e.g. a
#                      newer analysis; the manifests record this ref's SHA)
#   BENCH_STUDIO_IMAGE / BENCH_GATEWAY_IMAGE   default tykio/*-ent:$BENCH_REF
#   BENCH_ENABLE_PROFILING  false; true serves pprof on <gateway private ip>:6060
#   BENCH_GATEWAY_ENV  extra edge settings, "KEY=VALUE KEY2=VALUE2"
#   BENCH_LOG_LEVEL    info; BENCH_PLUGINS_CONFIG_PATH= (set, empty) turns the
#                      analytics pulse off (both for scenario s5m)
#   BENCH_TYPE_{LOADGEN,GATEWAY,MOCK,HUB}      c7i.2xlarge c7i.xlarge c7i.2xlarge m7i.xlarge
#   BENCH_ENV_FILE     dev/.env.secrets (must hold TYK_AI_LICENSE)
#   BENCH_VENDORS_DIR  test-secrets (vendors.env, only for seed -vendors / S3)
#   BENCH_SSH_CIDR     your public IP/32 (SSH is open to nothing else)
#
# State (key, instance ids, generated secrets) lives in
# benchmarks/gateway/.cloud/<name>/, which is gitignored. `down` finds
# resources by tag, so it works even if that directory is lost.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BENCH_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
REPO_ROOT=$(cd "$BENCH_DIR/../.." && pwd)

REGION=${BENCH_AWS_REGION:-ap-southeast-2}
AZ=${BENCH_AZ:-${REGION}a}
NAME=${BENCH_NAME:-gwbench}
REF=${BENCH_REF:-v2.2.0-rc10.1}
TOOLS_REF=${BENCH_TOOLS_REF:-$REF}
STUDIO_IMAGE=${BENCH_STUDIO_IMAGE:-tykio/tyk-ai-studio-ent:$REF}
GATEWAY_IMAGE=${BENCH_GATEWAY_IMAGE:-tykio/tyk-microgateway-ent:$REF}
ENV_FILE=${BENCH_ENV_FILE:-$REPO_ROOT/dev/.env.secrets}
VENDORS_DIR=${BENCH_VENDORS_DIR:-$REPO_ROOT/test-secrets}
ROLES=(loadgen gateway mock hub)
STATE=$BENCH_DIR/.cloud/$NAME
KEY=$STATE/id_ed25519
TAG_KEY=gwbench:deployment

type_for() {
  case $1 in
    loadgen) echo "${BENCH_TYPE_LOADGEN:-c7i.2xlarge}" ;;
    gateway) echo "${BENCH_TYPE_GATEWAY:-c7i.xlarge}" ;;
    mock)    echo "${BENCH_TYPE_MOCK:-c7i.2xlarge}" ;;
    hub)     echo "${BENCH_TYPE_HUB:-m7i.xlarge}" ;;
  esac
}

# The edge keeps analytics rows (with up to ANALYTICS_MAX_BODY_SIZE of each
# request and response body, whatever ANALYTICS_STORE_* say, in rc10.1) for
# ANALYTICS_RETENTION_DAYS: on the benchmark that is ~10 GB per 40 minutes
# of load, so the gateway gets room for a full suite including the soak.
# The hub's Postgres keeps every proxy log and analytics row (~0.5 KB each,
# ~10M per S5 run at the ceiling) and the loadgen keeps ~1 GB of raw results
# per S5 run: 40 GB filled both mid-suite on 2026-09-25.
disk_for() {
  case $1 in
    gateway) echo "${BENCH_DISK_GATEWAY:-200}" ;;
    hub) echo "${BENCH_DISK_HUB:-200}" ;;
    loadgen) echo "${BENCH_DISK_LOADGEN:-200}" ;;
    *) echo 40 ;;
  esac
}

log() { echo "$(date +%H:%M:%S) $*" >&2; }
die() { log "error: $*"; exit 1; }
awsr() { aws --region "$REGION" "$@"; }

# --- state ------------------------------------------------------------------

ip() { # ip <role> public|private
  local f=$STATE/hosts.json
  [ -f "$f" ] || die "no hosts for '$NAME' (run: aws.sh up)"
  jq -r --arg r "$1" --arg k "$2" '.[$r][$k]' "$f"
}

SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$STATE/known_hosts"
  -o ConnectTimeout=10 -o ServerAliveInterval=30 -o LogLevel=ERROR)
rsh() { local role=$1; shift; ssh -i "$KEY" "${SSH_OPTS[@]}" "ubuntu@$(ip "$role" public)" "$@"; }
rcp() { local role=$1 dest=$2; shift 2; scp -q -i "$KEY" "${SSH_OPTS[@]}" "$@" "ubuntu@$(ip "$role" public):$dest"; }

# --- up ---------------------------------------------------------------------

# Key pairs and placement groups are account-wide names. aws.sh only reuses
# or deletes one that carries this deployment's tag; an untagged one of the
# same name belongs to someone else and stops the run.
owned() { # owned <describe output of the tag value> -> ok if it is ours
  [ "$1" = "$NAME" ]
}

ensure_key() {
  mkdir -p "$STATE"; chmod 700 "$STATE"
  [ -f "$KEY" ] || ssh-keygen -q -t ed25519 -N '' -C "$NAME" -f "$KEY"
  local tag
  if tag=$(awsr ec2 describe-key-pairs --key-names "$NAME" \
      --query "KeyPairs[0].Tags[?Key=='$TAG_KEY'].Value|[0]" --output text 2>/dev/null); then
    owned "$tag" || die "key pair '$NAME' exists in $REGION and is not tagged $TAG_KEY=$NAME; pick another BENCH_NAME"
  else
    awsr ec2 import-key-pair --key-name "$NAME" --public-key-material "fileb://$KEY.pub" \
      --tag-specifications "ResourceType=key-pair,Tags=[{Key=$TAG_KEY,Value=$NAME}]" >/dev/null
    log "key pair $NAME imported"
  fi
}

tagspec() { echo "ResourceType=$1,Tags=[{Key=$TAG_KEY,Value=$NAME},{Key=Name,Value=$NAME}]"; }
by_tag() { echo "Name=tag:$TAG_KEY,Values=$NAME"; }

# ensure_network creates a VPC of its own for the deployment, rather than
# using the default VPC: default VPCs are shared with anything else in the
# account, and their network ACL may deny SSH (as in the account this was
# written for). Prints "<vpc> <subnet>".
ensure_network() {
  local vpc igw subnet rt
  vpc=$(awsr ec2 describe-vpcs --filters "$(by_tag)" --query 'Vpcs[0].VpcId' --output text)
  if [ "$vpc" = None ]; then
    vpc=$(awsr ec2 create-vpc --cidr-block 10.77.0.0/16 --tag-specifications "$(tagspec vpc)" --query Vpc.VpcId --output text)
    awsr ec2 wait vpc-available --vpc-ids "$vpc"
    log "vpc $vpc created"
  fi
  igw=$(awsr ec2 describe-internet-gateways --filters "$(by_tag)" --query 'InternetGateways[0].InternetGatewayId' --output text)
  if [ "$igw" = None ]; then
    igw=$(awsr ec2 create-internet-gateway --tag-specifications "$(tagspec internet-gateway)" \
      --query InternetGateway.InternetGatewayId --output text)
    awsr ec2 attach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$vpc"
  fi
  subnet=$(awsr ec2 describe-subnets --filters "$(by_tag)" --query 'Subnets[0].SubnetId' --output text)
  if [ "$subnet" = None ]; then
    subnet=$(awsr ec2 create-subnet --vpc-id "$vpc" --cidr-block 10.77.1.0/24 --availability-zone "$AZ" \
      --tag-specifications "$(tagspec subnet)" --query Subnet.SubnetId --output text)
    awsr ec2 modify-subnet-attribute --subnet-id "$subnet" --map-public-ip-on-launch
    rt=$(awsr ec2 create-route-table --vpc-id "$vpc" --tag-specifications "$(tagspec route-table)" \
      --query RouteTable.RouteTableId --output text)
    awsr ec2 create-route --route-table-id "$rt" --destination-cidr-block 0.0.0.0/0 --gateway-id "$igw" >/dev/null
    awsr ec2 associate-route-table --route-table-id "$rt" --subnet-id "$subnet" >/dev/null
    log "subnet $subnet in $AZ"
  fi
  echo "$vpc $subnet"
}

ensure_sg() { # ensure_sg <vpc>
  local vpc=$1 sg cidr
  sg=$(awsr ec2 describe-security-groups --filters Name=group-name,Values="$NAME" Name=vpc-id,Values="$vpc" \
    --query 'SecurityGroups[0].GroupId' --output text)
  if [ "$sg" = None ]; then
    sg=$(awsr ec2 create-security-group --group-name "$NAME" --vpc-id "$vpc" \
      --description "gateway latency benchmark $NAME" \
      --tag-specifications "$(tagspec security-group)" \
      --query GroupId --output text)
    # Everything between the benchmark VMs; nothing else but SSH.
    awsr ec2 authorize-security-group-ingress --group-id "$sg" \
      --ip-permissions "IpProtocol=-1,UserIdGroupPairs=[{GroupId=$sg}]" >/dev/null
    log "security group $sg created"
  fi
  cidr=${BENCH_SSH_CIDR:-$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')/32}
  awsr ec2 authorize-security-group-ingress --group-id "$sg" \
    --ip-permissions "IpProtocol=tcp,FromPort=22,ToPort=22,IpRanges=[{CidrIp=$cidr,Description=gwbench-ssh}]" \
    >/dev/null 2>&1 && log "SSH allowed from $cidr" || true
  echo "$sg"
}

ensure_pg() {
  local tag
  if tag=$(awsr ec2 describe-placement-groups --group-names "$NAME" \
      --query "PlacementGroups[0].Tags[?Key=='$TAG_KEY'].Value|[0]" --output text 2>/dev/null); then
    owned "$tag" || die "placement group '$NAME' exists in $REGION and is not tagged $TAG_KEY=$NAME; pick another BENCH_NAME"
  else
    awsr ec2 create-placement-group --group-name "$NAME" --strategy cluster \
      --tag-specifications "ResourceType=placement-group,Tags=[{Key=$TAG_KEY,Value=$NAME}]" >/dev/null
    log "cluster placement group $NAME created"
  fi
}

instance_ids() { # all live instances of this deployment
  awsr ec2 describe-instances \
    --filters "Name=tag:$TAG_KEY,Values=$NAME" Name=instance-state-name,Values=pending,running,stopping,stopped \
    --query 'Reservations[].Instances[].InstanceId' --output text
}

write_hosts() {
  awsr ec2 describe-instances \
    --filters "Name=tag:$TAG_KEY,Values=$NAME" Name=instance-state-name,Values=running \
    --query 'Reservations[].Instances[].{role: Tags[?Key==`gwbench:role`]|[0].Value, id: InstanceId, type: InstanceType, az: Placement.AvailabilityZone, public: PublicIpAddress, private: PrivateIpAddress}' \
    --output json | jq 'map({(.role): .}) | add' > "$STATE/hosts.json"
}

cmd_up() {
  ensure_key
  local vpc sg subnet ami existing role
  read -r vpc subnet < <(ensure_network)
  sg=$(ensure_sg "$vpc")
  ensure_pg
  ami=$(awsr ssm get-parameter --name /aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id \
    --query Parameter.Value --output text)
  existing=$(awsr ec2 describe-instances \
    --filters "Name=tag:$TAG_KEY,Values=$NAME" Name=instance-state-name,Values=pending,running \
    --query 'Reservations[].Instances[].Tags[?Key==`gwbench:role`].Value[]' --output text)
  local ids=()
  for role in "${ROLES[@]}"; do
    if grep -qw "$role" <<<"$existing"; then log "$role already running"; continue; fi
    local id
    id=$(awsr ec2 run-instances --image-id "$ami" --instance-type "$(type_for "$role")" \
      --key-name "$NAME" --security-group-ids "$sg" --subnet-id "$subnet" \
      --placement "GroupName=$NAME" \
      --user-data "fileb://$SCRIPT_DIR/host-init.sh" \
      --block-device-mappings "DeviceName=/dev/sda1,Ebs={VolumeSize=$(disk_for "$role"),VolumeType=gp3,DeleteOnTermination=true}" \
      --metadata-options HttpTokens=required \
      --instance-initiated-shutdown-behavior terminate \
      --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=$NAME-$role},{Key=$TAG_KEY,Value=$NAME},{Key=gwbench:role,Value=$role}]" \
        "ResourceType=volume,Tags=[{Key=$TAG_KEY,Value=$NAME}]" \
      --query 'Instances[0].InstanceId' --output text)
    log "$role: $id ($(type_for "$role"))"
    ids+=("$id")
  done
  [ ${#ids[@]} -eq 0 ] || awsr ec2 wait instance-running --instance-ids "${ids[@]}"
  write_hosts
  for role in "${ROLES[@]}"; do
    log "waiting for $role ($(ip "$role" public)) to finish cloud-init"
    local n=0
    until rsh "$role" true 2>/dev/null; do n=$((n+1)); [ $n -lt 60 ] || die "no SSH to $role"; sleep 5; done
    rsh "$role" 'cloud-init status --wait >/dev/null; test -f /var/lib/gwbench-init-done' || die "cloud-init failed on $role"
  done
  log "up: $(jq -r 'to_entries|map("\(.key)=\(.value.private)")|join(" ")' "$STATE/hosts.json")"
}

# --- deploy -----------------------------------------------------------------

ensure_secrets() {
  local f=$STATE/secrets.env
  [ -f "$f" ] && return
  umask 077
  {
    echo "BENCH_SECRET_KEY=$(openssl rand -hex 16)"
    echo "BENCH_GRPC_TOKEN=$(openssl rand -hex 24)"
    echo "BENCH_METRICS_TOKEN=$(openssl rand -hex 24)"
    echo "BENCH_PG_PASSWORD=$(openssl rand -hex 16)"
    echo "GWBENCH_ADMIN_PASSWORD=Bench#$(openssl rand -hex 8)"
  } > "$f"
}

# build_tools compiles gwbench and mockllm for linux/amd64 from $TOOLS_REF
# (default $REF), so the load generator and mock match the release under test.
build_tools() {
  local src=$STATE/src sha
  sha=$(git -C "$REPO_ROOT" rev-parse "$TOOLS_REF^{commit}") || die "unknown ref $TOOLS_REF (git fetch?)"
  if [ -f "$STATE/bin/.sha" ] && [ "$(cat "$STATE/bin/.sha")" = "$sha" ]; then return; fi
  log "building gwbench + mockllm from $TOOLS_REF ($sha)"
  rm -rf "$src"; mkdir -p "$src" "$STATE/bin"
  git -C "$REPO_ROOT" archive "$sha" | tar -x -C "$src"
  # The root module's replace directive needs the enterprise submodule's go.mod.
  local ent_sha
  ent_sha=$(git -C "$REPO_ROOT" ls-tree "$sha" enterprise | awk '{print $3}')
  mkdir -p "$src/enterprise"
  git -C "$REPO_ROOT/enterprise" archive "$ent_sha" | tar -x -C "$src/enterprise" \
    || die "cannot read enterprise@$ent_sha (git -C enterprise fetch?)"
  (cd "$src" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
      -o "$STATE/bin/" ./benchmarks/gateway/cmd/gwbench ./benchmarks/gateway/cmd/mockllm)
  echo "$sha" > "$STATE/bin/.sha"
  cp "$src/go.mod" "$STATE/bin/go.mod"
  rm -rf "$STATE/bin/scenarios"; cp -R "$src/benchmarks/gateway/scenarios" "$STATE/bin/scenarios"
  rm -rf "$src"
}

ping_p99() { # from loadgen to the gateway, as the runbook asks
  rsh loadgen "ping -q -c 200 -i 0.2 $(ip gateway private) >/dev/null; ping -c 200 -i 0.2 $(ip gateway private)" \
    | awk -F'time=' '/time=/{split($2,a," "); print a[1]}' | sort -n \
    | awk '{v[NR]=$1} END {i=int(NR*0.99); if (i<1) i=1; printf "%.3f", v[i]}'
}

cmd_deploy() {
  [ -f "$ENV_FILE" ] || die "$ENV_FILE not found (BENCH_ENV_FILE, needs TYK_AI_LICENSE)"
  local licence
  licence=$(sed -n 's/^TYK_AI_LICENSE=//p' "$ENV_FILE" | tail -1)
  [ -n "$licence" ] || die "TYK_AI_LICENSE missing in $ENV_FILE"
  ensure_secrets
  build_tools
  local hub=$(ip hub private) gw=$(ip gateway private) mock=$(ip mock private)
  local benv=$STATE/bench.env
  (umask 077
   { cat "$STATE/secrets.env"
     echo "TYK_AI_LICENSE=$licence"
     echo "HUB_IP=$hub"
     echo "STUDIO_IMAGE=$STUDIO_IMAGE"
     echo "GATEWAY_IMAGE=$GATEWAY_IMAGE"
     # S8 variants; the compose file's defaults apply when unset.
     echo "EDGE_TOKEN_CACHE_ENABLED=${EDGE_TOKEN_CACHE_ENABLED:-true}"
     echo "ENABLE_TRACING=${ENABLE_TRACING:-false}"
     # Minimal-configuration variant (scenario s5m): set but empty turns the
     # analytics pulse off.
     echo "BENCH_LOG_LEVEL=${BENCH_LOG_LEVEL:-info}"
     echo "BENCH_PLUGINS_CONFIG_PATH=${BENCH_PLUGINS_CONFIG_PATH-/bench/analytics-pulse.yaml}"
     # Profiling: BENCH_ENABLE_PROFILING=true serves pprof on the gateway's
     # private address, port 6060 (see cloud/pprof-watch.sh).
     echo "BENCH_ENABLE_PROFILING=${BENCH_ENABLE_PROFILING:-false}"
     echo "BENCH_PROFILING_ADDR=${BENCH_PROFILING_ADDR:-$( [ "${BENCH_ENABLE_PROFILING:-false}" = true ] && echo 0.0.0.0:6060 || echo 127.0.0.1:6060)}"
   } > "$benv")

  # Extra edge settings for the build under test: BENCH_GATEWAY_ENV is a
  # space-separated list of KEY=VALUE pairs, written to the gateway's
  # gateway-extra.env (empty when unset, so a redeploy clears earlier ones).
  local genv=$STATE/gateway-extra.env
  (umask 077; : > "$genv"; for kv in ${BENCH_GATEWAY_ENV:-}; do echo "$kv" >> "$genv"; done)

  # hub + gateway: compose with the released images
  for role in hub gateway; do
    rcp "$role" /opt/gwbench/ "$SCRIPT_DIR/docker-compose.yml" "$BENCH_DIR/compose/analytics-pulse.yaml"
    rcp "$role" /opt/gwbench/bench.env "$benv"
    if [ "$role" = gateway ]; then rcp gateway /opt/gwbench/gateway-extra.env "$genv"; fi
    # --ignore-pull-failures: an image from build-gateway exists only locally.
    rsh "$role" "chmod 600 /opt/gwbench/bench.env && sudo docker compose -f /opt/gwbench/docker-compose.yml --env-file /opt/gwbench/bench.env --profile $role pull -q --ignore-pull-failures"
  done
  log "starting hub"
  rsh hub "sudo docker compose -f /opt/gwbench/docker-compose.yml --env-file /opt/gwbench/bench.env --profile hub up -d"
  wait_http hub "http://127.0.0.1:8080/health"

  log "starting mock"
  # Upload beside the running binary and rename: overwriting it in place
  # fails with "text file busy" on a redeploy.
  rcp mock /opt/gwbench/mockllm.new "$STATE/bin/mockllm"
  rsh mock "mv -f /opt/gwbench/mockllm.new /opt/gwbench/mockllm"
  rsh mock "sudo tee /etc/systemd/system/mockllm.service >/dev/null <<'EOF'
[Unit]
Description=gwbench mock LLM upstream
After=network-online.target
[Service]
ExecStart=/opt/gwbench/mockllm -addr :9999
LimitNOFILE=1048576
Restart=always
[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable mockllm >/dev/null 2>&1 && sudo systemctl restart mockllm"
  wait_http mock "http://127.0.0.1:9999/stats"

  # Every deploy starts the edge with an empty database: what an earlier run
  # left behind (analytics rows, a large write-ahead log) changes its latency.
  # Re-seed afterwards so Studio pushes the configuration to the new edge.
  log "starting gateway (fresh database)"
  rsh gateway "sudo docker compose -f /opt/gwbench/docker-compose.yml --env-file /opt/gwbench/bench.env --profile gateway down >/dev/null 2>&1; \
    sudo find /opt/gwbench/data -mindepth 1 -delete && \
    sudo docker compose -f /opt/gwbench/docker-compose.yml --env-file /opt/gwbench/bench.env --profile gateway up -d --force-recreate"
  wait_http gateway "http://127.0.0.1:8080/health"

  log "installing gwbench on loadgen"
  rsh loadgen "mkdir -p /opt/gwbench/bin /opt/gwbench/benchmarks/gateway/results /opt/gwbench/benchmarks/gateway/.state /opt/gwbench/test-secrets"
  rcp loadgen /opt/gwbench/bin/gwbench.new "$STATE/bin/gwbench"
  rsh loadgen "mv -f /opt/gwbench/bin/gwbench.new /opt/gwbench/bin/gwbench"
  rcp loadgen /opt/gwbench/ "$STATE/bin/go.mod" "$SCRIPT_DIR/suite.sh"
  rsh loadgen "rm -rf /opt/gwbench/benchmarks/gateway/scenarios"
  scp -q -r -i "$KEY" "${SSH_OPTS[@]}" "$STATE/bin/scenarios" "ubuntu@$(ip loadgen public):/opt/gwbench/benchmarks/gateway/scenarios"
  if [ -f "$VENDORS_DIR/vendors.env" ]; then
    rcp loadgen /opt/gwbench/test-secrets/vendors.env "$VENDORS_DIR/vendors.env"
    rsh loadgen "chmod 600 /opt/gwbench/test-secrets/vendors.env"
  fi

  local p99 label
  p99=$(ping_p99)
  label="aws $REGION $(jq -r .gateway.az "$STATE/hosts.json"), cluster PG; gateway $(type_for gateway) (whole VM), loadgen $(type_for loadgen), mock $(type_for mock), hub $(type_for hub); images $STUDIO_IMAGE + $GATEWAY_IMAGE; ping p99 ${p99}ms"
  log "label: $label"
  local lenv=$STATE/loadgen.env metrics
  metrics=$(sed -n 's/^BENCH_METRICS_TOKEN=//p' "$STATE/secrets.env")
  (umask 077
   { echo "GWBENCH_STUDIO_URL=http://$hub:8080"
     echo "GWBENCH_GATEWAY_URL=http://$gw:8080"
     echo "GWBENCH_MOCK_URL=http://$mock:9999"
     echo "GWBENCH_MOCK_UPSTREAM_URL=http://$mock:9999"
     echo "GWBENCH_METRICS_TOKEN=$metrics"
     echo "GWBENCH_ADMIN_PASSWORD=$(sed -n 's/^GWBENCH_ADMIN_PASSWORD=//p' "$STATE/secrets.env")"
     echo "GWBENCH_STATE=/opt/gwbench/benchmarks/gateway/.state/state.json"
     echo "GWBENCH_OUT=/opt/gwbench/benchmarks/gateway/results"
     echo "GWBENCH_GIT_SHA=$(cat "$STATE/bin/.sha")"
     echo "GWBENCH_GIT_DIRTY=false"
     echo "VENDOR_TESTS_ENV_FILE=/opt/gwbench/test-secrets/vendors.env"
     printf 'GWBENCH_LABEL=%q\n' "${BENCH_LABEL:-$label}"
   } > "$lenv")
  rcp loadgen /opt/gwbench/loadgen.env "$lenv"
  rsh loadgen "chmod 600 /opt/gwbench/loadgen.env && chmod +x /opt/gwbench/suite.sh"
  log "deployed $REF to $NAME"
}

wait_http() { # wait_http <role> <url>, checked on that host
  local n=0
  until rsh "$1" "curl -fsS -o /dev/null $2" 2>/dev/null; do
    n=$((n+1)); [ $n -lt 60 ] || { rsh "$1" "sudo docker ps -a; sudo docker compose -f /opt/gwbench/docker-compose.yml --env-file /opt/gwbench/bench.env logs --tail 50 2>/dev/null" || true; die "$1 not healthy at $2"; }
    sleep 5
  done
  log "$1 healthy"
}

# cmd_build_gateway builds the edge binary from a git ref the way the release
# does (CGO, -tags=enterprise, Debian glibc toolchain) on the gateway VM, and
# swaps it into the released image, so the image differs from the release only
# by that binary. Prints the image name; deploy it with
#   BENCH_GATEWAY_IMAGE=<name> aws.sh deploy && aws.sh seed
cmd_build_gateway() {
  local ref=${1:-main} sha ent_sha short base img
  sha=$(git -C "$REPO_ROOT" rev-parse "$ref^{commit}") || die "unknown ref $ref"
  short=${sha:0:8}
  ent_sha=$(git -C "$REPO_ROOT" ls-tree "$sha" enterprise | awk '{print $3}')
  base=${BENCH_GATEWAY_BASE:-tykio/tyk-microgateway-ent:$REF}
  img=gwbench-microgateway:$short
  log "building the edge from $ref ($short, enterprise $ent_sha) into $base"
  git -C "$REPO_ROOT" archive "$sha" | gzip > "$STATE/src.tgz"
  git -C "$REPO_ROOT/enterprise" archive --prefix=enterprise/ "$ent_sha" | gzip > "$STATE/ent.tgz" \
    || die "cannot read enterprise@$ent_sha (git -C enterprise fetch?)"
  rcp gateway /opt/gwbench/ "$STATE/src.tgz" "$STATE/ent.tgz"
  rm -f "$STATE/src.tgz" "$STATE/ent.tgz"
  rsh gateway "set -e; sudo rm -rf /opt/gwbench/src /opt/gwbench/img && mkdir -p /opt/gwbench/src /opt/gwbench/img && \
    tar -xzf /opt/gwbench/src.tgz -C /opt/gwbench/src && tar -xzf /opt/gwbench/ent.tgz -C /opt/gwbench/src && rm /opt/gwbench/*.tgz && \
    sudo docker run --rm -v /opt/gwbench/src:/src -v gwbench-gomod:/go/pkg/mod -v gwbench-gocache:/root/.cache \
      -w /src/microgateway -e CGO_ENABLED=1 -e GOFLAGS=-tags=enterprise golang:1.26-bookworm \
      go build -ldflags '-X main.Version=$ref-$short -X main.BuildHash=$sha -X main.BuildTime=$(date -u +%FT%TZ) -X main.BuiltBy=gwbench' \
      -o /src/tyk-microgateway ./cmd/microgateway && \
    sudo cp /opt/gwbench/src/tyk-microgateway /opt/gwbench/img/ && \
    printf 'FROM $base\nCOPY --chown=999:999 tyk-microgateway /opt/tyk-microgateway/tyk-microgateway\n' > /opt/gwbench/img/Dockerfile && \
    sudo docker build -q -t $img /opt/gwbench/img >/dev/null && sudo rm -rf /opt/gwbench/src"
  log "built $img"
  echo "$img"
}

# --- run --------------------------------------------------------------------

on_loadgen() { # run a command in /opt/gwbench with loadgen.env loaded
  rsh loadgen "cd /opt/gwbench && set -a && . ./loadgen.env && set +a && ulimit -n 1048576 && $*"
}

cmd_seed() { on_loadgen "./bin/gwbench seed $*"; }
cmd_run() { on_loadgen "./bin/gwbench run $*"; }
cmd_smoke() {
  local s=benchmarks/gateway/scenarios
  cmd_run -quick $s/s0-aa-calibration.yaml $s/s1-overhead-floor.yaml $s/s4-capacity-ramp.yaml
}

cmd_suite() {
  local vars="$*"
  rsh loadgen "cd /opt/gwbench && if pgrep -x suite.sh >/dev/null; then echo 'suite already running' >&2; exit 1; fi; \
    env $vars nohup setsid ./suite.sh >> suite.log 2>&1 < /dev/null & echo started"
  log "suite started on loadgen ($vars); follow with: aws.sh logs"
}
cmd_logs() { rsh loadgen "tail -n ${LINES:-40} -F /opt/gwbench/suite.log"; }
cmd_status() {
  [ -f "$STATE/hosts.json" ] && jq -r 'to_entries[]|"\(.key)\t\(.value.id)\t\(.value.type)\t\(.value.public)\t\(.value.private)"' "$STATE/hosts.json"
  rsh loadgen "cd /opt/gwbench; cat suite.status 2>/dev/null; ls -1 benchmarks/gateway/results | tail -n 20; grep -hE 'VALID|INVALID|FAILED' suite.log 2>/dev/null | tail -n 20" || true
}

cmd_fetch() {
  local dest=$BENCH_DIR/results/aws-$NAME
  mkdir -p "$dest"
  rsync -a -e "ssh -i $KEY ${SSH_OPTS[*]}" "ubuntu@$(ip loadgen public):/opt/gwbench/benchmarks/gateway/results/" "$dest/"
  rsync -a -e "ssh -i $KEY ${SSH_OPTS[*]}" "ubuntu@$(ip loadgen public):/opt/gwbench/suite.log" "$dest/" 2>/dev/null || true
  cp "$STATE/hosts.json" "$dest/hosts.json"
  log "results in $dest"
}

cmd_ssh() { local role=${1:?role}; shift; rsh "$role" "$@"; }
cmd_tunnel() {
  log "Studio: http://localhost:18180 (Ctrl-C to close)"
  ssh -i "$KEY" "${SSH_OPTS[@]}" -N -L "18180:127.0.0.1:8080" "ubuntu@$(ip hub public)"
}

# --- down -------------------------------------------------------------------

# cmd_down deletes only resources tagged gwbench:deployment=$NAME in $REGION:
# every lookup below filters on that tag, never on a name alone.
cmd_down() {
  local ids x
  ids=$(instance_ids)
  if [ -n "$ids" ]; then
    log "terminating $ids"
    awsr ec2 terminate-instances --instance-ids $ids >/dev/null
    awsr ec2 wait instance-terminated --instance-ids $ids
  fi
  for x in $(awsr ec2 describe-security-groups --filters "$(by_tag)" --query 'SecurityGroups[].GroupId' --output text); do
    awsr ec2 delete-security-group --group-id "$x" >/dev/null && log "deleted security group $x"
  done
  for x in $(awsr ec2 describe-subnets --filters "$(by_tag)" --query 'Subnets[].SubnetId' --output text); do
    awsr ec2 delete-subnet --subnet-id "$x" && log "deleted subnet $x"
  done
  for x in $(awsr ec2 describe-route-tables --filters "$(by_tag)" --query 'RouteTables[].RouteTableId' --output text); do
    awsr ec2 delete-route-table --route-table-id "$x" && log "deleted route table $x"
  done
  for x in $(awsr ec2 describe-internet-gateways --filters "$(by_tag)" --query 'InternetGateways[].[InternetGatewayId,Attachments[0].VpcId]' --output text | tr '\t' ,); do
    [ "${x#*,}" = None ] || awsr ec2 detach-internet-gateway --internet-gateway-id "${x%,*}" --vpc-id "${x#*,}"
    awsr ec2 delete-internet-gateway --internet-gateway-id "${x%,*}" && log "deleted internet gateway ${x%,*}"
  done
  for x in $(awsr ec2 describe-vpcs --filters "$(by_tag)" --query 'Vpcs[].VpcId' --output text); do
    awsr ec2 delete-vpc --vpc-id "$x" && log "deleted vpc $x"
  done
  for x in $(awsr ec2 describe-placement-groups --filters "$(by_tag)" --query 'PlacementGroups[].GroupName' --output text); do
    awsr ec2 delete-placement-group --group-name "$x" && log "deleted placement group $x"
  done
  for x in $(awsr ec2 describe-key-pairs --filters "$(by_tag)" --query 'KeyPairs[].KeyPairId' --output text); do
    awsr ec2 delete-key-pair --key-pair-id "$x" >/dev/null && log "deleted key pair $x"
  done
  rm -f "$STATE/hosts.json" "$STATE/known_hosts"
  log "down: $NAME in $REGION (local secrets and built tools kept in $STATE)"
}

cmd=${1:-}; shift || true
case $cmd in
  build) mkdir -p "$STATE"; build_tools ;;
  build-gateway) cmd_build_gateway "$@" ;;
  up|deploy|seed|smoke|suite|run|logs|status|fetch|ssh|tunnel|down) "cmd_$cmd" "$@" ;;
  *) sed -n '2,/^set -euo/p' "$0" | sed '$d; s/^# \{0,1\}//'; exit 2 ;;
esac
