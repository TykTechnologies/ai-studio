package mcpserver

import "encoding/json"

// Implementation describes the name and version of an MCP implementation
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities defines the capabilities supported by the server
type ServerCapabilities struct {
	Logging   *struct{} `json:"logging,omitempty"`
	Resources *struct {
		ListChanged bool `json:"listChanged"`
		Subscribe   bool `json:"subscribe"`
	} `json:"resources,omitempty"`
	Prompts *struct {
		ListChanged bool `json:"listChanged"`
	} `json:"prompts,omitempty"`
	Tools *struct {
		ListChanged bool `json:"listChanged"`
	} `json:"tools,omitempty"`
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

// ClientCapabilities defines the capabilities supported by the client
type ClientCapabilities struct {
	Roots *struct {
		ListChanged bool `json:"listChanged"`
	} `json:"roots,omitempty"`
	Sampling     *struct{}              `json:"sampling,omitempty"`
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result"`
}

// JSONRPCError represents a JSON-RPC 2.0 error response
type JSONRPCError struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Error   struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	} `json:"error"`
}

// Resource related types
type Resource struct {
	Name        string     `json:"name"`
	URI         string     `json:"uri"`
	MimeType    string     `json:"mimeType,omitempty"`
	Description string     `json:"description,omitempty"`
	Annotations *Annotated `json:"annotations,omitempty"`
}

type ResourceTemplate struct {
	Name        string     `json:"name"`
	URITemplate string     `json:"uriTemplate"`
	MimeType    string     `json:"mimeType,omitempty"`
	Description string     `json:"description,omitempty"`
	Annotations *Annotated `json:"annotations,omitempty"`
}

type ResourceReference struct {
	Type string `json:"type"` // Must be "ref/resource"
	URI  string `json:"uri"`
}

type TextResourceContents struct {
	URI      string `json:"uri"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType,omitempty"`
}

type BlobResourceContents struct {
	URI      string `json:"uri"`
	Blob     []byte `json:"blob"`
	MimeType string `json:"mimeType,omitempty"`
}

// Prompt related types
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type PromptReference struct {
	Type string `json:"type"` // Must be "ref/prompt"
	Name string `json:"name"`
}

type PromptMessage struct {
	Role    Role        `json:"role"`
	Content interface{} `json:"content"` // Can be TextContent, ImageContent, or EmbeddedResource
}

// Tool related types
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Content types
type TextContent struct {
	Type        string     `json:"type"` // Must be "text"
	Text        string     `json:"text"`
	Annotations *Annotated `json:"annotations,omitempty"`
}

type ImageContent struct {
	Type        string     `json:"type"` // Must be "image"
	Data        []byte     `json:"data"`
	MimeType    string     `json:"mimeType"`
	Annotations *Annotated `json:"annotations,omitempty"`
}

type EmbeddedResource struct {
	Type        string      `json:"type"`     // Must be "resource"
	Resource    interface{} `json:"resource"` // Can be TextResourceContents or BlobResourceContents
	Annotations *Annotated  `json:"annotations,omitempty"`
}

// Common types
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Annotated struct {
	Audience []Role  `json:"audience,omitempty"`
	Priority float64 `json:"priority,omitempty"`
}

// Result types
type InitializeResult struct {
	ServerInfo      Implementation     `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ProtocolVersion string             `json:"protocolVersion"`
	Instructions    string             `json:"instructions,omitempty"`
}

type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type ReadResourceResult struct {
	Contents []interface{} `json:"contents"` // Can contain TextResourceContents or BlobResourceContents
}

type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

type GetPromptResult struct {
	Messages    []PromptMessage `json:"messages"`
	Description string          `json:"description,omitempty"`
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type CallToolResult struct {
	Content []interface{} `json:"content"` // Can contain TextContent, ImageContent, or EmbeddedResource
	IsError bool          `json:"isError,omitempty"`
}

type CompleteResult struct {
	Completion struct {
		Values  []string `json:"values"`
		HasMore bool     `json:"hasMore,omitempty"`
		Total   int      `json:"total,omitempty"`
	} `json:"completion"`
}

// Request types
type ListResourcesRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

type ReadResourceRequest struct {
	URI string `json:"uri"`
}

type SubscribeRequest struct {
	URI string `json:"uri"`
}

type UnsubscribeRequest struct {
	URI string `json:"uri"`
}

type ListPromptsRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

type GetPromptRequest struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

type ListToolsRequest struct {
	Cursor string `json:"cursor,omitempty"`
}

type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type SetLevelRequest struct {
	Level LoggingLevel `json:"level"`
}

type CompleteRequest struct {
	Argument struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"argument"`
	Ref interface{} `json:"ref"` // Can be PromptReference or ResourceReference
}

type LoggingLevel string

const (
	LoggingLevelEmergency LoggingLevel = "emergency"
	LoggingLevelAlert     LoggingLevel = "alert"
	LoggingLevelCritical  LoggingLevel = "critical"
	LoggingLevelError     LoggingLevel = "error"
	LoggingLevelWarning   LoggingLevel = "warning"
	LoggingLevelNotice    LoggingLevel = "notice"
	LoggingLevelInfo      LoggingLevel = "info"
	LoggingLevelDebug     LoggingLevel = "debug"
)

// Sampling related types
type ModelPreferences struct {
	SpeedPriority        float64     `json:"speedPriority,omitempty"`
	CostPriority         float64     `json:"costPriority,omitempty"`
	IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
	Hints                []ModelHint `json:"hints,omitempty"`
}

type ModelHint struct {
	Name string `json:"name,omitempty"`
}

type SamplingMessage struct {
	Role    Role        `json:"role"`
	Content interface{} `json:"content"` // Can be TextContent or ImageContent
}

type CreateMessageRequest struct {
	Messages         []SamplingMessage `json:"messages"`
	MaxTokens        int               `json:"maxTokens"`
	Temperature      float64           `json:"temperature,omitempty"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	SystemPrompt     string            `json:"systemPrompt,omitempty"`
	IncludeContext   string            `json:"includeContext,omitempty"`
	ModelPreferences ModelPreferences  `json:"modelPreferences,omitempty"`
	Metadata         interface{}       `json:"metadata,omitempty"`
}

type CreateMessageResult struct {
	Content    interface{} `json:"content"` // Can be TextContent or ImageContent
	Model      string      `json:"model"`
	Role       Role        `json:"role"`
	StopReason string      `json:"stopReason,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification
type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// NotificationType represents all possible notification methods
type NotificationType string

const (
	NotificationCancelled           NotificationType = "notifications/cancelled"
	NotificationInitialized         NotificationType = "notifications/initialized"
	NotificationProgress            NotificationType = "notifications/progress"
	NotificationResourceListChanged NotificationType = "notifications/resources/list_changed"
	NotificationResourceUpdated     NotificationType = "notifications/resources/updated"
	NotificationPromptListChanged   NotificationType = "notifications/prompts/list_changed"
	NotificationToolListChanged     NotificationType = "notifications/tools/list_changed"
	NotificationLoggingMessage      NotificationType = "notifications/logging/message"
)

// BaseNotification contains common fields for all notifications
type BaseNotification struct {
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

// LoggingMessageNotification represents a logging message from the server
type LoggingMessageNotification struct {
	BaseNotification
	Level  LoggingLevel `json:"level"`
	Data   interface{}  `json:"data"`
	Logger string       `json:"logger,omitempty"`
}

// ResourceListChangedNotification represents a change in the resource list
type ResourceListChangedNotification struct {
	BaseNotification
}

// ResourceUpdatedNotification represents an update to a specific resource
type ResourceUpdatedNotification struct {
	BaseNotification
	URI string `json:"uri"`
}

// PromptListChangedNotification represents a change in the prompt list
type PromptListChangedNotification struct {
	BaseNotification
}

// ToolListChangedNotification represents a change in the tool list
type ToolListChangedNotification struct {
	BaseNotification
}

// InitializedNotification represents the client's initialized notification
type InitializedNotification struct {
	BaseNotification
}

// CancelledNotification represents a cancellation of a previous request
type CancelledNotification struct {
	BaseNotification
	RequestID string `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

// ProgressNotification represents progress on a long-running operation
type ProgressNotification struct {
	BaseNotification
	Progress      float64 `json:"progress"`
	ProgressToken string  `json:"progressToken"`
	Total         float64 `json:"total,omitempty"`
}

type RequestError struct {
	Code    int
	Message string
}

func (e *RequestError) Error() string {
	return e.Message
}

// SubscriptionResponse represents a response to a subscription request
type SubscriptionResponse struct {
	SubscriptionID string `json:"subscriptionId"`
}

// Add validation for protocol versions
var SupportedProtocolVersions = []string{"1.0"}
