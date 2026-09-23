# Producing publishable numbers on cloud VMs

Local runs share one machine between the gateway, the mock and the load
generator. Numbers you give a customer come from dedicated hosts, following
this runbook exactly, and the report must say how they were produced.

## Topology

Four Linux VMs of the same instance family, in one availability zone:

| VM | Runs | Size (suggested) |
|---|---|---|
| `loadgen` | `gwbench` | 8 vCPU, compute-optimised |
| `gateway` | edge microgateway | 4 vCPU (state it in the report) |
| `mock` | `mockllm` | 8 vCPU |
| `hub` | Studio + Postgres | 4 vCPU, 16GB |

- For S3, put the region next to the vendor's API (for example `us-east-1`
  for OpenAI and Anthropic) and run nothing else in the account.
- The hub is off the request path, but the edge sends analytics to it and
  refreshes tokens from it, so it must not be starved.

## One-time setup (all VMs)

1. Install Docker and clone this repository at the commit under test on each VM.
2. Tune the kernel for many short connections:

   ```bash
   sudo sysctl -w net.core.somaxconn=65535
   sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"
   sudo sysctl -w net.ipv4.tcp_tw_reuse=1
   sudo sysctl -w net.core.netdev_max_backlog=65535
   ulimit -n 1048576
   ```

3. Check time sync (`chronyc tracking`). The analysis only differences times
   taken on one host, but the logs from different hosts must line up.
4. Check that network latency between the VMs is stable before you start:
   `ping -c 200 -i 0.2 <gateway-ip>` from `loadgen`. The p99 should be under
   0.5ms. Put the result in the run label.

## Build and start

On each VM, build the image it runs from the same commit:

```bash
# hub
docker compose -f benchmarks/gateway/compose/docker-compose.yml --env-file license.env up -d postgres studio
# gateway: point it at the hub. Secrets go in an env file readable only by
# you, not on the command line (where they reach shell history and `ps`).
docker compose -f benchmarks/gateway/compose/docker-compose.yml --env-file license.env build gateway
install -m 600 /dev/null gateway.env
cat > gateway.env <<'ENV'
GATEWAY_MODE=edge
CONTROL_ENDPOINT=<hub-ip>:50051
EDGE_ID=bench-edge-1
EDGE_NAMESPACE=default
EDGE_AUTH_TOKEN=<same as studio GRPC_AUTH_TOKEN>
EDGE_ALLOW_INSECURE=true
EDGE_HEARTBEAT_INTERVAL=5s
ENCRYPTION_KEY=<same as studio TYK_AI_SECRET_KEY>
TYK_AI_LICENSE=<licence>
PORT=8080
LOG_LEVEL=info
ALLOW_INTERNAL_NETWORK_ACCESS=true
PLUGINS_CONFIG_PATH=/bench/analytics-pulse.yaml
ENABLE_METRICS=true
METRICS_AUTH_TOKEN=<metrics-token>
GATEWAY_SERVER_TIMING=true
ENV
docker run -d --name gateway --network host --ulimit nofile=1048576:1048576 \
  --env-file gateway.env \
  -v $PWD/benchmarks/gateway/compose/analytics-pulse.yaml:/bench/analytics-pulse.yaml:ro \
  gwbench-microgateway:ent
# mock
docker compose -f benchmarks/gateway/compose/docker-compose.yml build mockllm
docker run -d --name mockllm --network host --ulimit nofile=1048576:1048576 gwbench-tools mockllm -addr :9999
# loadgen
go build -o gwbench ./benchmarks/gateway/cmd/gwbench
```

Use `--network host` on the VMs, so Docker's bridge NAT is not in the path.
Restrict the metrics port and the gRPC port to the private network with the
cloud firewall.

## Seed and run (from `loadgen`)

```bash
export GWBENCH_STUDIO_URL=http://<hub-ip>:8080
export GWBENCH_GATEWAY_URL=http://<gateway-ip>:8080
export GWBENCH_MOCK_URL=http://<mock-ip>:9999
export GWBENCH_METRICS_TOKEN=<metrics-token>   # the gateway's METRICS_AUTH_TOKEN
export GWBENCH_LABEL="aws c7i.xlarge gateway, us-east-1a, ping p99 0.21ms"

./gwbench seed -mock-upstream http://<mock-ip>:9999 -vendors
./gwbench run  benchmarks/gateway/scenarios/s0-aa-calibration.yaml \
               benchmarks/gateway/scenarios/s1-overhead-floor.yaml \
               benchmarks/gateway/scenarios/s2-payload-size.yaml
./gwbench run  benchmarks/gateway/scenarios/s4-capacity-ramp.yaml \
               benchmarks/gateway/scenarios/s5-throughput-ceiling.yaml
# read S4's "Capacity" line, then scale the burst and soak from it:
./gwbench run -rate-scale <sustained/200> benchmarks/gateway/scenarios/s6-spike.yaml
./gwbench run -rate-scale <sustained/200> benchmarks/gateway/scenarios/s7-soak.yaml
./gwbench run -vendors benchmarks/gateway/scenarios/s0-aa-calibration.yaml \
               benchmarks/gateway/scenarios/s3-real-vendors.yaml
```

- **Repeat each of S0–S5 three times** and report the spread across repeats. A
  single run is an anecdote.
- Do not delete or cherry-pick runs. An INVALID run is rerun and kept.
- Run S3 at different times of day if the customer cares about variance: vendor
  latency changes through the day, the overhead should not.

## What goes to the customer

Send the `report.html` of every run, and state:

1. The topology and instance types above, and the gateway's CPU and memory.
2. The commit SHA (it is in every report).
3. The S3 paired overhead for TTFT and total time, with confidence intervals.
   This is the end-user number.
4. The S1 overhead per endpoint, which is the gateway's floor.
5. The S4 capacity at the SLO, per gateway size.
6. The S6/S7 result: whether latency and resources stayed flat, and the
   analytics completeness check.
7. The known behaviours listed in the README that apply to their setup.
