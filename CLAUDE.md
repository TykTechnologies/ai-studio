# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Tyk AI Studio (Midsommar) is an open source AI management platform for secure, governed, and scalable AI integration. It provides an AI Gateway, Portal, and Chat Interface with support for multiple LLM vendors.

## Prerequisites

- **Docker** and **Docker Compose v2** (included with Docker Desktop)
- **Go 1.21+** (for local builds)
- **Node.js 18+** (for frontend builds)
- At least **8GB RAM** available for Docker

## Key Commands

### Development Environment (Docker-based, Recommended)

The recommended way to run the development environment is using Docker Compose with hot reloading:

```bash
# Start minimal env (Studio + Frontend + Postgres)
make dev

# Start full stack (+ Gateway + Plugin watcher)
make dev-full

# Start in detached mode (for automation/Claude)
make dev-start        # Minimal, returns immediately
make dev-start-full   # Full stack, returns immediately

# Stop all containers
make dev-down

# Check status
make dev-status

# View logs (non-blocking, last 100 lines)
make dev-tail-studio
make dev-tail-gateway
make dev-tail-frontend

# Clean and start fresh
make dev-clean
```

**Access points:**
- Frontend: http://localhost:3000
- Backend API: http://localhost:8080
- Gateway API: http://localhost:8081 (full mode only)

### Enterprise Edition

```bash
# Initialize enterprise submodule (one-time)
make init-enterprise

# Create secrets file (gitignored)
echo "TYK_AI_LICENSE=your-license-key" > dev/.env.secrets
echo "OPENAI_API_KEY=sk-..." >> dev/.env.secrets

# Start enterprise dev environment
make dev-ent
make dev-full-ent
```

### Building

```bash
# Build frontend first (required before Go build)
cd ui/admin-frontend && npm run build

# Build for local development (includes frontend build)
make build-local

# Build for all architectures
make build

# Clean build artifacts
make clean
```

### Testing

```bash
# Run all tests
go test ./...

# Run unified test suite
make test-all

# Run quick unit tests only
make test-quick

# Run tests with coverage
make test-ci
```

## Architecture Overview

The system follows a clean three-tier architecture:

- **Model Layer**: Data structures and database-level CRUD operations
- **Service Layer**: Business logic and data access to the model layer
- **API Layer**: REST interface to the service layer

### Hub-Spoke Architecture

Tyk AI Studio uses a hub-spoke (control plane / data plane) architecture:

**Hub (Control Plane) - AI Studio:**
- Central management of configuration, policies, and analytics
- Stores LLM configs, Apps, Tools, Datasources, Filters, Plugins
- Serves admin UI and receives heartbeats from edge instances
- Contains an **embedded gateway** for direct usage (chat, portal)

**Spoke (Data Plane) - Microgateway:**
- Standalone binary deployed at edge locations
- Processes AI requests locally with low latency
- Receives configuration from hub via gRPC (port 50051)
- Can operate independently if hub is temporarily unavailable
- Reports heartbeats with config checksums for sync tracking

**Key Difference:** The embedded gateway in AI Studio handles unified studio operations (chat interface, portal). The Microgateway is for distributed edge deployments with regional compliance and high availability needs.

**Communication:** Hub and spoke communicate via gRPC. The control plane generates configuration snapshots with SHA-256 checksums. Edge gateways report their loaded config checksum in heartbeats. When checksums mismatch, edges are marked as "Pending" or "Stale" and administrators can push configuration updates.

For detailed architecture, see `docs/site/docs/edge-gateways.md`.

### Key Directories

- `api/` - REST API handlers and routes
- `models/` - Database models and CRUD operations
- `services/` - Business logic layer
- `chat_session/` - Chat session management with queue interface
- `proxy/` - LLM proxy gateway
- `auth/` - Authentication and authorization
- `config/` - Configuration management
- `dev/` - Docker Compose development environment (see `dev/README.md`)
- `features/` - Feature specifications (check here for detailed documentation)
- `ui/admin-frontend/` - React-based admin interface
- `microgateway/` - Microgateway (data plane) for edge deployments
- `tests/` - Test utilities and data
- `pkg/plugin_sdk/` - Unified Plugin SDK for building plugins
- `examples/plugins/` - Example plugins demonstrating SDK capabilities
- `.claude/skills/` - Claude Code skills for dev environment management

### Core Components

1. **LLM Proxy**: Centralized gateway for LLM provider interactions with authentication, rate limiting, and policy enforcement
2. **Microgateway**: Distributed edge gateway for data plane processing (see Hub-Spoke Architecture above)
3. **Chat Sessions**: Stateful conversation management with tool integration and message queues
4. **Tool System**: External service integration via OpenAPI specifications
5. **User Management & RBAC**: Authentication, authorization, and role-based access control
6. **Budget Control**: Cost tracking and spending limits with real-time enforcement
7. **Filters**: Custom logic for content moderation and policy enforcement
8. **Plugin System**: Extensible plugin architecture for custom functionality (works in both Studio and Gateway)
9. **Hub-Spoke Communication**: gRPC-based config sync with checksum verification between control and data planes
10. **Events System**: Pub/sub with directional routing (Local/Up/Down) for cross-component communication

### Plugin Development

The system uses a **Unified Plugin SDK** (`pkg/plugin_sdk/`) that works in both AI Studio and Microgateway contexts with automatic runtime detection:

- **Single SDK**: One import, one API, works everywhere
- **11 Capabilities**: PreAuth, Auth, PostAuth, Response, DataCollection, CustomEndpoints, ObjectHooks, Agent, UIProvider, ConfigProvider, EdgePayload
- **Runtime-aware**: Use `ctx.Runtime` to detect Studio vs Gateway context
- **Type-safe**: Clean Go types, no manual proto handling
- **Context-aware**: Rich context with app/user/LLM metadata
- **Service access**: Built-in KV storage, logging, events, and management APIs

**Plugin Communication:** Uses HashiCorp's go-plugin framework. Plugins run as isolated processes communicating via gRPC, providing security (plugin crashes don't affect main platform) and language flexibility.

**Runtime-Specific Capabilities:**

| Capability | Studio | Gateway | Description |
|------------|--------|---------|-------------|
| PreAuth | ✓ | ✓ | Process before authentication |
| Auth | ✓ | ✓ | Custom authentication |
| PostAuth | ✓ | ✓ | Process after authentication (most common) |
| Response | ✓ | ✓ | Modify responses |
| DataCollection | ✓ | ✓ | Telemetry/analytics |
| CustomEndpoints | ✗ | ✓ | Custom HTTP endpoints under /plugins/{slug}/ |
| ObjectHooks | ✓ | ✗ | CRUD interception |
| Agent | ✓ | ✗ | Conversational AI |
| UIProvider | ✓ | ✗ | Dashboard extensions |
| ConfigProvider | ✓ | ✓ | JSON Schema configuration |
| EdgePayload | ✓ | ✗ | Receive data from gateways |

**Quick start for plugin development**:
```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *plugin_sdk.EnrichedRequest) (*plugin_sdk.Response, error) {
    // Check runtime if needed
    if ctx.Runtime == plugin_sdk.RuntimeStudio {
        // Studio-specific logic
    }
    return &plugin_sdk.Response{Modified: false}, nil
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

### Plugin Development Workflow

When developing plugins:

1. **Check existing examples first** in `examples/plugins/`:
   | Capability | Example | Path |
   |------------|---------|------|
   | **auth** | custom-auth-ui | `examples/plugins/studio/custom-auth-ui/server/` |
   | studio_ui | custom-auth-ui | `examples/plugins/studio/custom-auth-ui/server/` |
   | object_hooks | hook-test-plugin | `examples/plugins/studio/hook-test-plugin/` |
   | data_collector | file-analytics-collector | `examples/plugins/data-collectors/file-analytics-collector/` |
   | multi-phase | llm-rate-limiter-multiphase | `examples/plugins/studio/llm-rate-limiter-multiphase/` |

2. **Scaffold using `/plugin-new`**:
   ```
   /plugin-new my-plugin studio auth,studio_ui
   ```

3. **Verify method signatures** against `pkg/plugin_sdk/capabilities.go`

4. **Build**: `cd examples/plugins/studio/<name> && go build -o <name>`

### Proto Type Quick Reference

| Capability | Request Type | Response Type |
|------------|--------------|---------------|
| PreAuth | `*pb.EnrichedRequest` | `*pb.PluginResponse` |
| **Auth** | `*pb.AuthRequest` | `*pb.AuthResponse` |
| PostAuth | `*pb.EnrichedRequest` | `*pb.PluginResponse` |
| OnResponse | `*pb.HeadersRequest` | `*pb.HeadersResponse` |

**Auth types:**
- `pb.AuthRequest`: `Credential`, `AuthType`, `Request` (contains `Headers`), `Context`
- `pb.AuthResponse`: `Authenticated` (bool), `UserId`, `AppId`, `Claims`, `ErrorMessage`
- To reject: `return &pb.AuthResponse{Authenticated: false, ErrorMessage: "reason"}, nil`

See `pkg/plugin_sdk/README.md` and `docs/site/docs/plugins-overview.md` for complete documentation.

### Events System

The platform includes a directional event system for communication across the hub-spoke architecture:

| Direction | Constant | Description |
|-----------|----------|-------------|
| Local | `DirLocal` | Stays on local bus only, never forwarded |
| Up | `DirUp` | Edge → Control (gateway to Studio) |
| Down | `DirDown` | Control → Edge (Studio to gateways) |

**Usage in plugins:**
```go
// Edge plugin sending metrics to control
ctx.Services.Events().Publish(ctx, "metrics.report", data, plugin_sdk.DirUp)

// Control plugin pushing cache invalidation to edges
ctx.Services.Events().Publish(ctx, "cache.invalidate", payload, plugin_sdk.DirDown)

// Local event (stays on this node only)
ctx.Services.Events().Publish(ctx, "local.processing", data, plugin_sdk.DirLocal)
```

**System CRUD Events:** AI Studio emits built-in events when objects change:
- `system.llm.created`, `system.llm.updated`, `system.llm.deleted`
- `system.app.created`, `system.app.updated`, `system.app.deleted`
- `system.user.created`, `system.user.updated`, `system.user.deleted`

See `docs/site/docs/plugins-service-api.md` for event subscription patterns.

### Edge-to-Control Communication

Gateway plugins can send data back to AI Studio (control plane) for aggregation, analytics, or shared state:

```go
// Edge plugin sending data to control
pendingCount, err := sdk.SendToControlJSON(ctx, stats, "", metadata)

// Studio plugin receiving (implements EdgePayload capability)
func (p *Plugin) AcceptEdgePayload(ctx plugin_sdk.Context, payload *plugin_sdk.EdgePayload) (bool, error) {
    // Process payload from edge
    return true, nil
}
```

**Key Features:**
- Payloads queued to SQLite (survives gateway restarts)
- Batched delivery during heartbeats (every 30s)
- Plugin ID matching (edge plugin 42 → studio plugin 42)
- 1 MB max payload size, 24-hour retention

See `docs/site/docs/plugins-edge-to-control.md` for details.

## Development Guidelines

### Code Conventions
- Follow existing code style and patterns
- Use domain-driven design principles
- Maintain independence between domains
- Always check for existing implementations before creating new ones

### Testing Approach
- Split tasks into small testable milestones
- Run tests after each milestone
- Focus on edge cases and basic logic coverage
- Use feature specifications from `features/` folder for context
- When fixing tests, get user permission before fixing underlying bugs

### Important Build Requirements
- **For Docker dev environment**: Just run `make dev` - everything is handled automatically
- **For local builds**: Frontend must be built before Go binary (the app embeds frontend assets)
- Running `go build` without building frontend first will fail with empty directory errors
- Use `make build-local` for local builds (includes frontend build)

### Queue System Architecture
The chat session system uses an interface-driven message queue:
- `MessageQueue` interface with pluggable implementations (in-memory, NATS)
- Configuration via environment variables (`QUEUE_TYPE`, `NATS_*` settings)
- Reliable message delivery with blocking semantics and timeout support

### Configuration

**Development environment files** (in `dev/` directory):
- `.env` - AI Studio settings (auto-created from `.env.dev`)
- `.env.gateway` - Microgateway settings (auto-created from `.env.gateway.dev`)
- `.env.secrets` - Your secrets (license key, API keys) - **gitignored, create manually**

**Database support:**
- Development: PostgreSQL (via Docker)
- Production: PostgreSQL or SQLite

**Queue configuration:** in-memory (default) or NATS JetStream

**LLM provider credentials:** Add to `dev/.env.secrets`:
```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_AI_KEY=sk-ant-...
TYK_AI_LICENSE=your-license-key  # For enterprise edition
```

### Privacy and Security
- Privacy scoring system ensures data handling compatibility
- Tool and datasource privacy scores must not exceed LLM privacy scores
- Secrets management with AES encryption
- RBAC controls access to tools, datasources, and LLMs
- Plugin command validation blocks internal network access by default
- For local development, set `ALLOW_INTERNAL_NETWORK_ACCESS=true` to bypass internal IP restrictions

## Feature Documentation

Always check the `features/` directory for detailed specifications of system components:
- `features/README.md` - System architecture overview
- `features/Chat.md` - Chat session system
- `features/ChatQueue.md` - Message queue architecture
- `features/Tools.md` - Tool integration system
- `features/LLM.md` - LLM management
- `features/Proxy.md` - LLM proxy system
- `features/Branding.md` - UI branding and customization system
- And many more detailed specifications

## Architecture Documentation

For detailed technical specifications, see the docs in `docs/site/docs/`:

| Topic | File |
|-------|------|
| Hub-Spoke Architecture | `edge-gateways.md` |
| Core Concepts | `core-concepts.md` |
| LLM Proxy | `proxy.md` |
| Plugin System Overview | `plugins-overview.md` |
| Gateway Plugins | `plugins-microgateway.md` |
| Plugin SDK Reference | `plugins-sdk.md` |
| Plugin Service APIs | `plugins-service-api.md` |
| Edge-to-Control Communication | `plugins-edge-to-control.md` |
| Plugin Best Practices | `plugins-best-practices.md` |

## Branding Customization

The platform supports comprehensive white-labeling capabilities for administrators:

### Overview
- **Access**: Admin-only via `/admin/branding` page
- **Scope**: System-wide customization (applies to all users)
- **Storage**: Database for settings, filesystem for assets (`./data/branding`)
- **Environment**: Configure storage path via `BRANDING_STORAGE_PATH`

### Customizable Elements
- **Logo**: Header logo (PNG/JPG/SVG, max 2MB) - served via `/api/v1/branding/logo`
- **Favicon**: Browser icon (ICO/PNG, max 100KB) - served via `/api/v1/branding/favicon`
- **Colors**: Primary, secondary, and background colors (hex format)
- **Title**: Application title (max 50 chars)
- **CSS**: Custom CSS overrides (admin-only, use with caution)

### Architecture
- **Backend**: `models/branding_settings.go`, `services/branding_*.go`, `api/branding_handlers.go`
- **Frontend**: Dynamic theme generation in `ui/admin-frontend/src/admin/theme.js`
- **Admin UI**: Full management interface at `ui/admin-frontend/src/admin/pages/BrandingSettings.js`
- **Integration**: Branding config included in `/auth/config` for frontend bootstrap

### Key Features
- Runtime theme changes without rebuild
- File validation and size limits
- Reset to defaults functionality
- Container volume mounting support
- Live preview for colors

### API Endpoints
- Public: GET settings, logo, favicon
- Admin: PUT settings, POST logo/favicon upload, POST reset

For complete details, see `features/Branding.md`

## Database
- Default: SQLite (`midsommar.db`)
- Production: PostgreSQL support available
- Models auto-migrate on startup
- GORM for ORM operations

## API Structure
- RESTful endpoints following service/model architecture
- Swagger documentation available
- Authentication required for most endpoints
- Role-based access control throughout

## Development Best Practices
- Respect domain boundaries and reduce cross-domain dependencies
- When adding test coverage, focus on meaningful error cases and logic paths
- Before finishing work, update relevant feature specifications in `features/`
- Always preserve existing comments and code structure
- Avoid big refactorings unless explicitly requested

## Claude Code Skills

This repository includes Claude Code skills for managing the development environment. These are available when using Claude Code in this project:

### `/dev-env` - Manage Dev Environment
Start, stop, or check status of the Docker-based dev environment.

```
/dev-env start           # Start minimal env (detached)
/dev-env start-full      # Start full stack (detached)
/dev-env stop            # Stop all containers
/dev-env status          # Check container status
/dev-env clean           # Stop and remove all data
```

### `/dev-restart` - Restart Components
Restart specific components after changes.

```
/dev-restart studio      # Restart AI Studio
/dev-restart gateway     # Restart Microgateway
/dev-restart frontend    # Restart React frontend
```

### `/dev-logs` - Fetch Logs
Get logs from components for debugging.

```
/dev-logs studio         # Last 100 lines of studio logs
/dev-logs gateway        # Last 100 lines of gateway logs
/dev-logs all            # All service logs
/dev-logs studio --lines 50  # Custom line count
```

### `/dev-test` - Run Tests
Run unit tests, integration tests, or the full test suite.

```
/dev-test quick          # Fast unit tests only
/dev-test all            # All tests (unit + integration)
/dev-test ci             # CI tests with coverage
/dev-test studio         # AI Studio unit tests
/dev-test gateway        # Microgateway unit tests
/dev-test frontend       # Frontend Jest tests
/dev-test all --verbose  # With verbose output
/dev-test all --coverage # With coverage report
```

### `/plugin-new` - Scaffold New Plugin
Create a new plugin using the plugin-scaffold tool.

```
/plugin-new my-limiter studio                           # Basic studio plugin
/plugin-new my-cache studio post_auth,on_response,studio_ui  # With UI
/plugin-new my-filter gateway post_auth,on_response     # Gateway plugin
/plugin-new my-assistant agent                          # Conversational agent
/plugin-new my-exporter data-collector                  # Telemetry collector
```

**Plugin types:** `studio`, `gateway`, `agent`, `data-collector`

**Capabilities:** `pre_auth`, `auth`, `post_auth`, `on_response`, `studio_ui`, `object_hooks`, `data_collector`

When lost or not sure where things are (this is a large repo), make sure to use pwd to orient the command line.