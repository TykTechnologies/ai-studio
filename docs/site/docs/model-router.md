# Model Router (Enterprise)

The **Model Router** is an Enterprise feature that provides intelligent request routing across multiple LLM vendors based on model name patterns. It enables organizations to create unified API endpoints that automatically route requests to the appropriate backend LLM based on configurable rules.

## Overview

Model Router allows you to:

- **Create unified endpoints** - Expose a single API endpoint that routes to multiple LLM vendors
- **Pattern-based routing** - Route requests based on model name patterns (e.g., `claude-*`, `gpt-4*`)
- **Load balancing** - Distribute requests across vendors using round-robin or weighted algorithms
- **Model name translation** - Map requested model names to vendor-specific model names
- **Failover support** - Configure multiple vendors per pool for redundancy

## Key Concepts

### Router

A **Router** is the top-level entity that defines a routing endpoint. Each router:

- Has a unique **slug** that becomes part of the API endpoint URL
- Can be **active** or **inactive**
- Contains one or more **pools** for routing logic
- Supports **namespaces** for multi-tenant deployments

Routers are exposed at `/router/{slug}/v1/chat/completions` and provide an OpenAI-compatible interface.

### Pool

A **Pool** groups vendors that handle specific model patterns. Each pool:

- Has a **model pattern** using glob syntax (e.g., `claude-*`, `gpt-4*`, `*`)
- Defines a **selection algorithm** for choosing among vendors
- Has a **priority** value (higher priority pools are checked first)
- Contains one or more **vendors**

When a request arrives, pools are checked in priority order. The first pool whose pattern matches the requested model name handles the request.

### Vendor

A **Vendor** represents an LLM configuration within a pool. Each vendor:

- References an existing [LLM configuration](./llm-management.md)
- Has a **weight** for weighted load balancing
- Can be **active** or **inactive**
- Contains optional **model mappings** for name translation

### Model Mapping

**Model Mappings** allow translating the requested model name to a different name when sending to a specific vendor. This is configured at the vendor level, enabling different translations for each backend.

For example, if you want to route `gpt-4` requests to both OpenAI and Anthropic:
- OpenAI vendor: No mapping needed (uses `gpt-4` as-is)
- Anthropic vendor: Map `gpt-4` → `claude-3-opus-20240229`

## Selection Algorithms

### Round Robin

The `round_robin` algorithm distributes requests evenly across active vendors in a pool. Each request goes to the next vendor in sequence.

### Weighted

The `weighted` algorithm distributes requests based on vendor weights. A vendor with weight 3 receives three times as many requests as a vendor with weight 1.

## Creating a Model Router

### Via the Admin UI

1. Navigate to **Model Routers** in the admin sidebar
2. Click **Create Router**
3. Fill in the basic details:
   - **Name**: Human-readable name for the router
   - **Slug**: URL-safe identifier (used in the endpoint path)
   - **Description**: Optional description
   - **Active**: Enable/disable the router

4. Add pools by clicking **Add Pool**:
   - **Name**: Pool identifier
   - **Model Pattern**: Glob pattern to match (e.g., `claude-*`)
   - **Selection Algorithm**: Choose `round_robin` or `weighted`
   - **Priority**: Higher values are checked first

5. For each pool, add vendors by clicking **Add Vendor**:
   - **LLM**: Select from your configured LLMs
   - **Weight**: Relative weight for load balancing
   - **Active**: Enable/disable this vendor

6. Optionally add model mappings to vendors:
   - **Source Model**: The model name in the incoming request
   - **Target Model**: The model name to send to this vendor

7. Click **Create** to save the router

### Via the API

```bash
curl -X POST "https://your-host/api/v1/model-routers" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "type": "model-routers",
      "attributes": {
        "name": "Production Router",
        "slug": "prod",
        "description": "Main production model router",
        "active": true,
        "pools": [
          {
            "name": "Claude Models",
            "model_pattern": "claude-*",
            "selection_algorithm": "round_robin",
            "priority": 100,
            "vendors": [
              {
                "llm_id": 1,
                "weight": 1,
                "active": true,
                "mappings": []
              }
            ]
          },
          {
            "name": "GPT Models",
            "model_pattern": "gpt-*",
            "selection_algorithm": "weighted",
            "priority": 90,
            "vendors": [
              {
                "llm_id": 2,
                "weight": 2,
                "active": true,
                "mappings": []
              },
              {
                "llm_id": 3,
                "weight": 1,
                "active": true,
                "mappings": [
                  {
                    "source_model": "gpt-4",
                    "target_model": "claude-3-opus-20240229"
                  }
                ]
              }
            ]
          },
          {
            "name": "Catch All",
            "model_pattern": "*",
            "selection_algorithm": "round_robin",
            "priority": 0,
            "vendors": [
              {
                "llm_id": 1,
                "weight": 1,
                "active": true,
                "mappings": []
              }
            ]
          }
        ]
      }
    }
  }'
```

## Using the Model Router

Once a router is active, you can send OpenAI-compatible requests to its endpoint:

```bash
curl -X POST "https://your-host/router/prod/v1/chat/completions" \
  -H "Authorization: Bearer YOUR_APP_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-sonnet-20240229",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

The router will:
1. Match the model name against pool patterns (in priority order)
2. Select a vendor using the pool's selection algorithm
3. Apply any model mappings for the selected vendor
4. Forward the request to the vendor's LLM endpoint
5. Return the response to the client

## Use Cases

### Multi-Vendor Failover

Create a pool with multiple vendors for the same model pattern. If one vendor is unavailable or disabled, requests automatically go to other active vendors.

```json
{
  "name": "Claude Failover",
  "model_pattern": "claude-*",
  "selection_algorithm": "weighted",
  "vendors": [
    {"llm_id": 1, "weight": 10, "active": true},
    {"llm_id": 2, "weight": 1, "active": true}
  ]
}
```

### Cost Optimization

Route requests to different vendors based on cost. Use weights to prefer cheaper providers while maintaining access to premium ones.

### Model Abstraction

Allow clients to request generic model names (e.g., `large-model`, `fast-model`) and map them to specific vendor models using model mappings.

### A/B Testing

Use weighted routing to gradually shift traffic between model versions or vendors.

## API Reference

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/model-routers` | List all model routers |
| `POST` | `/api/v1/model-routers` | Create a new model router |
| `GET` | `/api/v1/model-routers/{id}` | Get a model router by ID |
| `PATCH` | `/api/v1/model-routers/{id}` | Update a model router |
| `DELETE` | `/api/v1/model-routers/{id}` | Delete a model router |
| `PATCH` | `/api/v1/model-routers/{id}/toggle` | Toggle router active status |

### Query Parameters (List)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page_size` | int | 10 | Number of items per page |
| `page_number` | int | 1 | Page number |
| `all` | bool | false | Return all items without pagination |

## Enterprise Feature

Model Router is an Enterprise Edition feature. Attempting to use Model Router endpoints without an Enterprise license will return a `402 Payment Required` error.

To enable Model Router functionality, ensure your deployment has a valid Enterprise license configured.
