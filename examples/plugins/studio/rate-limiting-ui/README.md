# Rate Limiting UI Plugin

This is an example AI Studio plugin that demonstrates the new plugin extension system. It provides enhanced UI for rate limiting configuration and monitoring, replacing the basic JSON configuration with a proper dashboard and settings interface.

## Plugin Features

This plugin demonstrates:

1. **Dual UI Components**:
   - **Dashboard**: Real-time rate limiting statistics and monitoring (`/admin/rate-limiting/dashboard`)
   - **Settings**: Global rate limiting configuration (`/admin/rate-limiting/settings`)

2. **Plugin Architecture**:
   - **Server Component**: Go-based gRPC plugin using hashicorp/go-plugin
   - **UI Components**: Web Components that integrate seamlessly with AI Studio
   - **OCI Packaging**: Complete plugin packaged as OCI artifact

3. **Better UX**:
   - Replaces static JSON config with interactive forms
   - Adds new sidebar menu item for global management
   - Real-time data visualization and monitoring

## Quick Start

### Prerequisites

- Docker installed and running
- AI Studio running locally
- Access to push to a container registry (or use local registry)

### 1. Build the Plugin

```bash
cd examples/plugins/rate-limiting-ui

# Build the plugin OCI artifact
make build
```

### 2. Start Local Registry (Optional)

If you don't have access to a container registry:

```bash
# Start a local Docker registry
make start-registry
```

### 3. Deploy Plugin

```bash
# Build and push to registry
make deploy
```

### 4. Install in AI Studio

1. Open AI Studio admin interface
2. Navigate to **Plugins** → **Create**
3. Fill in the form:
   - **Name**: `Rate Limiting UI`
   - **Slug**: `rate-limiting-ui`
   - **Plugin Type**: `AI Studio Plugin`
   - **Hook Type**: `data_collection` (required field)
   - **Enable OCI Plugin**: ✓
   - **OCI Reference**: `localhost:5000/tyk/rate-limiting-ui:1.0.0`
4. Click **Add Plugin**
5. Navigate to the plugin detail page and click **Parse Manifest** to register the UI components

### 5. Use the Plugin

After installation:

1. **New Sidebar Item**: You'll see "Rate Limiting" in the admin sidebar
2. **Dashboard**: Click to view real-time rate limiting statistics
3. **Settings**: Global configuration for rate limiting backend
4. **Enhanced Config**: When configuring the gateway rate limiting plugin, you get the enhanced UI instead of JSON

## Development

### Plugin Structure

```
rate-limiting-ui/
├── plugin.manifest.json     # Plugin manifest (UI declarations)
├── server/                  # Go gRPC server
│   ├── main.go             # Plugin implementation
│   ├── go.mod              # Go dependencies
│   └── go.sum
├── ui/webc/                # Web Components
│   ├── dashboard.js        # Dashboard component
│   └── settings.js         # Settings component
├── assets/                 # Static assets
│   └── rate-limit.svg      # Icon
├── Dockerfile              # OCI artifact build
├── Makefile               # Build automation
└── README.md              # This file
```

### Local Development

```bash
# Build just the server for testing
make build-server

# Test the server
make test

# Clean build artifacts
make clean
```

### Plugin Commands

The plugin server responds to these RPC commands:

- `get_rate_limits` - Returns current rate limit configurations
- `set_rate_limit` - Updates a rate limit configuration
- `get_global_settings` - Returns global rate limiting settings
- `set_global_settings` - Updates global settings
- `get_statistics` - Returns rate limiting statistics

## Architecture

This plugin follows the architecture described in `Hot-load-ui-plugins-plan.md`:

1. **OCI Artifact**: Single artifact containing server binary, UI components, and manifest
2. **Web Components**: Framework-agnostic UI components that integrate with React
3. **gRPC Communication**: UI components communicate with backend via RPC endpoints
4. **Manifest-Driven**: UI registration declared in `plugin.manifest.json`
5. **Sandboxed Security**: Web Components run in Shadow DOM with CSP restrictions

## Testing the Integration

After installing the plugin:

1. **Verify Sidebar**: Check that "Rate Limiting" appears in admin sidebar
2. **Test Dashboard**: Navigate to dashboard and verify statistics load
3. **Test Settings**: Update global settings and verify they save
4. **Test Hot Reload**: Update plugin and verify changes appear without restart

## Production Considerations

- Replace mock data with real rate limiting backend integration
- Implement proper Redis connectivity testing
- Add authentication/authorization for plugin endpoints
- Set up proper monitoring and alerting
- Configure CSP policies for production security

## Troubleshooting

### Plugin Not Loading
- Check AI Studio logs for OCI fetch errors
- Verify registry connectivity and authentication
- Ensure manifest.json is valid JSON

### UI Components Not Appearing
- Check browser console for JavaScript errors
- Verify Web Component registration
- Check that plugin manifest was parsed successfully

### RPC Calls Failing
- Verify plugin server is running
- Check gRPC connectivity
- Validate RPC endpoint permissions in manifest