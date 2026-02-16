# Custom Echo Endpoint

A Tyk AI Studio example plugin demonstrating **Custom Endpoints** combined with a **Studio UI**.

## Capabilities

- **CustomEndpointHandler** — serves a catch-all HTTP endpoint on the microgateway
- **UIProvider** — provides a WebComponent admin UI for configuring the endpoint content
- **ConfigProvider** — JSON Schema for plugin configuration

## What It Does

1. Registers a `/*` catch-all endpoint at `/plugins/custom-echo-endpoint/` on the gateway
2. Every request returns JSON containing:
   - Full request metadata (method, path, headers, query string, body, etc.)
   - A `custom_content` field whose value is configured via the Studio UI
3. The Studio UI provides a simple text editor for changing the custom content
4. Content changes are persisted to the plugin config in the DB and sync to gateways

## Building

```bash
cd examples/plugins/gateway/custom-echo-endpoint
go build -o custom-echo-endpoint
```

## Registering

1. Start the dev environment: `make dev-full`
2. Open the Admin UI: http://localhost:3000
3. Go to Admin > Plugins > Register Plugin
4. Set:
   - **Name**: `custom-echo-endpoint`
   - **Command**: `file:///app/examples/plugins/gateway/custom-echo-endpoint/custom-echo-endpoint`
   - **Hook type**: `custom_endpoint`
   - **Hook types**: `["custom_endpoint", "studio_ui"]`
   - **Config**: `{"custom_content": "Hello World"}`

## Testing the Endpoint

```bash
# GET request
curl http://localhost:8081/plugins/custom-echo-endpoint/hello?foo=bar

# POST request with body
curl -X POST http://localhost:8081/plugins/custom-echo-endpoint/echo \
  -H "Content-Type: application/json" \
  -d '{"message": "hello"}'
```

## Configuring via Studio UI

1. Navigate to the Studio admin panel
2. Look for "Echo Endpoint" in the sidebar
3. Click "Configure"
4. Edit the custom content text and click Save
5. The gateway endpoint will serve the new content after config sync

## Config Flow

```
Studio UI → save_content RPC → UpdatePluginConfig API → DB
  → gRPC ConfigurationSnapshot → Gateway reloads plugin
  → Initialize(config["custom_content"]) → endpoint serves new content
```

## Documentation

- [Custom Endpoints Guide](../../../../docs/site/docs/plugins-custom-endpoints.md)
- [Plugin SDK Reference](../../../../docs/site/docs/plugins-sdk.md)
- [Plugin System Overview](../../../../docs/site/docs/plugins-overview.md)
