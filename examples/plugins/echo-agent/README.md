# Echo Agent Plugin

A simple test agent plugin that wraps all LLM responses in `<<` `>>` markers.

## Purpose

This plugin is designed for end-to-end testing of the AI Studio agent plugin architecture. It verifies the complete flow:

**UI → Studio → Plugin → LLM (via Studio) → Studio → UI**

## Functionality

1. Receives user messages via the agent session
2. Forwards messages to the configured LLM using the AI Studio SDK
3. Wraps the LLM response in `<<` and `>>`
4. Streams the wrapped response back to the UI

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
   - Slug: echo-agent
   - Command: `/path/to/echo-agent`
   - Hook Type: agent
3. Create an agent configuration:
   - Select the Echo Agent plugin
   - Select an app with at least one LLM
   - Configure access groups if needed
4. Test via the portal agent chat interface

## Expected Output

When you send a message like "Hello, how are you?", the response will be wrapped:

```
<< I'm doing well, thank you for asking! How can I help you today? >>
```

This wrapping confirms the plugin is successfully processing the messages in the flow.
