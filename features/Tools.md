# Tools

## Introduction

This document provides a comprehensive overview of Midsommar's **Tool System**, detailing its architecture, components, and integration points across the platform. The Tool System enables extending LLM capabilities with external services and APIs while maintaining security and privacy controls.

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Key Components](#key-components)
4. [Tool Management](#tool-management)
5. [Privacy and Security](#privacy-and-security)
6. [Integration Points](#integration-points)
7. [Code References](#code-references)
8. [Testing Strategy](#testing-strategy)
9. [UI Integration](#ui-integration)
10. [LLM Provider Integration](#llm-provider-integration)
11. [MCP Server Integration](#mcp-server-integration)
12. [Tool Operations](#tool-operations)
13. [Tool Dependencies](#tool-dependencies)
14. [Universal Client Integration](#universal-client-integration)
15. [Future Enhancements](#future-enhancements)

---

## Overview

The **Midsommar Tool System** provides a framework for integrating external services and APIs with LLM interactions. Its core objectives are:

- **External Service Integration:** Enable LLMs to interact with external services via OpenAPI specifications
- **Privacy Control:** Enforce privacy scores to ensure tools are only used with compatible LLM providers
- **Access Management:** Control tool access through user groups and chat room assignments
- **Security:** Manage authentication and authorization for external service access
- **Extensibility:** Support easy addition of new tools and tool types

---

## System Architecture

1. **Database Models:**
   - `Tool` model ([models/tool.go](../models/tool.go)) - Core tool configuration and metadata
   - `ToolCatalogue` model ([models/tool_catalogue.go](../models/tool_catalogue.go)) - Groups of related tools

2. **Services:**
   - `ToolService` ([services/tool_service.go](../services/tool_service.go)) - Tool CRUD and management
   - `ToolCatalogueService` ([services/tool_catalogue_service.go](../services/tool_catalogue_service.go)) - Catalogue operations

3. **API Layer:**
   - Tool management endpoints ([api/tool_handlers.go](../api/tool_handlers.go))
   - Tool catalogue endpoints ([api/tool_catalogue_handlers.go](../api/tool_catalogue_handlers.go))

4. **Frontend Components:**
   - Tool management UI ([ui/admin-frontend/src/admin/pages/ToolList.js](../ui/admin-frontend/src/admin/pages/ToolList.js))
   - Tool configuration forms ([ui/admin-frontend/src/admin/components/tools/ToolForm.js](../ui/admin-frontend/src/admin/components/tools/ToolForm.js))

---

## Key Components

### 1. Tool Model
```go
type Tool struct {
    ID          uint   
    Name        string 
    Description string 
    ToolType    string 
    OASSpec     string // Base64 encoded OpenAPI specification
    AvailableOperations string
    PrivacyScore       int    
    AuthKey            string 
    AuthSchemaName     string 
    FileStores         []FileStore
    Filters            []Filter   
    Dependencies       []*Tool    
}
```

### 2. Tool Filters
- Tools can have associated filters that control their behavior
- Filters operate independently of LLM provider and chat room filters
- Filter hierarchy: Tool filters → LLM Provider filters → Chat Room filters

### 3. Privacy Scoring
- Tools have a privacy score that determines compatibility with LLM providers
- Higher scores indicate more stringent privacy requirements
- Tools can only be used with LLM providers meeting their privacy threshold

---

## Tool Operations

### OpenAPI Specification Management

1. **OAS Storage:**
   - OpenAPI specifications are stored in Base64 encoded format
   - Encoding/decoding handled automatically by the service layer
   - Specs are validated during import

2. **Operation Management:**
   ```go
   // Adding operations
   tool.AddOperation("operation_name")
   
   // Removing operations
   tool.RemoveOperation("operation_name")
   
   // Getting operations
   operations := tool.GetOperations()
   ```

3. **Operation Discovery:**
   - Automatic operation discovery from OpenAPI specs
   - Manual operation registration support
   - Operation validation against spec

### Operation Execution

1. **Execution Flow:**
   ```go
   result, err := toolService.CallToolOperation(
       toolID,
       operationID,
       params,
       payload,
       headers,
   )
   ```

2. **Parameter Types:**
   - Query parameters (map[string][]string)
   - Request payload (map[string]interface{})
   - Custom headers (map[string][]string)

3. **Response Handling:**
   - JSON response formatting
   - Error propagation
   - Response validation

---

## Tool Dependencies

### Dependency Management

1. **Adding Dependencies:**
   ```go
   err := tool.AddDependency(db, dependencyTool)
   ```

2. **Circular Dependency Prevention:**
   - Automatic detection of circular references
   - Recursive dependency checking
   - Self-dependency prevention

3. **Dependency Operations:**
   - Get all dependencies
   - Remove specific dependencies
   - Clear all dependencies
   - Check dependency existence

### Implementation Details

1. **Circular Reference Detection:**
   ```go
   func (t *Tool) WouldCreateCircularDependency(db *gorm.DB, newDependency *Tool) (bool, error) {
       // Direct dependency check
       // Recursive dependency check
       // Cycle detection
   }
   ```

2. **Dependency Validation:**
   - Existence validation
   - Type compatibility
   - Privacy score compatibility

---

## Universal Client Integration

### Client Configuration

1. **Client Options:**
   ```go
   options := []universalclient.ClientOption{
       universalclient.WithResponseFormat(universalclient.ResponseFormatJSON),
       universalclient.WithAuth(schemaName, authKey),
   }
   ```

2. **Authentication Integration:**
   - Dynamic auth schema selection
   - Multiple auth method support
   - Secure key management

3. **Response Handling:**
   - Format standardization
   - Error normalization
   - Response transformation

### Operation Execution

1. **Operation Flow:**
   ```go
   client, err := universalclient.NewClient(decodedSpec, "", options...)
   result, err := client.CallOperation(operationID, params, payload, headers)
   ```

2. **Error Handling:**
   - Connection errors
   - Authentication failures
   - Operation-specific errors
   - Response validation errors

---

## MCP Server Integration

### Protocol Overview

1. **JSON-RPC Based Communication:**
   - Protocol Version: 2.0
   - Request/Response format
   - Error handling with standard codes

2. **Server Initialization:**
   ```go
   type ServerConfig struct {
       Implementation      Implementation
       Capabilities       ServerCapabilities
       Handler            ServerHandler
       NotificationHandler NotificationHandler
       ValidationOptions  *ContentValidationOptions
   }
   ```

### Tool-Related Methods

1. **List Tools:**
   ```go
   // Request
   type ListToolsRequest struct {
       Cursor string `json:"cursor,omitempty"`
   }

   // Response
   type ListToolsResult struct {
       Tools      []Tool
       NextCursor string
   }
   ```

2. **Call Tool:**
   ```go
   // Request
   type CallToolRequest struct {
       Name      string
       Arguments map[string]interface{}
   }

   // Response
   type CallToolResult struct {
       Result interface{}
   }
   ```

### Notification System

1. **Tool List Changes:**
   ```go
   func (s *Server) SendToolListChanged(ctx context.Context) error
   ```

2. **Resource Updates:**
   ```go
   func (s *Server) SendResourceUpdate(ctx context.Context, uri string) error
   ```

### Resource Validation

1. **Content Validation:**
   ```go
   type ContentValidationOptions struct {
       MaxTextSize      int64
       MaxBlobSize      int64
       AllowedMIMETypes []string
   }
   ```

2. **Validation Rules:**
   - MIME type validation
   - Size limits enforcement
   - URI format validation
   - Content type checking

### Content Types

1. **Text Content:**
   ```go
   type TextContent struct {
       Type        string     `json:"type"` // Must be "text"
       Text        string     `json:"text"`
       Annotations *Annotated `json:"annotations,omitempty"`
   }
   ```

2. **Image Content:**
   ```go
   type ImageContent struct {
       Type        string     `json:"type"` // Must be "image"
       Data        []byte     `json:"data"`
       MimeType    string     `json:"mimeType"`
       Annotations *Annotated `json:"annotations,omitempty"`
   }
   ```

3. **Embedded Resources:**
   ```go
   type EmbeddedResource struct {
       Type        string      `json:"type"`     // Must be "resource"
       Resource    interface{} `json:"resource"` // TextResourceContents or BlobResourceContents
       Annotations *Annotated  `json:"annotations,omitempty"`
   }
   ```

### Notification Types

1. **System Notifications:**
   ```go
   const (
       NotificationCancelled           = "notifications/cancelled"
       NotificationInitialized         = "notifications/initialized"
       NotificationProgress            = "notifications/progress"
       NotificationResourceListChanged = "notifications/resources/list_changed"
       NotificationResourceUpdated     = "notifications/resources/updated"
       NotificationPromptListChanged   = "notifications/prompts/list_changed"
       NotificationToolListChanged     = "notifications/tools/list_changed"
       NotificationLoggingMessage      = "notifications/logging/message"
   )
   ```

2. **Logging Levels:**
   ```go
   const (
       LoggingLevelEmergency = "emergency"
       LoggingLevelAlert     = "alert"
       LoggingLevelCritical  = "critical"
       LoggingLevelError     = "error"
       LoggingLevelWarning   = "warning"
       LoggingLevelNotice    = "notice"
       LoggingLevelInfo      = "info"
       LoggingLevelDebug     = "debug"
   )
   ```

### Sampling Capabilities

1. **Model Preferences:**
   ```go
   type ModelPreferences struct {
       SpeedPriority        float64     `json:"speedPriority,omitempty"`
       CostPriority         float64     `json:"costPriority,omitempty"`
       IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
       Hints                []ModelHint `json:"hints,omitempty"`
   }
   ```

2. **Message Creation:**
   ```go
   type CreateMessageRequest struct {
       Messages         []SamplingMessage `json:"messages"`
       MaxTokens        int               `json:"maxTokens"`
       Temperature      float64           `json:"temperature,omitempty"`
       StopSequences    []string          `json:"stopSequences,omitempty"`
       SystemPrompt     string            `json:"systemPrompt,omitempty"`
       ModelPreferences ModelPreferences  `json:"modelPreferences,omitempty"`
   }
   ```

3. **Response Handling:**
   ```go
   type CreateMessageResult struct {
       Content    interface{} `json:"content"` // TextContent or ImageContent
       Model      string      `json:"model"`
       Role       Role        `json:"role"`
       StopReason string      `json:"stopReason,omitempty"`
   }
   ```

### Error Handling

1. **Error Codes:**
   ```go
   const (
       ErrorCodeParseError     = -32700
       ErrorCodeInvalidRequest = -32600
       ErrorCodeMethodNotFound = -32601
       ErrorCodeInvalidParams  = -32602
       ErrorCodeInternalError  = -32603
   )
   ```

2. **Error Response Format:**
   ```go
   type JSONRPCError struct {
       Code    int
       Message string
       Data    interface{}
   }
   ```

3. **Validation Errors:**
   - Field-specific error messages
   - Detailed validation failure information
   - Error propagation through RPC

[Previous sections remain unchanged...]

---

## Codebase Structure

This section provides a comprehensive overview of all files involved in the Tool system, organized by their purpose and responsibility.

### Core Models and Types

1. **Models**
   - `models/tool.go` - Core `Tool` struct and CRUD operations
   - `models/analytics.go` - Tool usage tracking and metrics
   - `models/filestore.go` - File storage integration for tools
   - `models/group.go` - Group-tool access control
   - `models/chat.go` - Chat-tool integration
   - `models/llm_settings.go` - LLM provider compatibility

2. **MCP Server**
   - `mcpserver/types.go` - MCP protocol tool types
   - `mcpserver/mcpserver.go` - Tool execution in MCP
   - `mcpserver/example/main.go` - Example implementations

### Services Layer

1. **Tool Services**
   - `services/tool_service.go` - Main tool business logic
   - `services/tool_catalogue_service.go` - Tool catalogue management
   - `services/tool_service_test.go` - Service layer testing

2. **Integration Services**
   - `services/user_service.go` - User-tool access management
   - `services/group_service.go` - Group-based permissions
   - `services/chat_service.go` - Chat session integration

### API Layer

1. **Handlers**
   - `api/tool_handlers.go` - Tool management endpoints
   - `api/tool_catalogue_handlers.go` - Catalogue operations
   - `api/chat_session_handler.go` - Chat integration
   - `api/group_handlers.go` - Group management
   - `api/analytics_handlers.go` - Usage analytics

2. **Tests**
   - `api/tool_handlers_test.go` - API endpoint testing
   - `api/tool_catalogue_handlers_test.go` - Catalogue API tests
   - `api/chat_handlers_test.go` - Chat integration tests
   - `api/group_handlers_test.go` - Group permission tests

### Analytics

1. **Core Analytics**
   - `analytics/analytics.go` - Usage tracking implementation
   - `analytics/stats.go` - Statistics processing
   - `analytics/analytics_test.go` - Analytics testing

### LLM Provider Integration

1. **Response Models**
   - `responses/response_model_openai.go` - OpenAI format
   - `responses/response_model_anthropic.go` - Anthropic format
   - `responses/response_model_googleai.go` - Google AI format

### Documentation

1. **Feature Specifications**
   - `features/Tools.md` - Main tool specification
   - `features/Filters.md` - Filter integration
   - `features/Notifications.md` - Tool notifications
   - `features/UserManagement.md` - Access control

2. **API Documentation**
   - `docs/swagger/docs.go` - API documentation
   - `docs/site/content/docs/tools.md` - User guide
   - `docs/site/content/docs/catalogs.md` - Catalogue guide
   - `docs/site/content/docs/groups.md` - Group permissions
   - `docs/site/content/docs/chat-rooms.md` - Chat integration
   - `docs/site/content/docs/filters.md` - Filter system
   - `docs/site/content/docs/apps.md` - Application integration
   - `docs/site/content/docs/dashboard.md` - Analytics dashboard

### Common and Utilities

1. **API Utilities**
   - `api/common.go` - Shared request/response handling
   - `api/api.go` - Route registration
   - `api/models.go` - API operation models

2. **Authentication**
   - `api/auth_handlers.go` - Tool authentication

### File Responsibilities

Each file in the codebase serves a specific purpose in the tool ecosystem:

1. **Core Functionality:**
   - Model files define data structures and relationships
   - Service files implement business logic
   - Handler files expose external API endpoints

2. **Integration:**
   - MCP server files handle tool execution protocol
   - LLM response models enable AI integration
   - Chat service manages interactive usage

3. **Management:**
   - Catalogue files organize tool discovery
   - Group files handle access control
   - Analytics track system usage

4. **Documentation:**
   - Feature specs provide technical details
   - API docs guide implementation
   - User guides assist operation

5. **Testing:**
   - Service tests verify business logic
   - API tests ensure endpoint functionality
   - Analytics tests validate metrics

This structure ensures:
- Clear separation of concerns
- Modular design
- Comprehensive testing
- Complete documentation
- Scalable architecture

---

## Future Enhancements

1. **Tool Features:**
   - Batch operation support
   - Custom operation creation
   - Advanced filtering options
   - Operation retry policies
   - Response caching

2. **Integration:**
   - Additional authentication methods
   - Expanded OpenAPI support
   - Custom tool types
   - Webhook support
   - Event-driven operations

3. **Management:**
   - Usage analytics
   - Performance monitoring
   - Automated testing tools
   - Operation metrics
   - Health checks

4. **Security:**
   - Enhanced key rotation
   - Advanced rate limiting
   - Audit logging
   - Request signing
   - Response validation
