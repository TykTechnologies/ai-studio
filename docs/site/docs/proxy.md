# Proxy & API Gateway

The Tyk AI Studio Proxy is the central gateway for all Large Language Model (LLM) interactions within the platform. It acts as a secure, observable, and policy-driven entry point, managing requests from client applications to the configured backend LLM services.

## Gateway Variants

The proxy core library is shared between two gateway variants:

| Variant | Where it runs | Capabilities | Use case |
|---------|---------------|-------------|----------|
| **Embedded Gateway** | Inside AI Studio | LLM proxying, tool calling (REST + MCP), datasource querying. No filters, no middleware, no plugins. | Testing LLM configurations, powering the Chat interface |
| **Microgateway** | Standalone binary, deployed at edge | Full middleware pipeline: authentication, filters, plugins, analytics, budget enforcement, tool calling (REST + MCP), datasource querying | Production data plane in hub-and-spoke deployments |

Both variants use the same core proxy library and access control mechanisms. The key difference is that the embedded gateway is intentionally lightweight, while the Microgateway provides the complete feature set. Tools, datasources, and OAuth state are synced to edge gateways via the hub-spoke configuration system.

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

Every LLM endpoint below authenticates with the same App API key and enforces the same policies, budgets, [Filters](./filters.md) and analytics. They differ in **how the target LLM is chosen** and **what request format they accept**:

| Endpoint | LLM chosen by | Request format | Use it when |
|----------|---------------|----------------|-------------|
| **Main Ingress** — `/v1/chat/completions` | The `model` field, namespaced `{llmSlug}/{model}` | OpenAI Chat Completions only | You want one base URL covering every LLM the App can reach, choosing the model — including across vendors — per request |
| **Vendor-native** — `/llm/call/{llmSlug}/...` | The URL | The vendor's own API | You are working with a vendor's own SDK, or need a capability the OpenAI schema cannot express |
| **OpenAI-compatible** — `/ai/{llmSlug}/v1/...` | The URL | OpenAI | A client should only ever reach one LLM, or cannot send a namespaced model string |
| **Anthropic Messages** — `/anthropic/{llmSlug}` | The URL | Anthropic Messages | The client speaks the Anthropic Messages API (Claude Code, for example). Bedrock-backed LLMs only |
| **Legacy** — `/llm/rest/{llmSlug}/...`, `/llm/stream/{llmSlug}/...` | The URL | The vendor's own API | Existing integrations only |

The [Developer Portal](./ai-portal.md) shows each of these on an App's detail page, filled in for that App.

### 1. Main Ingress (`/v1/chat/completions`)

A single OpenAI-compatible endpoint at the root of the gateway, fronting every LLM the calling App has access to. The LLM is selected per request rather than by URL, so a client can switch model or vendor without changing its base URL or credential.

*   **Models are namespaced:** send `{llmSlug}/{model}` (for example `openai-prod/gpt-4o`) rather than a bare model name — the prefix selects the LLM and is stripped before the request reaches the vendor. A model string without a prefix is rejected with **400**. The split is on the *first* slash, so model names that themselves contain slashes (fine-tune identifiers, for instance) survive intact.
*   **OpenAI format only:** requests and responses use the OpenAI Chat Completions schema whatever the upstream vendor is. For a vendor's own SDK and native parameters, use the vendor-native endpoint instead.
*   **Streaming** is detected from `stream: true`, as on the other endpoints.
*   **Model discovery:** `GET /v1/models` lists every model the authenticated App can call, already namespaced and ready to send back. The list is built from each LLM's allowed-models list plus its default model, so an LLM configured with neither does not appear.
*   `/v1/completions` is served alongside `/v1/chat/completions` for legacy completion-style clients.

    ```bash
    # Example using curl
    curl -X POST "https://your-gateway-host/v1/chat/completions" \
      -H "Authorization: Bearer YOUR_APP_API_KEY" \
      -H "Content-Type: application/json" \
      -d '{
        "model": "my-openai-config/gpt-4-turbo",
        "messages": [{"role": "user", "content": "Hello!"}]
      }'
    ```

    ```python
    # Example using OpenAI Python SDK - one client for every LLM in the App
    import openai

    client = openai.OpenAI(
        base_url="https://your-gateway-host/v1",
        api_key="YOUR_APP_API_KEY"
    )

    response = client.chat.completions.create(
        model="my-openai-config/gpt-4-turbo",       # or "my-bedrock-config/claude-3-5-sonnet"
        messages=[{"role": "user", "content": "Hello!"}]
    )
    print(response.choices[0].message.content)
    ```

*   **Configuration:** the base path defaults to `/v1` but is configurable, and the ingress can be removed entirely — see [Configuring the Main Ingress](#configuring-the-main-ingress) below.

### 2. Vendor-native Endpoint (`/llm/call/{llmSlug}/...`)

*   **Purpose:** pass-through to the vendor's native API for one LLM, with no request manipulation beyond the fields needed for analytics and budgets. Everything after the slug is forwarded to the vendor as-is.
*   **Benefits:** full access to vendor-specific features and resilience to vendor API changes — a new parameter works the day the vendor ships it, with no gateway change.
*   **Streaming:** handled automatically. The endpoint inspects the request and routes it down the streaming or non-streaming path, so a single URL serves both.

    ```bash
    # Example using curl against an Anthropic-backed LLM, in Anthropic's own format
    curl -X POST "https://your-gateway-host/llm/call/my-anthropic-config/v1/messages" \
      -H "Authorization: Bearer YOUR_APP_API_KEY" \
      -H "Content-Type: application/json" \
      -d '{
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 1024,
        "messages": [{"role": "user", "content": "Hello!"}]
      }'
    ```

### 3. OpenAI-compatible Endpoint (`/ai/{llmSlug}/v1/...`)

*   **Purpose:** accepts OpenAI-format requests and translates them to the target vendor's API, translating the response back. Unlike the Main Ingress it is pinned to one LLM by its URL, so the `model` field takes a plain vendor model name — or can be omitted, falling back to the default model configured on that LLM.
*   **Use it when** a client should only ever reach a single LLM, or when a tool cannot be made to send a namespaced `{llmSlug}/{model}` model string.
*   **Tradeoff:** vendor-specific features not expressible in the OpenAI schema are unavailable.

    ```python
    # Example using OpenAI Python SDK, pinned to one LLM
    import openai

    client = openai.OpenAI(
        base_url="https://your-gateway-host/ai/my-openai-config/v1",
        api_key="YOUR_APP_API_KEY"
    )

    response = client.chat.completions.create(
        model="gpt-4-turbo",                        # bare model name, not namespaced
        messages=[{"role": "user", "content": "Hello!"}]
    )
    print(response.choices[0].message.content)
    ```

### 4. Anthropic Messages Endpoint (`/anthropic/{llmSlug}`)

*   **Purpose:** accepts native Anthropic Messages API requests and translates them to Bedrock Converse, so clients that speak the Anthropic API — Claude Code, for example — can drive a Bedrock-backed LLM through the gateway.
*   **Scope:** **Bedrock-backed LLMs only.** Other vendors already speak their own API through the vendor-native endpoint; pointing this endpoint at one returns **400**.
*   **Usage:** set the URL as `ANTHROPIC_BASE_URL` and pass the App key as `ANTHROPIC_AUTH_TOKEN` (or `ANTHROPIC_API_KEY`). The client appends `/v1/messages` itself.

    ```bash
    export ANTHROPIC_BASE_URL="https://your-gateway-host/anthropic/my-bedrock-config"
    export ANTHROPIC_AUTH_TOKEN="YOUR_APP_API_KEY"
    ```

### 5. Legacy Endpoints (`/llm/rest/...`, `/llm/stream/...`)

*   `/llm/rest/{llmSlug}/...` handles non-streaming (synchronous) requests only; `/llm/stream/{llmSlug}/...` handles Server-Sent Events streaming only.
*   **Note:** these predate the vendor-native endpoint, which auto-detects streaming and covers both. They remain supported for existing integrations, and their response-handling code is still used internally, but new integrations should use `/llm/call/{llmSlug}/...`.

### Configuring the Main Ingress

The Main Ingress sits at the root of the gateway, which is awkward when the gateway is embedded in a host that already owns `/v1`. Its base path is therefore configurable, and it can be switched off entirely — the per-LLM endpoints (`/ai/`, `/llm/`, `/anthropic/`) are unaffected either way.

The hub (AI Studio) and the edge (Microgateway) read **different environment variables** for the same setting, so set both, to the same value, in a hub-and-spoke deployment:

| Setting | AI Studio (hub) | Microgateway (edge) | Default |
|---------|-----------------|---------------------|---------|
| Base path | `UNIFIED_ROUTER_PATH` | `GATEWAY_UNIFIED_ROUTER_PATH` | `/v1` |
| Disable entirely | `UNIFIED_ROUTER_DISABLED` | `GATEWAY_UNIFIED_ROUTER_DISABLED` | `false` |

Setting the base path to `/ai-gateway/v1`, for example, moves all three routes to `/ai-gateway/v1/chat/completions`, `/ai-gateway/v1/completions` and `/ai-gateway/v1/models`. The value is normalized on load: a leading slash is added, a trailing slash and any empty or dot segments are cleaned, and a value of `/` is refused in favour of the default so the ingress can never swallow the whole gateway.

The Developer Portal reads the **hub's** value when it advertises the Main Ingress on an App's detail page — that is why the two must agree — and hides the Main Ingress entirely when the hub has it disabled.

### LLM Slug

The `{llmSlug}` in the endpoint path is automatically generated from the LLM configuration name when you create it. For example, an LLM named "My OpenAI Config" would have a slug like `my-openai-config`.

## Configuration & Security

The behavior of the Proxy for a specific route is determined by the corresponding [LLM Configuration](./llm-management.md), which includes details about the backend vendor, model access, budget limits, and associated filters.

By centralizing LLM access through the Proxy, Tyk AI Studio provides a robust layer for security, control, and observability over AI interactions.

For the metrics and traces the Proxy emits — request latency, time to first token, token usage, cost and policy blocks — see [Observability](./observability.md).
