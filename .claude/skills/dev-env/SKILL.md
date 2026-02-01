---
name: dev-env
description: Start, stop, or manage the Docker-based development environment for Tyk AI Studio. Use when the user wants to start the dev environment, stop it, check status, or clean up.
argument-hint: [start|stop|status|clean|start-full|start-ent|start-full-ent]
allowed-tools: Bash
---

# Dev Environment Management

Manage the Docker Compose development environment for Tyk AI Studio.

## Arguments

- `start` - Start minimal environment (Studio + Frontend + Postgres) in detached mode
- `start-full` - Start full stack (+ Gateway + Plugin watcher) in detached mode
- `start-ent` - Start enterprise minimal environment in detached mode
- `start-full-ent` - Start full enterprise stack in detached mode
- `stop` - Stop all development containers
- `status` - Show current container status
- `clean` - Stop containers and remove all data (fresh start)

## Usage

When the user provides an argument, run the corresponding make command:

| Argument | Command |
|----------|---------|
| `start` | `make dev-start` |
| `start-full` | `make dev-start-full` |
| `start-ent` | `make dev-start-ent` |
| `start-full-ent` | `make dev-start-full-ent` |
| `stop` | `make dev-down` |
| `status` | `make dev-status` |
| `clean` | `make dev-clean` |

If no argument is provided, show the user the available options and ask what they want to do.

## Services

- **postgres** - PostgreSQL 17 database (port 5432)
- **studio** - AI Studio control plane with hot reload (ports 8080, 9090)
- **frontend** - React dev server with HMR (port 3000)
- **gateway** - Microgateway data plane (port 8081) - full mode only
- **plugins** - Plugin watcher/builder - full mode only

## Notes

- All start commands run in detached mode (`-d` flag) and return immediately
- Use `status` to check if services are running after starting
- Use `clean` for a fresh start - this removes all data including the database
