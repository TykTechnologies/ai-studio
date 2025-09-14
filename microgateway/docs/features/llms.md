# LLM Provider Management

The microgateway supports multiple LLM providers through a unified interface, allowing centralized management of AI/LLM configurations.

## Overview

LLM management features:
- **Multi-Provider Support**: OpenAI, Anthropic, Google AI, Vertex AI, Ollama
- **Dynamic Configuration**: Hot-reload configurations without restart
- **Vendor Abstraction**: Unified interface across different providers
- **Usage Tracking**: Per-LLM analytics and performance metrics
- **Budget Controls**: Individual budget limits per LLM
- **Rate Limiting**: Configurable request rate controls

## Supported Providers

### OpenAI
```bash
# Create OpenAI LLM
mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=1000.0 \
  --rate-limit=100

# Supported models: gpt-3.5-turbo, gpt-4, gpt-4-turbo, etc.
```

### Anthropic
```bash
# Create Anthropic LLM
mgw llm create \
  --name="Claude 3.5 Sonnet" \
  --vendor=anthropic \
  --model=claude-3-5-sonnet-20241022 \
  --api-key=$ANTHROPIC_API_KEY \
  --budget=500.0

# Supported models: claude-3-5-sonnet-*, claude-3-*, etc.
```

### Google AI
```bash
# Create Google AI LLM
mgw llm create \
  --name="Gemini Pro" \
  --vendor=google \
  --model=gemini-pro \
  --api-key=$GOOGLE_AI_API_KEY \
  --budget=300.0
```

### Vertex AI
```bash
# Create Vertex AI LLM
mgw llm create \
  --name="Vertex Gemini" \
  --vendor=vertex \
  --model=gemini-pro \
  --api-key=$VERTEX_AI_CREDENTIALS \
  --budget=400.0
```

### Ollama (Local/Self-Hosted)
```bash
# Create Ollama LLM
mgw llm create \
  --name="Local Llama" \
  --vendor=ollama \
  --model=llama3.1:8b \
  --endpoint=http://localhost:11434 \
  --budget=0  # No cost for local models
```

## LLM Configuration

### Required Parameters
```bash
# Minimum required for all providers
--name="Display Name"        # Human-readable name
--vendor=[provider]          # openai, anthropic, google, vertex, ollama
--model=[model-name]         # Provider-specific model identifier
```

### Provider-Specific Parameters
```bash
# OpenAI and Anthropic require API keys
--api-key=$API_KEY

# Ollama requires endpoint URL
--endpoint=http://localhost:11434

# All providers support optional parameters
--budget=1000.0              # Monthly budget limit
--rate-limit=100             # Requests per minute
--timeout=30                 # Request timeout in seconds
--retry-count=3              # Number of retries on failure
--max-tokens=4096            # Maximum tokens per request
```

### Advanced Configuration
```bash
# Create LLM with all options
mgw llm create \
  --name="GPT-4 Production" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=2000.0 \
  --rate-limit=200 \
  --timeout=60 \
  --retry-count=5 \
  --max-tokens=8192 \
  --active=true
```

## LLM Management

### List LLMs
```bash
# List all LLMs
mgw llm list

# Filter by vendor
mgw llm list --vendor=openai

# Include inactive LLMs
mgw llm list --active=false

# Paginated results
mgw llm list --page=2 --limit=5
```

### Get LLM Details
```bash
# Get specific LLM
mgw llm get 1

# JSON output for scripting
mgw llm get 1 --format=json
```

### Update LLMs
```bash
# Update budget
mgw llm update 1 --budget=1500.0

# Update multiple settings
mgw llm update 1 \
  --budget=2000.0 \
  --rate-limit=150 \
  --active=true

# Update API key
mgw llm update 1 --api-key=$NEW_API_KEY
```

### Delete LLMs
```bash
# Soft delete (sets deleted_at timestamp)
mgw llm delete 1

# LLM remains in database but becomes inactive
```

## LLM Usage

### Gateway Endpoints
Once configured, LLMs are accessible via gateway endpoints:

```bash
# OpenAI-compatible endpoint
POST /llm/rest/{llm-slug}/chat/completions

# Streaming endpoint
POST /llm/stream/{llm-slug}/chat/completions

# LLM slug is generated from name (e.g., "GPT-4" -> "gpt-4")
```

### Example Usage
```bash
# Make request through gateway
curl -X POST http://localhost:8080/llm/rest/gpt-4/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $APP_TOKEN" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello, world!"}
    ]
  }'
```

## LLM Statistics

### Usage Statistics
```bash
# Get LLM usage stats
mgw llm stats 1

# Example output shows:
# - Request count
# - Total tokens used
# - Total cost
# - Average latency
```

### Performance Monitoring
```bash
# Monitor LLM performance
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.llm_id == 1) | {latency_ms, status_code, total_tokens}'

# Average response time
mgw analytics summary 1 --format=json | \
  jq '.data.average_latency'
```

## LLM Configuration Management

### Configuration Templates
```bash
# Export LLM configurations
mgw llm list --format=yaml > llm-configs.yaml

# Import configurations (manual process)
# Parse YAML and create LLMs via CLI
```

### Bulk Operations
```bash
# Create multiple OpenAI models
for model in gpt-3.5-turbo gpt-4 gpt-4-turbo; do
  mgw llm create \
    --name="OpenAI $model" \
    --vendor=openai \
    --model=$model \
    --api-key=$OPENAI_API_KEY \
    --budget=500.0
done
```

### Environment-Specific Configurations
```bash
# Development LLMs (lower budgets)
mgw llm create \
  --name="Dev GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$DEV_OPENAI_KEY \
  --budget=100.0

# Production LLMs (higher budgets)
mgw llm create \
  --name="Prod GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$PROD_OPENAI_KEY \
  --budget=5000.0
```

## Application-LLM Associations

### Managing Associations
```bash
# View LLMs associated with an app
mgw app llms 1

# Associate app with specific LLMs
mgw app llms 1 --set="1,2,3"

# Add LLMs to existing associations
mgw app llms 1 --add="4,5"

# Remove LLMs from associations
mgw app llms 1 --remove="3"
```

### Access Control
Applications can only access LLMs they are associated with:
- Association controls which LLMs an app can use
- Budget enforcement applies across all associated LLMs
- Analytics track usage per LLM per application

## Error Handling and Retries

### Retry Configuration
```bash
# Configure retry behavior
mgw llm create \
  --name="Resilient GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --retry-count=5 \
  --timeout=60
```

### Error Handling
The microgateway handles:
- Network timeouts and connection errors
- API rate limits from providers
- Authentication failures
- Model availability issues
- Cost estimation errors

### Fallback Behavior
```bash
# Monitor error rates
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.status_code >= 400) | {llm_id, status_code, error_message}'
```

## Provider-Specific Features

### OpenAI Features
- Support for all GPT models
- Fine-tuned model support
- Embedding model support
- Function calling capabilities

### Anthropic Features
- Claude model family support
- Streaming response support
- Large context window handling

### Ollama Features
- Local model hosting
- Custom model support
- No external API dependencies
- Zero cost operation

## Configuration Examples

### High-Performance Setup
```bash
# Optimized for performance
mgw llm create \
  --name="Fast GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --timeout=15 \
  --retry-count=1 \
  --rate-limit=500
```

### Cost-Optimized Setup
```bash
# Optimized for cost
mgw llm create \
  --name="Cheap GPT-3.5" \
  --vendor=openai \
  --model=gpt-3.5-turbo \
  --api-key=$OPENAI_API_KEY \
  --budget=100.0 \
  --max-tokens=1000
```

### Development Setup
```bash
# Development environment
mgw llm create \
  --name="Dev Local Llama" \
  --vendor=ollama \
  --model=llama3.1:8b \
  --endpoint=http://localhost:11434 \
  --budget=0 \
  --timeout=120
```

## Troubleshooting

### Connection Issues
```bash
# Test LLM connectivity
mgw llm get 1

# Check LLM status
mgw llm list --format=json | jq '.data[] | {id, name, is_active}'

# Review error logs
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.llm_id == 1 and .status_code >= 400)'
```

### Authentication Issues
```bash
# Update API key
mgw llm update 1 --api-key=$NEW_API_KEY

# Test with curl
curl -H "Authorization: Bearer $API_KEY" \
  https://api.openai.com/v1/models
```

### Performance Issues
```bash
# Check response times
mgw analytics summary 1 --format=json | \
  jq '.data.average_latency'

# Analyze slow requests
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.latency_ms > 5000)'

# Adjust timeout settings
mgw llm update 1 --timeout=60
```

---

LLM management provides the foundation for AI/LLM gateway functionality. For usage analytics, see [Analytics](analytics.md). For budget controls, see [Budgets](budgets.md).
