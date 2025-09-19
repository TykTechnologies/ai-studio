# Plugin Compilation and Installation Guide

This guide walks you through compiling the rate limiting UI plugin and installing it in AI Studio.

## Prerequisites

1. **Docker** - For building OCI artifacts
2. **Go 1.21+** - For building the plugin server
3. **AI Studio** - Running locally with the new plugin system

## Step-by-Step Compilation

### 1. Setup Local Registry (Optional)

If you don't have access to a container registry, start a local one:

```bash
# Start local Docker registry on port 5000
make start-registry

# Verify it's running
curl http://localhost:5000/v2/
```

### 2. Build the Plugin

```bash
cd examples/plugins/rate-limiting-ui

# Option A: Build everything at once
make build

# Option B: Build step by step
make build-server  # Build Go binary
docker build -t localhost:5000/tyk/rate-limiting-ui:1.0.0 .
```

### 3. Push to Registry

```bash
# Push to registry
make push

# Verify the push
curl http://localhost:5000/v2/tyk/rate-limiting-ui/manifests/1.0.0
```

## Installation in AI Studio

### 1. Access Admin Interface

Navigate to your AI Studio admin interface:
```
http://localhost:8080/admin/plugins
```

### 2. Create AI Studio Plugin

Click **Add Plugin** and fill in:

```
Name: Rate Limiting UI
Slug: rate-limiting-ui
Description: Enhanced UI for rate limiting configuration and monitoring
Plugin Type: AI Studio Plugin
Hook Type: data_collection
OCI Plugin: ✓ (checked)
OCI Reference: localhost:5000/tyk/rate-limiting-ui:1.0.0
Active: ✓ (checked)
```

### 3. Parse Plugin Manifest

1. After creating the plugin, navigate to its detail page
2. Click **Parse Manifest** button (this extracts the UI configuration)
3. Verify that the manifest was parsed successfully

### 4. Load Plugin UI

1. Click **Load UI** to activate the plugin components
2. Refresh the admin interface
3. Check that "Rate Limiting" appears in the sidebar

## Verification Steps

### 1. Check Sidebar Integration

- Navigate to AI Studio admin
- Verify "Rate Limiting" section appears in left sidebar
- Should show two sub-items:
  - "Rate Limiting Dashboard"
  - "Global Settings"

### 2. Test Dashboard Component

- Click on "Rate Limiting Dashboard"
- Should load at `/admin/rate-limiting/dashboard`
- Verify statistics and tables populate with mock data
- Test the "Refresh" button

### 3. Test Settings Component

- Click on "Global Settings"
- Should load at `/admin/rate-limiting/settings`
- Verify form fields populate with current settings
- Test "Save Settings" functionality
- Test "Test Connection" button

### 4. Verify API Integration

Check that plugin API endpoints work:

```bash
# Get UI registry (should include rate limiting components)
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/plugins/ui-registry

# Get sidebar menu items (should include rate limiting)
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/plugins/sidebar-menu

# Get AI Studio plugins with manifests
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/plugins/ai-studio/manifests
```

## Troubleshooting

### Build Issues

**Go Module Issues:**
```bash
cd server
go mod tidy
go mod download
```

**Docker Build Fails:**
```bash
# Check Docker is running
docker ps

# Build with verbose output
docker build --progress=plain -t localhost:5000/tyk/rate-limiting-ui:1.0.0 .
```

### Registry Issues

**Push Fails:**
```bash
# Check registry is accessible
curl http://localhost:5000/v2/

# Check Docker can push
docker push localhost:5000/tyk/rate-limiting-ui:1.0.0
```

### AI Studio Integration Issues

**Plugin Not Found:**
- Verify OCI reference is correct
- Check AI Studio logs for fetch errors
- Ensure registry is accessible from AI Studio

**UI Not Loading:**
- Check browser console for JavaScript errors
- Verify manifest was parsed successfully
- Check that Web Components are registered

**Sidebar Not Updating:**
- Force refresh the admin interface
- Check plugin is marked as "loaded"
- Verify UI registry entries exist

### Common Fixes

**Clear Plugin Cache:**
```bash
# In AI Studio, clear OCI plugin cache
curl -X DELETE http://localhost:8080/api/v1/plugins/oci/cache
```

**Reload Plugin:**
```bash
# Refresh plugin from registry
curl -X POST http://localhost:8080/api/v1/plugins/{PLUGIN_ID}/refresh
```

**Re-parse Manifest:**
```bash
# Re-parse plugin manifest
curl -X POST http://localhost:8080/api/v1/plugins/{PLUGIN_ID}/manifest/parse
```

## Development Workflow

### 1. Make Changes

Edit plugin files:
- `server/main.go` - Backend logic
- `ui/webc/*.js` - UI components
- `plugin.manifest.json` - UI declarations

### 2. Rebuild and Deploy

```bash
make clean
make deploy
```

### 3. Update AI Studio

```bash
# Refresh the plugin in AI Studio
curl -X POST -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/plugins/{PLUGIN_ID}/refresh

# Re-parse manifest if UI changed
curl -X POST -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:8080/api/v1/plugins/{PLUGIN_ID}/manifest/parse
```

### 4. Test Changes

- Refresh admin interface
- Navigate to plugin pages
- Verify changes are reflected

## Next Steps

After successful installation:

1. **Extend Plugin**: Add more rate limiting features
2. **Real Integration**: Connect to actual rate limiting backend
3. **Additional Plugins**: Create more AI Studio plugins following this pattern
4. **Production Deploy**: Push to production registry and install

This example demonstrates the complete flow from plugin development to UI integration, proving the AI Studio plugin extension system works end-to-end.