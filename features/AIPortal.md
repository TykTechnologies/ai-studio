## AI Portal

**1. Overview & Purpose**

The AI Portal is a centralized interface that allows users to discover, create, and manage AI applications. It serves as the main entry point for users to interact with the platform's AI capabilities, including LLMs, data sources, and tools.

**Core Objectives:**

* **Simplified Access:** Provide a unified interface for accessing AI capabilities.
* **Resource Discovery:** Enable users to discover available LLMs, data sources, and tools.
* **App Management:** Allow users to create, configure, and manage AI applications.
* **Usage Monitoring:** Track and display usage metrics for AI resources.
* **User Experience:** Deliver an intuitive and responsive user interface.

**User Roles & Interactions:**

* **End User:** Discovers and uses AI applications, interacts with LLMs through chat interfaces.
* **App Developer:** Creates and configures apps with specific LLMs, data sources, and tools.
* **Administrator:** Manages global settings, monitors usage, and controls access to resources.

**2. Architecture & Components**

The AI Portal consists of several key components that work together to provide a comprehensive user experience:

* **Overview (`/portal/dashboard`):** The developer's landing page. "Your apps" lists every app the user owns with one status (Active / Awaiting approval / Disabled), spend against the monthly budget (a bar; "No limit" when none; the budget alone in Community Edition, where spend is not tracked), last gateway access and the 30-day request count, most recently used first; it links to the app page and to My Apps. "Build with" has a search box that opens Browse, one count tile per asset type (and per plugin resource type) linking to the filtered Browse page, and the six newest assets as cards. Data: `GET /common/apps?all=true`, `GET /common/apps/usage-summary`, `GET /common/catalog`.
* **Browse (`/portal/catalog`, `/portal/catalog/{llms|datasources|tools}`, `/portal/catalog/resources/{pluginId}/{slug}`):** One searchable catalog of everything the user's teams can use, newest first (UX review D4). Search (debounced), filters (type tabs, kind, privacy level, catalog, community submissions), sort (newest / name / privacy level) and paging (the shared `PaginationControls`) are sent to `GET /common/catalog` as parameters and applied there; the page renders the one page it gets back and builds its filter menus from the server's facets. All state lives in the query string. The overview asks the same endpoint for `sort=newest&page_size=6` and reads the counts from the facets. Cards are the same `AssetCard` everywhere: a letter avatar (initials on a colour derived from the asset key, replaced by the logo when one is set), type chip, kind, description, privacy level chip, community badge, "Added ..." and the Details / Build app (Get access for data sources) actions. The old per-catalog routes (`/portal/llms/:catalogueId`, `/portal/databases/:catalogueId`, `/portal/tools/:catalogueId`, `/portal/resources/:pluginId/:slug`) redirect into Browse filtered by that catalog; the sidebar's "Catalogs" tree became a "Browse" section with one entry per type and per plugin resource type with instances.
* **Asset detail (`/portal/catalog/llms/:id` and the equivalents):** Replaces the "More" modals. Header with avatar, name, type, kind, privacy level and the primary action; About (descriptions, added/updated, the catalogs the asset is available through, tags); a type section (LLM providers: the allow list and a Models table with the default model and per-million-token prices from the model price table filtered by the allow list, the OpenAI-compatible base URL and the namespaced model name for the unified ingress, and the provider's metadata with secrets redacted; data sources: store, embedding vendor and model; tools: operations and a link to the API documentation page; plugin resources: their type); Governance (portal-visible governed metadata); and "Your apps" (the caller's apps that already include the asset, with status). Anything outside the caller's visibility is a 404 ("not available to you"), never a 403, so the endpoint does not confirm ids exist.
* **App Creation:** Enables users to create and configure new applications. The builder accepts `?llm=`, `?datasource=`, `?tool=` and `?plugin_resource=<pluginId>:<slug>:<instanceId>` to preselect an asset.
* **Chat Interface:** Provides a conversational interface for interacting with LLMs.
* **Settings:** Allows users to manage their profile and preferences.

**3. Key Features**

* **App Management:**
  * Create, edit, and delete applications.
  * Configure applications with specific LLMs, data sources, and tools.
  * Set budget limits and monitor usage.
  * Share applications with other users or groups.

* **Resource Integration:**
  * Browse and select from available LLMs, data sources, and tools.
  * Subscribe applications to specific resources.
  * Configure resource-specific settings.
  * Monitor resource usage and performance.

* **Tool Subscription:**
  * Select tools to subscribe to when creating or editing an app.
  * Multi-select interface for choosing tools.
  * Tool selection validated against user permissions and privacy settings.
  * Similar interface to LLM and data source selection for consistency.

* **App Endpoint Documentation (App detail view):**
  * The App detail page documents every way the app's credential can reach an LLM, grouped so the intended use case of each is explicit:
    * **Main Ingress** — the gateway's unified OpenAI-compatible ingress (`{proxy}{unified base path}/chat/completions`, base path default `/v1`). One URL for every LLM the app can access; the LLM is chosen per request. Shown once per app, above the per-LLM cards.
    * **Per-LLM vendor-native endpoint** — `/llm/call/{llm-slug}`, pass-through in the vendor's own API format, streaming auto-detected.
    * **Per-LLM OpenAI shim** — `/ai/{llm-slug}/v1`, OpenAI format pinned to one LLM, bare model names.
    * **Per-LLM Anthropic shim** — `/anthropic/{llm-slug}` (Bedrock LLMs only), for clients speaking the Anthropic Messages API.
    * **Legacy** — `/llm/rest/{llm-slug}` and `/llm/stream/{llm-slug}`, collapsed, for existing integrations only.
  * The Main Ingress carries two caveats the per-LLM endpoints do not, both surfaced in the UI because getting either wrong is a 400: the `model` field must be namespaced as `<llm-slug>/<model>`, and the payload must be OpenAI Chat Completions format regardless of upstream vendor. The view lists the namespaced model name for each of the app's LLMs (using each LLM's default model) and points at `GET {base}/models` for the full list.
  * The ingress base path is configurable (`UNIFIED_ROUTER_PATH`) and can be disabled (`UNIFIED_ROUTER_DISABLED`), so it is served to the frontend as `unifiedRouterPath` on `/auth/config`; an empty value hides the Main Ingress section rather than advertising a URL that would 404.

* **Chat Experience:**
  * Conversational interface for interacting with LLMs.
  * Support for tool usage within conversations.
  * History tracking and conversation management.
  * File upload and sharing capabilities.

**4. Implementation Details**

* **Frontend Components:**
  * React-based UI with Material-UI components.
  * Responsive design for desktop and mobile devices.
  * State management using React hooks and context.
  * Form validation and error handling.

* **Backend Integration:**
  * RESTful API for communication with backend services.
  * Authentication and authorization using JWT tokens.
  * Real-time updates using WebSockets.
  * Error handling and logging.

* **App Form Tool Selection:**
  * Multi-select dropdown component for tool selection.
  * Tool options filtered based on user permissions.
  * Validation to ensure tool compatibility with other selected resources.
  * Similar interface to LLM and data source selection for consistency.

**5. API Endpoints**

* **Unified catalog (portal users, `api/portal_catalog_handlers.go`):**
  * `GET /common/catalog`: one page of the LLM providers, data sources, tools and plugin resources the caller can use, in one item shape (`{type, id, attributes:{name, short_description, long_description, logo_url, kind, kind_label, privacy_score, community_submitted, tags, catalogs:[{id,name}], created_at, updated_at, default_model, allowed_models, embed_vendor, embed_model, operations, resource_type}, governed_metadata}`) Search, filters, sort and paging are query parameters applied on the server (`api/portal_catalog_query.go`): `q` (every term must match the name, descriptions, kind and vendor label, type, model names, operations, tags or catalog names), `type`, `kind`, `privacy` (public / internal / confidential / restricted, the bands of `privacyLevels.js`), `catalog` (`<type>:<id>`), `community`, `sort` (newest default, name, privacy_asc, privacy_desc), `page`, `page_size` (default 25, max 100; a value outside the vocabulary is a 400). `meta` carries `total` (matches), `page`, `page_size`, `total_pages` and the facets over the caller's whole accessible set, so the filter controls never shrink as filters are applied: `counts` per type, `kinds` (`{type, kind, label, count}`), `catalogs` and `resource_types`. The accessible set is assembled once per request (the existing group/catalog queries bound it), then filtered, ordered and paged in memory; the client only ever holds one page. Visibility is exactly the per-catalog pages' rule: the caller's teams → their catalogs → active objects (`models.User.GetAccessible*`), plus the teams' plugin resource grants (`accessiblePluginResourceInstances`, shared with `/common/accessible-plugin-resources`). Catalog membership comes from `models.LLMCatalogueMemberships` and friends, one query per type.
  * `GET /common/catalog/llms/:id`, `/common/catalog/datasources/:id`, `/common/catalog/tools/:id`, `/common/catalog/resources/:plugin_id/:slug/:id`: one item, 404 when it is not visible to the caller. The LLM response adds `attributes.models` (`catalogModels`: default model first, literal allow-list entries, then the vendor's priced models that pass `modelmatch.Allowed`, with prices per million tokens) and `attributes.metadata` (redacted).
  * `GET /common/apps/usage-summary`: per owned app, `current_spend`, `monthly_budget`, `percentage`, `budget_start_date`, `last_access_at` and `requests_30d` (`analytics.GetAppActivity`: one grouped query over `llm_chat_records`, the table the app page's charts and the budget spend read, so the two pages agree; `proxy_logs` also holds rejected and failed attempts, which the app page does not count), plus `spend_tracked` (false in Community Edition).
  * `GET /analytics/budget-usage-for-app` (portal route) now refuses apps the caller does not own unless they hold analytics read.
* **App Management:**
  * `GET /apps`: List all apps (filtered by user).
  * `GET /apps/{id}`: Get details of a specific app.
  * `POST /apps`: Create a new app.
  * `PUT /apps/{id}`: Update an existing app.
  * `DELETE /apps/{id}`: Delete an app.

* **Resource Subscription:**
  * `GET /apps/{app_id}/llms`: Get all LLMs associated with an app.
  * `GET /apps/{app_id}/datasources`: Get all data sources associated with an app.
  * `GET /apps/{app_id}/tools`: Get all tools associated with an app.
  * `POST /apps/{app_id}/tools/{tool_id}`: Associate a tool with an app.
  * `DELETE /apps/{app_id}/tools/{tool_id}`: Disassociate a tool from an app.

**6. Use Cases**

* **Creating AI Applications:**
  * Users can create custom AI applications for specific use cases.
  * Applications can be configured with specific LLMs, data sources, and tools.
  * Applications can be shared with other users or kept private.

* **Managing AI Resources:**
  * Users can browse and select from available LLMs, data sources, and tools.
  * Resources can be configured for specific application needs.
  * Usage can be monitored and controlled through budgets.

* **Interacting with AI:**
  * Users can interact with AI applications through chat interfaces.
  * Tool integration enhances AI capabilities with external services.
  * Conversation history is preserved for reference and continuity.

**7. Future Enhancements**

* **App Templates:** Pre-configured templates for common use cases.
* **Advanced Filtering:** More sophisticated filtering options for resources.
* **Collaborative Features:** Shared workspaces and collaborative editing.
* **Integration with External Systems:** APIs for embedding AI Portal functionality in other applications.
* **Enhanced Analytics:** More detailed usage analytics and performance metrics.
