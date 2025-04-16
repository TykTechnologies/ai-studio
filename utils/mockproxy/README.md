# MockProxy

A simple utility to demonstrate the embedded proxy implementation of the Midsommar LLM proxy library.

## Overview

This utility provides a standalone implementation of the proxy server using the embedded proxy feature. It allows you to test and experiment with the proxy functionality without requiring the full Midsommar platform infrastructure.

## Features

- Simple JSON configuration for proxy setup
- Mock implementations of the required interfaces
- Support for multiple LLMs and datasources
- Basic command-line interface
- Built-in analytics recording without database connection
- Configurable analytics output (console, file, or both)

## Usage

### Building

Build the mockproxy utility using the provided build script:

```bash
./build.sh
```

This will compile the utility and create an executable named `mockproxy`.

### Running

Run the utility with a configuration file:

```bash
./mockproxy --conf ./conf.json
```

Additional command-line options for analytics:

```bash
# Default - logs to both console and file
./mockproxy --conf ./conf.json

# Log analytics to console only
./mockproxy --conf ./conf.json --analytics console

# Log analytics to file only
./mockproxy --conf ./conf.json --analytics file --log-file ./my-analytics.log

# Log analytics to both console and file with custom filename
./mockproxy --conf ./conf.json --analytics both --log-file ./my-analytics.log
```

### Configuration

The configuration file (`conf.json`) allows you to specify:

- Proxy server port
- LLM configurations (name, vendor, API endpoint, etc.)
- Datasource configurations
- User credentials

Example configuration:

```json
{
  "proxyPort": 8080,
  "llms": [
    {
      "name": "MockGPT",
      "vendor": "MOCK_VENDOR",
      "apiEndpoint": "http://localhost:9000",
      "apiKey": "mock_api_key",
      "active": true,
      "models": ["gpt-3.5-turbo", "gpt-4"]
    }
  ],
  "datasources": [
    {
      "name": "MockVectorDB",
      "type": "vector",
      "active": true
    }
  ],
  "users": [
    {
      "email": "test@example.com",
      "apiKey": "test_api_key"
    }
  ]
}
```

## Limitations

This is a simplified implementation for demonstration purposes only:

- Authentication is bypassed
- Budget checks always pass
- No real connections to LLM providers
- No persistent storage

## Implementation Details

The mockproxy demonstrates the "Embedded Usage with Custom Implementations" pattern from the proxy library documentation. It implements the following interfaces:

- `ProxyServiceInterface` - Mock service providing access to LLMs, datasources, etc.
- `BudgetServiceInterface` - Mock budget service that approves all requests
- `AuthServiceInterface` - Mock authentication service
- `ProxyDependencies` - Container for all the above dependencies

## Analytics

The mockproxy includes a lightweight analytics implementation that doesn't require a database connection:

- Records the same types of analytics data as the full Midsommar platform:
  - Proxy logs (request/response details)
  - Chat records (token usage, costs)
  - Tool calls
  - Content messages
- Formats all data as human-readable JSON for easy inspection
- Provides flexible output options:
  - Console output for real-time monitoring
  - File output for persistent logging
  - Both simultaneously
- Thread-safe implementation using mutex locks

This analytics implementation is useful for:
- Debugging proxy interactions
- Testing analytics data generation
- Demonstrating the analytics capabilities without requiring a database
- Learning about the structure of analytics data in the Midsommar platform

## Testing the Proxy

Once the proxy is running, you can test it using curl:

```bash
# Test an LLM endpoint
curl -X POST http://localhost:8080/llm/rest/mockgpt/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}'

# Test a datasource endpoint
curl -X POST http://localhost:8080/datasource/mockvectordb \
  -H "Content-Type: application/json" \
  -d '{"query": "test query", "n": 5}'
```

Note: Since this is a mock implementation, the LLM responses will be empty or placeholder data.
