# Plugin System Overview

Tyk AI Studio's plugin system enables powerful extensibility across the entire platform through a **Unified Plugin SDK**. Built on [HashiCorp's go-plugin](https://github.com/hashicorp/go-plugin) framework, plugins run as isolated processes with gRPC communication, providing security and fault tolerance.

## Unified Plugin SDK

All plugins now use a single SDK (`pkg/plugin_sdk`) that works seamlessly in both AI Studio and Microgateway contexts. The SDK automatically detects the runtime environment and provides appropriate capabilities.

### Key Features

- **Single Import**: One SDK works everywhere
- **10 Plugin Capabilities**: Mix and match to build exactly what you need
- **Runtime Detection**: Automatic AI Studio vs Microgateway detection
- **Service API Access**: Built-in KV storage, logging, and management APIs
- **Type-Safe**: Clean Go interfaces, no manual proto handling

## Plugin Capabilities

Plugins implement one or more of these 10 capabilities:

| Capability | Where It Works | Purpose | Common Use Cases |
|------------|----------------|---------|------------------|
| **Pre-Auth** | Studio + Gateway | Process before authentication | IP filtering, request validation |
| **Auth** | Studio + Gateway | Custom authentication | OAuth, API keys, JWT validation |
| **Post-Auth** | Studio + Gateway | Process after authentication | Request enrichment, policy enforcement |
| **Response** | Studio + Gateway | Modify responses | Content filtering, header injection |
| **Data Collection** | Studio + Gateway | Collect telemetry | Export to Elasticsearch, ClickHouse |
| **Object Hooks** | Studio only | Intercept CRUD operations | Validation, approval workflows |
| **Agent** | Studio only | Conversational AI | Chat-based agents, LLM wrapping |
| **UI Provider** | Studio only | Dashboard extensions | Custom dashboards, admin tools |
| **Config Provider** | Studio + Gateway | Provide JSON Schema config | Dynamic configuration |
| **Manifest Provider** | Gateway only | Plugin manifest | Gateway-only plugins |

### Multi-Capability Plugins

A single plugin can implement multiple capabilities. For example, a rate limiter might:
- Implement **Post-Auth** to check limits before request
- Implement **Response** to update counters after response
- Implement **UI Provider** to show rate limit dashboard

## Plugin Types Overview

While all plugins use the unified SDK, they generally fall into three categories based on their primary use case:

## 1. Microgateway Plugins

Microgateway plugins provide middleware hooks in the LLM proxy request/response pipeline using the unified SDK.

### Common Capabilities

- **PreAuthHandler**: Execute before authentication
- **AuthHandler**: Custom authentication logic
- **PostAuthHandler**: Execute after authentication (most common)
- **ResponseHandler**: Modify LLM responses
- **DataCollector**: Collect proxy logs, analytics, and budget data

### Common Use Cases

- Custom authentication and authorization
- Request/response transformation
- Content filtering and policy enforcement
- Data export to external systems (Elasticsearch, ClickHouse, etc.)
- Custom rate limiting logic
- Request enrichment with metadata

### Example: Request Enrichment

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type EnrichmentPlugin struct {
    plugin_sdk.BasePlugin
}

func NewEnrichmentPlugin() *EnrichmentPlugin {
    return &EnrichmentPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "request-enricher",
            "1.0.0",
            "Enriches requests with metadata",
        ),
    }
}

// Implement PostAuthHandler capability
func (p *EnrichmentPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Add custom metadata
    req.Headers["X-Request-ID"] = generateID()
    req.Headers["X-App-Name"] = ctx.AppName

    ctx.Services.Logger().Info("Request enriched",
        "app_id", ctx.AppID,
        "request_id", req.Headers["X-Request-ID"],
    )

    return &pb.PluginResponse{
        Modified: true,
        Request:  req,
    }, nil
}

func main() {
    plugin_sdk.Serve(NewEnrichmentPlugin())
}
```

[Learn more →](plugins-microgateway.md)

## 2. AI Studio UI Plugins

AI Studio UI plugins extend the dashboard with custom WebComponents, adding new pages, sidebars, and interactive features to the admin interface. These also use the unified SDK and can combine UI capabilities with middleware hooks.

### Capabilities

- **UI Slots**: Register components in sidebar, routes, settings pages
- **WebComponents**: Custom HTML elements with full JavaScript framework support
- **Service API Access**: Call platform APIs to manage LLMs, apps, tools, etc.
- **Asset Serving**: Serve JS/CSS bundles and static assets
- **RPC Methods**: Define custom RPC endpoints for UI interactions

### Example: Sidebar Extension

```json
{
  "id": "com.example.my-plugin",
  "name": "My Plugin",
  "version": "1.0.0",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": ["llms.read", "apps.read", "kv.readwrite"]
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "My Plugin",
        "icon": "settings",
        "route": "/my-plugin"
      }
    ],
    "routes": [
      {
        "path": "/my-plugin",
        "component": "my-plugin-view"
      }
    ]
  }
}
```

[Learn more →](plugins-studio-ui.md)

## 3. AI Studio Agent Plugins

> **Experimental Feature**: Agent plugins are currently experimental. The API and behavior may change in future releases.

Agent plugins enable conversational AI experiences in the Chat Interface using the unified SDK. These plugins can wrap LLMs, add custom logic, integrate external services, and create sophisticated multi-turn conversations.

### Architecture

Agent plugins follow the **Plugin → Agent Object → App Object** pattern:

- **Plugin**: Long-running gRPC plugin implementing `HandleAgentMessage`
- **Agent Object**: Configuration binding the plugin to an App with access controls
- **App Object**: Provides LLMs, tools, datasources, credentials, and budget control

Agents appear in the **Chat** section of the AI Portal alongside managed chats. Users access agents based on group membership.

### Capabilities

- **Streaming Responses**: Server-streaming gRPC for real-time responses
- **LLM Integration**: Call managed LLMs via SDK with automatic authentication
- **Tool & Datasource Access**: Execute tools and query datasources
- **Conversation History**: Access full conversation context
- **Custom Configuration**: Per-agent config with JSON schema validation

### Example: Echo Agent

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type EchoAgentPlugin struct {
    plugin_sdk.BasePlugin
    prefix string
    suffix string
}

func NewEchoAgentPlugin() *EchoAgentPlugin {
    return &EchoAgentPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "echo-agent",
            "1.0.0",
            "Wraps LLM responses with prefix/suffix",
        ),
        prefix: "[AGENT] ",
        suffix: " [/AGENT]",
    }
}

// Implement AgentPlugin capability
func (p *EchoAgentPlugin) HandleAgentMessage(
    req *pb.AgentMessageRequest,
    stream pb.PluginService_HandleAgentMessageServer) error {

    // Call LLM via Service API
    llmStream, err := ai_studio_sdk.CallLLM(
        stream.Context(),
        req.AvailableLlms[0].Id,
        req.AvailableLlms[0].DefaultModel,
        messages,
        0.7, 1000, nil, false,
    )

    // Receive and wrap response
    var llmContent string
    for {
        resp, err := llmStream.Recv()
        if err == io.EOF {
            break
        }
        llmContent += resp.Content
    }

    wrappedContent := fmt.Sprintf("%s%s%s", p.prefix, llmContent, p.suffix)

    // Stream back to user
    stream.Send(&pb.AgentMessageChunk{
        Type:    pb.AgentMessageChunk_CONTENT,
        Content: wrappedContent,
    })

    stream.Send(&pb.AgentMessageChunk{
        Type:    pb.AgentMessageChunk_DONE,
        IsFinal: true,
    })

    return nil
}

func main() {
    plugin_sdk.Serve(NewEchoAgentPlugin())
}
```

[Learn more →](plugins-studio-agent.md)

## 4. Object Hooks Plugins

**Object Hooks** are a powerful AI Studio-only capability that allows plugins to intercept and control CRUD operations on key objects before they reach the database. This is particularly useful for validation, approval workflows, and policy enforcement.

### Supported Objects

Object hooks work with four core object types:
- **llm**: LLM provider configurations
- **datasource**: Data source connections
- **tool**: External tool definitions
- **user**: User accounts

### Hook Types

Each object supports six lifecycle hooks:
- **before_create**: Validate/modify before creation (can block)
- **after_create**: Notification after creation
- **before_update**: Validate/modify before update (can block)
- **after_update**: Notification after update
- **before_delete**: Prevent deletion if needed (can block)
- **after_delete**: Cleanup after deletion

### Common Use Cases

- **Validation**: Enforce HTTPS endpoints, naming conventions, privacy scores
- **Approval Workflows**: Require manager approval for datasource creation
- **Policy Enforcement**: Block vendors, check compliance requirements
- **External Integration**: Sync with LDAP, ticketing systems, CMDBs
- **Audit Trails**: Log all changes with detailed metadata
- **Auto-Enrichment**: Add default values, tags, or metadata

### Example: LLM Validator

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type LLMValidatorPlugin struct {
    plugin_sdk.BasePlugin
}

// Register hooks
func (p *LLMValidatorPlugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
    return []*pb.ObjectHookRegistration{
        {
            ObjectType: "llm",
            HookTypes:  []string{"before_create", "before_update"},
            Priority:   10, // Lower runs first
        },
    }, nil
}

// Handle hook invocation
func (p *LLMValidatorPlugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
    var llm LLM
    json.Unmarshal([]byte(req.ObjectJson), &llm)

    // Enforce HTTPS
    if !strings.HasPrefix(llm.APIEndpoint, "https://") {
        return &pb.ObjectHookResponse{
            AllowOperation:  false,
            RejectionReason: "API endpoint must use HTTPS",
        }, nil
    }

    // Allow with validation metadata
    return &pb.ObjectHookResponse{
        AllowOperation: true,
        PluginMetadata: map[string]string{
            "validated_by": "llm-validator",
            "validated_at": time.Now().Format(time.RFC3339),
        },
    }, nil
}

func main() {
    plugin_sdk.Serve(NewLLMValidatorPlugin())
}
```

[Learn more →](plugins-object-hooks.md)

## Plugin Architecture

### Monolithic Plugin Architecture

A single plugin binary can run in **both** AI Studio and the Microgateway, so long as the requisite interfaces are implemented and the requirements are clear in the manifest file. This means one plugin can provide UI extensions in Studio, middleware hooks in the gateway, and even use the event bus to communicate between its Studio and gateway components in near-real-time.

### Scheduled Tasks

Studio plugins can register **scheduled tasks** — periodic calls from the host to the plugin on a configurable interval. This enables long-running background jobs such as analytics analysis, compliance checks, log scanning, or data synchronization.

### Process Isolation

Plugins run as separate processes, communicating with the main platform via gRPC. This provides:

- **Security**: Plugin crashes don't affect the main platform
- **Language Flexibility**: While the gRPC protocol theoretically supports any language, **only Go plugins have been tested** in production. If you plan to write plugins in another language, expect to do additional integration work.
- **Resource Management**: Plugins can be restarted independently
- **Version Independence**: Update plugins without platform restarts

Plugins can also run as standalone gRPC services in a sidecar or elsewhere on the network, rather than as local sub-processes. See [Plugin Deployment](./plugins-deployment.md) for details.

### Communication Flow

```
┌─────────────────────┐
│   AI Studio Host    │
│  (Main Process)     │
└──────────┬──────────┘
           │ go-plugin
           │ gRPC
           ├──────────────────────┬──────────────────────┐
           │                      │                      │
┌──────────▼──────────┐ ┌─────────▼────────┐ ┌─────────▼────────┐
│  Microgateway       │ │   UI Plugin      │ │  Agent Plugin    │
│  Plugin Process     │ │   Process        │ │  Process         │
│                     │ │                  │ │                  │
│  - Pre/Post Auth    │ │  - WebComponents │ │  - HandleMessage │
│  - Data Collection  │ │  - Service API   │ │  - LLM Calls     │
│  - Request Filter   │ │  - RPC Methods   │ │  - Tool Execute  │
└─────────────────────┘ └──────────────────┘ └──────────────────┘
```

### Service API (AI Studio Plugins Only)

AI Studio UI and Agent plugins can access the Service API via a reverse gRPC broker connection:

```
┌─────────────────────┐           ┌─────────────────────┐
│   Plugin Process    │◄─────────►│   AI Studio Host    │
│                     │           │                     │
│  Plugin Service     │           │  Service API        │
│  (Host→Plugin)      │  Broker   │  (Plugin→Host)      │
│                     │  Pattern  │                     │
│  - HandleMessage    │           │  - CallLLM          │
│  - GetAsset         │           │  - ExecuteTool      │
│  - Call (RPC)       │           │  - QueryDatasource  │
└─────────────────────┘           └─────────────────────┘
```

The Service API provides 100+ gRPC operations for managing LLMs, apps, tools, datasources, analytics, and more. Access is controlled via permission scopes declared in the plugin manifest.

[Learn more about Service API →]([plugins-service-api](https://docs.claude.com/en/docs/plugins-service-api))

## Deployment Options

Plugins support three deployment methods:

### file://

Local filesystem plugins for development and testing:

```
file:///path/to/plugin-binary
```

### grpc://

Remote gRPC plugins running as network services:

```
grpc://plugin-host:50051
```

### oci://

Container registry plugins (OCI artifacts):

```
oci://registry.example.com/plugins/my-plugin:v1.0.0
```

[Learn more about deployment →]([plugins-deployment](https://docs.claude.com/en/docs/plugins-deployment))

## Permissions and Scopes

AI Studio plugins declare required permissions in their manifest:

```json
{
  "permissions": {
    "services": [
      "llms.proxy",      // Call LLMs via proxy
      "llms.read",       // List and read LLM configs
      "tools.execute",   // Execute tools
      "datasources.query", // Query datasources
      "kv.readwrite",    // Key-value storage
      "analytics.read"   // Read analytics data
    ]
  }
}
```

Permissions are validated when plugins call the Service API. The platform enforces least-privilege access based on declared scopes.

[Learn more about manifests →]([plugins-manifests](https://docs.claude.com/en/docs/plugins-manifests))

## Getting Started

### Choose Your Plugin Type

1. **Need to intercept/modify LLM requests?** → Microgateway Plugin
2. **Building dashboard UI features?** → AI Studio UI Plugin
3. **Creating conversational AI experiences?** → AI Studio Agent Plugin

### Development Workflow

1. Choose your plugin type
2. Read the specific plugin guide
3. Review example plugins in `examples/plugins/` and `community/plugins/`
4. Use the SDK to implement required interfaces
5. Build and test with `file://` deployment (see [Development Workflow Guide](plugins-development-workflow.md) for fast iteration)
6. Deploy with `grpc://` or `oci://` for production

**Pro tip**: Use the reload API (`POST /api/v1/plugins/{id}/reload`) to test changes instantly without reinstalling. See the [Development Workflow Guide](plugins-development-workflow.md) for the fastest iteration loop.

### SDK Installation

All plugins use the unified SDK:

```bash
go get github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk
```

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

**Note**: If you have existing plugins using the old SDKs (`microgateway/plugins/sdk` or `pkg/ai_studio_sdk`), see the [Migration Guide](plugins-migration-guide.md) for upgrade instructions.

## Next Steps

- **[Development Workflow Guide](plugins-development-workflow.md)** - Fast iteration with file:// and reload API
- **[Plugin SDK Reference](plugins-sdk.md)** - Complete SDK documentation
- **[Plugin Examples](plugins-examples.md)** - Browse working examples (including production-ready community plugins)
- **[Object Hooks Guide](plugins-object-hooks.md)** - Intercept CRUD operations
- **[Microgateway Plugins Guide](plugins-microgateway.md)** - Gateway-specific patterns
- **[AI Studio UI Plugins Guide](plugins-studio-ui.md)** - Build plugin UIs
- **[AI Studio Agent Plugins Guide](plugins-studio-agent.md)** - Build conversational agents
- **[Service API Reference](plugins-service-api.md)** - Complete API documentation
- **[Plugin Deployment](plugins-deployment.md)** - Production deployment options
- **[Migration Guide](plugins-migration-guide.md)** - Upgrade from old SDKs
