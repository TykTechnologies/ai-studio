# Plugin Development Workflow

This guide covers the fastest way to develop and test plugins locally. The key insight: **use `file://` paths and the reload API for instant iteration without reinstalling the plugin**.

## Quick Start: 5-Minute Setup

### 1. Create Your Plugin

```bash
mkdir my-plugin && cd my-plugin
go mod init my-plugin
```

Create `main.go`:

```go
package main

import (
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
    pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin("my-plugin", "1.0.0", "My test plugin"),
    }
}

func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    ctx.Services.Logger().Info("Plugin initialized!")
    return nil
}

func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    ctx.Services.Logger().Info("Request intercepted", "path", req.Request.Path)
    return &pb.PluginResponse{Modified: false}, nil
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

### 2. Build and Get Absolute Path

```bash
go build -o my-plugin .
PLUGIN_PATH="$(pwd)/my-plugin"
echo $PLUGIN_PATH  # e.g., /Users/you/projects/my-plugin/my-plugin
```

### 3. Register Plugin Once

```bash
# Get your auth token (login first)
TOKEN="your-auth-token"

curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Plugin",
    "slug": "my-plugin",
    "command": "file://'$PLUGIN_PATH'",
    "hook_type": "post_auth",
    "plugin_type": "gateway",
    "is_active": true
  }'
```

**Save the plugin ID from the response!**

### 4. Development Loop

Now you can iterate without reinstalling:

```bash
# Make changes to main.go, then:
go build -o my-plugin . && curl -X POST http://localhost:3000/api/v1/plugins/{PLUGIN_ID}/reload \
  -H "Authorization: Bearer $TOKEN"
```

That's it! Your changes are live instantly.

---

## The Development Loop Explained

```
┌─────────────────────────────────────────────────────────────────┐
│                    PLUGIN DEVELOPMENT LOOP                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   1. Edit Code                                                  │
│      └── main.go, manifest.json, etc.                          │
│                           ↓                                     │
│   2. Build                                                      │
│      └── go build -o my-plugin .                               │
│                           ↓                                     │
│   3. Reload (ONE command!)                                      │
│      └── curl -X POST .../plugins/{id}/reload                  │
│                           ↓                                     │
│   4. Test                                                       │
│      └── Make requests, check logs, verify behavior            │
│                           ↓                                     │
│   5. Repeat from Step 1                                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### When Do You Need to Reinstall?

You **only** need to update the plugin registration (not just reload) when:

| Change | Reload Sufficient? | Action Needed |
|--------|-------------------|---------------|
| Code logic changes | ✅ Yes | Just rebuild + reload |
| Adding new log statements | ✅ Yes | Just rebuild + reload |
| Configuration handling changes | ✅ Yes | Just rebuild + reload |
| Adding new capabilities (interfaces) | ✅ Yes | Just rebuild + reload |
| **Manifest permission changes** | ❌ No | Update plugin or recreate |
| **Hook type changes** | ❌ No | Update plugin `hook_type` field |
| **Plugin type changes** | ❌ No | Recreate plugin |

---

## Development Environment Setup

### Option 1: Local AI Studio (Recommended for Speed)

Run AI Studio locally for fastest iteration:

```bash
# Start AI Studio
make start-dev

# Set environment for plugin development
export ALLOW_INTERNAL_NETWORK_ACCESS=true
```

### Option 2: Docker Compose

Mount your plugin directory:

```yaml
# docker-compose.override.yml
services:
  ai-studio:
    volumes:
      - ./my-plugins:/plugins
    environment:
      - ALLOW_INTERNAL_NETWORK_ACCESS=true
```

Then use container paths:
```bash
"command": "file:///plugins/my-plugin"
```

### Option 3: Remote AI Studio

For remote instances, use `grpc://` deployment during development:

```bash
# Run plugin as gRPC server locally
go run main.go --grpc-server :50051

# Register with remote instance
"command": "grpc://your-local-ip:50051"
```

---

## Helper Scripts

### `dev.sh` - One-Command Development

Create this script in your plugin directory:

```bash
#!/bin/bash
# dev.sh - Build and reload plugin in one command

PLUGIN_ID="${PLUGIN_ID:-your-plugin-id}"
API_URL="${API_URL:-http://localhost:3000}"
TOKEN="${TOKEN:-your-token}"

echo "Building..."
go build -o my-plugin . || exit 1

echo "Reloading plugin $PLUGIN_ID..."
curl -s -X POST "$API_URL/api/v1/plugins/$PLUGIN_ID/reload" \
  -H "Authorization: Bearer $TOKEN" | jq .

echo "Done! Check logs with: tail -f /path/to/ai-studio.log | grep my-plugin"
```

Usage:
```bash
chmod +x dev.sh
export PLUGIN_ID=123 TOKEN=xxx
./dev.sh
```

### `watch.sh` - Auto-Rebuild on Save

```bash
#!/bin/bash
# watch.sh - Watch for changes and auto-reload

PLUGIN_ID="${PLUGIN_ID:-your-plugin-id}"
API_URL="${API_URL:-http://localhost:3000}"
TOKEN="${TOKEN:-your-token}"

echo "Watching for changes... (Ctrl+C to stop)"

# Using fswatch (macOS: brew install fswatch)
fswatch -o *.go | while read; do
  echo "Change detected, rebuilding..."
  go build -o my-plugin . && \
  curl -s -X POST "$API_URL/api/v1/plugins/$PLUGIN_ID/reload" \
    -H "Authorization: Bearer $TOKEN" > /dev/null && \
  echo "✓ Reloaded at $(date +%H:%M:%S)"
done
```

---

## Debugging Tips

### View Plugin Logs

Plugin logs go to AI Studio's log output:

```bash
# If running locally
tail -f /path/to/logs | grep "my-plugin"

# Docker
docker logs -f ai-studio 2>&1 | grep "my-plugin"

# Filter by plugin name
... | grep "\[my-plugin\]"
```

### Add Debug Logging

```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Debug logging - shows in AI Studio logs
    ctx.Services.Logger().Debug("Request details",
        "method", req.Request.Method,
        "path", req.Request.Path,
        "headers", req.Request.Headers,
        "body_length", len(req.Request.Body),
    )

    // Add more detailed logging during development
    ctx.Services.Logger().Info("Processing request",
        "app_id", ctx.AppID,
        "user_id", ctx.UserID,
        "runtime", ctx.Runtime,
    )

    return &pb.PluginResponse{Modified: false}, nil
}
```

### Check Plugin Status

```bash
# Get plugin status
curl http://localhost:3000/api/v1/plugins/$PLUGIN_ID/status \
  -H "Authorization: Bearer $TOKEN" | jq .

# List all loaded plugins
curl http://localhost:3000/api/v1/plugins/loaded \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### Test Plugin Standalone

Before registering, test your plugin runs:

```bash
# This should start and wait for gRPC connection
./my-plugin

# If it crashes immediately, check for:
# - Missing dependencies
# - Invalid manifest
# - Initialization errors
```

---

## Common Issues and Solutions

### Plugin Not Loading After Reload

**Symptoms**: Reload returns success but changes aren't reflected

**Solutions**:
```bash
# 1. Verify build succeeded
go build -o my-plugin . && echo "Build OK"

# 2. Check the binary was actually updated
ls -la my-plugin

# 3. Verify reload endpoint returned success
curl -X POST .../reload -H "..." | jq .status

# 4. Check AI Studio logs for errors
tail -100 /path/to/logs | grep -i error
```

### "Permission Denied" on Reload

**Symptoms**: Reload fails with permission error

**Solutions**:
```bash
# Make binary executable
chmod +x my-plugin

# Check file ownership (Docker)
ls -la my-plugin
# May need: chown 1000:1000 my-plugin (for Docker user)
```

### Changes Not Taking Effect

**Symptoms**: Old behavior persists after reload

**Solutions**:
```bash
# 1. Verify you're building to the right path
which my-plugin  # vs ./my-plugin

# 2. Check plugin command points to your binary
curl http://localhost:3000/api/v1/plugins/$PLUGIN_ID | jq .command

# 3. Force deactivate and reactivate
curl -X PATCH .../plugins/$PLUGIN_ID -d '{"is_active": false}'
curl -X PATCH .../plugins/$PLUGIN_ID -d '{"is_active": true, "load_immediately": true}'
```

### Manifest Changes Not Applied

**Symptoms**: New UI components or permissions not appearing

**Solutions**:
```bash
# Manifest requires explicit re-parsing after reload
curl -X POST http://localhost:3000/api/v1/plugins/$PLUGIN_ID/manifest/parse \
  -H "Authorization: Bearer $TOKEN"

# Or update the plugin entirely
curl -X PATCH http://localhost:3000/api/v1/plugins/$PLUGIN_ID \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"manifest": {...}}'
```

---

## Project Structure Recommendations

```
my-plugin/
├── main.go              # Plugin entry point
├── plugin.go            # Core plugin logic
├── config.go            # Configuration handling
├── manifest.json        # Plugin manifest (embed with //go:embed)
├── config.schema.json   # Config JSON schema (embed)
├── dev.sh               # Development helper script
├── Makefile             # Build targets
└── ui/                  # UI assets (if UI plugin)
    ├── index.html
    └── bundle.js
```

### Makefile Example

```makefile
PLUGIN_ID ?= your-plugin-id
API_URL ?= http://localhost:3000
TOKEN ?= your-token

.PHONY: build reload dev clean

build:
	go build -o my-plugin .

reload:
	curl -s -X POST $(API_URL)/api/v1/plugins/$(PLUGIN_ID)/reload \
		-H "Authorization: Bearer $(TOKEN)"

dev: build reload
	@echo "Plugin reloaded!"

clean:
	rm -f my-plugin

# First-time setup
install: build
	curl -X POST $(API_URL)/api/v1/plugins \
		-H "Authorization: Bearer $(TOKEN)" \
		-H "Content-Type: application/json" \
		-d '{"name":"My Plugin","slug":"my-plugin","command":"file://$(PWD)/my-plugin","hook_type":"post_auth","plugin_type":"gateway","is_active":true}'
```

Usage:
```bash
make dev  # Build and reload in one command
```

---

## Reference: Plugin API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/plugins` | POST | Create new plugin |
| `/api/v1/plugins/{id}` | PATCH | Update plugin config |
| `/api/v1/plugins/{id}/reload` | POST | **Reload plugin** (main dev endpoint) |
| `/api/v1/plugins/{id}/status` | GET | Check if plugin is loaded/healthy |
| `/api/v1/plugins/{id}/manifest/parse` | POST | Re-parse manifest |
| `/api/v1/plugins/loaded` | GET | List all loaded plugins |

---

## Next Steps

- **[Plugin SDK Reference](plugins-sdk.md)** - Core SDK documentation
- **[Plugin Examples](plugins-examples.md)** - Working examples to learn from
- **[Plugin Deployment](plugins-deployment.md)** - Production deployment options
- **[Plugin Best Practices](plugins-best-practices.md)** - Production-ready patterns
