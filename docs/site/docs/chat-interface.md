---
title: "Chat Interface"
weight: 70
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Chat Interface

Tyk AI Studio's Chat Interface provides a secure and interactive environment for users to engage with Large Language Models (LLMs), leveraging integrated tools and data sources. It serves as the primary front-end for conversational AI interactions within the platform.

## Purpose

The main goals of the Chat Interface are:

*   **User-Friendly Interaction:** Offer an intuitive web-based chat experience for users of all technical levels.
*   **Unified Access:** Provide a single point of access to various configured LLMs, Tools, and Data Sources.
*   **Context Management:** Maintain conversation history and manage context, including system prompts and retrieved data (RAG).
*   **Secure & Governed:** Enforce access controls based on user groups and apply configured Filters.

## Key Features

*   **Chat Sessions:** Each conversation happens within a session, preserving history and context. Sessions can be continued later from the Past Conversations list.
*   **Streaming Responses:** LLM responses are streamed token by token with a smooth reveal, rendered as GitHub-flavoured Markdown with syntax-highlighted, copyable code blocks.
*   **Tool Integration:** Seamlessly uses configured [Tools](./tools.md) when the LLM determines they are necessary. Every tool call is shown as a card with its arguments, status and result (JSON, table or text), and can be expanded for detail. The available tools depend on the Chat Experience configuration and the user's group permissions.
*   **Human-in-the-loop tools:** A tool of type *Client* is answered by the person in the chat instead of a server. The model calls it like any function; the chat shows an approval or a form card, and the conversation continues once the user answers. See [Client tools](#client-tools-human-in-the-loop).
*   **Plugin renderers:** Portal plugins can replace the default tool card for specific tools with their own web component (charts, tables, forms). See [Custom tool renderers](#custom-tool-renderers-from-plugins).
*   **Data Source (RAG) Integration:** Automatically queries configured [Data Sources](./datasources-rag.md) to retrieve relevant information; the retrieved context is shown on the message as a collapsible block.
*   **Edit and regenerate:** Users can edit one of their earlier messages (the conversation rewinds to that point and the model answers again) or regenerate the last reply, and stop a reply while it is streaming.
*   **System Prompts:** Administrators can define specific system prompts for different Chat Experiences.
*   **History:** Users can view and continue their past chat sessions.
*   **File Upload (Context):** Users can attach files to a message to provide temporary context for the LLM (depending on configuration).
*   **Status and errors:** Governance status lines (filters running, tools added) and categorised errors are shown inline and can be hidden with the *System and Context Messages* toggle.
*   **Print to PDF:** The conversation can be printed through the browser's print dialog.
*   **Access Control:** Users only see and can interact with Chat Experiences assigned to their [Groups](./user-management.md).

## Using the Chat Interface

Users access the Chat Interface through the Tyk AI Studio web UI.

1.  **Select Chat Experience:** Users choose from a list of available Chat Experiences (pre-configured chat environments) they have access to.
2.  **Interact:** Users type their prompts or questions, or pick one of the prompt templates offered on an empty conversation.
3.  **Receive Responses:** The LLM processes the request, potentially using tools or data sources behind the scenes, and streams the response back.

## Configuration (Admin)

Administrators configure the available "Chat Experiences" (formerly known as Chat Rooms) via the UI or API. Configuration involves:

*   **Naming:** Giving the Chat Experience a descriptive name.
*   **Assigning LLM:** Linking to a specific [LLM Configuration](./llm-management.md).
*   **Enabling Tools:** Selecting which [Tool Catalogues](./tools.md) are available.
*   **Enabling Data Sources:** Selecting which [Data Source Catalogues](./datasources-rag.md) are available.
*   **Setting System Prompt:** Defining the guiding prompt for the LLM.
*   **Applying Filters:** Associating specific [Filters](./filters.md) for governance.
*   **Assigning Groups:** Determining which [User Groups](./user-management.md) can access this Chat Experience.
*   **Enabling/Disabling Features:** Toggling features like file uploads or direct tool usage.

### Choosing the chat front end

The chat pages are built on [assistant-ui](https://www.assistant-ui.com/) primitives styled with the platform theme. The previous chat implementation is still shipped as a fallback:

| Setting | Effect |
|---------|--------|
| `CHAT_UI_V2_ENABLED=true` (default) | Chat rooms and agent chats use the new front end and the v2 chat API. |
| `CHAT_UI_V2_ENABLED=false` | The previous chat pages and the v1 SSE API are used. |
| `CHAT_SESSION_IDLE_TTL` (default `10m`) | How long an idle chat or agent session stays in memory between turns before it is stopped. Continuing a chat room session after that reloads it from the database; agent sessions have to be started again. |

The flag is reported to the browser as `chat_ui_v2` by `GET /common/system`.

## Client tools (human-in-the-loop)

A tool with type **Client** is executed by the person in the chat rather than by a server. It is created on the Tools page like any other tool:

1.  Choose **Client (human-in-the-loop)** as the tool type. The **Description** is what the model reads to decide when to use the tool, so write it as an instruction ("Collect a shipping address whenever an order needs a delivery destination").
2.  Pick the interaction: **Approval** (the user approves or rejects what the model asked for) or **Form** (the user fills in a form).
3.  Describe the fields with the **field builder**: for an approval, the details the model must state (the action, the reason, ...); for a form, additionally the fields the person fills in. Each field has a label, an identifier, a type (short or long text, number, yes/no, date, email, phone, web address or a dropdown of choices), a required flag and help text. The page ships with **examples** that fill in everything, ready to be edited: *Shipping address (US)*, *Shipping address (international)* and *Contact information* for forms, *Confirm a destructive action* and *Approve a purchase or spend* for approvals. The **preview** beside the builder shows the function the model sees and the card the person gets, and answering the preview card shows exactly what the model would receive. An **Edit as JSON** switch exposes the underlying JSON Schema for anything the builder cannot express.
4.  Attach the tool to a Chat Experience (default tools) or let users add it from the chat sidebar, exactly like a REST tool. Privacy levels, group entitlements and filters apply as usual.

The model sees the tool as a function named after the tool's slug (or its first configured operation). When it calls the tool the reply pauses, the chat shows the card, and the answer is sent back to the model as the tool's result, so it can carry on. If the user sends a new message instead of answering, the pending call is closed with an error result so the conversation stays valid.

The definition is stored in the tool's spec field as JSON:

```json
{
  "parameters": { "type": "object", "properties": { "action": { "type": "string" } }, "required": ["action"] },
  "ui": {
    "kind": "approval",
    "title": "Approve this action?",
    "description": "Shown above the Approve / Reject buttons"
  }
}
```

Approval answers are sent as `{"approved": true|false, "comment": "..."}`; form answers are the form's data.

Because an answer comes from the browser rather than from a tool backend, the server does not pass it to the model as-is. It is checked against the shape the card produces (an approval must be an object with a boolean `approved` and an optional `comment`; a form answer must match the form's response schema; anything over 32 KiB is refused), and it is stored as a labelled envelope, `{"source": "user", "untrusted": true, "note": "...", "answer": ...}`, so the model reads it as data the person supplied and not as a trusted system response. A rejected answer reaches the model as an error result. The chat still shows the person's own answer.

### Generative UI (the `present` tool)

The third interaction kind, **Generative UI (present)**, lets the model compose its own interface instead of answering in prose: dashboards, cards, key figures, tables, charts, alerts, lists, images, forms and buttons, assembled from a fixed component vocabulary (the [assistant-ui generative UI library](https://www.assistant-ui.com/docs/tools/generative-ui)). The chat draws the tree the model produces; no user input is needed, so the call resolves by itself and the model carries on.

The tool ships with the platform: a Client tool called **Generative UI** (function name `present`) is created on first start, the same way the default LLM configurations are, and placed in the Default tool catalogue. To enable it for a room, pick it under **Default Tools** in the chat room form, or let users add it from the chat sidebar. Its schema is built in: it is generated from the installed library (`ui/admin-frontend/scripts/gen-present-schema.mjs` writes `models/generative_ui_present_schema.json`) so the model and the renderer always share the same vocabulary. A system prompt such as "Use the present tool whenever a visual layout would be clearer than text" makes the model reach for it more readily.

Interactive elements (`Button`, `Select`, `Input`, `DatePicker`, `Checkbox`, `RadioGroup`, `Form`, clickable `ListViewItem`) carry an `$action` the model chooses. When the user activates one, its payload is sent back to the model as the next user message (`Action: <type> <input> <payload>`), so the model can respond to the choice.

Prompts to try: "Show me a dashboard of this quarter's sales: three headline numbers, a bar chart by month and a table by region", "Present the pros and cons as two cards side by side", or "Build me a short form to collect a shipping address, then confirm it back".

## Custom tool renderers from plugins

A portal plugin can replace the default tool card for particular tools with its own web component by declaring the `chat.tool_renderer` slot in its manifest:

```json
{
  "portal": {
    "slots": [
      {
        "slot": "chat.tool_renderer",
        "label": "Chat renderers",
        "items": [
          { "type": "component", "tool": "getWeather", "mount": { "kind": "webc", "tag": "x-weather-card", "entry": "/ui/weather.js" } }
        ]
      }
    ]
  }
}
```

`tool` is the tool operation name (the operationId of a REST tool, or the slug of a client tool). The element is mounted for every call of that tool and receives:

*   attributes `data-tool-name`, `data-args` (JSON), `data-result` (JSON, absent while the tool is running), `data-is-error` and `data-status` (`running`, `complete`, `requires-action`, `incomplete`);
*   the property `toolCall` with the same information as an object, and a `render()` call after each update if the element defines one;
*   the portal plugin API (`element.portalPluginAPI`) for RPC calls to the plugin.

To answer a human-in-the-loop call from a renderer, dispatch a `tool-result` `CustomEvent` whose `detail` is the result (or `{ "result": ..., "isError": true }` to decline). Group visibility of the slot follows the usual portal rules; a renderer that fails to load is logged and the default card is used.

## API Access

Beyond the UI, Tyk AI Studio provides APIs for programmatic interaction with the chat system.

### v2 chat API (used by the new front end)

Every turn is one streamed request. The stream is the [AI SDK UI message stream](https://ai-sdk.dev/docs/ai-sdk-ui/stream-protocol) (server-sent events of JSON chunks, terminated by `data: [DONE]`, header `x-vercel-ai-ui-message-stream: v1`), so any client that speaks that protocol can consume it.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/common/chat/:chat_id/sessions` | Create a session, or resume one by passing `{"session_id": "..."}`. Returns the session id, the tools and datasources attached, the client tools, and the chat's name, description and prompt templates. |
| `POST` | `/common/chat-sessions/:session_id/runs` | Run one turn and stream the reply. Body: `{"message": "...", "file_refs": []}` for a new turn; add `"after_message_id"` to rewind the history first (an edit); `{"regenerate": true}` to answer the last user message again; `{"tool_results": [{"tool_call_id": "...", "result": ...}]}` to resume after a client tool. |
| `POST` | `/common/chat-sessions/:session_id/cancel` | Stop the reply in flight. |
| `GET` | `/common/chat-sessions/:session_id/messages/v2` | The transcript as assistant-ui parts, newest page first: `?limit=` (default 200, max 1000) and `?before=<row id>` page backwards; the response carries `has_more` and `next_before`. |
| `GET` | `/common/chat-sessions/:session_id/messages/v2` | The conversation as thread messages: `text`, `tool-call` (with arguments and result) and `data` parts (retrieved context). |
| `POST` | `/common/chat-sessions/:session_id/tools` / `datasources` / `upload` | Unchanged: attach tools and datasources, upload files for the next message. |

Stream chunks: `start`, `text-start` / `text-delta` / `text-end`, `reasoning-delta`, `tool-input-start` / `tool-input-delta` / `tool-input-available`, `tool-output-available` / `tool-output-error`, `data-status` (governance status lines), `data-context` (retrieved context), `data-error` (with a `code`: `llm_config`, `api`, `connection`, `auth`, `filter`, `tool`, `session`, `internal`), `finish` (`finishReason` `stop`, `tool-calls` when waiting on a client tool, `error`, `cancelled`) and a transient `data-message-ids` carrying the database ids of the turn's rows.

Agents expose the same shape under `/common/agents/:id/sessions` and `/common/agent-sessions/:session_id/{runs,cancel,messages/v2}`.

### v1 chat API

The original long-lived SSE API remains available: `GET /common/chat/:chat_id` (events `session_id`, `stream_chunk`, `message`, `system`, `error`) with `POST /common/chat/:chat_id/messages?session_id=` for user messages. A session is bound to whichever API version first attached to it.

This comprehensive system provides a powerful yet controlled way for users to interact with AI capabilities managed by Tyk AI Studio.
