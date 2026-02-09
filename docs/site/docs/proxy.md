# Proxy & API Gateway

The Tyk AI Studio Proxy is the central gateway for all Large Language Model (LLM) interactions within the platform. It acts as a secure, observable, and policy-driven entry point, managing requests from client applications to the configured backend LLM services.

## Gateway Variants

The proxy core library is shared between two gateway variants:

| Variant | Where it runs | Capabilities | Use case |
|---------|---------------|-------------|----------|
| **Embedded Gateway** | Inside AI Studio | Basic LLM proxying only. No filters, no middleware, no plugins. | Testing LLM configurations, powering the Chat interface |
| **Microgateway** | Standalone binary, deployed at edge | Full middleware pipeline: authentication, filters, plugins, analytics, budget enforcement | Production data plane in hub-and-spoke deployments |

Both variants use the same core proxy library and access control mechanisms. The key difference is that the embedded gateway is intentionally lightweight, while the Microgateway provides the complete feature set.

> **Note:** Tools and Data Sources are only available through the embedded gateway in AI Studio. They are not proxied by the Microgateway. See [Architecture Overview](./architecture.md) for details.

## Purpose

The Proxy serves several critical functions:

*   **Unified Access Point:** Provides a single, consistent endpoint for applications to interact with various LLMs.
*   **Security Enforcement:** Handles authentication, authorization, and applies security policies.
*   **Policy Management:** Enforces rules related to budget limits, model access, and applies custom [Filters](./filters.md).
*   **Observability:** Logs detailed analytics data for each request, feeding the [Analytics & Monitoring](./analytics.md) system.
*   **Vendor Abstraction:** Hides the complexities of different LLM provider APIs, especially through the OpenAI-compatible endpoint.

## Core Functions

1.  **Request Routing:** Incoming requests include an `llmSlug` in their path (e.g., `/llm/call/{llmSlug}/...`). The Proxy uses this slug (auto-generated from the LLM configuration name) to identify the target [LLM Configuration](./llm-management.md) and route the request accordingly.

2.  **Authentication & Authorization:**
    *   Validates the API key provided by the client application.
    *   Identifies the associated Application and User.
    *   Checks if the Application/User group has permission to access the requested LLM Configuration based on [RBAC rules](./user-management.md).

3.  **Policy Enforcement:** Before forwarding the request to the backend LLM, the Proxy enforces policies defined in the LLM Configuration or globally:
    *   **Budget Checks:** Verifies if the estimated cost exceeds the configured [Budgets](./llm-management.md) for the App or LLM.
    *   **Model Access:** Ensures the requested model is allowed for the specific LLM configuration.
    *   **Filters:** Applies configured request [Filters](./filters.md) to modify the incoming request payload.

4.  **Analytics Logging:** After receiving the response from the backend LLM (and potentially applying response Filters), the Proxy logs detailed information about the interaction (user, app, model, tokens used, cost, latency, etc.) to the [Analytics](./analytics.md) database.

## Endpoints

Tyk AI Studio exposes proxy endpoints for LLM interactions via two modes, plus legacy endpoints:

### Choosing a Proxy Mode

| Mode | Endpoint | How it works | Best for | Tradeoff |
|------|----------|-------------|----------|----------|
| **SDK-Compatible** | `/llm/call/{llmSlug}/...` | Pass-through to the vendor's native API. No request manipulation beyond tracking for analytics and budgets. | Users working directly with a vendor's SDK or API spec | Full vendor feature access, resilient to API changes |
| **OpenAI-Compatible** | `/llm/call/{llmSlug}/v1/chat/completions` | Accepts OpenAI-format input objects and translates them to the upstream vendor's API format | Maximum client-side compatibility — one format for all vendors | Reduced access to vendor-specific features |

Both modes support streaming and non-streaming responses.

### 1. Unified Endpoint (`/llm/call/{llmSlug}/...`) - Recommended

*   **Purpose:** The primary endpoint for all LLM interactions. It automatically handles both streaming and non-streaming requests based on the request parameters.
*   **SDK-Compatible Mode:** When sending requests in the vendor's native format, the gateway acts as a pass-through with minimal manipulation (only tracking fields for analytics and budgets). This provides full feature access and resilience to API changes.
*   **OpenAI-Compatible Mode:** When sending OpenAI-formatted requests (to `/v1/chat/completions`), Tyk AI Studio translates requests into the format required by the target backend LLM and translates responses back into OpenAI format. This provides maximum compatibility at the cost of vendor-specific features.
*   **Benefits:** Simplifies integration by providing a single endpoint that works for all use cases.

    ```bash
    # Example using curl
    curl -X POST "https://your-ai-studio-host/llm/call/my-openai-config/v1/chat/completions" \
      -H "Authorization: Bearer YOUR_APP_API_KEY" \
      -H "Content-Type: application/json" \
      -d '{
        "model": "gpt-4-turbo",
        "messages": [{"role": "user", "content": "Hello!"}]
      }'
    ```

    ```python
    # Example using OpenAI Python SDK
    import openai

    client = openai.OpenAI(
        base_url="https://your-ai-studio-host/llm/call/my-openai-config/v1",
        api_key="YOUR_APP_API_KEY"
    )

    response = client.chat.completions.create(
        model="gpt-4-turbo",
        messages=[{"role": "user", "content": "Hello!"}]
    )
    print(response.choices[0].message.content)
    ```

### 2. REST Endpoint (`/llm/rest/{llmSlug}/...`) — Legacy

*   **Purpose:** Dedicated endpoint for non-streaming (synchronous) LLM requests only.
*   **Usage:** Use when you explicitly want to ensure the request is processed synchronously without streaming.
*   **Note:** This is a legacy endpoint from before the unified endpoint existed. The underlying response handling code is still used internally by the unified endpoint.

### 3. Stream Endpoint (`/llm/stream/{llmSlug}/...`) — Legacy

*   **Purpose:** Dedicated endpoint for streaming LLM responses using Server-Sent Events (SSE).
*   **Usage:** Use when you need real-time token-by-token streaming responses.
*   **Note:** This is a legacy endpoint. The unified endpoint now auto-detects whether to stream based on request parameters. The underlying streaming code is still used internally.

### LLM Slug

The `{llmSlug}` in the endpoint path is automatically generated from the LLM configuration name when you create it. For example, an LLM named "My OpenAI Config" would have a slug like `my-openai-config`.

## Configuration & Security

The behavior of the Proxy for a specific route is determined by the corresponding [LLM Configuration](./llm-management.md), which includes details about the backend vendor, model access, budget limits, and associated filters.

By centralizing LLM access through the Proxy, Tyk AI Studio provides a robust layer for security, control, and observability over AI interactions.
