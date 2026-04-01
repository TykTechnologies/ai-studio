---
name: dev-restart
description: Restart a specific component in the development environment. Use when a service needs to be restarted after config changes, code changes, or if it's misbehaving. Components: studio, gateway, frontend, postgres, plugins.
argument-hint: <component>
allowed-tools: Bash
---

# Dev Component Restart

Restart a specific component in the Docker Compose development environment.
Restarting a Go service (studio, gateway) triggers a rebuild of the binary from source.

## Arguments

The argument should be one of the following component names:

- `studio` - AI Studio control plane (Go backend) — rebuilds on restart
- `gateway` - Microgateway data plane (requires full mode) — rebuilds on restart
- `frontend` - React frontend dev server
- `postgres` - PostgreSQL database
- `plugins` - Plugin watcher/builder (requires full mode)

## Usage

When the user provides a component name, run:

```bash
make dev-restart-<component>
```

For example:
- `/dev-restart studio` runs `make dev-restart-studio`
- `/dev-restart gateway` runs `make dev-restart-gateway`
- `/dev-restart frontend` runs `make dev-restart-frontend`

This restarts the container, which for Go services (studio, gateway) triggers a full rebuild from source before starting.

## Notes

- Gateway and plugins services are only available in full mode (`dev-start-full`)
- For studio and gateway, restart = rebuild + run (code changes are picked up automatically)
- If you need to rebuild the Docker image itself (e.g., after changing system deps), use `make dev-rebuild-<component>` instead
- After restart, use `/dev-logs <component>` to verify the service started correctly
