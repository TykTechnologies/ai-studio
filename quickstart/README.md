# Tyk AI Studio Quickstart

This directory contains Docker Compose configurations to quickly spin up a demo instance of Tyk AI Studio.

## Prerequisites

- Docker and Docker Compose installed
- A valid Tyk AI license (Enterprise edition only)

## Setup

### 1. Generate your secrets (both editions)

The env files in `confs/` ship with `__REPLACE_WITH_…__` placeholders instead of sample secrets. Replace them before the first start: left in place, AI Studio and the Microgateway stop at startup with an error about the encryption key length.

| Placeholder | Variables | Generate with |
|-------------|-----------|---------------|
| `__REPLACE_WITH_RANDOM_SECRET_KEY__` | `TYK_AI_SECRET_KEY` (AI Studio: encrypts stored secrets, authenticates SSO) | `openssl rand -hex 32` |
| `__REPLACE_WITH_RANDOM_TOKEN__` | `GRPC_AUTH_TOKEN` (AI Studio) and `EDGE_AUTH_TOKEN` (Microgateway): must match | `openssl rand -hex 16` |
| `__REPLACE_WITH_RANDOM_32B_KEY__` | `MICROGATEWAY_ENCRYPTION_KEY` (AI Studio) and `ENCRYPTION_KEY` (Microgateway): must match, exactly 32 characters | `openssl rand -hex 16` |
| `__REPLACE_WITH_RANDOM_SECRET__` | `JWT_SECRET` (Microgateway) | `openssl rand -hex 32` |

To fill them all in one go, run this from the `quickstart/` directory. Each placeholder gets one fresh value in every file, so the pairs that must match do:

```bash
for p in RANDOM_SECRET_KEY:32 RANDOM_TOKEN:16 RANDOM_32B_KEY:16 RANDOM_SECRET:32; do
  sed -i.bak "s/__REPLACE_WITH_${p%%:*}__/$(openssl rand -hex ${p##*:})/g" confs/*.env
done
rm confs/*.env.bak
grep -l __REPLACE_WITH_ confs/*.env || echo "All placeholders replaced"
```

Keep the generated files private: they now hold your secrets.

**Upgrading a quickstart from an earlier release:** keep the `TYK_AI_SECRET_KEY` your install has been using (earlier quickstarts shipped `your-secret-key-here`). Secrets stored under one key cannot be read with another. If you change it, re-enter your stored secrets afterwards.

### 2. Enterprise Edition

1. Add your Tyk AI license to `required.env`:
   ```
   TYK_AI_LICENSE=<your-license-key>
   ```

2. Start the stack:
   ```bash
   docker compose -f ./enterprise/compose.yaml up -d
   ```

### 2. Community Edition

1. Start the stack:
   ```bash
   docker compose -f ./ce/compose.yaml up -d
   ```

## Services

Once running, the following services are available:
| Service | Port | Description |
|---------|------|-------------|
| AI Studio UI | 3000 (CE) / 8585 (ENT) | Main web interface |
| AI Studio API | 9595 | REST API |
| Microgateway | 9091 | LLM proxy gateway |

## Directory Structure

```
quickstart/
├── ce/                    # Community Edition compose files
│   └── compose.yaml
├── enterprise/            # Enterprise Edition compose files
│   └── compose.yaml
├── confs/                 # Configuration files
│   ├── midsommar-ce.env   # AI Studio config (CE)
│   ├── midsommar-ent.env  # AI Studio config (Enterprise)
│   ├── mgw-ce.env         # Microgateway config (CE)
│   └── mgw-ent.env        # Microgateway config (Enterprise)
└── required.env           # License configuration
```

## Stopping

```bash
docker compose -f ./enterprise/compose.yaml down
# or
docker compose -f ./ce/compose.yaml down
```

## Data Persistence

Data is persisted in the `data/` subdirectory within each edition folder (`ce/data/` or `enterprise/data/`).
