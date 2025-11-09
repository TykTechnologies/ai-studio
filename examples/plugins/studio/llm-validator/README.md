# LLM Validator Plugin

An example plugin demonstrating Object Hooks for validating LLM configurations.

## Features

- Validates LLM API endpoints require HTTPS
- Blocks specific vendors based on organizational policy
- Enforces minimum privacy scores
- Requires descriptions for all LLMs
- Stores validation metadata in LLM object

## Configuration

```json
{
  "require_https": true,
  "blocked_vendors": ["untrusted-vendor"],
  "min_privacy_score": 5,
  "require_description": true
}
```

## Hooks

- **before_create**: Validates LLM before creation
- **before_update**: Validates LLM updates

## Building

```bash
go build -o llm-validator
```

## Installing in AI Studio

1. Build the plugin
2. Place binary in plugins directory
3. Create plugin in AI Studio UI:
   - Name: "LLM Validator"
   - Command: `/path/to/llm-validator`
   - Config: (JSON configuration above)
4. Enable the plugin

## Testing

Try creating an LLM with:
- HTTP endpoint → Should be rejected
- Privacy score below minimum → Should be rejected
- Missing description → Should be rejected
- Valid configuration → Should succeed with validation metadata

## Metadata Stored

The plugin stores validation information in the LLM's metadata field:

```json
{
  "plugin_1_validated_by": "llm-validator",
  "plugin_1_validated_at": "request-id-123",
  "plugin_1_validation_rules": "https=true,privacy>=5"
}
```

## Development

This plugin demonstrates:
- Implementing `ObjectHookHandler` interface for object interception
- Implementing `ConfigProvider` interface for configuration schema
- Implementing `ManifestProvider` interface for plugin metadata
- Embedding manifest.json and config.schema.json files
- Parsing and validating object data
- Rejecting operations with clear error messages
- Storing plugin metadata in objects
- Configurable validation rules

## Required Files

This plugin includes the following embedded files:

- **manifest.json** - Declares plugin capabilities, permissions, and metadata
- **config.schema.json** - JSON Schema for plugin configuration UI
- **main.go** - Plugin implementation with all required interfaces
