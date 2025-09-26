# AI Studio Plugin Service Access Guide

## Quick Start

This guide shows how AI Studio plugins can securely access core management services (Analytics, LLMs, Tools, Apps, Plugins) via gRPC.

## 1. Plugin Manifest Declaration

Declare required service scopes in your plugin manifest:

```json
{
  "id": "com.example.my-plugin",
  "version": "1.0.0",
  "name": "My Plugin",
  "permissions": {
    "services": [
      "analytics.read",
      "plugins.read",
      "llms.read",
      "tools.read",
      "tools.operations"
    ]
  }
}
```

### Available Service Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `plugins.read` | Read plugin information | ListPlugins, GetPlugin |
| `plugins.write` | Modify plugins | CreatePlugin, UpdatePlugin |
| `plugins.config` | Update plugin configurations | UpdatePluginConfig |
| `llms.read` | Read LLM information | ListLLMs, GetLLM, GetLLMPlugins |
| `llms.write` | Modify LLMs | CreateLLM, UpdateLLM |
| `llms.config` | Update LLM configurations | UpdateLLMConfig |
| `analytics.read` | Access analytics data | GetAnalyticsSummary, GetUsageStatistics |
| `apps.read` | Read app information | ListApps, GetApp |
| `apps.write` | Modify apps | CreateApp, UpdateApp |
| `tools.read` | Read tool information | ListTools, GetTool |
| `tools.operations` | Get tool operations | GetToolOperations |
| `tools.call` | Execute tool operations | CallToolOperation |

## 2. Plugin Implementation

### Setup gRPC Client Connection

```go
package main

import (
    mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

type MyPlugin struct {
    pb.UnimplementedPluginServiceServer
    managementClient mgmtpb.AIStudioManagementServiceClient
    pluginID        uint32
}

func (p *MyPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
    // Extract plugin ID from config
    if pluginIDStr, ok := req.Config["plugin_id"]; ok {
        if pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32); err == nil {
            p.pluginID = uint32(pluginID)
        }
    }

    // Connect to AI Studio Management Service
    managementEndpoint := "localhost:50052" // Configurable
    if endpoint, ok := req.Config["management_endpoint"]; ok {
        managementEndpoint = endpoint
    }

    conn, err := grpc.Dial(managementEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, fmt.Errorf("failed to connect to management service: %v", err)
    }

    p.managementClient = mgmtpb.NewAIStudioManagementServiceClient(conn)

    return &pb.InitResponse{Success: true}, nil
}
```

### Access Service Methods

```go
func (p *MyPlugin) getAnalyticsData(ctx context.Context) error {
    // Create authenticated context
    pluginCtx := &mgmtpb.PluginContext{
        PluginId:    p.pluginID,
        MethodScope: "analytics.read",
    }

    // Call analytics service
    resp, err := p.managementClient.GetAnalyticsSummary(ctx, &mgmtpb.GetAnalyticsSummaryRequest{
        Context:   pluginCtx,
        TimeRange: "24h",
    })
    if err != nil {
        return fmt.Errorf("failed to get analytics: %v", err)
    }

    // Use analytics data
    log.Printf("Total requests: %d", resp.TotalRequests)
    log.Printf("Total cost: %.2f %s", resp.TotalCost, resp.Currency)

    return nil
}

func (p *MyPlugin) listAvailableTools(ctx context.Context) error {
    pluginCtx := &mgmtpb.PluginContext{
        PluginId:    p.pluginID,
        MethodScope: "tools.read",
    }

    resp, err := p.managementClient.ListTools(ctx, &mgmtpb.ListToolsRequest{
        Context: pluginCtx,
        Page:    1,
        Limit:   20,
    })
    if err != nil {
        return fmt.Errorf("failed to list tools: %v", err)
    }

    for _, tool := range resp.Tools {
        log.Printf("Tool: %s (%s)", tool.Name, tool.ToolType)
    }

    return nil
}
```

### Embed Manifest with go:embed

```go
//go:embed plugin.manifest.json
var manifestFile []byte

func (p *MyPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
    return &pb.GetManifestResponse{
        Success:      true,
        ManifestJson: string(manifestFile),
    }, nil
}
```

## 3. Admin Authorization Workflow

### Current Database Methods (No UI Yet)

```go
// Extract scopes from manifest and store in database
err := pluginService.ExtractAndStoreServiceScopes(pluginID)

// Admin reviews requested scopes and authorizes
err := pluginService.AuthorizePluginServiceAccess(pluginID, true) // true = authorize

// Plugin can now access authorized services
```

### Plugin Service Access Lifecycle

1. **Plugin Loaded**: AI Studio loads plugin, calls `GetManifest`
2. **Scope Extraction**: `ExtractAndStoreServiceScopes()` parses manifest and stores scopes
3. **Admin Review**: Admin sees requested scopes in plugin details (UI needed)
4. **Authorization**: Admin clicks "Authorize Service Access" (UI needed)
5. **Runtime Access**: Plugin can call gRPC services with authorized scopes
6. **Enforcement**: gRPC interceptor validates every method call

## 4. Error Handling

### Common gRPC Errors

| Error Code | Reason | Solution |
|------------|--------|----------|
| `Unauthenticated` | No plugin ID in context | Check plugin initialization |
| `PermissionDenied` | Service access not authorized | Admin must authorize access |
| `PermissionDenied` | Insufficient scope | Check manifest declares required scope |
| `InvalidArgument` | Invalid request parameters | Validate request data |
| `NotFound` | Resource not found | Check resource ID exists |

### Example Error Handling

```go
resp, err := p.managementClient.GetAnalyticsSummary(ctx, req)
if err != nil {
    st := status.Convert(err)
    switch st.Code() {
    case codes.Unauthenticated:
        log.Error("Plugin not authenticated - check initialization")
    case codes.PermissionDenied:
        log.Error("Access denied - check admin authorized service access and manifest declares scope")
    case codes.InvalidArgument:
        log.Error("Invalid request - check parameters")
    default:
        log.Error("Service error: %v", err)
    }
    return err
}
```

## 5. Best Practices

### Security
- **Declare minimum required scopes** - Only request scopes actually needed
- **Handle authorization errors gracefully** - Provide fallback functionality
- **Validate all input data** - Don't trust external data
- **Use context timeouts** - Prevent hanging requests

### Performance
- **Reuse gRPC connections** - Don't create new connections per request
- **Cache frequently accessed data** - Store analytics summaries locally
- **Batch requests when possible** - Minimize round trips
- **Handle offline scenarios** - Provide fallback when service unavailable

### Development
- **Use go:embed for manifests** - Embed manifest in binary
- **Test with mock data** - Provide fallback when gRPC unavailable
- **Log service calls** - Debug authorization and permission issues
- **Version manifests** - Update version when changing scopes

## 6. Complete Example: Rate Limiting Plugin

See `examples/plugins/rate-limiting-ui/server/rate-limiting-plugin.go` for a complete implementation demonstrating:

- ✅ **Manifest embedding** with `go:embed`
- ✅ **Service scope declarations** in manifest
- ✅ **gRPC client initialization** with fallback
- ✅ **Real analytics data access** via GetAnalyticsSummary
- ✅ **Real tools data access** via ListTools
- ✅ **Graceful fallback** to mock data when service unavailable
- ✅ **Error handling** with detailed logging

## 7. Testing Your Plugin

### Verify Manifest Scopes
```bash
# Check plugin declares correct scopes
cat plugin.manifest.json | jq '.permissions.services'
```

### Test Service Access
```bash
# Verify plugin can access services after admin authorization
# (Integration testing tools needed)
```

### Debug Authorization
```bash
# Check plugin authorization status in database
# SELECT service_access_authorized, service_scopes FROM plugins WHERE id = ?
```

## Framework Status

- ✅ **Core Services**: Plugin, LLM, Analytics, Apps, Tools implemented
- ✅ **Authentication**: Plugin ID-based authentication working
- ✅ **Authorization**: Scope-based authorization with admin approval
- ✅ **Testing**: Comprehensive test coverage
- ✅ **Documentation**: Complete guide and examples
- ❌ **UI Integration**: Admin authorization workflow needs frontend
- ❌ **Extended Services**: Datasources, advanced analytics, catalogues pending