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
    *   The same check applies however the caller authenticated (API key, `Authorization: Bearer` app secret, or an auth plugin) and on every entry point: `/llm/...`, `/ai/...`, `/anthropic/...`, the unified `/v1` endpoint (the vendor prefix of the `model` string) and `/datasource/...`, on AI Studio's embedded gateway and on edge microgateways alike. An App may only call the LLMs and data sources it was granted, plus the fallbacks of a granted LLM's failover waterfall when reached through that waterfall. Naming anything else, including a slug that does not exist, returns `403`.

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
| **Decisions** — `/v1/decisions` | The `model` field, namespaced `{llmSlug}/{model}` | OpenAI Decisions | Asking a [decision model](./decision-models.md) typed questions; System One providers (TypeSafe Jev, self-hosted) are translated |
| **Vendor-native** — `/llm/call/{llmSlug}/...` | The URL | The vendor's own API | You are working with a vendor's own SDK, or need a capability the OpenAI schema cannot express |
| **OpenAI-compatible** — `/ai/{llmSlug}/v1/...` | The URL | OpenAI | A client should only ever reach one LLM, or cannot send a namespaced model string |
| **Anthropic Messages** — `/anthropic/{llmSlug}` | The URL | Anthropic Messages | The client speaks the Anthropic Messages API (Claude Code, for example). Bedrock-backed LLMs only |
| **Legacy** — `/llm/rest/{llmSlug}/...`, `/llm/stream/{llmSlug}/...` | The URL | The vendor's own API | Existing integrations only |

The [Developer Portal](./ai-portal.md) shows each of these on an App's detail page, filled in for that App.

### How the vendor URL is built

Every entry point ends up sending the vendor `<LLM API endpoint path> + <the path the request carries>`, with any overlap between the two removed. So an LLM whose endpoint is `https://api.anthropic.com`, `https://api.anthropic.com/v1`, `https://my-proxy/anthropic` or `https://my-proxy/anthropic/v1` is called at `.../v1/messages` in every case, whether the request came through the Main Ingress, the OpenAI-compatible endpoint, or a vendor SDK on the vendor-native endpoint. A version segment is never doubled (`/v1/v1/...`) and never dropped. The same applies to OpenAI-compatible providers that mount their API under a prefix, such as `.../inference/v1` or `.../openai/v1`.

Studio's own chat and agents use the vendor's client library directly rather than the gateway; for Anthropic, that library expects an endpoint ending in `/v1`, and Studio appends it when the configured endpoint has no version segment. If you front a vendor with your own proxy, configure the endpoint as the path your proxy expects to see immediately before the vendor's versioned path.

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

#### Claude Code and the Anthropic SDKs

Point an Anthropic client at the vendor-native endpoint of an Anthropic LLM and authenticate with the App's API key. Everything the SDK sends to `/v1/messages`, streaming or not, passes through as it would to Anthropic.

```bash
# Claude Code
export ANTHROPIC_BASE_URL="https://your-gateway-host/llm/call/my-anthropic-config"
export ANTHROPIC_AUTH_TOKEN="YOUR_APP_API_KEY"   # sent as Authorization: Bearer
claude
```

```python
# Anthropic Python SDK (the TypeScript SDK works the same way)
import anthropic
client = anthropic.Anthropic(
    base_url="https://your-gateway-host/llm/call/my-anthropic-config",
    api_key="YOUR_APP_API_KEY",   # sent as x-api-key
)
```

Use an edge gateway's address (for example `http://edge-host:8081/llm/call/my-anthropic-config`) to go through a Microgateway instead.

#### Message Batches and other native API operations

Some SDK features are not a single `POST` with a `model`, so a plain pass-through cannot govern them. The gateway serves these as **native API operations**. For Anthropic that is the [Message Batches API](https://docs.anthropic.com/en/api/creating-message-batches): `client.messages.batches.create`, `retrieve`, `list`, `cancel`, `results` and `delete` all work against the native endpoint above, with the same governance as a synchronous call:

*   **Every request in a batch is checked.** Each `params.model` must be one of the LLM's allowed models (otherwise 403, naming the `custom_id`), and the LLM's filters run on every request. A body with a duplicated key (two `model`s, or `messages` and `Messages`) is refused with 400, as it is on a synchronous `/v1/messages` call, because the gateway and Anthropic could read different values from it. Keys that differ only in case inside tool schemas or tool call inputs (`id` and `ID`) are allowed. A request a filter refuses rejects the whole batch (400) before it reaches Anthropic. Response filters run on every line of the results; a blocked line comes back as an `errored` result with the same `custom_id`.
*   **Budgets apply.** A batch create passes the same budget check as a synchronous call. Its usage is recorded once, when the results are first downloaded or, if nobody downloads them, by a background reconciler on the gateway that created the batch. It is priced at the batch rate (half the model's list price) and counts toward the App's budget and analytics like any other usage, including usage on an edge gateway. If you have negotiated pricing, set the LLM's **price multipliers** (**Add price override** in the LLM form, or `price_multipliers` in the LLM API), a map from service tier to the share of the list price, for example `{"batch": 0.4}` to bill batch usage at 40% of list price. A tier you don't list keeps the vendor's rate. The setting reaches edge gateways with the LLM's configuration, and it applies to usage recorded after the change, not to existing records.
*   **Batches belong to an App.** Batch IDs returned by the gateway are signed for the App and LLM that created them, for example `msgbatch_01Hk….t1k3f2a9c1d.9b…`. Use them exactly as returned. Another App's key, another LLM or an altered ID gets `404 not_found_error`. The gateway's listing shows only the calling App's batches.
*   **Results come through the gateway.** `results_url` points at the gateway, never at `api.anthropic.com`, and results are streamed line by line. With `PROXY_URL` (AI Studio) or `GATEWAY_PUBLIC_URL` (Microgateway) set, `results_url` is an absolute URL. Without them it is a path such as `/v1/messages/batches/{id}/results`, which the Python and TypeScript SDKs resolve against their base URL.

Limitations:

*   **The gateway that created a batch owns it.** It keeps the list of batches it created, and it accounts their usage. On edge gateways, `list` shows only the batches created through that edge, and a batch can only be deleted through the edge that created it (other gateways answer `409`). AI Studio replicas share one list.
*   **Deleting a batch needs its usage accounted first.** A batch that is still in progress cannot be deleted (`409`). An ended batch is accounted first, then deleted.
*   **Results downloads are refused (`501`) on an LLM with response plugins.** A streamed results file cannot be handed to a plugin that rewrites response bodies.
*   **A failover rung or router does not grant batch access.** Batches need the App to hold the LLM directly (otherwise `403`). Plugin upstream overrides do not apply to batch calls, because the batches live in the LLM's own Anthropic account.
*   **Headers and query parameters are filtered.** Only `anthropic-version`, `anthropic-beta`, `content-type`, `accept` and `x-request-id` are forwarded, and only the `beta` query parameter. A client's `anthropic-workspace-id` and `anthropic-user-profile-id` are dropped, here and on ordinary `/v1/messages` calls. To run an LLM's calls in a specific Anthropic workspace, set the LLM's **Anthropic workspace ID** (under **Access Details** in the LLM form, or `vendor_workspace_id` in the LLM API): the gateway then sends it as `anthropic-workspace-id` on every call it makes for that LLM (batches, messages, Chat sessions and the `/ai/` and `/v1` bridges alike), on AI Studio and on edge gateways. Without it, calls run in the API key's default workspace.
*   **A dropped database connection can lose a batch's usage.** If the connection drops just after the gateway marks a batch as accounted, but before it gets the confirmation, the batch stays accounted and its usage is never recorded. The App is undercharged, never charged twice. This is rare. An affected batch shows as accounted in the `native_resources` table but has no matching usage in analytics.
*   **Batch IDs are signed with a key AI Studio generates for each LLM**, not with the API key, so rotating the LLM's Anthropic API key leaves outstanding batch IDs valid. To revoke outstanding IDs, rotate the signing key: `POST /api/v1/llms/{id}/resource-signing-key/rotate` with an optional `{"grace_period_hours": N}` body. IDs signed with the old key keep working for the grace period (default 720 hours, `0` revokes them at once), and edge gateways get the new key with the next configuration push. `GET /api/v1/llms/{id}/resource-signing-key` shows the current and previous key IDs (the `t1k…` part of a batch ID), never the key. In the admin UI, the LLM's details page shows the same key IDs under **Native API resource IDs**, and users who can edit LLMs can rotate the key there with **Rotate signing key**. LLMs that existed before this version keep accepting the batch IDs they already handed out for 30 days after the upgrade, even if you change the API key in that time. Standalone Microgateways (no AI Studio) still sign with the API key.

| Setting | AI Studio | Microgateway | Default |
|---------|-----------|--------------|---------|
| Public base URL for `results_url` | `PROXY_URL` | `GATEWAY_PUBLIC_URL` | unset (relative URL) |
| Batch create body limit (bytes) | `NATIVE_API_MAX_BODY_BYTES` | `NATIVE_API_MAX_BODY_BYTES` | `67108864` (64 MiB; at most 256 MiB) |
| Batch creates admitted at once | `NATIVE_API_MAX_CONCURRENT_CREATES` | same | `4` |
| Max requests per batch when the LLM has filters | `NATIVE_API_MAX_FILTERED_ITEMS` | same | `10000` |
| Max remote guardrail provider calls per batch create or upload | `NATIVE_API_MAX_GUARDRAIL_CALLS` | same | `10000` |
| Max in-progress batches per App and LLM | `NATIVE_API_MAX_IN_PROGRESS_PER_APP` | same | `0` (no cap) |
| Reconciler pass interval | `NATIVE_API_RECONCILE_INTERVAL` | same | `30s` |
| Batches per reconciler pass | `NATIVE_API_RECONCILE_BATCH` | same | `50` |
| Batches reconciled at once | `NATIVE_API_RECONCILE_WORKERS` | same | `4` |
| Poll interval of an unfinished batch | `NATIVE_API_POLL_INTERVAL` | same | `2m` |
| Longest wait after failed polls | `NATIVE_API_POLL_MAX_BACKOFF` | same | `30m` |

`client.messages.count_tokens` (used by Claude Code before most turns) is checked the same way as a synchronous call: the `model` must be allowed (403 otherwise) and the LLM's filters run on the request. It is logged, and costs nothing against the budget.

#### OpenAI Responses

For OpenAI LLMs the [Responses API](https://platform.openai.com/docs/api-reference/responses) works on the native endpoint (`client.responses.create`, `retrieve`, `cancel`, `delete` and `input_items.list`), and so does the Conversations API (`client.conversations.create`, `retrieve`, `update`, `delete` and `items`). Responses and conversations belong to the App that created them:

*   **Response IDs are signed for the App and LLM.** The `id` (and `previous_response_id`) the gateway returns looks like `resp_67cc….t1k3f2a9c1d.9b…`, in JSON responses and in every streamed event. Use it exactly as returned. Continuing from another App's response (`previous_response_id`), or retrieving, cancelling or deleting it, gets `404`, and the request never reaches OpenAI. The same check applies to `responses.input_tokens` and `responses.compact`.
*   **Conversation IDs are signed too.** Using another App's conversation (the `conversation` parameter, or any `/v1/conversations/...` call) gets `404`. Items stored in a conversation are screened by the LLM's filters when they are stored.
*   **Filters and guardrails see the prompt.** They read `instructions`, a string `input` or each input item (message text, tool call outputs, tool call arguments, and the text of built-in items such as MCP call results, code interpreter logs and reasoning summaries) and `prompt` variables, and a redaction is written back into the same field. With filters attached, a request whose prompt fields the gateway cannot read, or that contains an input item type the gateway does not know, is refused (`400`). Files attached as `input_file` are not screened. Batch lines with `url` `/v1/responses` are screened the same way.
*   **Usage is recorded as for chat completions.** A synchronous or streamed response is priced from its `usage` (cached input tokens at the cache price). A background response (`background: true`) is accounted once, by the gateway that created it, when it ends: a background reconciler polls it, and deleting it accounts it first.

Limitations:

*   **Response IDs handed out before this version are not accepted** as `previous_response_id`: start a new chain.
*   **A background response can only be deleted through the gateway that created it**, once it has ended (otherwise `409`). Other responses can be deleted through any gateway.
*   **Resuming a background stream (`retrieve(..., stream=True)`) is not supported** (`400`): poll the response instead.
*   **Every Responses call must name `model`.** A call that relies on a stored prompt's model or names only a conversation gets `400` ("model field is required"), because the gateway checks the model against the LLM's allowed models. Moderations are the exception: `/v1/moderations` without a `model` is checked as `omni-moderation-latest`, the model OpenAI then uses, so it works with no allowed-models list or one that includes that model.
*   **Attached files are not screened.** `input_file` parts (including base64 PDFs) pass the filters unread, and a `file_id` in an `input_file` or `input_image` part is passed to OpenAI unchecked. Files other than batch input cannot be uploaded through the gateway.
*   **Item IDs inside responses and conversations (`msg_…`, `fc_…`) are passed through unchanged.** They are random and only returned to the App that owns them.
*   `OpenAI-Organization` and `OpenAI-Project` headers are dropped from every Responses request, so responses always live in the project of the LLM's API key.

For OpenAI LLMs the filters also read the text of `/v1/embeddings`, `/v1/completions` and `/v1/moderations` requests (`input`, `prompt`, `suffix`). Inputs given as token arrays cannot be screened, so they are refused (`400`) on an LLM with request filters.

#### OpenAI Batches and Files

For OpenAI LLMs the [Batch API](https://platform.openai.com/docs/guides/batch) works on the native endpoint: `client.files.create(file=..., purpose="batch")`, `client.batches.create`, `retrieve`, `list` and `cancel`, and `client.files.content`, `retrieve`, `list` and `delete`. It is governed like Anthropic batches:

*   **Every request in the uploaded file is checked** when the file is uploaded: its `body.model` must be allowed (otherwise 403, naming the `custom_id`), and the LLM's filters run on its body. A request a filter refuses rejects the upload. A line with a duplicate key, an unsupported `url` or a method other than `POST` is refused (400). Only `purpose="batch"` uploads are accepted, in the plain multipart form the OpenAI SDKs send; unusual multipart encodings are refused (400), and so is a file name containing a backslash or a double quote (rename the file).
*   **A batch runs only under the filters that checked its file.** Because the file's requests are checked at upload, a batch create on an LLM with filters is refused (`400`, asking you to upload the file again) when the LLM's request filters have changed since the upload, for example when a guardrail was attached or edited. The same applies to files uploaded before AI Studio 2.3.0 and, on edge gateways, to a batch created through a different edge from the one that received the upload. Upload the file again and create the batch from the new file ID.
*   **Files and batches belong to an App.** File and batch IDs are signed for the App and LLM, so a batch can only be created from the App's own uploaded file, and another App gets `404` for both. Listings show only the calling App's files and batches.
*   **Usage is recorded once, at the batch rate** (half the list price), by the gateway that created the batch, after it ends. An output file cannot be deleted until its batch's usage has been accounted (`409`).

Limitations: uploads are limited to `NATIVE_API_MAX_BODY_BYTES` (64 MiB by default, up to 256 MiB) and, when the LLM has filters, to `NATIVE_API_MAX_FILTERED_ITEMS` requests (10,000 by default). On edge gateways, files and batches are listed and deleted only through the edge that created them, and on an LLM with filters a batch must be created through the edge that received its file. The `purpose` and `order` parameters of `files.list` are ignored.

The operations are counted in the `aistudio_native_*` metrics (see [Observability](./observability.md#governance-metrics)).

### 3. OpenAI-compatible Endpoint (`/ai/{llmSlug}/v1/...`)

*   **Purpose:** accepts OpenAI-format requests and translates them to the target vendor's API, translating the response back. Unlike the Main Ingress it is pinned to one LLM by its URL, so the `model` field takes a plain vendor model name — or can be omitted, falling back to the default model configured on that LLM.
*   **Use it when** a client should only ever reach a single LLM, or when a tool cannot be made to send a namespaced `{llmSlug}/{model}` model string.
*   **Tradeoff:** vendor-specific features not expressible in the OpenAI schema are unavailable.
*   **Vendors:** **Vertex and Hugging Face LLMs are not supported** here or on the unified `/v1` endpoint: a request returns **400** with code `unsupported_vendor`, and no request reaches the vendor. Call them through the vendor-native endpoint (`/llm/rest/{llmSlug}` or `/llm/stream/{llmSlug}`). In a failover waterfall, such a rung is skipped and the next one is tried.

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

*   **Model discovery (Claude Code `/model` picker):** every request on this endpoint goes to the LLM's configured default model, whatever model name the client sends. So that Claude Code's picker shows that model, not Anthropic's built-in list, the endpoint also answers `GET /anthropic/{llmSlug}/v1/models`. Developers turn discovery on in Claude Code:

    ```bash
    export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1
    ```

    The picker then shows one extra entry, marked "From gateway", labelled `<LLM name> — <model id>`, for example `Bedrock Claude — eu.anthropic.claude-3-5-sonnet-20241022-v2:0`. The id is the configured Bedrock model id, unchanged.

    ```json
    {"data":[{"type":"model","id":"eu.anthropic.claude-3-5-sonnet-20241022-v2:0","display_name":"Bedrock Claude — eu.anthropic.claude-3-5-sonnet-20241022-v2:0"}],"has_more":false}
    ```

    *   It uses the same authentication and App access check as `/v1/messages`. It calls no model, so it spends no budget, runs no filters and records no usage.
    *   The list is empty (`"data":[]`) when the LLM has no default model or its allowed models refuse the default model. Claude Code then falls back to its built-in list.
    *   **Use Claude Code v2.1.223 or later.** Claude Code keeps an id only if it contains "claude" or "anthropic". Versions from v2.1.129 (when discovery arrived) up to v2.1.222 needed the id to *start* with one of those words, so they hide region-prefixed ids such as `us.anthropic...` or `eu.anthropic...`.
    *   **Application inference profiles are not shown.** If the default model is an application inference profile ARN (`arn:aws:bedrock:...:application-inference-profile/...`), the id contains neither word and Claude Code drops it. Requests still work; only the picker entry is missing. System inference profile ARNs contain "anthropic" and are shown.
    *   **Don't redirect this path.** Claude Code treats any redirect as a failed discovery, including an ingress redirect from `http://` to `https://`. Point `ANTHROPIC_BASE_URL` at the final URL.

### How the translating endpoints reach the vendor

The OpenAI-compatible endpoints (`/ai/{llmSlug}`, the Main Ingress `/v1`, routers) and the Anthropic Messages endpoint translate the request and send it through the gateway's own vendor-native endpoint, `/llm/call/{llmSlug}/`, before it goes to the vendor. Plugins, model checks, budgets, request and response filters, signing with the LLM's credentials and analytics all run there, once per attempt, on the vendor's own request and response.

**Bedrock, from AI Studio 2.3.** Bedrock LLMs take the same route; before 2.3 the Bedrock bridges called AWS directly. What changes for Bedrock traffic on `/ai/`, `/v1`, routers and `/anthropic/`:

*   **Plugins run.** Pre-auth, post-auth and response plugins attached to a Bedrock LLM run on these endpoints and see the Converse request and response (see [Microgateway Plugins](./plugins-microgateway.md#where-request-plugins-run)). Before, only auth plugins ran there.
*   **Filters read Converse.** Request and response filters, guardrails and filter scripts run on the internal hop against the Converse request and response, so scripts see `input.format == "bedrock"` instead of the OpenAI or Anthropic body the client sent. Check custom filter scripts on Bedrock LLMs that parse `raw_input` in the client's shape; `input.messages` and the redaction helpers handle Converse.
*   **Retries.** The AWS SDK no longer retries a call on its own. Retries are the failover waterfall's: a 5xx or 429 moves to the next rung, a 4xx does not.
*   **Errors keep their status.** A refusal on the internal hop reaches the client in the endpoint's own error format with its real status: a guardrail block is a `400`, a plugin block keeps its status and message, a budget refusal is a `403`. On `/anthropic/`, a stream refused before it opens now gets that status instead of a `200` with an error event.
*   **Headers.** `X-Cache*`, `X-Tyk-Compression*`, `X-LB-*` and `X-RateLimit-*` response headers from plugins reach the client, buffered and streaming (for every vendor on these endpoints). A client's `X-Tyk-Compression` header reaches the plugins and never AWS.
*   **Request logs.** A buffered Bedrock call's request log names the LLM in its model column, as for every vendor on the vendor-native endpoint; the chat record keeps the model ID. Cache read and write tokens are priced. A failed native Bedrock stream leaves a request log row.

AI Studio's embedded gateway shares this code, so Studio's own `/ai/` traffic to Bedrock (Chat included) takes the internal hop too. Plugins do not run on the embedded gateway, but its budget check, filters and analytics for Bedrock run on the internal hop, on the Converse body.

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

Setting the base path to `/ai-gateway/v1`, for example, moves all four routes to `/ai-gateway/v1/chat/completions`, `/ai-gateway/v1/completions`, `/ai-gateway/v1/decisions` and `/ai-gateway/v1/models`. The value is normalized on load: a leading slash is added, a trailing slash and any empty or dot segments are cleaned, and a value of `/` is refused in favour of the default so the ingress can never swallow the whole gateway.

The Developer Portal reads the **hub's** value when it advertises the Main Ingress on an App's detail page — that is why the two must agree — and hides the Main Ingress entirely when the hub has it disabled.

### LLM Slug

The `{llmSlug}` in the endpoint path is automatically generated from the LLM configuration name when you create it. For example, an LLM named "My OpenAI Config" would have a slug like `my-openai-config`.

## Request IDs

Every request through the gateway (AI Studio's and every Microgateway's) gets an id of its own, returned in the **`X-Tyk-AI-Request-ID`** response header, for example `X-Tyk-AI-Request-ID: tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b`.

- **One id per request.** The `/ai/` and `/anthropic/` bridges route through the gateway's own `/llm/call/` hop, and a failover waterfall makes one attempt per rung; all of them carry the id the client was answered with.
- **No collisions.** The id is `tyai_` followed by a UUIDv7 (time-ordered, 74 random bits), so ids from different edges and Studio replicas never collide, and the prefix tells them apart from other systems' ids in shared logs.
- **Your `X-Request-ID` is left alone.** The gateway never sets or overwrites `X-Request-ID`, which belongs to the caller and to proxies in front of the gateway. When a request carries one, it is recorded beside the gateway's id as the *client request id* (up to 128 characters of letters, digits and `- _ . : /`).
- **A client cannot choose the gateway's id.** An `X-Tyk-AI-Request-ID` sent by a client is ignored.

Where the id appears:

| Where | Field |
|---|---|
| Request logs (App and LLM pages, `GET /api/v1/analytics/proxy-logs-for-app`) | `request_id`, `client_request_id`; the log search matches either exactly |
| Guardrail and filter compliance events | `request_id`, `client_request_id` in the metadata; **Open the request log** in the event's details |
| HTTP guardrail classifiers | `meta.request_id` in the classify request |
| Plugins and hooks | the `request_id` value of the request context, `PluginContext.RequestID` |
| Analytics plugins on edges | `gateway_request_id`, `client_request_id` on `AnalyticsData` (`request_id` there is the analytics row's own key) |

Requests that produce no usage (a policy block, a vendor or gateway error) are request log rows too, from edges as well as from AI Studio's gateway: an edge sends them to AI Studio on the analytics pulse marked `no_usage`, and AI Studio records them without a chat record or spend.

## Configuration & Security

The behavior of the Proxy for a specific route is determined by the corresponding [LLM Configuration](./llm-management.md), which includes details about the backend vendor, model access, budget limits, and associated filters.

By centralizing LLM access through the Proxy, Tyk AI Studio provides a robust layer for security, control, and observability over AI interactions.

### Outbound network policy

Since 2.3, AI Studio and the Microgateway refuse outbound connections to internal network addresses by default: loopback (`localhost`, `127.0.0.0/8`, `::1`), private ranges (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`), shared address space (`100.64/10`), link-local and cloud metadata addresses (`169.254/16`, `fe80::/10`), multicast, reserved and benchmarking ranges, and IPv6 prefixes that embed an IPv4 address (NAT64, 6to4). The check runs on the address actually dialled, after DNS resolution, so a hostname that resolves to an internal address, or is re-pointed at one later, is refused too, and so is a redirect to one.

| Setting | Effect |
|---|---|
| `LLM_UPSTREAM_BLOCK_INTERNAL` | Unset or `true`: internal addresses are refused. `false`: allowed. Any other value stops AI Studio or the Microgateway at startup. |
| `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` | Comma-separated hosts that may be internal while blocking stays on for everything else. `.suffix` matches a domain and its subdomains (`.svc.cluster.local`). Use it for a local Ollama (`ollama`, `localhost`), an in-cluster model server or classifier. |
| `LLM_UPSTREAM_ALLOWED_HOSTS` | Optional global allowlist: when set, every other host is refused. |
| `ALLOW_INTERNAL_NETWORK_ACCESS` | `true` lifts every internal-address check in the process, plugin hosts included. For development only. |

AI Studio and the Microgateway log a warning at startup whenever internal addresses are allowed. Each process applies its own setting: an edge refuses an internal upstream that the hub configured when blocking is on at the edge, whatever the hub's setting.

**What the policy covers:**

- LLM upstreams on every gateway route (`/llm/`, `/ai/`, `/anthropic/`, `/v1`, native and Bedrock routes), in AI Studio's embedded gateway and the Microgateway;
- guardrail providers (Enterprise);
- `tyk.makeHTTPRequest` in filter scripts (Enterprise).

OCI plugin registries, their signatures and token servers, and `grpc://` plugin hosts follow the plugin host rule, which refuses internal addresses whatever `LLM_UPSTREAM_BLOCK_INTERNAL` says (see [Plugin Security](./plugins-security.md)). Webhooks and Tyk Gateway connections (Enterprise) have their own always-on URL policies.

**What it does not cover yet:** Studio chat's own calls to an LLM (chat uses the vendor clients directly, not the gateway), embedders and datasource or vector store connections, tool (OpenAPI) calls, MCP servers, SSO and identity providers, SMTP, the hub-to-edge gRPC connection and marketplace index fetches. Restrict those with network policy (a Kubernetes `NetworkPolicy`, firewall rules or an egress proxy).

For the metrics and traces the Proxy emits — request latency, time to first token, token usage, cost and policy blocks — see [Observability](./observability.md).
