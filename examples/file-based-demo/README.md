# AI Gateway Library - File-Based Demo

This demo application showcases how to use the AI Gateway Library with JSON configuration files instead of a database. It provides a complete, standalone example of setting up an AI gateway using file-based configurations for LLMs, credentials, apps, pricing, and budget management.

## 🚀 Features Demonstrated

- **File-based Configuration**: Complete setup using JSON files instead of database
- **Multiple LLM Providers**: OpenAI, Anthropic, Google AI support with environment variable resolution
- **Budget Management**: Real-time budget tracking and enforcement
- **Authentication**: Token-based authentication with multiple credential levels
- **Analytics**: File-based analytics with JSON logs for all gateway activity
- **Graceful Shutdown**: Proper server lifecycle management
- **Comprehensive Testing**: Full test suite for all components

## 📁 Project Structure

```
examples/file-based-demo/
├── main.go                          # Main demo application
├── go.mod                          # Go module definition
├── README.md                       # This file
├── config/                         # JSON configuration files
│   ├── llms.json                  # LLM provider configurations
│   ├── credentials.json           # Authentication credentials
│   ├── apps.json                  # Application definitions
│   ├── pricing.json               # Model pricing information
│   └── budgets.json               # Budget limits and tracking
├── services/                       # File-based service implementations
│   ├── file_gateway_service.go    # Gateway service implementation
│   ├── file_budget_service.go     # Budget service implementation
│   ├── file_analytics_handler.go  # Analytics handler implementation
│   ├── file_gateway_service_test.go    # Gateway service tests
│   ├── file_budget_service_test.go     # Budget service tests
│   └── file_analytics_handler_test.go  # Analytics handler tests
└── analytics/                     # Runtime analytics logs (created automatically)
    ├── chat_records.json          # LLM usage records
    ├── tool_calls.json            # Tool execution logs
    └── proxy_logs.json            # Proxy request/response logs
```

## 🔧 Setup & Installation

### Prerequisites

- Go 1.21+ installed
- API keys for the LLM providers you want to test

### Installation

1. **Navigate to the demo directory:**
   ```bash
   cd examples/file-based-demo
   ```

2. **Set up environment variables for API keys:**
   ```bash
   export OPENAI_API_KEY=your_openai_key_here
   export ANTHROPIC_API_KEY=your_anthropic_key_here  
   export GOOGLE_AI_API_KEY=your_google_ai_key_here
   ```

3. **Run the demo:**
   ```bash
   go run main.go
   ```

The server will start on `http://localhost:9090` and display detailed usage instructions.

## 📊 Configuration Files

### LLMs Configuration (`config/llms.json`)

Defines the available LLM providers with their endpoints, models, and API keys:

```json
{
  "llms": [
    {
      "id": 1,
      "name": "GPT-4",
      "slug": "gpt4",
      "vendor": "openai",
      "endpoint": "https://api.openai.com/v1/chat/completions",
      "api_key": "$OPENAI_API_KEY",
      "model": "gpt-4",
      "active": true,
      "monthly_budget": 500.0
    }
  ]
}
```

**Features:**
- Environment variable resolution for API keys (`$VARIABLE_NAME`)
- Active/inactive status control
- Per-LLM budget limits
- Support for multiple vendors (OpenAI, Anthropic, Google AI, etc.)

### Credentials Configuration (`config/credentials.json`)

Defines authentication tokens for accessing the gateway:

```json
{
  "credentials": [
    {
      "id": 1,
      "name": "demo-key-12345",  
      "secret": "demo-key-12345",
      "active": true,
      "description": "Demo credential with full access"
    }
  ]
}
```

### Apps Configuration (`config/apps.json`)

Links credentials to specific LLMs and sets app-level budgets:

```json
{
  "apps": [
    {
      "id": 1,
      "name": "Demo Chat App",
      "credential_id": 1,
      "llm_ids": [1, 2, 3],
      "monthly_budget": 100.0,
      "budget_start_date": "2025-01-01T00:00:00Z"
    }
  ]
}
```

### Pricing Configuration (`config/pricing.json`)

Defines cost per token for different models:

```json
{
  "model_prices": [
    {
      "id": 1,
      "model": "gpt-4",
      "vendor": "openai", 
      "prompt_price": 0.03,
      "response_price": 0.06,
      "currency": "USD",
      "per_tokens": 1000
    }
  ]
}
```

### Budget Configuration (`config/budgets.json`)

Tracks real-time usage and budget limits:

```json
{
  "app_budgets": [
    {
      "app_id": 1,
      "monthly_limit": 100.0,
      "current_usage": 25.50,
      "currency": "USD",
      "reset_date": "2025-02-01T00:00:00Z",
      "notifications": {
        "50_percent": false,
        "80_percent": false, 
        "90_percent": false,
        "100_percent": false
      }
    }
  ]
}
```

## 🧪 Testing

The demo includes comprehensive tests for all components:

```bash
# Run all tests
go test ./services -v

# Run specific test files
go test -v ./services -run TestFileGatewayService
go test -v ./services -run TestFileBudgetService
go test -v ./services -run TestFileAnalyticsHandler
```

**Test Coverage:**
- File-based service implementations
- Configuration loading and validation
- Budget tracking and enforcement
- Analytics recording and retrieval  
- Error handling and edge cases
- Concurrent access safety
- Configuration reloading

## 🌐 API Usage Examples

Once the server is running, you can test it with these curl commands:

### 1. Chat with GPT-4

```bash
curl -X POST http://localhost:9090/llm/rest/gpt4/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer demo-key-12345" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello! Can you tell me about AI gateways?"}
    ],
    "max_tokens": 150
  }'
```

### 2. Chat with Claude

```bash
curl -X POST http://localhost:9090/llm/rest/claude35sonnet/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer demo-key-12345" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 150,
    "messages": [
      {"role": "user", "content": "Explain the benefits of AI gateways"}
    ]
  }'
```

### 3. Test Budget Limits

```bash
curl -X POST http://localhost:9090/llm/rest/gpt35turbo/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer budget-key-67890" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Available Endpoints

- `POST /llm/rest/{llmSlug}/chat/completions` - Standard LLM API calls
- `POST /llm/stream/{llmSlug}/chat/completions` - Streaming LLM calls  
- `GET /.well-known/oauth-protected-resource` - OAuth2 metadata

## 📈 Real-time Monitoring

The demo provides real-time monitoring through:

### Console Output
- **Configuration Summary**: Shows loaded LLMs, credentials, and budgets
- **Real-time Analytics**: Live updates for each API call with token counts and costs
- **Budget Notifications**: Warnings when approaching or exceeding limits
- **Error Logging**: Detailed error information for troubleshooting

### Analytics Files
All activity is logged to JSON files in the `analytics/` directory:

- `chat_records.json` - Complete LLM usage records with tokens and costs
- `tool_calls.json` - Tool execution logs (when tools are used)
- `proxy_logs.json` - HTTP proxy request/response logs

### Budget Tracking
- Real-time budget usage updates
- Automatic budget enforcement  
- Threshold notifications (50%, 80%, 90%, 100%)
- Monthly reset functionality

## 🔄 Hot Reloading

The demo supports configuration hot reloading without server restart:

```bash
# Send SIGHUP to reload configurations
kill -HUP <server_pid>

# Or use the reload endpoint (if implemented)
curl -X POST http://localhost:9090/admin/reload \
  -H "Authorization: Bearer admin-key"
```

## 🏗️ Architecture

The demo demonstrates the AI Gateway Library's pluggable architecture:

```go
// File-based services implementing standard interfaces
gatewayService := services.NewFileGatewayService(configDir)
budgetService := services.NewFileBudgetService(configDir)  
analyticsHandler := services.NewFileAnalyticsHandler(analyticsDir)

// Create gateway with custom implementations
gateway := aigateway.NewWithAnalytics(
    gatewayService,
    budgetService, 
    analyticsHandler,
    &aigateway.Config{Port: 9090},
)
```

This design allows easy swapping between:
- **Database vs File-based storage**
- **Different analytics backends** (HTTP, message queues, files)
- **Custom authentication providers**
- **Alternative budget enforcement strategies**

## 🚦 Production Considerations

While this demo uses file-based storage for simplicity, production deployments should consider:

### Scalability
- Use database-backed services for high throughput
- Implement distributed caching for configuration data
- Add load balancing for multiple gateway instances

### Security  
- Use proper secret management (HashiCorp Vault, AWS Secrets Manager)
- Implement TLS/SSL encryption
- Add rate limiting and DDoS protection
- Audit logging and compliance features

### Reliability
- Database replication and backups
- Health checks and monitoring
- Circuit breakers for LLM providers
- Retry logic with exponential backoff

### Observability
- Integration with Prometheus/Grafana
- Distributed tracing with Jaeger/Zipkin
- Structured logging with ELK stack
- Custom dashboards for cost and usage analytics

## 🤝 Contributing

This demo serves as both a functional example and a template for building custom AI gateway implementations. Contributions are welcome for:

- Additional LLM provider integrations
- Enhanced monitoring and alerting
- Performance optimizations  
- Security improvements
- Documentation updates

## 📚 Related Documentation

- [AI Gateway Library README](../../pkg/aigateway/README.md)
- [Feature Specification](../../features/AIGatewayLibrary.md)
- [Main Midsommar Documentation](../../README.md)

## 🔗 Support

For questions, issues, or feature requests related to this demo:

1. Check the main AI Gateway Library documentation
2. Review the comprehensive test suite for usage examples
3. Examine the configuration files for setup guidance
4. Open an issue in the main Midsommar repository

---

**Note**: This demo is designed for development and testing purposes. For production use, consider the architectural recommendations above and implement appropriate security, scalability, and reliability measures.
