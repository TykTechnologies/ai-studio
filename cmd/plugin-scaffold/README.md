# Plugin Scaffold Tool

A CLI tool for scaffolding new Tyk AI Studio plugins with support for multiple capabilities.

## Quick Start

```bash
# Build the scaffolding tool
make tools

# Show help
make plugin-help

# Create a basic studio plugin
make plugin-new NAME=my-limiter TYPE=studio

# Create a multi-capability plugin with UI
make plugin-new NAME=my-cache TYPE=studio CAPABILITIES=studio_ui,post_auth,on_response
```

## Usage

```bash
make plugin-new NAME=<name> TYPE=<type> [CAPABILITIES=<cap1,cap2,...>]
```

### Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `NAME` | Yes | Plugin name in kebab-case (e.g., `my-rate-limiter`) |
| `TYPE` | Yes | Plugin type: `studio`, `gateway`, `agent`, `data-collector` |
| `CAPABILITIES` | No | Comma-separated list of capabilities |

### Plugin Types

| Type | Description | Default Capability | Output Directory |
|------|-------------|-------------------|------------------|
| `studio` | AI Studio control plane plugin | `post_auth` | `examples/plugins/studio/<name>/` |
| `gateway` | Microgateway data plane plugin | `post_auth` | `examples/plugins/gateway/<name>/` |
| `agent` | Conversational AI agent | `agent` | `examples/plugins/studio/<name>/server/` |
| `data-collector` | Telemetry and analytics | `data_collector` | `examples/plugins/data-collectors/<name>/` |

### Available Capabilities

| Capability | Studio | Gateway | Description |
|------------|--------|---------|-------------|
| `pre_auth` | ✓ | ✓ | Process requests before authentication |
| `auth` | ✓ | ✓ | Custom authentication handler |
| `post_auth` | ✓ | ✓ | Process requests after authentication |
| `on_response` | ✓ | ✓ | Modify response headers and body |
| `studio_ui` | ✓ | - | Dashboard UI with WebComponents |
| `object_hooks` | ✓ | - | Intercept CRUD operations |
| `data_collector` | ✓ | ✓ | Telemetry collection handlers |

## Examples

### Basic Studio Plugin

Creates a plugin with `post_auth` capability:

```bash
make plugin-new NAME=my-limiter TYPE=studio
```

Generated files:
- `main.go` - Plugin implementation with `HandlePostAuth()`
- `go.mod` - Go module with SDK dependency
- `manifest.json` - Plugin metadata
- `config.schema.json` - Configuration schema for Admin UI
- `README.md` - Plugin documentation

### Multi-Capability Plugin with UI

Creates a plugin suitable for caching, rate limiting, or similar use cases:

```bash
make plugin-new NAME=my-cache TYPE=studio CAPABILITIES=studio_ui,post_auth,on_response
```

Additional generated files:
- `ui/webc/dashboard.js` - WebComponent for the Admin UI
- `assets/icon.svg` - Plugin icon for sidebar

The plugin will have:
- `HandlePostAuth()` - Intercept requests
- `OnBeforeWrite()` - Intercept responses
- `HandleRPC()` - Handle UI API calls
- `GetAsset()` / `ListAssets()` - Serve UI assets

### Gateway Plugin

Creates a lightweight plugin for the microgateway:

```bash
make plugin-new NAME=my-filter TYPE=gateway CAPABILITIES=post_auth,on_response
```

### Conversational Agent

Creates an agent plugin with LLM streaming support:

```bash
make plugin-new NAME=my-assistant TYPE=agent
```

Includes:
- `HandleAgentMessage()` - Stream responses to chat
- `OnSessionReady()` - Service API warmup
- LLM calling via `ai_studio_sdk.CallLLM()`

### Data Collector

Creates a telemetry collection plugin:

```bash
make plugin-new NAME=my-exporter TYPE=data-collector
```

Includes handlers for:
- `HandleProxyLog()` - Request/response logs
- `HandleAnalytics()` - Analytics events
- `HandleBudgetUsage()` - Budget tracking

## After Scaffolding

1. **Build the plugin:**
   ```bash
   cd examples/plugins/studio/my-plugin
   go mod tidy
   go build -o my-plugin
   ```

2. **Start the dev environment:**
   ```bash
   make dev-full
   ```
   The plugin watcher will auto-rebuild on file changes.

3. **Register in Admin UI:**
   - Open http://localhost:3000
   - Go to Admin > Plugins > Register Plugin
   - Command: `file:///app/examples/plugins/studio/my-plugin/my-plugin`

4. **Reload after changes:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/plugins/{plugin_id}/reload
   ```

## Make Commands

| Command | Description |
|---------|-------------|
| `make tools` | Build the plugin-scaffold tool |
| `make plugin-help` | Show detailed scaffolding help |
| `make plugin-new NAME=x TYPE=y` | Scaffold a new plugin |
| `make plugins` | Build all example plugins |

## Architecture

```
cmd/plugin-scaffold/
├── main.go           # CLI entry point, flag parsing
├── scaffold.go       # Core scaffolding logic
├── capabilities.go   # Capability definitions and validation
├── templates.go      # All embedded templates
└── README.md         # This file
```

### How It Works

1. **Parse input** - Validates name (kebab-case), type, and capabilities
2. **Build config** - Creates `PluginConfig` with computed fields (struct name, display name, flags)
3. **Select templates** - Chooses templates based on plugin type
4. **Process templates** - Uses Go's `text/template` with conditional blocks
5. **Generate files** - Writes files to output directory
6. **Add UI assets** - If `studio_ui` capability, adds WebComponent and icon

### Template Variables

| Variable | Example | Description |
|----------|---------|-------------|
| `{{.Name}}` | `my-rate-limiter` | Plugin name (kebab-case) |
| `{{.StructName}}` | `MyRateLimiter` | Go struct name (PascalCase) |
| `{{.DisplayName}}` | `My Rate Limiter` | Human-readable name |
| `{{.Type}}` | `studio` | Plugin type |
| `{{.Capabilities}}` | `[post_auth, on_response]` | List of capabilities |
| `{{.PrimaryHook}}` | `post_auth` | First non-UI capability |
| `{{.HasUI}}` | `true` | Has `studio_ui` capability |
| `{{.HasPostAuth}}` | `true` | Has `post_auth` capability |
| `{{.HasOnResponse}}` | `true` | Has `on_response` capability |

### Conditional Template Blocks

Templates use Go's `text/template` syntax:

```go
{{if .HasUI}}
//go:embed ui assets manifest.json
var embeddedAssets embed.FS
{{end}}

{{if .HasPostAuth}}
func (p *Plugin) HandlePostAuth(...) { ... }
{{end}}
```

## Direct CLI Usage

The tool can also be used directly:

```bash
# Build the tool (preferred)
make tools

# Or build manually
go build -o bin/plugin-scaffold ./cmd/plugin-scaffold

# Run directly
./bin/plugin-scaffold -name=my-plugin -type=studio -capabilities=post_auth,on_response

# Show help
./bin/plugin-scaffold -help
```

## Extending

To add a new capability:

1. Add to `AllCapabilities` in `capabilities.go`
2. Add flag field to `PluginConfig` in `scaffold.go`
3. Set the flag in `NewPluginConfig()`
4. Add template conditional in `templates.go`

To add a new plugin type:

1. Add to `ValidTypes` in `scaffold.go`
2. Add case in `NewPluginConfig()` for default capabilities and output directory
3. Add `generate<Type>Plugin()` function
4. Add templates in the `templates` map
