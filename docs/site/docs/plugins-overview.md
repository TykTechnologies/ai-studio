# Plugin System Overview

Tyk AI Studio's plugin system enables powerful extensibility across the entire platform through three distinct plugin types. Built on [HashiCorp's go-plugin](https://github.com/hashicorp/go-plugin) framework, plugins run as isolated processes with gRPC communication, providing security and fault tolerance.

## Plugin Types at a Glance

| Feature | Microgateway Plugins | AI Studio UI Plugins | AI Studio Agent Plugins |
|---------|---------------------|---------------------|------------------------|
| **Purpose** | Proxy middleware & data collection | Dashboard UI extensions | Conversational agents |
| **Hook Points** | 5 request/response hooks | UI slots & routes | Agent message handling |
| **Technology** | Go interfaces | WebComponents + gRPC | Streaming gRPC |
| **Service API** | None | Full access | Full access |
| **Use Cases** | Auth, filtering, analytics | Custom dashboards, admin tools | Chat-based AI agents |
| **SDK** | `microgateway/plugins/sdk` | `pkg/ai_studio_sdk` | `pkg/ai_studio_sdk` |

## Microgateway Plugins

Microgateway plugins provide middleware hooks in the LLM proxy request/response pipeline. These plugins enable custom authentication, request modification, response filtering, and data collection.

### Hook Types

1. **pre_auth**: Execute before authentication
2. **auth**: Custom authentication logic
3. **post_auth**: Execute after authentication
4. **on_response**: Modify LLM responses
5. **data_collection**: Collect proxy logs, analytics, and budget data

### Common Use Cases

- Custom authentication and authorization
- Request/response transformation
- Content filtering and policy enforcement
- Data export to external systems (Elasticsearch, ClickHouse, etc.)
- Custom rate limiting logic
- Request enrichment with metadata

### Example: Custom Authentication

```go
type CustomAuthPlugin struct {
    validToken string
}

func (p *CustomAuthPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypeAuth
}

func (p *CustomAuthPlugin) Authenticate(ctx context.Context, req *sdk.AuthRequest,
    pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {

    if req.Credential == p.validToken {
        return &sdk.AuthResponse{
            Authenticated: true,
            UserID:        "plugin-user",
            AppID:         "plugin-app",
        }, nil
    }

    return &sdk.AuthResponse{
        Authenticated: false,
        ErrorMessage:  "Invalid credentials",
    }, nil
}

func main() {
    plugin := &CustomAuthPlugin{validToken: "secret"}
    sdk.ServePlugin(plugin)
}
```

[Learn more →]([plugins-microgateway](https://docs.claude.com/en/docs/plugins-microgateway))

## AI Studio UI Plugins

AI Studio UI plugins extend the dashboard with custom WebComponents, adding new pages, sidebars, and interactive features to the admin interface.

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

[Learn more →]([plugins-studio-ui](https://docs.claude.com/en/docs/plugins-studio-ui))

## AI Studio Agent Plugins

Agent plugins enable conversational AI experiences in the Chat Interface. These plugins can wrap LLMs, add custom logic, integrate external services, and create sophisticated multi-turn conversations.

### Capabilities

- **Streaming Responses**: Server-streaming gRPC for real-time responses
- **LLM Integration**: Call managed LLMs via SDK with automatic authentication
- **Tool & Datasource Access**: Execute tools and query datasources
- **Conversation History**: Access full conversation context
- **Custom Configuration**: Per-agent config with JSON schema validation

### Example: Echo Agent

```go
type EchoAgentPlugin struct {
    serviceAPI mgmt.AIStudioManagementServiceClient
    prefix     string
    suffix     string
}

func (p *EchoAgentPlugin) HandleAgentMessage(
    req *pb.AgentMessageRequest,
    stream pb.PluginService_HandleAgentMessageServer) error {

    // Call LLM via SDK
    llmStream, err := ai_studio_sdk.CallLLM(
        stream.Context(),
        req.AvailableLlms[0].Id,
        req.AvailableLlms[0].DefaultModel,
        messages,
        0.7, 1000, nil, false,
    )

    // Stream wrapped response
    var llmContent string
    for {
        resp, err := llmStream.Recv()
        if err == io.EOF {
            break
        }
        llmContent += resp.Content
    }

    wrappedContent := fmt.Sprintf("%s %s %s", p.prefix, llmContent, p.suffix)

    stream.Send(&pb.AgentMessageChunk{
        Type:    pb.AgentMessageChunk_CONTENT,
        Content: wrappedContent,
        IsFinal: false,
    })

    stream.Send(&pb.AgentMessageChunk{
        Type:    pb.AgentMessageChunk_DONE,
        Content: "completed",
        IsFinal: true,
    })

    return nil
}

func main() {
    plugin := &EchoAgentPlugin{}
    ai_studio_sdk.ServeAgentPlugin(plugin)
}
```

[Learn more →]([plugins-studio-agent](https://docs.claude.com/en/docs/plugins-studio-agent))

## Plugin Architecture

### Process Isolation

Plugins run as separate processes, communicating with the main platform via gRPC. This provides:

- **Security**: Plugin crashes don't affect the main platform
- **Language Flexibility**: Plugins can be written in any language with gRPC support
- **Resource Management**: Plugins can be restarted independently
- **Version Independence**: Update plugins without platform restarts

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
3. Review example plugins in `examples/plugins/` and `microgateway/plugins/examples/`
4. Use the SDK to implement required interfaces
5. Build and test with `file://` deployment
6. Deploy with `grpc://` or `oci://` for production

### SDK Installation

```go
import (
    // Microgateway plugins
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"

    // AI Studio plugins
    "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)
```

## Next Steps

- [Microgateway Plugins Guide]([plugins-microgateway](https://docs.claude.com/en/docs/plugins-microgateway))
- [AI Studio UI Plugins Guide]([plugins-studio-ui](https://docs.claude.com/en/docs/plugins-studio-ui))
- [AI Studio Agent Plugins Guide]([plugins-studio-agent](https://docs.claude.com/en/docs/plugins-studio-agent))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
- [Service API Reference]([plugins-service-api](https://docs.claude.com/en/docs/plugins-service-api))
