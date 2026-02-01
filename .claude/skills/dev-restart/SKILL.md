---
name: dev-restart
description: Restart a specific component in the development environment. Use when a service needs to be restarted after config changes, code changes, or if it's misbehaving. Components: studio, gateway, frontend, postgres, plugins.
argument-hint: <component>
allowed-tools: Bash
---

# Dev Component Restart

Restart a specific component in the Docker Compose development environment.

## Arguments

The argument should be one of the following component names:

- `studio` - AI Studio control plane (Go backend)
- `gateway` - Microgateway data plane (requires full mode)
- `frontend` - React frontend dev server
- `postgres` - PostgreSQL database
- `plugins` - Plugin watcher/builder (requires full mode)

## Usage

When the user provides a component name, run:

```bash
make dev-rebuild-<component>
```

For example:
- `/dev-restart studio` runs `make dev-rebuild-studio`
- `/dev-restart gateway` runs `make dev-rebuild-gateway`
- `/dev-restart frontend` runs `make dev-rebuild-frontend`

This rebuilds and restarts the specified service with `docker compose up --build -d <service>`.

## Notes

- Gateway and plugins services are only available in full mode (`dev-start-full`)
- The rebuild includes building the container image, so code changes will be picked up
- For quick restarts without rebuild, you can use `cd dev && docker compose restart <service>` directly
- After restart, use `/dev-logs <component>` to verify the service started correctly
