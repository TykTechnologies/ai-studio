# AI Gateway Library - Filter Support Demo

This demo shows how to use the AI Gateway library with file-based filter support for scripting/policy enforcement.

## Overview

Filters allow you to implement custom logic using Tengo scripts to:
- Block requests containing sensitive content
- Log incoming requests for monitoring
- Detect and prevent PII leakage
- Implement custom validation rules

## Configuration Files

### 1. filters.json
Contains filter definitions with Tengo scripts:

```json
{
  "filters": [
    {
      "id": 1,
      "name": "Content Blocker",
      "description": "Blocks requests containing sensitive content",
      "script": "// Tengo script here..."
    }
  ]
}
```

### 2. llms.json (Updated)
LLM configurations now support `filter_ids` to associate filters:

```json
{
  "llms": [
    {
      "id": 1,
      "name": "GPT-4",
      "vendor": "openai",
      "filter_ids": [1, 2]
    }
  ]
}
```

## Sample Filters

### 1. Content Blocker
Blocks requests containing "sensitive", "confidential", or "secret":

```tengo
text := strings.lower(payload)
if strings.contains(text, "sensitive") || strings.contains(text, "confidential") || strings.contains(text, "secret") {
    result = false  // Block the request
} else {
    result = true   // Allow the request
}
```

### 2. Request Logger
Logs all incoming requests (always allows):

```tengo
fmt.println("Filter: Processing request with payload length:", len(payload))
fmt.println("First 100 chars:", strings.substr(payload, 0, min(100, len(payload))))
result = true  // Always allow
```

### 3. PII Detector
Detects potential email addresses:

```tengo
text := strings.lower(payload)
if strings.contains(text, "@") && strings.contains(text, ".com") {
    fmt.println("PII Filter: Potential email detected, blocking request")
    result = false
} else {
    result = true
}
```

## Running the Demo

### Test Filter Configuration
```bash
cd examples/file-based-demo
go run test_filters_demo.go
```

### Use with AI Gateway Library
```go
// Create file-based services
gatewayService, err := services.NewFileGatewayService("./config")
budgetService := services.NewFileBudgetService("./config")

// Create gateway with filter support
gateway := aigateway.New(gatewayService, budgetService, &aigateway.Config{Port: 9090})

// Start gateway - filters will be automatically applied
gateway.Start()
```

## Filter Associations

Based on the current configuration:
- **GPT-4**: Content Blocker + Request Logger
- **GPT-3.5 Turbo**: Request Logger only
- **Claude-3.5 Sonnet**: Content Blocker + PII Detector
- **Google Gemini Pro**: No filters

## How Filters Work

1. **Request Processing**: When a request comes to an LLM with filters
2. **Filter Execution**: Each associated filter script runs in sequence
3. **Decision Making**: If any filter returns `result = false`, the request is blocked (HTTP 403)
4. **Pass Through**: If all filters return `result = true`, the request proceeds to the LLM

## Adding New Filters

1. Add filter definition to `filters.json`
2. Associate with LLMs by adding filter ID to `filter_ids` array in `llms.json`
3. Reload the gateway configuration

## Tengo Script Guidelines

- Set `result = true` to allow the request
- Set `result = false` to block the request (returns HTTP 403)
- Access request payload via `payload` variable
- Use standard Tengo functions like `strings.lower()`, `strings.contains()`
- Use `fmt.println()` for logging/debugging

## Advanced Features

- **Script Extensions**: Access to `tyk.makeHTTPRequest()` and `tyk.llm()` functions
- **Hot Reloading**: Call `gateway.Reload()` to refresh configuration
- **Error Handling**: Malformed scripts or runtime errors block requests
- **Performance**: Scripts are cached and executed efficiently

This implementation provides a working demonstration of the filtering system while keeping the file-based demo simple and educational.
