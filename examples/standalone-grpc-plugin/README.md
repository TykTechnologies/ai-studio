# Standalone gRPC Plugin Example

This is a standalone version of the message modifier plugin that runs as an independent gRPC server, demonstrating the new external plugin connectivity feature.

## Overview

This plugin modifies outbound LLM requests by adding a custom instruction to the last user message in chat completions. It's designed to test the new `grpc://` plugin configuration support in the microgateway.

## Features

- **Standalone Operation**: Runs as independent gRPC server
- **Message Modification**: Adds custom instructions to user messages
- **Health Monitoring**: Supports ping-based health checks
- **Graceful Shutdown**: Handles SIGINT/SIGTERM signals
- **Configurable**: Custom instruction via command line flag

## Building

```bash
cd examples/standalone-grpc-plugin
go mod tidy
go build -o message-modifier-grpc .
```

## Running

### Basic Usage
```bash
./message-modifier-grpc
```

### Custom Configuration
```bash
# Custom port and instruction
./message-modifier-grpc -port=9090 -instruction="Please respond in pirate speak!"
```

### Command Line Options
- `-port`: gRPC server port (default: 8080)
- `-instruction`: Custom instruction to add to messages (default: "Say Moo! at the end of your response")

## Testing Connectivity

### 1. Start the Plugin Server
```bash
./message-modifier-grpc -port=8080
```

You should see output like:
```
🚀 Standalone Message Modifier Plugin starting on port 8080
📝 Instruction: Say Moo! at the end of your response
🔧 Plugin: standalone-message-modifier v1.0.0
⚡ Hook Type: pre_auth
✅ gRPC server listening on :8080
📊 Test with: grpc://localhost:8080
🔄 Use Ctrl+C to stop
```

### 2. Configure Microgateway Plugin
Add a plugin record to your database:

```sql
INSERT INTO plugins (name, slug, description, command, hook_type, is_active) VALUES
('standalone-message-modifier', 'standalone-message-modifier', 'External gRPC message modifier plugin', 'grpc://localhost:8080', 'pre_auth', true);
```

### 3. Associate Plugin with LLM
```sql
-- Associate with an LLM (replace 1 with your LLM ID)
INSERT INTO llm_plugins (llm_id, plugin_id) VALUES (1, LAST_INSERT_ID());
```

### 4. Test the Plugin
Make a request through the microgateway to an LLM that has this plugin associated. The plugin will modify chat completion requests by adding the custom instruction to the last user message.

## Expected Behavior

When a chat completion request like this:
```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "user", "content": "Hello, how are you?"}
  ]
}
```

Is processed, the plugin will modify it to:
```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "user", "content": "Hello, how are you?\n\nSay Moo! at the end of your response"}
  ]
}
```

## Logs

The plugin provides detailed logging:
- Connection attempts and status
- Request processing details
- Message modification activities
- Health check responses
- Shutdown handling

## Docker Deployment

### Dockerfile
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o message-modifier-grpc .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/message-modifier-grpc .
EXPOSE 8080
CMD ["./message-modifier-grpc", "-port=8080"]
```

### Docker Compose
```yaml
version: '3.8'
services:
  message-modifier:
    build: .
    ports:
      - "8080:8080"
    environment:
      - PLUGIN_PORT=8080
    command: ["./message-modifier-grpc", "-port=8080", "-instruction=Add some sparkle! ✨"]

  microgateway:
    image: tyk/microgateway:latest
    ports:
      - "8081:8080"
    depends_on:
      - message-modifier
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/mgw
```

## Troubleshooting

### Connection Issues
- Ensure the plugin server is running and accessible
- Check firewall settings for the gRPC port
- Verify the `grpc://` URL format is correct
- Review microgateway logs for connection retry attempts

### Plugin Not Executing
- Confirm the plugin is associated with the correct LLM
- Verify the hook type is `pre_auth`
- Check that the plugin is marked as `is_active = true`
- Ensure the request matches the plugin's criteria (POST with messages)

### Health Check Failures
- Plugin server should respond to ping requests
- Check server logs for health check errors
- Verify gRPC service is properly registered

## Integration with Microgateway

This plugin demonstrates the new external gRPC plugin architecture:
- **Automatic Retry**: Microgateway will retry failed connections
- **Health Monitoring**: Regular ping-based health checks
- **Graceful Degradation**: Continues operation if plugin unavailable
- **Load Balancing**: Can be deployed behind load balancers for HA

The plugin integrates seamlessly with the existing microgateway plugin system while providing the benefits of independent deployment and scaling.