---
title: "Tools"
weight: 30
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Tools

Tyk AI Studio's Tool System allows Large Language Models (LLMs) to interact with external APIs and services, dramatically extending their capabilities beyond simple text generation. This enables LLMs to perform actions, retrieve real-time data, and integrate with other systems.

## Purpose

Tools bridge the gap between conversational AI and external functionalities. By defining tools, you allow LLMs interacting via the [Chat Interface](./chat-interface.md) or API to:

*   Access real-time information (e.g., weather, stock prices, database records).
*   Interact with other software (e.g., search JIRA tickets, update CRM records, trigger webhooks).
*   Perform complex calculations or data manipulations using specialized services.

## Core Concepts

*   **Tool Definition:** A Tool in Tyk AI Studio is essentially a wrapper around an external API. Its structure and available operations are defined using an **OpenAPI Specification (OAS)** (v3.x, JSON or YAML).
*   **Allowed Operations:** From the provided OAS, administrators select the specific `operationIds` that the LLM is permitted to invoke. This provides granular control over which parts of an API are exposed.
*   **Authentication:** Tools often require authentication to access the target API. Tyk AI Studio handles this securely by integrating with [Secrets Management](./secrets.md). You configure the authentication method (e.g., Bearer Token, Basic Auth) defined in the OAS and reference a stored Secret containing the actual credentials.
*   **Privacy Levels:** Each Tool is assigned a privacy level. This level is compared against the privacy level of the [LLM Configuration](./llm-management.md) being used. A Tool can only be used if its privacy level is less than or equal to the LLM's level, preventing sensitive tools from being used with potentially less secure or external LLMs.

    Privacy levels define how data is protected by controlling LLM access based on its sensitivity:
    - **Public (0)** – Safe to share (e.g., blogs, press releases).
    - **Internal (25)** – Company-only info (e.g., reports, policies).
    - **Confidential (50)** – Sensitive business data (e.g., financials, strategies).
    - **Restricted/PII (100)** – Personal data (e.g., names, emails, customer info).

    *Note: Privacy levels are stored as integer scores in the system. The values shown in parentheses are the typical score mappings.*
*   **Tool Catalogues:** Tools are grouped into logical collections called Catalogues. This simplifies management and access control.
*   **Filters:** Optional [Filters](./filters.md) can be applied to tool interactions to pre-process requests sent to the tool or post-process responses received from it (e.g., for data sanitization).
*   **Documentation:** Administrators can provide additional natural language documentation or instructions specifically for the LLM, guiding it on how and when to use the tool effectively.
*   **Dependencies:** Tools can declare dependencies on other tools, although the exact usage pattern might vary.

## Availability

Tools are available on both **AI Studio** (embedded gateway) and **Microgateway** (edge gateways). Tool configurations, OpenAPI specs, auth credentials, and app access associations are synced to edge gateways via the hub-spoke configuration system. Tools support namespace filtering for enterprise multi-tenant deployments.

## Access methods

A tool is a chat capability first. It can also be opened to [Apps](./apps.md) on the AI Studio gateway, and each way of doing so is an **access method** that an administrator switches on per tool:

| Access method | What it gives you | Default |
|---|---|---|
| **Chat** | LLMs invoke the tool's operations during conversations. This is the primary use case. | Always on |
| **REST API** | Developers call the tool's operations over HTTP at `/tools/{slug}`, with their App credential. | Off for a new tool |
| **MCP** | MCP clients such as Claude Desktop connect to the tool at `/tools/{slug}/mcp`, with the App credential or by signing in with OAuth. The tool's operations become MCP tools. | Off for a new tool |

The switches are on the tool form and in the last step of the import wizard, under **Access methods**, together with the gateway URL of each method that is on. The Tools list shows them in its **Access** column. Tools that existed before the switches were introduced keep both methods on.

A tool with both methods off is **chat only**:

*   It is offered in the chat tool picker as before.
*   It is not shown in the AI Portal: not in Browse, not on an asset or documentation page, and not in the App builder.
*   It cannot be added to an App. If an administrator switches both methods off on a tool that Apps already use, those Apps keep the tool, the gateway refuses their calls with `403`, and the App page tells the owner why.

Client (human-in-the-loop) tools, including the built-in **Generative UI** tool, run in the chat interface and are always chat only.

Both methods enforce the same rules: the calling App must hold the tool, only the operations selected on the tool may be called, and the tool's [Filters](./filters.md) run on the way in and on the way out. A call over a method that is switched off is refused with `403`.

### Tools and MCP servers

In the Enterprise Edition the admin console also has an **MCP servers** section. The two are different things, even though a tool can be reached over MCP:

| | Tool | [MCP server](./tyk-mcp-integration.md) |
|---|---|---|
| Served by | AI Studio (embedded gateway and edge gateways) | A Tyk Gateway. AI Studio catalogues it; its traffic never passes through AI Studio. |
| Built from | An OpenAPI specification | An MCP proxy on a Tyk Dashboard (a remote MCP server, or a REST API converted to MCP) |
| Used by | Chats and agents, and Apps over REST or MCP | Apps, over MCP |
| Credential | The App's own credential | A Tyk access key that AI Studio issues for the App |
| Governed by | AI Studio filters, privacy levels and tool analytics | Tyk policies, rate limits and quotas |

Choose a **tool** when chats and agents in AI Studio should use the API, or when AI Studio filters should apply to it. Choose an **MCP server** when the MCP traffic should be served and governed by your Tyk Gateway. The portal labels every endpoint with who serves it, and the App page gives a developer one MCP client configuration that covers both.

## How it Works

When a user interacts with an LLM via the [Chat Interface](./chat-interface.md):

1.  The LLM receives the user prompt and the definitions of available tools (based on user group permissions and Chat Experience configuration).
2.  If the LLM determines that using one or more tools is necessary to answer the prompt, it generates a request to invoke the specific tool operation(s) with the required parameters.
3.  Tyk AI Studio intercepts this request.
4.  It validates the request, checks permissions, and retrieves necessary secrets for authentication.
5.  Tyk AI Studio applies any configured request Filters.
6.  It calls the external API defined by the Tool.
7.  It receives the response from the external API.
8.  Tyk AI Studio applies any configured response Filters.
9.  It sends the tool's response back to the LLM.
10. The LLM uses the tool's response to formulate its final answer to the user.

## Creating & Managing Tools (Admin)

Administrators define and manage Tools via the UI or API:

1.  **Define Tool:** Provide a name, description, and privacy level.
2.  **Upload OpenAPI Spec:** Provide the OAS document (JSON/YAML).
3.  **Select Operations:** Choose the specific `operationIds` the LLM can use.
4.  **Configure Authentication:** Select the OAS security scheme and link to a stored [Secret](./secrets.md) for credentials.
5.  **Add Documentation:** Provide natural language instructions for the LLM.
6.  **Assign Filters (Optional):** Add request/response filters.
7.  **Choose Access Methods (Optional):** Switch on **REST API** or **MCP** if Apps should reach the tool on the gateway. A new tool is chat only. See [Access methods](#access-methods).

Over the API the two switches are the `rest_access_enabled` and `mcp_access_enabled` attributes of a tool. Leaving them out of a create request gives a chat-only tool; leaving them out of an update keeps their current values. Tool responses also carry `slug`, `app_grantable`, `rest_endpoint_url` and `mcp_endpoint_url`.

### Importing from a Tyk Dashboard (Enterprise)

The **Import OpenAPI** button on the Tools list opens a wizard with two methods: a direct import (URL, file or pasted document) and, in the Enterprise Edition, an import from a **Tyk Dashboard**. The Dashboard import uses the same [Tyk connections](./tyk-mcp-integration.md#connections-and-trust-modes) the MCP integration uses, so a Dashboard is connected once, with its access token stored encrypted, and reused everywhere.

1.  **Choose a connection.** The wizard lists the saved connections with their status. If none fits, **Add connection** opens an inline form (name, Dashboard URL, access token, optional organisation id) with a **Test connection** button that runs the capability probe; saving creates the connection in `catalogue` mode and selects it. Adding a connection needs the `tyk-connections:write` permission; the import itself needs `tools:write`. Everything else about the connection (trust mode, gateway URLs, MDCB, API template) is edited later under Settings → Tyk Connections.
2.  **Select an API.** The Tyk OAS APIs on that Dashboard are listed by name and listen path. Classic (non-OAS) definitions cannot be imported as tools.
3.  **Configure the tool.** Name, description, privacy level and the security scheme are pre-filled from the definition and can be edited before the tool is created.

The imported document keeps its `x-tyk-api-gateway` extension, but every credential under it (upstream authentication, credential-looking request-header transforms) is masked before it reaches AI Studio. Configure the tool's own credentials through a stored [Secret](./secrets.md) as for any other tool.

## Organizing & Assigning Tools (Admin)

*   **Create Catalogues:** Group related tools into Tool Catalogues (e.g., "CRM Tools", "Search Tools").
*   **Assign to Groups:** Assign Tool Catalogues to specific [User Groups](./user-management.md). This grants users in those groups *potential* access to the tools within the catalogue.

## Using Tools (User)

Tools become available to end-users within the [Chat Interface](./chat-interface.md) if:

1.  The specific Chat Experience configuration includes the relevant Tool Catalogue.
2.  The user belongs to a Group that has been assigned that Tool Catalogue.
3.  The Tool's privacy level is compatible with the LLM being used in the Chat Experience.

The LLM will then automatically decide when to use these available tools based on the conversation.
