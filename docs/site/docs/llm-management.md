# LLM Management

Tyk AI Studio provides a centralized system for managing Large Language Model (LLM) providers, models, associated costs, and usage budgets. This allows administrators to control which models are available, how they are used, and track associated expenses.

## Overview

The LLM Management system allows you to:

*   **Configure LLM Providers:** Connect to various LLM vendors (OpenAI, Anthropic, Gemini Platform (Vertex), Google AI, Hugging Face, Ollama).
*   **Manage Models:** Specify which models from a provider are available for use within Tyk AI Studio.
*   **Define Pricing:** Set input and output token costs for each model to enable accurate cost tracking.
*   **Set Budgets:** Establish monthly spending limits for LLM usage, either globally for a model or per Application.
*   **Control Access:** Determine which user groups can access specific LLM configurations (via associated Apps).

## Configuring LLM Providers

Administrators can configure connections to different LLM providers through the UI or API.

1.  **Navigate:** Go to the LLM Configuration section in the Admin UI.
2.  **Add New LLM:** Click "Add LLM Configuration".
3.  **Provider Details:**
    *   **Name:** A user-friendly name for this configuration (e.g., "OpenAI GPT-4 Turbo").
    *   **Vendor:** Select the LLM vendor. Supported vendors: `openai`, `anthropic`, `vertex` (Gemini Platform (Vertex); formerly shown as Vertex AI), `google_ai`, `huggingface`, `ollama`.
    *   **API Key/Credentials:** Securely provide the necessary authentication credentials. Use the **Secrets Management** system (`$SECRET/YourSecretName`) for best practice.
        AI Studio and the Microgateway never read a vendor key, base URL or host from their own environment (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `HF_TOKEN`, `OLLAMA_HOST` and the like). A provider that needs a key and has none is refused rather than filled in from the host. The same applies to embedders and to Pinecone and Chroma data sources. The one exception is Gemini Platform (Vertex), and only when an operator opts in with `AI_STUDIO_VERTEX_ADC=true`: see [Gemini Platform (Vertex) credentials](#gemini-platform-vertex-credentials).
    *   **Base URL (Optional):** Override the default API endpoint if needed (e.g., for self-hosted models or custom endpoints).
        Since 2.3 the gateway refuses a Base URL on an internal address (a local Ollama at `http://localhost:11434`, a model server on a private IP or in the cluster). An administrator can turn on **Allow internal host** for the LLM, which exempts its own endpoint (on every route, in Studio chat and on edges, and for its linked embedders), or name the host in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS`. See [Outbound network policy](./proxy.md#outbound-network-policy).

    *   **Gemini Platform (Vertex):** the API endpoint is the Google Cloud project and location, `project:location` (for example `my-project:us-central1`, `my-project:eu` or `my-project:global`), or a URL ending in `/v1/projects/{project}/locations/{location}`. The API key is a Google credential JSON. See [Gemini Platform (Vertex) credentials](#gemini-platform-vertex-credentials).

4.  **Model Selection:**
    *   **Allowed Models:** Specify the exact model names from the vendor that can be used via this configuration (e.g., `gpt-4-turbo`, `claude-3-opus-20240229`).
    *   **Default Model:** The model used if a request doesn't specify one.

5.  **LLM Slug:** A unique identifier (auto-generated from the LLM name) used in API paths to target this specific LLM configuration. Two LLMs cannot share a slug, in any namespace. A name that differs from another LLM's only in case, spaces or punctuation is refused (`400` from the admin API, `AlreadyExists` from the plugin service API). The proxy endpoints use this slug:
    *   `/llm/call/{llmSlug}/...` - Unified endpoint (handles both streaming and non-streaming)
    *   `/llm/rest/{llmSlug}/...` - REST-only requests
    *   `/llm/stream/{llmSlug}/...` - Streaming-only requests

6.  **Privacy Level:** Assign a privacy level to the LLM. This interacts with the Tool system, preventing tools with higher privacy levels from being used with LLMs having lower levels.

    Privacy levels define how data is protected by controlling LLM access based on its sensitivity:
    - **Public (0)** – Safe to share (e.g., blogs, press releases).
    - **Internal (25)** – Company-only info (e.g., reports, policies).
    - **Confidential (50)** – Sensitive business data (e.g., financials, strategies).
    - **Restricted/PII (100)** – Personal data (e.g., names, emails, customer info).

    *Note: Privacy levels are stored as integer scores in the system. The values shown in parentheses are the typical score mappings.*

7.  **Save:** Save the configuration.

## Failover

Each LLM provider can carry a **failover waterfall**: an ordered list of fallback providers, each with the model to ask it for. When the provider's upstream fails, the gateway retries the same request against each fallback in turn before returning an error, so an outage at one vendor does not become an outage for your apps.

Configure it in the provider's edit form under **Failover**:

1. Click **Add fallback** and pick another LLM provider.
2. Choose the model to use on that provider. It must be one of that provider's allowed models (the picker offers them; an empty allowed-models list means any model).
3. Order the rungs with the arrows. The first is tried first.

Rules enforced when you save:

- A fallback must be active, must not be the provider itself, and cannot be listed twice with the same model.
- A fallback must have a privacy level at least as high as the provider, and must be in the same namespace or global.
- At most 10 fallbacks per provider.
- A provider cannot be deleted while another provider's waterfall points at it.

**Access is inherited.** Apps that are allowed to use a provider are automatically routed to its fallbacks when it fails, even if they were never granted those fallbacks directly. Budgets are still enforced on the fallback provider. The edit form shows this note beside the waterfall.

**When failover triggers.** By default a request moves to the next rung when the upstream answers 408, 429, 500, 502, 503 or 504, when the attempt times out, or when the upstream cannot be reached. Under **Failover Triggers (advanced)** you can narrow the status list (only 5xx, 408 and 429 are accepted: other 4xx responses are caller or configuration errors that every fallback would repeat), switch off the timeout and connection-error triggers, and set a per-attempt timeout in seconds so a hung primary does not consume the whole request budget. A 401 or 403 from the upstream never triggers failover: it means the provider's credentials or configuration are wrong, and every fallback would repeat the same request, so the error is returned to the caller as it is.

**What the caller sees.** The response carries `X-Tyk-Served-LLM` and `X-Tyk-Served-Model`, plus `X-Tyk-Failover: true` when a fallback answered. The `model` field in the body is the model that actually answered. If every rung fails, the error from the last rung is returned.

**Scope and limits.**

- Failover applies to the OpenAI-compatible chat endpoints: `/ai/{provider}/v1/chat/completions`, the unified `/v1/chat/completions` router and the Enterprise model router. The vendor-native pass-through endpoints, the Anthropic Messages bridge and chat sessions are not covered.
- A streamed response can only fail over before its first token has been sent. After that the request is committed to the provider that started streaming.
- Only the provider's own waterfall is consulted; a fallback's waterfall is not followed.
- Every attempt that gets an answer from its upstream is recorded in the proxy logs, so a request that failed over on a status code leaves one failed row for the provider and one row for the fallback that served it. A rung abandoned on the per-attempt timeout or on a connection error writes no row of its own, because its attempt is cancelled before anything is logged; only the fallback's row appears. Fallback rows are marked with the provider they failed over from: the proxy-log endpoints (`/analytics/proxy-logs-for-llm`, `/analytics/proxy-logs-for-app`) return `failover_attempt` (the 1-based rung index, `0` for the primary attempt) and `failover_from_llm_id` (the provider the request failed over from, absent on primary rows), and the LLM and App detail pages show them in a **Failover** column. The `aistudio_llm_failover_total` metric counts each hop by source, target and reason.
- Edge gateways receive the waterfall with their configuration and fail over the same way.

## Portal visibility

In Community Edition a newly saved provider is added to the Default catalog automatically. In Enterprise Edition it is not: the provider stays out of the portal and of Browse until you add it to a catalog held by the teams that should use it (see [Catalogs](./catalogs.md)).

## Model Pricing

To enable cost tracking in the Analytics system, you need to define the price per token for each model.

1.  **Navigate:** Go to the Model Prices section in the Admin UI.
2.  **Add Price:** Define prices for specific models.
    *   **Vendor:** Select the vendor.
    *   **Model Name:** Enter the exact model name.
    *   **Input Token Price:** Cost per input token (usually stored as integer * 10000 for precision).
    *   **Output Token Price:** Cost per output token (usually stored as integer * 10000 for precision).

3.  **Save:** Save the pricing information.

The Analytics system uses these prices along with token counts from LLM interactions (recorded by the Proxy and Chat systems) to calculate usage costs.

## Finding who uses a model

When a vendor deprecates a model, or you want to move teams onto a newer one, you need to know which apps are still calling it. The LLM provider details page answers this without leaving the admin UI.

1.  **Navigate:** Go to **LLMs** and open the provider (for example, your OpenAI entry).
2.  **Models in use:** Below the usage charts is a "Models in use" table listing every model served through that provider in the selected date range. Each row shows:
    *   **Requests** and **Apps**: how many calls were made and how many distinct apps made them.
    *   **Last used**: when the model was most recently called (relative, with the exact time underneath).
    *   **Total cost** and token totals, with the same breakdown as before.
    *   The prompt, response and cache cost columns split each request's recorded cost, so they always add up to the total. Batch requests are billed at half price, so their columns are half the list price.

    Click any column header to sort. The default order is most recently used first. Adjust the date range picker above the table to widen or narrow the period.
3.  **Model details:** Click a model name to open its detail view. This shows token and cost charts for that model alone, plus an **Apps using this model** table with each app's owner email, request count, tokens, cost, and the first and last time the app called the model in the period. App names link to the app's details page; owner emails are `mailto:` links so you can contact the team directly.

Notes:

*   Model names are recorded from the vendor's response, so an alias such as `gpt-4o` appears under the snapshot it resolved to (for example `gpt-4o-2024-08-06`).
*   The view is scoped to the provider entry you opened. If the same model name is configured under two provider entries, each shows only its own traffic.
*   Requests routed through edge gateways are included; they arrive via the analytics pulse and are attributed to the same provider entry and app. Edge requests with no model name are listed as `unknown-model`.
*   "Last used" is bounded by the selected date range. An app whose most recent call falls before the range start does not appear until you widen the range.
*   Both chat-interface and proxy traffic are counted.

The data behind these views is available via the API: `GET /api/v1/analytics/total-cost-per-vendor-and-model?llm_id=…` (per-model rows, now including `requestCount`, `appCount` and `lastUsed`), `GET /api/v1/analytics/apps-for-model?llm_id=…&model_name=…` (apps for one model) and `GET /api/v1/analytics/usage?llm_id=…&model_name=…` (usage over time for one model). All accept `start_date` and `end_date` in `YYYY-MM-DD` form.

## Budget Control

Tyk AI Studio allows setting monthly spending limits to control AI costs.

*   **LLM Budget:** A global monthly budget can be set directly on an LLM configuration. This limits the total spending across *all* applications using that specific LLM configuration.
*   **Application Budget:** A monthly budget can be set on an Application (`Apps` section). This limits the spending *for that specific application*, potentially across multiple LLM configurations it might use.

**How it Works:**

1.  Budgets are checked *before* an LLM request is forwarded by the Proxy.
2.  The system calculates the current monthly spending for the relevant entity (LLM or App) based on data from the Analytics system.
3.  If the current spending plus the estimated cost of the *incoming* request (if calculable, otherwise based on past usage) exceeds the budget, the request is blocked (e.g., 429 Too Many Requests).
4.  The **Notification System** can be configured to send alerts when budget thresholds (e.g., 80%, 100%) are reached.

**Configuration:**

*   **LLM Budget:** Set the `MonthlyBudget` field when creating/editing an LLM configuration.
*   **App Budget:** Set the `MonthlyBudget` field when creating/editing an App configuration.

By combining LLM configuration, pricing, and budgeting, administrators gain granular control over AI model access and expenditure within Tyk AI Studio.

## Gemini Platform (Vertex) credentials

A Gemini Platform (Vertex) LLM (vendor code `vertex`) authenticates to Vertex AI with short-lived Google access tokens. The gateway mints them from the LLM's API key and refreshes them before they expire, on AI Studio and on every edge gateway that serves the LLM. The key is stored encrypted and synced to edges like any other LLM credential. The Google identity needs the **Vertex AI User** role (`roles/aiplatform.user`) on the project.

The API key is checked in this order:

1.  **A Google credential JSON**, pasted whole into the API key (or stored in a secret and referenced as `$SECRET/...`):
    *   a **service account key** (`"type": "service_account"`), the usual choice. It needs no other setting.
    *   a **Workload Identity Federation** config (`"type": "external_account"`, or an `impersonated_service_account` whose source is one), which exchanges an identity the host supplies (a Kubernetes projected token, the instance metadata service, the AWS environment or certificate files) for a Google token, so no long-lived key is stored. Because it acts with the host's identity, every Workload Identity Federation credential is refused unless the operator sets `AI_STUDIO_VERTEX_ADC=true` on AI Studio **and on every edge gateway** that serves the LLM. This is a deliberate security default: anyone who can edit an LLM could otherwise borrow the identity of the machine the gateway runs on.
    *   `authorized_user` credentials (from `gcloud auth application-default login`), for development.

    Token endpoints in the JSON must be Google's own (`https://*.googleapis.com`, `https://accounts.google.com`), and executable credential sources are refused.
2.  **An access token** starting with `ya29.`, sent as it is. It expires within an hour, so use it only for tests and short manual checks, never in production.
3.  **No key**: the host's Application Default Credentials, only when the operator sets `AI_STUDIO_VERTEX_ADC=true`. The lookup order is `GOOGLE_APPLICATION_CREDENTIALS`, then gcloud user credentials, then the GCP metadata server (GKE Workload Identity, Cloud Run, Compute Engine). Each edge then needs its own Google identity with access to Vertex. See [Gemini Platform (Vertex) host credentials](./configuration.md#gemini-platform-vertex-host-credentials).
4.  Otherwise the LLM is refused. When it is saved, a key that is not a supported credential is refused (`400`), and a missing key is accepted with a warning. A request on a gateway route (`/llm/`, `/ai/`, `/v1`, routers) whose credential cannot be used is answered `502` with `LLM credential not usable` and the reason, which names `AI_STUDIO_VERTEX_ADC` where it applies. Nothing is sent to Vertex, and no credential value is ever included in an error or a log.

**Studio Chat and embedders** use the same credentials. A key-less Vertex LLM or embedder in Studio Chat still falls back to the host's Application Default Credentials without `AI_STUDIO_VERTEX_ADC`, for one more release, and logs a deprecation warning once per LLM or embedder. Give it a credential JSON, or set `AI_STUDIO_VERTEX_ADC=true`, before that fallback is removed. An embedder key that is neither a credential JSON nor an access token (a Google API key, for example) is refused.
