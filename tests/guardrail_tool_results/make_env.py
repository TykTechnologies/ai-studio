#!/usr/bin/env python3
"""Write studio.env and gateway.env for the tool-result guardrail E2E.

    E2E_DIR=$(mktemp -d) E2E_SECRETS_FILE=/path/to/dev/.env.secrets python3 make_env.py

E2E_SECRETS_FILE is a dotenv file holding TYK_AI_LICENSE (for example the
main checkout's dev/.env.secrets; worktrees have none). The env files carry
the license and the stack's keys, so the directory is made 0700 and every
file in it 0600. Nothing secret is printed.
"""
import os
import secrets as rnd
import sys

E = os.environ.get("E2E_DIR")
SECRETS = os.environ.get("E2E_SECRETS_FILE")
if not E or not SECRETS:
    sys.exit("make_env.py: set E2E_DIR (a private directory, e.g. $(mktemp -d)) and E2E_SECRETS_FILE (a dotenv file with TYK_AI_LICENSE)")
if not os.path.isfile(SECRETS):
    sys.exit(f"make_env.py: E2E_SECRETS_FILE {SECRETS} does not exist")
os.umask(0o077)
os.makedirs(os.path.join(E, "branding"), exist_ok=True)
os.chmod(E, 0o700)

PORTS = {
    "studio": int(os.environ.get("E2E_STUDIO_PORT", "18080")),
    "proxy": int(os.environ.get("E2E_PROXY_PORT", "19090")),
    "grpc": int(os.environ.get("E2E_GRPC_PORT", "50071")),
    "edge": int(os.environ.get("E2E_EDGE_PORT", "18081")),
}

lic = ""
for line in open(SECRETS):
    line = line.strip()
    if line.startswith("TYK_AI_LICENSE="):
        lic = line.split("=", 1)[1].strip().strip('"')

# Fresh random secrets on every run: the encryption key and gRPC token
# shared by hub and edge, and the edge's admin-API JWT secret. start.sh, the
# only caller, wipes the databases first, so nothing encrypted under an old
# key survives; restart.sh reuses the env files as written.
key = rnd.token_hex(16)
jwt = rnd.token_hex(32)

# Optional extra settings for both nodes, e.g. a small native API guardrail
# call budget for the batch-over-budget scenario.
extra = ""
if os.environ.get("E2E_NATIVE_MAX_GUARDRAIL_CALLS"):
    extra += "NATIVE_API_MAX_GUARDRAIL_CALLS=" + os.environ["E2E_NATIVE_MAX_GUARDRAIL_CALLS"] + "\n"

studio = f"""DEVMODE=true
ALLOW_REGISTRATIONS=true
SERVER_PORT={PORTS['studio']}
PROXY_PORT={PORTS['proxy']}
DOCS_DISABLED=true
SITE_URL=http://localhost:3000
PROXY_URL=http://localhost:{PORTS['proxy']}
DATABASE_TYPE=sqlite
DATABASE_URL={E}/studio.db
TYK_AI_SECRET_KEY={key}
MICROGATEWAY_ENCRYPTION_KEY={key}
GATEWAY_MODE=control
GRPC_PORT={PORTS['grpc']}
GRPC_HOST=127.0.0.1
GRPC_TLS_INSECURE=true
GRPC_AUTH_TOKEN={key}
ALLOW_INTERNAL_NETWORK_ACCESS=true
WEBHOOKS_ALLOW_INTERNAL_TARGETS=true
MARKETPLACE_ENABLED=false
LOG_LEVEL=info
BRANDING_STORAGE_PATH={E}/branding
TYK_AI_LICENSE={lic}
""" + extra
open(os.path.join(E, "studio.env"), "w").write(studio)

gw = f"""PORT={PORTS['edge']}
HOST=127.0.0.1
DATABASE_TYPE=sqlite
DATABASE_DSN="file:{E}/edge.db?cache=shared&mode=rwc"
DB_AUTO_MIGRATE=true
ENCRYPTION_KEY={key}
JWT_SECRET={jwt}
GATEWAY_MODE=edge
CONTROL_ENDPOINT=127.0.0.1:{PORTS['grpc']}
EDGE_ID=edge-guardrails
EDGE_NAMESPACE=
EDGE_AUTH_TOKEN={key}
EDGE_TLS_ENABLED=false
EDGE_ALLOW_INSECURE=true
EDGE_HEARTBEAT_INTERVAL=5s
ALLOW_INTERNAL_NETWORK_ACCESS=true
ANALYTICS_ENABLED=true
GATEWAY_ENABLE_ANALYTICS=true
ANALYTICS_FLUSH_INTERVAL=2s
PLUGINS_CONFIG_PATH={E}/pulse.yaml
LOG_LEVEL=info
TYK_AI_LICENSE={lic}
""" + extra
open(os.path.join(E, "gateway.env"), "w").write(gw)

open(os.path.join(E, "pulse.yaml"), "w").write("""version: "1.0"
data_collection_plugins:
  - name: "analytics_pulse"
    enabled: true
    hook_types: ["analytics", "budget", "proxy_log"]
    replace_database: false
    priority: 100
    config:
      interval_seconds: 3
      max_batch_size: 1000
      max_buffer_size: 10000
      compression_enabled: true
      include_proxy_summaries: true
      include_request_response_data: true
      edge_retention_hours: 24
      excluded_vendors: []
      timeout_seconds: 30
      max_retries: 3
      retry_interval_secs: 5
""")
for name in os.listdir(E):
    path = os.path.join(E, name)
    if os.path.isfile(path):
        os.chmod(path, 0o600)
if not lic:
    sys.exit(f"make_env.py: no TYK_AI_LICENSE in {SECRETS}; the Enterprise build needs one")
print(f"env written to {E} (0700, files 0600); license present")
