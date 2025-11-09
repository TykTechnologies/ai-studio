# Echo Agent Plugin

A simple test agent plugin that wraps user messages in configurable markers (default: `<<` `>>`).

## Purpose

This plugin is designed for end-to-end testing of the AI Studio agent plugin architecture. It verifies the complete flow:

**UI → Studio → Plugin → Studio → UI**

## Functionality

1. Receives user messages via the agent session
2. Echoes back the user message wrapped in configurable prefix/suffix markers
3. Streams the wrapped response back to the UI

This simple echo behavior confirms the agent messaging pipeline works without requiring an actual LLM integration.

## Building

```bash
cd server
make build
```

## Running

```bash
cd server
make run
```

Or specify a custom port:

```bash
PLUGIN_PORT=50052 ./echo-agent
```

## Installation in AI Studio

1. Build the plugin binary
2. Register the plugin in AI Studio admin UI:
   - Name: Echo Agent
   - Slug: com.tyk.echo-agent
   - Command: `/path/to/examples/plugins/echo-agent/server/echo-agent`
   - Plugin Type: agent
3. Create an agent configuration:
   - Select the Echo Agent plugin
   - Select an app (LLMs are not required for this test plugin)
   - Optionally configure custom prefix/suffix in config JSON
   - Configure access groups if needed
4. Test via the portal agent chat interface

## Configuration

The plugin supports optional configuration via the config schema:

```json
{
  "prefix": "<<",
  "suffix": ">>",
  "include_metadata": false
}
```

## Expected Output

When you send a message like "Hello, how are you?", the response will be:

```
<< Hello, how are you? >>
```

This confirms the plugin is successfully processing messages in the agent flow.
