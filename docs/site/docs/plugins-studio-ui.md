# AI Studio UI Plugins Guide

AI Studio UI plugins extend the dashboard with custom interfaces using WebComponents. Build rich admin panels, custom dashboards, monitoring tools, and specialized management interfaces that integrate seamlessly with the AI Studio platform.

## Overview

UI plugins enable you to:

- **Add Custom Pages**: Register new routes in the dashboard
- **Extend Sidebar**: Add sections, links, and navigation
- **Serve WebComponents**: Use any frontend framework (React, Vue, Lit, etc.)
- **Call Service APIs**: Access LLMs, tools, datasources, analytics, and more
- **Store Plugin Data**: Use built-in key-value storage
- **Define RPC Methods**: Create custom backend endpoints

##Quick Start

### 1. Project Structure

```
my-ui-plugin/
├── server/
│   ├── main.go              # Plugin server
│   ├── plugin.manifest.json # Plugin manifest
│   └── config.schema.json   # Configuration schema
├── ui/
│   ├── webc/
│   │   └── dashboard.js     # WebComponent
│   └── assets/
│       └── icon.svg         # Static assets
└── go.mod
```

### 2. Create Manifest

[`server/plugin.manifest.json`](server/plugin.manifest.json):

```json
{
  "id": "com.example.my-plugin",
  "name": "My Plugin",
  "version": "1.0.0",
  "plugin_type": "ai_studio",
  "permissions": {
    "services": [
      "llms.read",
      "apps.read",
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
                "apiBase": "/plugin/com.example.my-plugin/rpc"
              }
            }
          }
        ]
      }
    ]
  },
  "rpc": {
    "basePath": "/plugin/com.example.my-plugin/rpc"
  }
}
```

### 3. Implement Plugin Server

[`server/main.go`](server/main.go):

```go
package main

import (
    "context"
    "embed"
    "encoding/json"
    "log"

    "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
    mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

//go:embed ui assets plugin.manifest.json
var embeddedAssets embed.FS

type MyPlugin struct {
    serviceAPI mgmtpb.AIStudioManagementServiceClient
    pluginID   uint32
}

// OnInitialize is called when plugin starts
func (p *MyPlugin) OnInitialize(
    serviceAPI mgmtpb.AIStudioManagementServiceClient,
    pluginID uint32) error {

    log.Printf("My Plugin initializing")
    p.serviceAPI = serviceAPI
    p.pluginID = pluginID
    return nil
}

// OnShutdown is called when plugin stops
func (p *MyPlugin) OnShutdown() error {
    log.Printf("My Plugin shutting down")
    return nil
}

// GetAsset serves static files
func (p *MyPlugin) GetAsset(assetPath string) ([]byte, string, error) {
    if assetPath[0] == '/' {
        assetPath = assetPath[1:]
    }

    content, err := embeddedAssets.ReadFile(assetPath)
    if err != nil {
        return nil, "", err
    }

    mimeType := detectMimeType(assetPath)
    return content, mimeType, nil
}

// GetManifest returns the plugin manifest
func (p *MyPlugin) GetManifest() ([]byte, error) {
    return embeddedAssets.ReadFile("plugin.manifest.json")
}

// HandleCall processes RPC methods
func (p *MyPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    ctx := context.Background()

    switch method {
    case "get_data":
        return p.getData(ctx)
    case "save_settings":
        return p.saveSettings(ctx, payload)
    default:
        return nil, fmt.Errorf("unknown method: %s", method)
    }
}

// RPC method implementation
func (p *MyPlugin) getData(ctx context.Context) ([]byte, error) {
    // Call service API to get LLMs
    llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
    if err != nil {
        return nil, err
    }

    // Return data as JSON
    return json.Marshal(map[string]interface{}{
        "llms": llmsResp.Llms,
        "total": llmsResp.TotalCount,
    })
}

func (p *MyPlugin) saveSettings(ctx context.Context, payload []byte) ([]byte, error) {
    // Store settings in KV storage
    _, err := ai_studio_sdk.WritePluginKV(ctx, "settings", payload)
    if err != nil {
        return nil, err
    }

    return json.Marshal(map[string]string{
        "status": "saved",
    })
}

// GetConfigSchema returns configuration schema
func (p *MyPlugin) GetConfigSchema() ([]byte, error) {
    schema := map[string]interface{}{
        "$schema": "http://json-schema.org/draft-07/schema#",
        "type": "object",
        "properties": map[string]interface{}{
            "api_key": map[string]interface{}{
                "type": "string",
                "description": "API key for external service",
            },
        },
    }
    return json.Marshal(schema)
}

func detectMimeType(path string) string {
    if strings.HasSuffix(path, ".js") {
        return "application/javascript"
    } else if strings.HasSuffix(path, ".css") {
        return "text/css"
    } else if strings.HasSuffix(path, ".svg") {
        return "image/svg+xml"
    }
    return "application/octet-stream"
}

func main() {
    log.Printf("Starting My Plugin")
    plugin := &MyPlugin{}
    ai_studio_sdk.ServePlugin(plugin)
}
```

### 4. Create WebComponent

[`ui/webc/dashboard.js`](ui/webc/dashboard.js):

```javascript
class MyPluginDashboard extends HTMLElement {
    connectedCallback() {
        this.innerHTML = `
            <div class="plugin-dashboard">
                <h1>My Plugin Dashboard</h1>
                <div id="content">Loading...</div>
                <button id="refresh">Refresh</button>
            </div>
        `;

        this.loadData();

        this.querySelector('#refresh').addEventListener('click', () => {
            this.loadData();
        });
    }

    async loadData() {
        try {
            const apiBase = this.getAttribute('apiBase') || '/plugin/com.example.my-plugin/rpc';

            const response = await fetch(`${apiBase}/get_data`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${this.getAuthToken()}`,
                },
                body: JSON.stringify({}),
            });

            const data = await response.json();

            this.querySelector('#content').innerHTML = `
                <p>Found ${data.total} LLMs</p>
                <ul>
                    ${data.llms.map(llm => `<li>${llm.name} (${llm.vendor})</li>`).join('')}
                </ul>
            `;
        } catch (error) {
            console.error('Failed to load data:', error);
            this.querySelector('#content').innerHTML = `<p>Error: ${error.message}</p>`;
        }
    }

    getAuthToken() {
        // Get token from localStorage or session
        return localStorage.getItem('auth_token') || '';
    }
}

customElements.define('my-plugin-dashboard', MyPluginDashboard);
```

### 5. Build and Deploy

```bash
# Build plugin
cd server
go build -o my-plugin

# Create plugin in AI Studio
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "My Plugin",
    "slug": "my-plugin",
    "command": "file:///path/to/my-plugin",
    "hook_type": "studio_ui",
    "plugin_type": "ai_studio",
    "is_active": true,
    "load_immediately": true
  }'
```

## Manifest Structure

### Top-Level Fields

```json
{
  "id": "com.example.plugin",           // Unique identifier (reverse domain)
  "name": "Plugin Name",                // Display name
  "version": "1.0.0",                   // Semantic version
  "plugin_type": "ai_studio",           // Must be "ai_studio"
  "description": "Plugin description",  // Optional
  "permissions": { },                   // Permission scopes
  "ui": { },                            // UI configuration
  "rpc": { },                           // RPC configuration
  "assets": [],                         // Static asset list
  "compat": { }                         // Compatibility info
}
```

### Permissions

```json
{
  "permissions": {
    "services": [
      "llms.read",           // List and read LLM configurations
      "llms.proxy",          // Call LLMs via proxy
      "llms.write",          // Create/update LLMs
      "tools.read",          // List and read tools
      "tools.execute",       // Execute tools
      "tools.write",         // Create/update tools
      "datasources.read",    // List and read datasources
      "datasources.query",   // Query datasources
      "apps.read",           // List and read apps
      "apps.write",          // Create/update apps
      "plugins.read",        // List and read plugins
      "analytics.read",      // Read analytics data
      "kv.read",             // Read KV storage
      "kv.readwrite"         // Read/write KV storage
    ]
  }
}
```

### UI Configuration

```json
{
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",        // UI slot name
        "label": "My Plugin",             // Section label
        "icon": "/assets/icon.svg",       // Icon path
        "items": [
          {
            "type": "route",              // Route type
            "path": "/admin/my-plugin",   // URL path
            "title": "Dashboard",         // Page title
            "mount": {
              "kind": "webc",             // WebComponent
              "tag": "my-dashboard",      // Custom element tag
              "entry": "/ui/webc/dashboard.js",  // JS file
              "props": {                  // Props passed to component
                "apiBase": "/plugin/com.example/rpc"
              }
            }
          }
        ]
      }
    ]
  }
}
```

### Available UI Slots

- `sidebar.section`: Add sidebar section with nested items
- `sidebar.link`: Add individual sidebar link
- `settings.section`: Add settings page section
- `app.detail.tab`: Add tab to app detail page
- `llm.detail.tab`: Add tab to LLM detail page

### RPC Configuration

```json
{
  "rpc": {
    "basePath": "/plugin/com.example/rpc"
  }
}
```

RPC calls are automatically routed to your plugin's `HandleCall()` method.

## Service API Access

The Service API provides 100+ gRPC operations accessible via the SDK:

### LLM Operations

```go
// List LLMs
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, page, limit)

// Get LLM by ID
llm, err := ai_studio_sdk.GetLLM(ctx, llmID)

// Call LLM via proxy (streaming)
stream, err := ai_studio_sdk.CallLLM(ctx, llmID, model, messages,
    temperature, maxTokens, tools, true)

// Simple non-streaming LLM call
response, err := ai_studio_sdk.CallLLMSimple(ctx, llmID, model,
    "Hello, world!")
```

### Tool Operations

```go
// List tools
toolsResp, err := ai_studio_sdk.ListTools(ctx, page, limit)

// Get tool by ID
tool, err := ai_studio_sdk.GetTool(ctx, toolID)

// Execute tool
result, err := ai_studio_sdk.ExecuteTool(ctx, toolID, operationID, params)
```

### Plugin Operations

```go
// List plugins
pluginsResp, err := ai_studio_sdk.ListPlugins(ctx, page, limit)

// Get plugin by ID
plugin, err := ai_studio_sdk.GetPlugin(ctx, pluginID)

// Get counts
pluginsCount, err := ai_studio_sdk.GetPluginsCount(ctx)
llmsCount, err := ai_studio_sdk.GetLLMsCount(ctx)
```

### KV Storage Operations

```go
// Write data
created, err := ai_studio_sdk.WritePluginKV(ctx, key, data)

// Read data
data, err := ai_studio_sdk.ReadPluginKV(ctx, key)

// Delete data
err := ai_studio_sdk.DeletePluginKV(ctx, key)

// List keys
keys, err := ai_studio_sdk.ListPluginKVKeys(ctx, prefix)
```

### App Operations

```go
// List apps
appsResp, err := ai_studio_sdk.ListApps(ctx, page, limit)

// Get app by ID
app, err := ai_studio_sdk.GetApp(ctx, appID)
```

### Analytics Operations

```go
// Get usage statistics
usage, err := ai_studio_sdk.GetUsageStats(ctx, startTime, endTime, filters)

// Get cost analytics
costs, err := ai_studio_sdk.GetCostAnalytics(ctx, startTime, endTime)
```

## Complete Example: Rate Limiting Dashboard

Here's a complete example showing all features:

### Manifest

```json
{
  "id": "com.tyk.rate-limiting-ui",
  "version": "1.0.0",
  "name": "Rate Limiting UI",
  "description": "Enhanced UI for rate limiting configuration",
  "permissions": {
    "services": [
      "plugins.read",
      "llms.read",
      "tools.read",
      "kv.readwrite"
    ]
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "Rate Limiting",
        "icon": "/assets/rate-limit.svg",
        "items": [
          {
            "type": "route",
            "path": "/admin/rate-limiting/dashboard",
            "title": "Rate Limiting Dashboard",
            "mount": {
              "kind": "webc",
              "tag": "rate-limiting-dashboard",
              "entry": "/ui/webc/dashboard.js"
            }
          },
          {
            "type": "route",
            "path": "/admin/rate-limiting/settings",
            "title": "Global Settings",
            "mount": {
              "kind": "webc",
              "tag": "rate-limiting-settings",
              "entry": "/ui/webc/settings.js"
            }
          }
        ]
      }
    ]
  },
  "rpc": {
    "basePath": "/plugin/com.tyk.rate-limiting-ui/rpc"
  }
}
```

### RPC Methods

```go
func (p *RateLimitingUIPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    ctx := context.Background()

    switch method {
    case "get_statistics":
        return p.getStatistics(ctx)
    case "get_rate_limits":
        return p.getRateLimits(ctx)
    case "set_rate_limit":
        return p.setRateLimit(ctx, payload)
    case "get_available_tools":
        return p.getAvailableTools(ctx)
    default:
        return nil, fmt.Errorf("unknown method: %s", method)
    }
}

func (p *RateLimitingUIPlugin) getStatistics(ctx context.Context) ([]byte, error) {
    // Get real plugin and LLM counts
    pluginsCount, err := ai_studio_sdk.GetPluginsCount(ctx)
    if err != nil {
        return nil, err
    }

    llmsCount, err := ai_studio_sdk.GetLLMsCount(ctx)
    if err != nil {
        return nil, err
    }

    stats := map[string]interface{}{
        "total_plugins": pluginsCount,
        "total_llms":    llmsCount,
        "blocked_requests": 142,
        "success_rate": 0.991,
    }

    return json.Marshal(stats)
}

func (p *RateLimitingUIPlugin) getAvailableTools(ctx context.Context) ([]byte, error) {
    // List tools using service API
    toolsResp, err := ai_studio_sdk.ListTools(ctx, 1, 50)
    if err != nil {
        return nil, err
    }

    // Convert to UI format
    tools := make([]map[string]interface{}, len(toolsResp.Tools))
    for i, tool := range toolsResp.Tools {
        tools[i] = map[string]interface{}{
            "id":          tool.Id,
            "name":        tool.Name,
            "slug":        tool.Slug,
            "description": tool.Description,
            "type":        tool.ToolType,
        }
    }

    return json.Marshal(map[string]interface{}{
        "tools": tools,
        "total": toolsResp.TotalCount,
    })
}

func (p *RateLimitingUIPlugin) setRateLimit(ctx context.Context, payload []byte) ([]byte, error) {
    var config map[string]interface{}
    if err := json.Unmarshal(payload, &config); err != nil {
        return nil, err
    }

    // Store in KV storage
    _, err := ai_studio_sdk.WritePluginKV(ctx, "rate_limits", payload)
    if err != nil {
        return nil, err
    }

    return json.Marshal(map[string]string{
        "status": "updated",
    })
}
```

## Frontend Frameworks

### React

```javascript
import React, { useState, useEffect } from 'react';
import ReactDOM from 'react-dom/client';

function RateLimitingDashboard({ apiBase }) {
    const [stats, setStats] = useState(null);

    useEffect(() => {
        fetchStats();
    }, []);

    async function fetchStats() {
        const response = await fetch(`${apiBase}/get_statistics`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${getAuthToken()}`,
            },
        });
        const data = await response.json();
        setStats(data);
    }

    if (!stats) return <div>Loading...</div>;

    return (
        <div className="dashboard">
            <h1>Rate Limiting Dashboard</h1>
            <div className="stats">
                <div>Plugins: {stats.total_plugins}</div>
                <div>LLMs: {stats.total_llms}</div>
                <div>Blocked: {stats.blocked_requests}</div>
            </div>
        </div>
    );
}

class RateLimitingDashboardElement extends HTMLElement {
    connectedCallback() {
        const apiBase = this.getAttribute('apiBase');
        const root = ReactDOM.createRoot(this);
        root.render(<RateLimitingDashboard apiBase={apiBase} />);
    }
}

customElements.define('rate-limiting-dashboard', RateLimitingDashboardElement);
```

### Vue

```javascript
import { createApp } from 'vue';

const DashboardApp = {
    data() {
        return {
            stats: null,
        };
    },
    mounted() {
        this.fetchStats();
    },
    methods: {
        async fetchStats() {
            const response = await fetch(`${this.apiBase}/get_statistics`, {
                method: 'POST',
            });
            this.stats = await response.json();
        },
    },
    template: `
        <div class="dashboard">
            <h1>Rate Limiting Dashboard</h1>
            <div v-if="stats">
                <div>Plugins: {{ stats.total_plugins }}</div>
                <div>LLMs: {{ stats.total_llms }}</div>
            </div>
        </div>
    `,
};

class RateLimitingDashboardElement extends HTMLElement {
    connectedCallback() {
        const apiBase = this.getAttribute('apiBase');
        createApp(DashboardApp, { apiBase }).mount(this);
    }
}

customElements.define('rate-limiting-dashboard', RateLimitingDashboardElement);
```

## Best Practices

### Security

- Always validate inputs in RPC methods
- Use HTTPS for production deployments
- Sanitize HTML in WebComponents
- Don't expose sensitive data in frontend
- Use CSP headers in manifest

### Performance

- Lazy-load heavy components
- Cache frequently accessed data
- Use KV storage for plugin state
- Minimize Service API calls
- Bundle and minify assets

### User Experience

- Show loading states
- Handle errors gracefully
- Provide feedback for actions
- Use consistent UI patterns
- Support dark mode if platform does

### Development

- Use TypeScript for type safety
- Add proper error handling
- Log important operations
- Test with different data scenarios
- Document RPC methods

## Troubleshooting

### Plugin Not Loading

- Check manifest syntax (valid JSON)
- Verify `plugin_type` is `"ai_studio"`
- Ensure `load_immediately` is `true` in plugin registration
- Check logs for initialization errors

### WebComponent Not Rendering

- Verify asset path is correct (leading `/`)
- Check browser console for JS errors
- Ensure custom element is defined
- Verify mime types are correct

### Service API Calls Failing

- Check permission scopes in manifest
- Verify SDK is initialized (`ai_studio_sdk.IsInitialized()`)
- Check context has valid authentication
- Review service API error messages

### Assets Not Serving

- Ensure assets are embedded (`//go:embed`)
- Check `GetAsset()` normalizes paths correctly
- Verify mime type detection
- Check asset paths in manifest match filesystem

## Next Steps

- [Plugin Manifests & Permissions]([plugins-manifests](https://docs.claude.com/en/docs/plugins-manifests))
- [Service API Reference]([plugins-service-api](https://docs.claude.com/en/docs/plugins-service-api))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
- [Plugin Deployment]([plugins-deployment](https://docs.claude.com/en/docs/plugins-deployment))
