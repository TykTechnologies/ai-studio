# Agent Plugins Implementation Status

**Status**: Backend implementation complete (Phases 1-5) - Ready for testing
**Date**: 2025-10-05
**Next Steps**: E2E testing with test agent plugin, then UI implementation (Phase 6)

---

## Overview

Agent plugins enable AI Studio to run agentic workflows with multi-step reasoning, tool usage, and LLM calls. Agents are installed as plugins (like other AI Studio plugins) and configured via `AgentConfig` records that reference an `App` for resource access.

### Architecture Flow

```
User → API → AgentSession → gRPC Plugin → Agent Logic
                    ↓
                 Queue (streaming)
                    ↓
         SSE to Frontend
```

Agent plugins call back to AI Studio via gRPC for:
- **ExecuteTool**: Call tools subscribed by the App
- **QueryDatasource**: RAG queries on App datasources
- **CallLLM**: LLM inference (routed through `/ai/` proxy endpoint)

---

## Phase 1: Model Layer ✅

**File**: `models/agent_config.go`

### AgentConfig Model
- References a `Plugin` (must be type "agent")
- References an `App` (provides LLMs, Tools, Datasources)
- Has `Config` map for plugin-specific settings
- Group-based access control
- Active/inactive status
- Namespace support

**Key Methods**:
- `Create()`, `Get()`, `Update()`, `Delete()`
- `Activate()`, `Deactivate()`
- `AddGroup()`, `RemoveGroup()` - Access control
- `ListWithPagination()` - Query with filters
- `Validate()` - Ensures plugin is agent type, app exists

**Database**: Auto-migrated table `agent_configs`

---

## Phase 2: gRPC Protocol Extensions ✅

**Files**:
- `proto/ai_studio_management.proto` - Host services for plugins
- `proto/plugin.proto` - Plugin services for host

### New RPC Methods (Host → Plugin)

**In `plugin.proto`**:
```protobuf
rpc HandleAgentMessage(AgentMessageRequest) returns (stream AgentMessageChunk);
```

Agent receives:
- User message
- Available tools/datasources/LLMs (from App)
- Agent config JSON
- Conversation history
- Plugin context

Agent streams back chunks:
- CONTENT - Text response
- TOOL_CALL - Calling a tool
- TOOL_RESULT - Tool execution result
- THINKING - Reasoning/planning
- ERROR - Error occurred
- DONE - Finished

### New RPC Methods (Plugin → Host)

**In `ai_studio_management.proto`**:
```protobuf
rpc ExecuteTool(ExecuteToolRequest) returns (ExecuteToolResponse);
rpc QueryDatasource(QueryDatasourceRequest) returns (QueryDatasourceResponse);
rpc CallLLM(CallLLMRequest) returns (stream CallLLMResponse);
```

These allow plugins to:
- Execute tools with credential injection
- Query datasources using RAG
- Call LLMs through proxy (budget/filters/analytics applied)

**Proto Files Generated**: `proto/*.pb.go` (package `proto`)

---

## Phase 3: gRPC Service Implementation ✅

**File**: `services/grpc/ai_studio_management_server.go`

### Implemented Handlers

#### ExecuteTool (lines 632-707)
- Validates plugin has `tools.call` scope
- Loads AgentConfig with App.Tools preloaded
- Verifies tool is in App's allowed tools
- Calls `service.CallToolOperation()` with credentials
- Returns JSON result

#### QueryDatasource (lines 714-822)
- Validates plugin has `datasources.query` scope
- Creates `DataSession` for similarity search
- Returns results with scores and metadata
- Filters by similarity threshold

#### CallLLM (lines 835-1031)
- Validates plugin has `llms.proxy` scope
- Makes **internal HTTP call** to `/ai/{llmSlug}/v1/chat/completions`
- Uses OpenAI shim endpoint (non-streaming)
- Includes App credential in Authorization header
- All proxy middleware applied: analytics, budget, filters
- Parses OpenAI response and streams via gRPC

**Key Decision**: Internal HTTP call ensures single code path for LLM logic (analytics, budget, filters, plugins)

---

## Phase 4: Agent Session Management ✅

**File**: `agent_session/agent_session.go`

### AgentSession Structure
- Manages runtime lifecycle of agent conversation
- Uses `MessageQueue` for streaming responses
- Calls plugin via gRPC `HandleAgentMessage`
- Builds context from App resources (tools, datasources, LLMs)

**Key Design**: Does NOT import `chat_session` to avoid circular dependencies. Defines minimal `MessageQueue` interface locally.

### Key Methods
- `NewAgentSession()` - Creates session with queue
- `SendMessage()` - Sends to plugin, starts streaming
- `receiveChunks()` - Goroutine that forwards plugin stream to queue
- `buildAgentRequest()` - Converts App resources to proto format
- `Close()` - Cleanup

**Context Building**:
- Tools → `AgentToolInfo` (ID, Name, Slug, Description)
- Datasources → `AgentDatasourceInfo` (ID, Name, Description, DBSourceType)
- LLMs → `AgentLLMInfo` (ID, Name, Vendor, DefaultModel)

---

## Phase 5: API Layer Integration ✅

**Files**:
- `api/agent_handler.go` - All agent endpoints
- `api/api.go` - Route registration (lines 574-582)
- `services/service.go` - Helper method `GetPluginClient()` (lines 124-136)

### REST Endpoints

#### POST /api/v1/agents/:id/message
**Purpose**: Send message to agent, stream responses via SSE

**Flow**:
1. Authenticate user
2. Load AgentConfig with all relationships
3. Check active status, plugin type, user access (groups)
4. Get plugin client via `service.GetPluginClient()`
5. Create queue via `chat_session.CreateDefaultQueueFactoryWithSharedDB()`
6. Create `AgentSession`
7. Send message (async)
8. Stream chunks from queue as SSE events

**SSE Events**: `session`, `content`, `tool_call`, `tool_result`, `thinking`, `error`, `done`

**Request Body**:
```json
{
  "message": "user question",
  "history": [{"role": "user", "content": "..."}, ...],
  "session_id": "optional"
}
```

#### GET /api/v1/agents
List accessible agents (filtered by user's groups), with pagination

#### GET /api/v1/agents/:id
Get specific agent details (requires group access)

#### POST /api/v1/agents (Admin only)
Create new AgentConfig
- Validates plugin is type "agent" and active
- Validates app exists
- Generates slug from name
- Associates groups

#### PUT /api/v1/agents/:id (Admin only)
Update AgentConfig (name, description, config, groups, active status)

#### DELETE /api/v1/agents/:id (Admin only)
Soft delete AgentConfig

#### POST /api/v1/agents/:id/activate (Admin only)
Activate agent

#### POST /api/v1/agents/:id/deactivate (Admin only)
Deactivate agent

---

## Key Architectural Decisions

### 1. No Circular Dependencies
- `agent_session` does NOT import `services` or full `chat_session`
- Defines minimal `MessageQueue` interface locally
- Services creates queue and passes it to session
- API layer orchestrates (not service layer)

### 2. Apps Provide Resources
- AgentConfig references App (not duplicate resources)
- App has: Credential, LLMs, Tools, Datasources
- Agent plugin receives list of available resources
- Agent calls back to host to use resources

### 3. Internal HTTP for LLM Calls
- CallLLM makes HTTP request to `/ai/{llmSlug}/v1/chat/completions`
- Ensures single code path (no duplicate middleware logic)
- Budget tracking, filters, analytics all applied
- Trade-off: Extra hop, but maintains consistency

### 4. Group-Based Access Control
- AgentConfigs have many-to-many Groups relationship
- Empty groups = public access
- Users must be in at least one group to access

### 5. Service Scopes
Plugins must declare required scopes in manifest:
- `tools.call` - Execute tools
- `datasources.query` - Query datasources
- `llms.proxy` - Call LLMs

Validated during gRPC calls.

---

## Testing Status

### ⚠️ Not Yet Tested
- No unit tests written for agent handlers
- No integration tests
- No manual E2E testing performed
- Only verified: **build succeeds**

### Recommended Tests
1. **Unit tests** (following `chat_handlers_test.go` patterns)
2. **E2E test** with simple agent plugin:
   - Install test agent plugin
   - Create AgentConfig referencing test App
   - POST to `/api/v1/agents/:id/message`
   - Verify SSE streaming works
   - Verify agent can call tools/datasources/LLMs

---

## Next Phase: Frontend Integration (Phase 6)

### Admin UI Needed
- Agent list page (`/agents`)
- Create/edit agent form
- Agent detail view
- Plugin installation flow integration

### Portal UI Needed
- Agent selector (shows accessible agents)
- Agent chat interface (similar to chat UI)
- Display different chunk types (thinking, tool calls, etc.)
- SSE integration for streaming

---

## Important Files Reference

### Models
- `models/agent_config.go` - AgentConfig domain model
- `models/plugin.go` - Plugin types (includes `PluginTypeAgent = "agent"`)

### Services
- `services/grpc/ai_studio_management_server.go` - gRPC handlers for plugins
- `services/service.go` - GetPluginClient() helper
- `services/ai_studio_plugin_manager.go` - Plugin loading/management

### Agent Session
- `agent_session/agent_session.go` - Complete session management

### API
- `api/agent_handler.go` - All REST endpoints
- `api/api.go` - Route registration (v1 group)

### Proto
- `proto/ai_studio_management.proto` - Host services
- `proto/plugin.proto` - Plugin services
- `proto/*.pb.go` - Generated Go code

---

## Known Limitations

1. **Budget checking**: Simplified - relies on proxy enforcement when agent calls LLMs
2. **No tests**: Test coverage needs to be added
3. **No UI**: Phase 6 not started
4. **Error handling**: May need refinement based on E2E testing
5. **Session persistence**: Sessions are ephemeral (in-memory or queue-based)

---

## How Agent Plugins Work

### Installation
1. Admin installs agent plugin (type="agent") via plugin management
2. Plugin registered in `plugins` table

### Configuration
1. Admin creates `AgentConfig` record
2. Links to installed plugin
3. Links to App (provides resources)
4. Sets groups for access control
5. Provides plugin-specific config

### Runtime Flow
1. User sends message via POST `/api/v1/agents/:id/message`
2. API loads AgentConfig, verifies access
3. Creates queue for streaming
4. Creates AgentSession, gets plugin client
5. AgentSession calls plugin's `HandleAgentMessage` via gRPC
6. Plugin receives context (tools, datasources, LLMs, config)
7. Plugin implements agentic logic (planning, loops, reasoning)
8. Plugin calls back to host:
   - `ExecuteTool` - Execute a tool
   - `QueryDatasource` - RAG query
   - `CallLLM` - LLM inference
9. Plugin streams chunks back:
   - Content, thinking, tool calls, errors
10. AgentSession forwards to queue
11. API streams as SSE to frontend

### Key Insight
Agent plugins are **orchestrators**. They don't execute tools or call LLMs directly. They decide **what** to do and **when**, then delegate execution back to AI Studio which enforces:
- Authentication (App credentials)
- Authorization (App subscriptions)
- Budget tracking
- Analytics
- Filters/policies
- Privacy scores

This keeps security/governance centralized in AI Studio while allowing plugins to implement diverse agentic strategies.
