# Plugin Examples Catalog

Comprehensive reference of working plugin examples in the Tyk AI Studio repository. All examples use the **Unified Plugin SDK** (`pkg/plugin_sdk`) and demonstrate real-world patterns for different plugin capabilities.

## AI Studio Plugins

### Echo Agent

**Path**: [`examples/plugins/studio/echo-agent/`](../../../examples/plugins/studio/echo-agent/)

**Capabilities**: Agent

**Description**: Simple conversational agent that wraps LLM responses with custom prefix/suffix formatting. Demonstrates basic agent implementation with streaming responses and LLM integration.

**Key Features**:
- Streaming server-side responses
- LLM integration via `ai_studio_sdk.CallLLM()`
- Per-agent configuration (prefix, suffix, metadata)
- Fallback echo mode when no LLM available
- JSON schema configuration

**Use Cases**:
- Learning agent plugin basics
- Response formatting and wrapping
- Custom agent configuration

**Complexity**: Beginner

---

### LLM Validator

**Path**: [`examples/plugins/studio/llm-validator/`](../../../examples/plugins/studio/llm-validator/)

**Capabilities**: Object Hooks (before_create, before_update)

**Description**: Validates LLM configurations before they're saved to the database. Enforces HTTPS endpoints, blocks specific vendors, validates privacy scores, and requires descriptions.

**Key Features**:
- Object hook registration for LLM objects
- `before_create` and `before_update` hooks
- Configurable validation rules
- Block operations with rejection reasons
- Add validation metadata to approved objects
- Priority ordering (runs early in chain)

**Use Cases**:
- Enforcing security policies (HTTPS-only endpoints)
- Vendor compliance and blocking
- Privacy score validation
- Required field enforcement

**Complexity**: Intermediate

---

### LLM Rate Limiter (Multi-Phase)

**Path**: [`examples/plugins/studio/llm-rate-limiter-multiphase/`](../../../examples/plugins/studio/llm-rate-limiter-multiphase/)

**Capabilities**: PostAuth, Response, UI Provider

**Description**: Comprehensive example showing a **multi-capability plugin** that implements rate limiting across the entire request/response lifecycle with a custom UI dashboard.

**Key Features**:
- **PostAuth**: Check rate limits before proxying to LLM
- **Response**: Update counters after successful response
- **UI Provider**: Custom dashboard showing rate limit status
- KV storage for rate limit state
- Per-app and per-user rate limiting
- WebComponent-based UI
- Custom RPC methods for UI interaction

**Use Cases**:
- Advanced rate limiting beyond built-in budget controls
- Multi-phase request processing
- Building plugins with custom UIs
- Stateful plugin logic with KV storage

**Complexity**: Advanced

---

### Hook Test Plugin

**Path**: [`examples/plugins/studio/hook-test-plugin/`](../../../examples/plugins/studio/hook-test-plugin/)

**Capabilities**: Object Hooks (all types)

**Description**: Comprehensive testing plugin demonstrating all object hook types (before/after create/update/delete) for all supported objects (llm, datasource, tool, user).

**Key Features**:
- Registers hooks for all 4 object types
- Demonstrates all 6 hook types per object
- Shows blocking vs non-blocking hooks
- Metadata enrichment patterns
- Priority ordering examples
- Extensive logging for debugging

**Use Cases**:
- Learning object hook patterns
- Testing hook behavior
- Understanding hook execution order
- Reference implementation for all hooks

**Complexity**: Intermediate

---

### Service API Test

**Path**: [`examples/plugins/studio/service-api-test/`](../../../examples/plugins/studio/service-api-test/)

**Capabilities**: PostAuth (for testing purposes)

**Description**: Comprehensive test plugin demonstrating all Studio Service API operations including LLMs, Tools, Apps, Datasources, Filters, Tags, and Plugins management.

**Key Features**:
- Complete Studio Services API coverage
- CRUD operations for all object types
- Broker ID initialization
- Error handling patterns
- Service API authentication

**Use Cases**:
- Learning Service API usage
- Testing Service API operations
- Reference for API method signatures
- Understanding broker connection setup

**Complexity**: Advanced

---

### Custom Auth UI

**Path**: [`examples/plugins/studio/custom-auth-ui/`](../../../examples/plugins/studio/custom-auth-ui/)

**Capabilities**: UI Provider, Auth

**Description**: UI plugin with custom authentication extension. Shows how to add custom pages, sidebars, and authentication flows to the AI Studio dashboard.

**Key Features**:
- Custom sidebar integration
- Route registration
- WebComponent implementation
- Asset serving (JS/CSS bundles)
- Custom authentication integration

**Use Cases**:
- Extending dashboard UI
- Custom authentication flows
- Adding new admin pages
- WebComponent integration

**Complexity**: Advanced

---

## Gateway Plugins

### Request Enricher

**Path**: [`examples/plugins/gateway/request_enricher/`](../../../examples/plugins/gateway/request_enricher/)

**Capabilities**: PostAuth

**Description**: Enriches authenticated requests with additional metadata and instructions before proxying to LLM. Most common gateway plugin pattern.

**Key Features**:
- PostAuth hook implementation
- Header injection
- Request body modification
- Configurable enrichment via plugin config
- Context-aware enrichment (app_id, user_id)

**Use Cases**:
- Adding custom headers
- Injecting additional instructions
- Request metadata enrichment
- Per-app request modification

**Complexity**: Beginner

---

### Response Modifier

**Path**: [`examples/plugins/gateway/response_modifier/`](../../../examples/plugins/gateway/response_modifier/)

**Capabilities**: Response (OnBeforeWriteHeaders, OnBeforeWrite)

**Description**: Modifies LLM responses before returning to client. Demonstrates two-phase response processing (headers then body).

**Key Features**:
- Header modification (OnBeforeWriteHeaders)
- Body transformation (OnBeforeWrite)
- Response filtering
- Content injection
- Streaming response handling

**Use Cases**:
- Content filtering and moderation
- Response formatting
- Adding custom response headers
- Injecting metadata into responses

**Complexity**: Intermediate

---

### Message Modifier

**Path**: [`examples/plugins/gateway/message_modifier/`](../../../examples/plugins/gateway/message_modifier/)

**Capabilities**: PostAuth

**Description**: Similar to request enricher but focuses on modifying the message content specifically for chat/completion requests.

**Key Features**:
- Message content transformation
- Chat-specific request handling
- System message injection
- Context-aware modifications

**Use Cases**:
- Chat message preprocessing
- System prompt injection
- Message format standardization

**Complexity**: Beginner

---

### Elasticsearch Collector

**Path**: [`examples/plugins/gateway/elasticsearch_collector/`](../../../examples/plugins/gateway/elasticsearch_collector/)

**Capabilities**: DataCollector

**Description**: Exports proxy logs, analytics, and budget data to Elasticsearch for external analysis and monitoring.

**Key Features**:
- HandleProxyLog implementation
- HandleAnalytics implementation
- HandleBudgetUsage implementation
- Elasticsearch bulk API integration
- Async export with error handling
- Configurable index names

**Use Cases**:
- Exporting logs to data warehouses
- Real-time analytics pipelines
- External monitoring systems
- Custom dashboards (Kibana, Grafana)

**Complexity**: Advanced

---

### Gateway Service Test

**Path**: [`examples/plugins/gateway/gateway-service-test/`](../../../examples/plugins/gateway/gateway-service-test/)

**Capabilities**: PostAuth (for testing purposes)

**Description**: Demonstrates all Gateway-specific Service API operations including app management, LLM info, budget status, and credential validation.

**Key Features**:
- GetApp, ListApps
- GetLLM, ListLLMs
- GetBudgetStatus
- GetModelPrice
- ValidateCredential
- Runtime detection (Gateway vs Studio)

**Use Cases**:
- Learning Gateway Services API
- Budget-aware request handling
- App configuration access
- Testing Gateway-specific features

**Complexity**: Intermediate

---

### File Data Collectors (Unified SDK)

**Path**: [`examples/plugins/unified/data-collectors/`](../../../examples/plugins/unified/data-collectors/)

**Capabilities**: DataCollector

**Examples**:
- `file-proxy-collector/` - Exports proxy logs to JSONL files
- `file-analytics-collector/` - Exports analytics to CSV or JSONL files
- `file-budget-collector/` - Exports budget data to CSV or JSONL files with optional aggregation

**Description**: Simple file-based data collectors for testing and local development. Write telemetry data (proxy logs, analytics, budget usage) to files instead of or in addition to the database.

**Features**:
- Multiple output formats (CSV, JSONL)
- Daily log rotation
- Optional aggregate summaries (budget collector)
- Configurable output directories
- Environment variable support
- Replace or supplement database storage

**Use Cases**:
- Local development and debugging
- Understanding DataCollector interface
- Simple log export without external dependencies
- Custom analytics pipelines
- Reduced database load in high-throughput scenarios
- Backup and archival of telemetry data

**Configuration Example**:
```yaml
data_collection_plugins:
  - name: "analytics-files"
    path: "./examples/plugins/unified/data-collectors/file-analytics-collector/file_analytics_collector"
    enabled: true
    priority: 200
    replace_database: false
    hook_types:
      - "analytics"
    config:
      output_directory: "./data/collected/analytics"
      enabled: "true"
      format: "jsonl"  # or "csv"
```

**Complexity**: Beginner

**Migration Status**: ✅ Migrated to unified SDK. Old examples remain available at [`examples/plugins/gateway/file_*_collector/`](../../../examples/plugins/gateway/) for reference.

---

## Example Organization

### By Capability

| Capability | Examples |
|------------|----------|
| **Agent** | echo-agent |
| **Object Hooks** | llm-validator, hook-test-plugin |
| **PostAuth** | request_enricher, message_modifier, service-api-test, gateway-service-test |
| **Response** | response_modifier, llm-rate-limiter-multiphase |
| **DataCollector** | elasticsearch_collector, file-analytics-collector, file-budget-collector, file-proxy-collector |
| **UI Provider** | llm-rate-limiter-multiphase, custom-auth-ui |
| **Multi-Capability** | llm-rate-limiter-multiphase (PostAuth + Response + UI) |

### By Runtime

| Runtime | Examples |
|---------|----------|
| **Studio Only** | echo-agent, llm-validator, hook-test-plugin, llm-rate-limiter-multiphase, custom-auth-ui, service-api-test |
| **Gateway Only** | request_enricher, response_modifier, message_modifier, elasticsearch_collector, gateway-service-test, file-analytics-collector, file-budget-collector, file-proxy-collector |
| **Both** | (Any plugin can work in both if designed correctly) |

### By Complexity

| Level | Examples |
|-------|----------|
| **Beginner** | echo-agent, request_enricher, message_modifier, file-analytics-collector, file-budget-collector, file-proxy-collector |
| **Intermediate** | llm-validator, hook-test-plugin, response_modifier, gateway-service-test |
| **Advanced** | llm-rate-limiter-multiphase, service-api-test, elasticsearch_collector, custom-auth-ui |

## Running Examples

### Build an Example

```bash
cd examples/plugins/studio/echo-agent
go build -o echo-agent server/main.go
```

### Deploy to AI Studio

```bash
# Create plugin
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Echo Agent",
    "slug": "echo-agent",
    "command": "file:///path/to/echo-agent",
    "hook_type": "agent",
    "plugin_type": "agent",
    "is_active": true
  }'
```

### Deploy to Gateway

Configure in gateway config:

```yaml
plugins:
  - name: request-enricher
    command: file:///path/to/request-enricher
    enabled: true
```

## Common Patterns

### 1. Basic Plugin Structure

All examples follow this pattern:

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin("name", "version", "desc"),
    }
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

### 2. Configuration Handling

```go
func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    // Parse plugin-specific config
    p.apiKey = config["api_key"]

    // Extract broker ID for Service API access (Studio plugins)
    if brokerIDStr, ok := config["_service_broker_id"]; ok {
        var brokerID uint32
        fmt.Sscanf(brokerIDStr, "%d", &brokerID)
        ai_studio_sdk.SetServiceBrokerID(brokerID)
    }

    return nil
}
```

### 3. Universal Services

```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Logging
    ctx.Services.Logger().Info("Processing", "app_id", ctx.AppID)

    // KV storage
    data, _ := ctx.Services.KV().Read(ctx, "key")
    ctx.Services.KV().Write(ctx, "key", []byte("value"))

    return &pb.PluginResponse{Modified: false}, nil
}
```

### 4. Runtime Detection

```go
if ctx.Runtime == plugin_sdk.RuntimeStudio {
    // Studio-specific code
    llms, _ := ctx.Services.Studio().ListLLMs(ctx, 1, 10)
} else if ctx.Runtime == plugin_sdk.RuntimeGateway {
    // Gateway-specific code
    app, _ := ctx.Services.Gateway().GetApp(ctx, ctx.AppID)
}
```

## Learning Path

### 1. Start with Basics
- **request_enricher**: Learn PostAuth hooks
- **echo-agent**: Learn agent basics and streaming

### 2. Explore Services
- **gateway-service-test**: Gateway Services API
- **service-api-test**: Studio Services API

### 3. Advanced Patterns
- **llm-validator**: Object hooks and validation
- **response_modifier**: Two-phase response handling
- **elasticsearch_collector**: Data collection and export

### 4. Multi-Capability
- **llm-rate-limiter-multiphase**: Combining capabilities with UI

## Next Steps

- **[Plugin SDK Reference](plugins-sdk.md)** - Core SDK documentation
- **[Service API Reference](plugins-service-api.md)** - All Service API operations
- **[Microgateway Plugins Guide](plugins-microgateway.md)** - Gateway-specific patterns
- **[AI Studio Agent Plugins Guide](plugins-studio-agent.md)** - Build conversational agents
- **[Object Hooks Guide](plugins-object-hooks.md)** - Intercept CRUD operations
- **[Plugin Best Practices](plugins-best-practices.md)** - Production-ready patterns
