# AI Gateway Library

**Status**: ✅ Implemented  
**Version**: v1.0  
**Package**: `pkg/aigateway`

## Overview & Purpose

The AI Gateway Library provides a simple interface to use the Midsommar AI Gateway as an importable library in standalone applications. This allows developers to create independent microproxy services or integrate AI gateway functionality into existing applications without requiring the full Midsommar platform.

## Key Features

### Core Functionality
- **LLM Proxying**: Secure reverse proxy for multiple LLM providers (OpenAI, Anthropic, Google AI, Vertex, etc.)
- **Authentication & Authorization**: Token-based authentication with app-level access control
- **Budget Enforcement**: Real-time budget tracking and limits
- **Request Filtering**: JavaScript-based policy enforcement
- **Analytics**: Usage tracking and cost calculation
- **Hot Reloading**: Dynamic configuration updates without restart

### Advanced Features
- **Tool Integration**: REST API tool execution and MCP (Model Context Protocol) support
- **Data Source Proxying**: Vector search capabilities for RAG applications
- **Streaming Support**: Real-time streaming responses from LLMs
- **Vendor Abstraction**: Support for multiple LLM providers through unified interface

## Library Interface

```go
type Gateway interface {
    // Start starts the gateway as an HTTP server on the configured port
    Start() error

    // Stop gracefully stops the gateway server
    Stop(ctx context.Context) error

    // Handler returns an http.Handler for integration with existing servers
    Handler() http.Handler

    // Reload reloads the gateway configuration (LLMs, datasources, filters)
    Reload() error
}
```

## Usage Patterns

### 1. Standalone Microproxy
```go
gateway := aigateway.New(service, &proxy.Config{Port: 9090}, budgetService)
gateway.Start() // Blocks and serves on :9090
```

### 2. Integration with Existing HTTP Server
```go
gateway := aigateway.New(service, &proxy.Config{}, budgetService)
http.Handle("/ai/", http.StripPrefix("/ai", gateway.Handler()))
```

### 3. Framework Integration
- **Gin**: `router.Any("/ai/*path", gin.WrapH(gateway.Handler()))`
- **Gorilla Mux**: `router.PathPrefix("/ai/").Handler(gateway.Handler())`
- **Chi**: `router.Mount("/ai", gateway.Handler())`

## Supported Endpoints

The library provides the same endpoints as the main Midsommar proxy:

### LLM Endpoints
- `POST /llm/rest/{llmSlug}/{path}` - REST API calls to LLMs
- `POST /llm/stream/{llmSlug}/{path}` - Streaming calls to LLMs

### Tool Endpoints
- `GET|POST|PUT|DELETE /tools/{toolSlug}` - Tool operation calls
- `POST /tools/{toolSlug}/mcp` - MCP StreamableHTTP transport
- `GET /tools/{toolSlug}/mcp/sse` - MCP SSE transport
- `POST /tools/{toolSlug}/mcp/message` - MCP message endpoint

### Data Source Endpoints
- `POST /datasource/{dsSlug}` - Vector search queries

### Metadata Endpoints
- `GET /.well-known/oauth-protected-resource` - OAuth2 resource metadata

## Dependencies

The library requires:
- **Database**: PostgreSQL database with Midsommar schema
- **Services Layer**: `services.Service` for business logic and database operations
- **Budget Service**: `services.BudgetService` for budget enforcement and tracking

## Implementation Details

### Package Structure
```
pkg/aigateway/
├── gateway.go          # Main Gateway interface and implementation
├── config.go           # Configuration provider interfaces (future)
├── gateway_test.go     # Unit tests
├── example_test.go     # Usage examples
└── README.md           # Comprehensive documentation
```

### Architecture
- **Wrapper Approach**: Thin wrapper around existing `proxy.Proxy` 
- **Zero Changes**: Current Midsommar usage remains identical
- **Clean API**: Simple interface with 4 methods
- **Thread Safety**: All operations are thread-safe

### New Components Added
1. **Handler Method**: Added `Handler()` method to `proxy.Proxy` for HTTP server integration
2. **Gateway Interface**: Clean abstraction over proxy functionality
3. **Wrapper Implementation**: `gateway` struct that implements the interface

## Use Cases

1. **Standalone Microproxy**: Deploy as dedicated AI gateway service
2. **API Gateway Integration**: Add AI capabilities to existing API gateways
3. **Application Integration**: Embed AI gateway into web applications
4. **Development/Testing**: Local AI proxy for development environments
5. **Multi-tenant SaaS**: AI gateway with tenant isolation and billing

## Testing

- ✅ Unit tests for interface compliance
- ✅ Compilation tests for all components
- ✅ Example programs demonstrating usage
- ✅ Integration with existing test suite

Coverage: Interface and basic functionality tested. Integration tests require database setup.

## Documentation

- ✅ Comprehensive README with examples
- ✅ API documentation with Go doc comments  
- ✅ Integration examples for popular frameworks
- ✅ Complete example programs

## Installation & Usage

```bash
# Import in your Go application
import "github.com/TykTechnologies/midsommar/v2/pkg/aigateway"

# Run example
go run examples/standalone-gateway/main.go

# Run tests
go test ./pkg/aigateway
```

## Future Enhancements

### Planned Features
- **Custom Configuration Providers**: Support for file-based, API-based configurations
- **Service Interface**: Abstract the services dependency for easier testing/mocking
- **Enhanced Testing**: Integration tests with database mocking
- **Plugin System**: Extensible middleware and vendor support

### Considerations
- Configuration provider interface defined but not fully implemented (requires service interface changes)
- Current implementation maintains full compatibility with existing Midsommar usage
- Library can be extended without breaking changes

## Examples

### Basic Standalone Service
See `examples/standalone-gateway/main.go` for a complete example.

### Integration Examples
The `pkg/aigateway/README.md` contains comprehensive integration examples for:
- Standard library HTTP server
- Gin framework
- Gorilla Mux
- Chi router
- Graceful shutdown patterns

## Backwards Compatibility

- ✅ No changes to existing Midsommar code
- ✅ Proxy functionality remains identical
- ✅ All existing tests pass
- ✅ Library is additive-only enhancement

## Success Criteria

- [x] Create importable library interface
- [x] Support standalone server usage
- [x] Support HTTP handler integration
- [x] Maintain all existing functionality
- [x] Provide comprehensive documentation
- [x] Include working examples
- [x] Pass all tests
- [x] Zero breaking changes
