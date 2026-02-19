---
name: plugin-new
description: Scaffold a new Tyk AI Studio plugin with the plugin-scaffold tool. Use when the user wants to create a new plugin for Studio, Gateway, or as an Agent.
argument-hint: <name> <type> [capabilities...]
allowed-tools: Bash
---

# Scaffold New Plugin

Create a new Tyk AI Studio plugin using the plugin-scaffold tool.

## Arguments

**name** (required): Plugin name in kebab-case (e.g., `my-rate-limiter`)

**type** (required): Plugin type - one of:
- `studio` - AI Studio control plane plugin
- `gateway` - Microgateway data plane plugin
- `agent` - Conversational AI agent
- `data-collector` - Telemetry and analytics plugin

**capabilities** (optional): Comma-separated list of capabilities to include

## Available Capabilities

| Capability | Studio | Gateway | Description |
|------------|--------|---------|-------------|
| `pre_auth` | ✓ | ✓ | Process requests before authentication |
| `auth` | ✓ | ✓ | Custom authentication handler |
| `post_auth` | ✓ | ✓ | Process requests after authentication (default) |
| `on_response` | ✓ | ✓ | Modify response headers and body |
| `studio_ui` | ✓ | - | Dashboard UI with WebComponents |
| `object_hooks` | ✓ | - | Intercept CRUD operations |
| `data_collector` | ✓ | ✓ | Telemetry collection handlers |

## Reference Examples

Before scaffolding, review existing implementations for patterns:

| Capability | Example Plugin | Path |
|------------|----------------|------|
| `auth` | **custom-auth-ui** | `examples/plugins/studio/custom-auth-ui/server/main.go` |
| `studio_ui` | custom-auth-ui | `examples/plugins/studio/custom-auth-ui/server/main.go` |
| `object_hooks` | hook-test-plugin | `examples/plugins/studio/hook-test-plugin/main.go` |
| `data_collector` | file-analytics-collector | `examples/plugins/data-collectors/file-analytics-collector/main.go` |
| `post_auth` + `on_response` | llm-rate-limiter-multiphase | `examples/plugins/studio/llm-rate-limiter-multiphase/main.go` |

## Usage

First, ensure the scaffolding tool is built:
```bash
make tools
```

Then scaffold the plugin:
```bash
make plugin-new NAME=<name> TYPE=<type> [CAPABILITIES=<cap1,cap2>]
```

## Examples

| Command | Description |
|---------|-------------|
| `/plugin-new my-limiter studio` | Basic studio plugin with post_auth |
| `/plugin-new my-cache studio post_auth,on_response,studio_ui` | Multi-capability with UI |
| `/plugin-new my-filter gateway post_auth,on_response` | Gateway plugin |
| `/plugin-new my-assistant agent` | Conversational agent |
| `/plugin-new my-exporter data-collector` | Telemetry collector |

## Make Commands

```bash
# Build the scaffolding tool (run once)
make tools

# Show scaffolding help
make plugin-help

# Scaffold a new plugin
make plugin-new NAME=my-plugin TYPE=studio
make plugin-new NAME=my-plugin TYPE=studio CAPABILITIES=post_auth,on_response,studio_ui

# Build all plugins
make plugins
```

## Output Directories

| Type | Output Directory |
|------|------------------|
| `studio` | `examples/plugins/studio/<name>/` |
| `gateway` | `examples/plugins/gateway/<name>/` |
| `agent` | `examples/plugins/studio/<name>/server/` |
| `data-collector` | `examples/plugins/data-collectors/<name>/` |

## After Scaffolding

1. **Build the plugin:**
   ```bash
   cd examples/plugins/studio/<name>
   go mod tidy
   go build -o <name>
   ```

2. **Start dev environment with plugin watcher:**
   ```bash
   make dev-full
   ```

3. **Register in Admin UI:**
   - Open http://localhost:3000
   - Go to Admin > Plugins > Register Plugin
   - Command: `file:///app/examples/plugins/studio/<name>/<name>`

## Post-Scaffold Verification

After scaffolding, verify:

1. **Method signatures match SDK**: Check `pkg/plugin_sdk/capabilities.go`
2. **Proto types are correct**: Check `proto/plugin.proto` if unsure
3. **Code compiles**: `go build -o <name>` in the plugin directory

## Notes

- The tool will be auto-built if not present when using `make plugin-new`
- Use `make plugin-help` for the full scaffolding documentation
- Plugin names should be in kebab-case (e.g., `my-rate-limiter`)
- See `cmd/plugin-scaffold/README.md` for detailed documentation
