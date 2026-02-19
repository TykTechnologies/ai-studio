---
name: dev-logs
description: Fetch logs from development environment components for debugging. Use when investigating errors, checking startup status, or debugging issues. Components: studio, gateway, frontend, postgres, plugins, all.
argument-hint: <component> [--lines N]
allowed-tools: Bash
---

# Dev Component Logs

Fetch logs from Docker Compose development environment components.

## Arguments

**Component** (required) - one of:
- `studio` - AI Studio control plane logs
- `gateway` - Microgateway logs (requires full mode)
- `frontend` - React dev server logs
- `postgres` - PostgreSQL database logs
- `plugins` - Plugin watcher logs (requires full mode)
- `all` - All services combined

**Options:**
- `--lines N` or `-n N` - Number of lines to fetch (default: 100)

## Usage

Run the corresponding make command with optional LINES parameter:

| Command | Make Target |
|---------|-------------|
| `/dev-logs studio` | `make dev-tail-studio` |
| `/dev-logs gateway` | `make dev-tail-gateway` |
| `/dev-logs frontend` | `make dev-tail-frontend` |
| `/dev-logs postgres` | `make dev-tail-postgres` |
| `/dev-logs plugins` | `make dev-tail-plugins` |
| `/dev-logs all` | `make dev-tail` |

With custom line count:
| Command | Make Target |
|---------|-------------|
| `/dev-logs studio --lines 50` | `make dev-tail-studio LINES=50` |
| `/dev-logs all -n 200` | `make dev-tail LINES=200` |

## Examples

```bash
# Get last 100 lines of studio logs
make dev-tail-studio

# Get last 50 lines of gateway logs
make dev-tail-gateway LINES=50

# Get last 200 lines of all logs
make dev-tail LINES=200
```

## Notes

- These commands return immediately (non-blocking) with the last N lines
- For real-time log following, use `make dev-logs-<component>` directly (but this blocks)
- Gateway and plugins logs are only available in full mode (`dev-start-full`)
