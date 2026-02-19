# Plugin Manifests and Permissions

Plugin manifests define plugin metadata, capabilities, permissions, and UI integration. Understanding manifests is essential for building secure, well-integrated plugins.

## Manifest Structure

### Microgateway Plugins

Microgateway plugins don't use JSON manifests. Configuration is provided via the API when creating the plugin:

```json
{
  "name": "Custom Auth Plugin",
  "slug": "custom-auth",
  "description": "Custom authentication logic",
  "command": "file:///path/to/plugin",
  "hook_type": "auth",
  "plugin_type": "gateway",
  "config": {
    "valid_token": "secret123"
  },
  "is_active": true
}
```

Hook types for microgateway plugins:
- `pre_auth` - Before authentication
- `auth` - Custom authentication
- `post_auth` - After authentication
- `on_response` - Response modification
- `data_collection` - Data export

### AI Studio UI Plugins

Complete manifest structure for UI plugins:

```json
{
  "id": "com.example.plugin",
  "name": "My Plugin",
  "version": "1.0.0",
  "description": "Plugin description",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": [
      "llms.read",
      "llms.write",
      "llms.proxy",
      "tools.read",
      "tools.execute",
      "tools.write",
      "datasources.read",
      "datasources.query",
      "apps.read",
      "apps.write",
      "plugins.read",
      "analytics.read",
      "kv.read",
      "kv.readwrite"
    ]
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "My Plugin",
        "icon": "/assets/icon.svg",
        "items": [
          {
            "type": "route",
            "path": "/admin/my-plugin",
            "title": "Dashboard",
            "mount": {
              "kind": "webc",
              "tag": "my-plugin-dashboard",
              "entry": "/ui/webc/dashboard.js",
              "props": {
                "apiBase": "/plugin/com.example.plugin/rpc"
              }
            }
          }
        ]
      }
    ]
  },
  "rpc": {
    "basePath": "/plugin/com.example.plugin/rpc"
  },
  "assets": [
    "/assets/icon.svg",
    "/ui/webc/dashboard.js"
  ],
  "compat": {
    "app": ">=2.6 <3.0",
    "api": ["ui-v1", "kv-v1", "rpc-v1"]
  },
  "security": {
    "csp": "script-src 'self'; object-src 'none'"
  }
}
```

### AI Studio Agent Plugins

Agent plugin manifests are simpler (no UI):

```json
{
  "id": "com.example.agent",
  "name": "My Agent",
  "version": "1.0.0",
  "description": "Custom conversational agent",
  "plugin_type": "agent",
  "permissions": {
    "services": [
      "llms.proxy",
      "tools.execute",
      "datasources.query",
      "kv.readwrite"
    ]
  },
  "ui": {
    "slots": []
  }
}
```

### Object Hooks Plugins

Object hooks plugins intercept CRUD operations on platform objects. These plugins use the unified SDK and register hooks programmatically, so the manifest doesn't need special hook configuration.

#### Basic Object Hooks Manifest

```json
{
  "id": "com.example.validator",
  "name": "LLM Validator",
  "version": "1.0.0",
  "description": "Validates LLM configurations before saving",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": [
      "llms.read",
      "kv.readwrite"
    ]
  },
  "ui": {
    "slots": []
  }
}
```

**Note**: Object hooks are registered via the `GetObjectHookRegistrations()` method in the plugin code, not in the manifest. The plugin_type is `"ai_studio"` since object hooks are Studio-only.

#### Hook Registration (Code, Not Manifest)

Object hooks are registered programmatically:

```go
func (p *ValidatorPlugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
    return []*pb.ObjectHookRegistration{
        {
            ObjectType: "llm",               // llm, datasource, tool, user
            HookTypes:  []string{            // before_create, after_create, etc.
                "before_create",
                "before_update",
            },
            Priority: 10,                    // Lower runs first
        },
        {
            ObjectType: "datasource",
            HookTypes:  []string{"before_create"},
            Priority:   5,                   // Runs before priority 10
        },
    }, nil
}
```

**Supported Objects**:
- `llm` - LLM provider configurations
- `datasource` - Data source connections
- `tool` - External tool definitions
- `user` - User accounts

**Supported Hook Types** (per object):
- `before_create` - Before object creation (can block)
- `after_create` - After object creation (notification only)
- `before_update` - Before object update (can block)
- `after_update` - After object update (notification only)
- `before_delete` - Before object deletion (can block)
- `after_delete` - After object deletion (notification only)

**Priority**: Lower numbers run first (e.g., priority 5 runs before priority 10). Use priority to control hook execution order when multiple plugins register hooks for the same object/event.

#### Multi-Capability with Hooks

Object hooks plugins can combine hooks with UI:

```json
{
  "id": "com.example.approval-system",
  "name": "Approval System",
  "version": "1.0.0",
  "description": "Requires approval for datasource creation",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": [
      "datasources.read",
      "kv.readwrite"
    ]
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "Approvals",
        "items": [
          {
            "type": "route",
            "path": "/admin/approvals",
            "title": "Pending Approvals",
            "mount": {
              "kind": "webc",
              "tag": "approval-dashboard",
              "entry": "/ui/webc/approvals.js"
            }
          }
        ]
      }
    ]
  },
  "rpc": {
    "basePath": "/plugin/com.example.approval-system/rpc"
  }
}
```

This plugin would:
1. Register `before_create` hooks for datasources (via code)
2. Block creation and add to pending approvals
3. Provide UI dashboard to approve/reject
4. Use RPC methods for approval actions

See `examples/plugins/studio/llm-validator/` and `examples/plugins/studio/hook-test-plugin/` for complete examples.

### Resource Provider Plugins

Resource Provider plugins register custom resource types that integrate into the App creation flow. Declare resource types in the manifest's `resource_types` section:

```json
{
  "id": "com.example.mcp-registry",
  "name": "MCP Registry",
  "version": "1.0.0",
  "description": "Manages MCP server resources",
  "capabilities": {
    "hooks": ["resource_provider", "studio_ui"]
  },
  "resource_types": [
    {
      "slug": "mcp_servers",
      "name": "MCP Servers",
      "description": "Model Context Protocol servers",
      "icon": "Hub",
      "has_privacy_score": true,
      "supports_submissions": true,
      "form_component": {
        "tag": "mcp-server-selector",
        "entry_point": "ui/webc/mcp-selector.js"
      }
    }
  ],
  "permissions": {
    "services": ["apps.read", "kv.readwrite"]
  }
}
```

Resource types declared in the manifest are automatically registered when the plugin loads. The plugin also implements `ResourceProvider` methods programmatically for runtime behavior.

| Field | Required | Description |
|-------|:---:|-------------|
| `slug` | Yes | Machine-readable identifier, unique per plugin |
| `name` | Yes | Display name shown in the App form |
| `has_privacy_score` | No | Whether instances carry privacy scores (default: `false`) |
| `supports_submissions` | No | Whether community users can submit instances (default: `false`) |
| `form_component` | No | Custom Web Component for the App form selector |

See [Resource Provider Plugins](plugins-resource-types.md) for the full guide.

## Service Scopes Reference

### LLM Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `llms.read` | Read LLM configurations | List LLMs, Get LLM details, Get LLM counts |
| `llms.write` | Create/update LLMs | Create LLM, Update LLM, Delete LLM |
| `llms.proxy` | Call LLMs via proxy | CallLLM (streaming/non-streaming) |

### Tool Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `tools.read` | Read tool configurations | List Tools, Get Tool details |
| `tools.execute` | Execute tools | ExecuteTool with operation and parameters |
| `tools.write` | Create/update tools | Create Tool, Update Tool, Delete Tool |
| `tools.operations` | Manage tool operations | Add/remove operations |

### Datasource Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `datasources.read` | Read datasource configurations | List Datasources, Get Datasource details |
| `datasources.query` | Query datasources | QueryDatasource with SQL/query DSL |
| `datasources.write` | Create/update datasources | Create, Update, Delete Datasources |

### App Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `apps.read` | Read app configurations | List Apps, Get App details |
| `apps.write` | Create/update apps | Create App, Update App, Delete App |

### Plugin Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `plugins.read` | Read plugin configurations | List Plugins, Get Plugin details, Get counts |
| `plugins.write` | Create/update plugins | Create Plugin, Update Plugin |

### KV Storage Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `kv.read` | Read plugin KV storage | ReadPluginKV, ListPluginKVKeys |
| `kv.readwrite` | Read/write plugin KV storage | Read + WritePluginKV, DeletePluginKV |

### Analytics Scopes

| Scope | Description | Operations |
|-------|-------------|------------|
| `analytics.read` | Read analytics data | GetUsageStats, GetCostAnalytics |
| `analytics.write` | Write analytics data | RecordCustomMetric |

## UI Slot System

### Available Slots

#### sidebar.section

Add a collapsible section to the sidebar with nested items:

```json
{
  "slot": "sidebar.section",
  "label": "My Plugin",
  "icon": "/assets/icon.svg",
  "items": [
    {
      "type": "route",
      "path": "/admin/my-plugin/dashboard",
      "title": "Dashboard",
      "mount": { }
    },
    {
      "type": "route",
      "path": "/admin/my-plugin/settings",
      "title": "Settings",
      "mount": { }
    }
  ]
}
```

#### sidebar.link

Add a single link to the sidebar:

```json
{
  "slot": "sidebar.link",
  "label": "Quick Action",
  "icon": "/assets/icon.svg",
  "path": "/admin/quick-action"
}
```

#### settings.section

Add a section to the Settings page:

```json
{
  "slot": "settings.section",
  "label": "Plugin Settings",
  "path": "/admin/settings/plugin",
  "mount": {
    "kind": "webc",
    "tag": "plugin-settings",
    "entry": "/ui/webc/settings.js"
  }
}
```

#### app.detail.tab

Add a tab to App detail pages:

```json
{
  "slot": "app.detail.tab",
  "label": "Custom View",
  "mount": {
    "kind": "webc",
    "tag": "app-custom-view",
    "entry": "/ui/webc/app-view.js",
    "props": {
      "appId": "{{app.id}}"
    }
  }
}
```

#### llm.detail.tab

Add a tab to LLM detail pages:

```json
{
  "slot": "llm.detail.tab",
  "label": "Analytics",
  "mount": {
    "kind": "webc",
    "tag": "llm-analytics",
    "entry": "/ui/webc/llm-analytics.js",
    "props": {
      "llmId": "{{llm.id}}"
    }
  }
}
```

### Mount Configuration

#### WebComponent Mount

```json
{
  "kind": "webc",
  "tag": "my-component",
  "entry": "/ui/webc/component.js",
  "props": {
    "apiBase": "/plugin/com.example/rpc",
    "theme": "dark"
  }
}
```

- `kind`: Must be `"webc"` for WebComponents
- `tag`: Custom element tag name
- `entry`: Path to JavaScript file (relative to plugin)
- `props`: Properties passed to the component

## Permission Validation

Permissions are validated when plugins call the Service API:

```go
// This call requires "llms.read" scope
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
if err != nil {
    // Error: insufficient permissions
    log.Printf("Permission denied: %v", err)
}
```

If your plugin doesn't declare the required scope in its manifest, Service API calls will fail with permission errors.

## Configuration Schema

Plugins can provide JSON Schema for their configuration:

```go
func (p *MyPlugin) GetConfigSchema() ([]byte, error) {
    schema := map[string]interface{}{
        "$schema": "http://json-schema.org/draft-07/schema#",
        "type":    "object",
        "title":   "My Plugin Configuration",
        "properties": map[string]interface{}{
            "api_key": map[string]interface{}{
                "type":        "string",
                "description": "API key for external service",
                "minLength":   1,
            },
            "endpoint": map[string]interface{}{
                "type":        "string",
                "format":      "uri",
                "description": "Service endpoint URL",
                "default":     "https://api.example.com",
            },
            "rate_limit": map[string]interface{}{
                "type":        "integer",
                "description": "Requests per minute",
                "minimum":     1,
                "maximum":     1000,
                "default":     100,
            },
            "enabled": map[string]interface{}{
                "type":        "boolean",
                "description": "Enable plugin",
                "default":     true,
            },
        },
        "required": []string{"api_key"},
    }

    return json.Marshal(schema)
}
```

The platform uses this schema to:
- Validate configuration on save
- Generate UI forms
- Provide inline documentation
- Set default values

## Security Best Practices

### Principle of Least Privilege

Only request scopes your plugin actually needs:

```json
{
  "permissions": {
    "services": [
      "llms.read",      // ✅ Need to list LLMs
      "kv.readwrite"    // ✅ Need to store settings
      // ❌ Don't add "llms.write" if not creating LLMs
      // ❌ Don't add "llms.proxy" if not calling LLMs
    ]
  }
}
```

### Content Security Policy

Define CSP headers for UI plugins:

```json
{
  "security": {
    "csp": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' https://api.example.com; object-src 'none'; frame-ancestors 'none'"
  }
}
```

### Input Validation

Always validate inputs in RPC methods:

```go
func (p *MyPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    // Validate method
    allowedMethods := map[string]bool{
        "get_data":    true,
        "save_config": true,
    }

    if !allowedMethods[method] {
        return nil, fmt.Errorf("invalid method: %s", method)
    }

    // Validate payload
    var data map[string]interface{}
    if err := json.Unmarshal(payload, &data); err != nil {
        return nil, fmt.Errorf("invalid payload: %w", err)
    }

    // Additional validation...
    return p.processMethod(method, data)
}
```

### Secrets Management

Never hardcode secrets in manifests or code:

```go
// ❌ Bad: Hardcoded secret
func (p *MyPlugin) OnInitialize(...) error {
    p.apiKey = "secret123"  // Don't do this!
}

// ✅ Good: Configuration from secure storage
func (p *MyPlugin) OnInitialize(...) error {
    // Read from config (stored securely by platform)
    config, _ := ai_studio_sdk.ReadPluginKV(ctx, "config")
    var cfg Config
    json.Unmarshal(config, &cfg)
    p.apiKey = cfg.APIKey
}
```

## Versioning and Compatibility

### Semantic Versioning

Use semantic versioning for plugin versions:

```json
{
  "version": "1.2.3"  // MAJOR.MINOR.PATCH
}
```

- MAJOR: Breaking changes
- MINOR: New features, backward compatible
- PATCH: Bug fixes, backward compatible

### Compatibility Declaration

Declare platform compatibility:

```json
{
  "compat": {
    "app": ">=2.6 <3.0",
    "api": ["ui-v1", "kv-v1", "rpc-v1"]
  }
}
```

## Testing Manifests

### Validation

Validate your manifest before deployment:

```bash
# Check JSON syntax
cat plugin.manifest.json | jq .

# Validate required fields
jq '.id, .name, .version, .plugin_type' plugin.manifest.json
```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "Invalid plugin_type" | Wrong plugin type | Use `"ai_studio"` or `"agent"` |
| "Missing required field" | Missing manifest field | Add `id`, `name`, `version` |
| "Invalid scope" | Unknown service scope | Check scope name spelling |
| "Duplicate UI slot" | Slot registered twice | Remove duplicate slot |
| "Asset not found" | Missing embedded file | Verify `//go:embed` directive |

## Complete Examples

### Minimal UI Plugin

```json
{
  "id": "com.example.minimal",
  "name": "Minimal Plugin",
  "version": "1.0.0",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": ["kv.readwrite"]
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.link",
        "label": "Minimal",
        "path": "/admin/minimal"
      }
    ]
  }
}
```

### Full-Featured UI Plugin

See [plugins-studio-ui.md]([plugins-studio-ui](https://docs.claude.com/en/docs/plugins-studio-ui)) for complete rate-limiting-ui example.

### Minimal Agent Plugin

```json
{
  "id": "com.example.simple-agent",
  "name": "Simple Agent",
  "version": "1.0.0",
  "plugin_type": "agent",
  "permissions": {
    "services": ["llms.proxy"]
  },
  "ui": {
    "slots": []
  }
}
```

### Advanced Agent Plugin

```json
{
  "id": "com.example.rag-agent",
  "name": "RAG Agent",
  "version": "1.0.0",
  "description": "Agent with retrieval-augmented generation",
  "plugin_type": "agent",
  "permissions": {
    "services": [
      "llms.proxy",
      "datasources.query",
      "tools.execute",
      "kv.readwrite",
      "analytics.read"
    ]
  },
  "ui": {
    "slots": []
  }
}
```

## Next Steps

- [Plugin Deployment Options]([plugins-deployment](https://docs.claude.com/en/docs/plugins-deployment))
- [Service API Reference]([plugins-service-api](https://docs.claude.com/en/docs/plugins-service-api))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
