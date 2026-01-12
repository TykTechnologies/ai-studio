# Tyk AI Studio Quickstart

This directory contains Docker Compose configurations to quickly spin up a demo instance of Tyk AI Studio.

## Prerequisites

- Docker and Docker Compose installed
- A valid Tyk AI license (Enterprise edition only)

## Setup

### Enterprise Edition

1. Add your Tyk AI license to `required.env`:
   ```
   TYK_AI_LICENSE=<your-license-key>
   ```

2. Start the stack:
   ```bash
   docker compose -f ./enterprise/compose.yaml up -d
   ```

### Community Edition

1. Start the stack:
   ```bash
   docker compose -f ./ce/compose.yaml up -d
   ```

## Services

Once running, the following services are available:

| Service | Port | Description |
|---------|------|-------------|
| AI Studio UI | 8585 | Main web interface |
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
