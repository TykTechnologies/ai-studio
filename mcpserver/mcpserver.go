package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"sync"
	"time"
)

const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// Define a type for request handlers
type requestHandler struct {
	handler   func(context.Context, json.RawMessage) (interface{}, error)
	needsInit bool
}

func wrapHandler[T any](h func(context.Context, json.RawMessage) (T, error)) func(context.Context, json.RawMessage) (interface{}, error) {
	return func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return h(ctx, params)
	}
}

// Subscription management
type subscriptionManager struct {
	subscribers map[string]map[string]ResourceSubscriber // URI -> SubID -> Subscriber
	mu          sync.RWMutex
}

func newSubscriptionManager() *subscriptionManager {
	return &subscriptionManager{
		subscribers: make(map[string]map[string]ResourceSubscriber),
	}
}

func (sm *subscriptionManager) addSubscriber(uri, subID string, callback func(string)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.subscribers[uri] == nil {
		sm.subscribers[uri] = make(map[string]ResourceSubscriber)
	}
	sm.subscribers[uri][subID] = ResourceSubscriber{
		URI:      uri,
		Callback: callback,
	}
}

func (sm *subscriptionManager) removeSubscriber(uri, subID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if subs, exists := sm.subscribers[uri]; exists {
		delete(subs, subID)
		if len(subs) == 0 {
			delete(sm.subscribers, uri)
		}
	}
}

func (sm *subscriptionManager) notifySubscribers(uri string) {
	sm.mu.RLock()
	subscribers := make([]ResourceSubscriber, 0)
	for _, subs := range sm.subscribers[uri] {
		subscribers = append(subscribers, subs)
	}
	sm.mu.RUnlock()

	for _, sub := range subscribers {
		sub.Callback(uri)
	}
}

type NotificationSender interface {
	SendResourceUpdate(ctx context.Context, uri string) error
	SendResourceListChanged(ctx context.Context) error
	SendPromptListChanged(ctx context.Context) error
	SendToolListChanged(ctx context.Context) error
	SendProgress(ctx context.Context, token string, progress, total float64) error
	SendLoggingMessage(ctx context.Context, level LoggingLevel, data interface{}, logger string) error
}

// ServerHandler defines the interface that must be implemented to provide
// server functionality
type ServerHandler interface {
	// Resource methods
	ListResources(ctx context.Context, cursor string) ([]Resource, string, error)
	ReadResource(ctx context.Context, uri string) ([]interface{}, error)
	SubscribeResource(ctx context.Context, uri string, callback func(string)) error
	UnsubscribeResource(ctx context.Context, uri string) error

	// Prompt methods
	ListPrompts(ctx context.Context, cursor string) ([]Prompt, string, error)
	GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error)

	// Tool methods
	ListTools(ctx context.Context, cursor string) ([]Tool, string, error)
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error)

	// Completion method
	Complete(ctx context.Context, req CompleteRequest) (*CompleteResult, error)

	// Sampling method
	CreateMessage(ctx context.Context, req CreateMessageRequest) (*CreateMessageResult, error)
}

// NotificationHandler is called when the server needs to send a notification to the client
type NotificationHandler func(context.Context, interface{}) error

// Server represents an MCP server instance
type Server struct {
	capabilities        ServerCapabilities
	serverInfo          Implementation
	handler             ServerHandler
	subscriptions       *subscriptionManager
	notificationHandler NotificationHandler
	validationOpts      ContentValidationOptions
	handlers            map[string]requestHandler
	initialized         bool
	mu                  sync.RWMutex
}

// ServerConfig contains the configuration for a new server
type ServerConfig struct {
	Implementation      Implementation
	Capabilities        ServerCapabilities
	Handler             ServerHandler
	NotificationHandler NotificationHandler
	ValidationOptions   *ContentValidationOptions
}

// NewServer creates a new MCP server instance
func NewServer(config ServerConfig) *Server {
	s := &Server{
		serverInfo:          config.Implementation,
		capabilities:        config.Capabilities,
		handler:             config.Handler,
		subscriptions:       newSubscriptionManager(),
		notificationHandler: config.NotificationHandler,
		validationOpts:      DefaultValidationOptions,
		handlers:            make(map[string]requestHandler),
	}

	// Register all handlers
	s.registerHandlers()

	return s
}

// Register all available handlers
func (s *Server) registerHandlers() {
	s.handlers = map[string]requestHandler{
		"initialize": {
			handler:   wrapHandler(s.handleInitialize),
			needsInit: false,
		},
		"ping": {
			handler:   func(ctx context.Context, _ json.RawMessage) (interface{}, error) { return struct{}{}, nil },
			needsInit: true,
		},
		"resources/list": {
			handler:   wrapHandler(s.handleListResources),
			needsInit: true,
		},
		"resources/read": {
			handler:   wrapHandler(s.handleReadResource),
			needsInit: true,
		},
		"resources/subscribe": {
			handler:   wrapHandler(s.handleSubscribe),
			needsInit: true,
		},
		"resources/unsubscribe": {
			handler:   wrapHandler(s.handleUnsubscribe),
			needsInit: true,
		},
		"prompts/list": {
			handler:   wrapHandler(s.handleListPrompts),
			needsInit: true,
		},
		"prompts/get": {
			handler:   wrapHandler(s.handleGetPrompt),
			needsInit: true,
		},
		"tools/list": {
			handler:   wrapHandler(s.handleListTools),
			needsInit: true,
		},
		"tools/call": {
			handler:   wrapHandler(s.handleCallTool),
			needsInit: true,
		},
		"completion/complete": {
			handler:   wrapHandler(s.handleComplete),
			needsInit: true,
		},
		"sampling/createMessage": {
			handler:   wrapHandler(s.handleCreateMessage),
			needsInit: true,
		},
	}
}

// getHandler returns the appropriate handler for a method
func (s *Server) getHandler(method string) (requestHandler, bool) {
	s.mu.RLock()
	handler, exists := s.handlers[method]
	s.mu.RUnlock()
	return handler, exists
}

// ResourceSubscriber represents a subscription to resource updates
type ResourceSubscriber struct {
	URI      string
	Callback func(string)
}

// NotifyResourceListChanged sends a notification that the resource list has changed
func (s *Server) NotifyResourceListChanged(ctx context.Context) error {
	// Only send if the server supports this capability
	if s.capabilities.Resources == nil || !s.capabilities.Resources.ListChanged {
		return nil
	}

	notification := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/resources/list_changed",
		Params:  struct{}{},
	}

	return s.notificationHandler(ctx, notification)
}

// HandleRequest processes an incoming JSON-RPC request
func (s *Server) HandleRequest(ctx context.Context, request []byte) ([]byte, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return createErrorResponse(nil, ErrorCodeParseError, "Parse error")
	}

	if req.JSONRPC != "2.0" {
		return createErrorResponse(req.ID, ErrorCodeInvalidRequest, "Invalid JSON-RPC version")
	}

	handler, exists := s.getHandler(req.Method)
	if !exists {
		return createErrorResponse(req.ID, ErrorCodeMethodNotFound, "Method not found")
	}

	// Check initialization state
	if handler.needsInit && !s.initialized {
		return createErrorResponse(req.ID, ErrorCodeInvalidRequest, "Server not initialized")
	}

	result, err := handler.handler(ctx, req.Params)
	if err != nil {
		if reqErr, ok := err.(*RequestError); ok {
			return createErrorResponse(req.ID, reqErr.Code, reqErr.Message)
		}
		return createErrorResponse(req.ID, ErrorCodeInternalError, err.Error())
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}

	return json.Marshal(response)
}

// Handler implementations

func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (*InitializeResult, error) {
	// Only allow initialization once
	s.mu.Lock()
	if s.initialized {
		s.mu.Unlock()
		return nil, &RequestError{
			Code:    ErrorCodeInvalidRequest,
			Message: "Server already initialized",
		}
	}

	var initParams struct {
		Capabilities    ClientCapabilities `json:"capabilities"`
		ClientInfo      Implementation     `json:"clientInfo"`
		ProtocolVersion string             `json:"protocolVersion"`
	}

	if err := json.Unmarshal(params, &initParams); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("invalid initialize params: %w", err)
	}

	// Check protocol version compatibility
	if err := s.checkProtocolVersion(initParams.ProtocolVersion); err != nil {
		s.mu.Unlock()
		return nil, &RequestError{
			Code:    ErrorCodeInvalidRequest,
			Message: fmt.Sprintf("Unsupported protocol version: %v", err),
		}
	}

	s.initialized = true
	s.mu.Unlock()

	return &InitializeResult{
		ServerInfo:      s.serverInfo,
		Capabilities:    s.capabilities,
		ProtocolVersion: "1.0",
	}, nil
}

func (s *Server) handleListResources(ctx context.Context, params json.RawMessage) (*ListResourcesResult, error) {
	var req ListResourcesRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid list resources params: %w", err)
	}

	resources, nextCursor, err := s.handler.ListResources(ctx, req.Cursor)
	if err != nil {
		return nil, err
	}

	return &ListResourcesResult{
		Resources:  resources,
		NextCursor: nextCursor,
	}, nil
}

func (s *Server) handleReadResource(ctx context.Context, params json.RawMessage) (*ReadResourceResult, error) {
	var req ReadResourceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid read resource params: %w", err)
	}

	contents, err := s.handler.ReadResource(ctx, req.URI)
	if err != nil {
		return nil, err
	}

	return &ReadResourceResult{
		Contents: contents,
	}, nil
}

func (s *Server) SendNotification(ctx context.Context, notificationType NotificationType, params interface{}) error {
	if s.notificationHandler == nil {
		return fmt.Errorf("no notification handler configured")
	}

	notification := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  string(notificationType),
		Params:  params,
	}

	return s.notificationHandler(ctx, notification)
}

// Update handleSubscribe with proper subscription management
func (s *Server) handleSubscribe(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if s.capabilities.Resources == nil || !s.capabilities.Resources.Subscribe {
		return nil, fmt.Errorf("resource subscription not supported")
	}

	var req SubscribeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid subscribe params: %w", err)
	}

	if err := validateURI(req.URI); err != nil {
		return nil, err
	}

	subID := generateSubscriptionID()
	callback := func(uri string) {
		s.SendResourceUpdate(ctx, uri)
	}

	s.subscriptions.addSubscriber(req.URI, subID, callback)

	if err := s.handler.SubscribeResource(ctx, req.URI, callback); err != nil {
		s.subscriptions.removeSubscriber(req.URI, subID)
		return nil, err
	}

	return struct {
		SubscriptionID string `json:"subscriptionId"`
	}{
		SubscriptionID: subID,
	}, nil
}

func (s *Server) handleUnsubscribe(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req UnsubscribeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid unsubscribe params: %w", err)
	}

	return struct{}{}, s.handler.UnsubscribeResource(ctx, req.URI)
}

func (s *Server) handleListPrompts(ctx context.Context, params json.RawMessage) (*ListPromptsResult, error) {
	var req ListPromptsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid list prompts params: %w", err)
	}

	prompts, nextCursor, err := s.handler.ListPrompts(ctx, req.Cursor)
	if err != nil {
		return nil, err
	}

	return &ListPromptsResult{
		Prompts:    prompts,
		NextCursor: nextCursor,
	}, nil
}

func (s *Server) handleGetPrompt(ctx context.Context, params json.RawMessage) (*GetPromptResult, error) {
	var req GetPromptRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid get prompt params: %w", err)
	}

	return s.handler.GetPrompt(ctx, req.Name, req.Arguments)
}

func (s *Server) handleListTools(ctx context.Context, params json.RawMessage) (*ListToolsResult, error) {
	var req ListToolsRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid list tools params: %w", err)
	}

	tools, nextCursor, err := s.handler.ListTools(ctx, req.Cursor)
	if err != nil {
		return nil, err
	}

	return &ListToolsResult{
		Tools:      tools,
		NextCursor: nextCursor,
	}, nil
}

func (s *Server) handleCallTool(ctx context.Context, params json.RawMessage) (*CallToolResult, error) {
	var req CallToolRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid call tool params: %w", err)
	}

	return s.handler.CallTool(ctx, req.Name, req.Arguments)
}

func (s *Server) handleComplete(ctx context.Context, params json.RawMessage) (*CompleteResult, error) {
	var req CompleteRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid complete params: %w", err)
	}

	return s.handler.Complete(ctx, req)
}

func (s *Server) handleCreateMessage(ctx context.Context, params json.RawMessage) (*CreateMessageResult, error) {
	var req CreateMessageRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid create message params: %w", err)
	}

	return s.handler.CreateMessage(ctx, req)
}

// NotifyResourceUpdate sends a resource update notification to subscribers
func (s *Server) NotifyResourceUpdate(ctx context.Context, uri string) {
	s.subscriptions.notifySubscribers(uri)
}

// Implement all notification methods
func (s *Server) SendResourceUpdate(ctx context.Context, uri string) error {
	if s.capabilities.Resources == nil || !s.capabilities.Resources.Subscribe {
		return nil
	}
	return s.SendNotification(ctx, NotificationResourceUpdated, ResourceUpdatedNotification{
		URI: uri,
	})
}

func (s *Server) validateResourceContent(content interface{}) error {
	switch c := content.(type) {
	case TextResourceContents:
		return validateTextResourceContent(c, s.validationOpts)
	case BlobResourceContents:
		return validateBlobResourceContent(c, s.validationOpts)
	default:
		return fmt.Errorf("invalid resource content type")
	}
}

// Add version handling
func (s *Server) checkProtocolVersion(clientVersion string) error {
	// Implement version compatibility check
	return nil
}

// Helper functions

func createErrorResponse(id interface{}, code int, message string) ([]byte, error) {
	response := JSONRPCError{
		JSONRPC: "2.0",
		ID:      id,
		Error: struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
		},
	}
	return json.Marshal(response)
}

// ValidationError represents an error during validation
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ContentValidationOptions contains configurable validation rules
type ContentValidationOptions struct {
	MaxTextSize      int64    // Maximum size for text content in bytes
	MaxBlobSize      int64    // Maximum size for blob content in bytes
	AllowedMIMETypes []string // List of allowed MIME types (empty means all allowed)
}

// DefaultValidationOptions provides sensible defaults
var DefaultValidationOptions = ContentValidationOptions{
	MaxTextSize: 1024 * 1024 * 10, // 10MB
	MaxBlobSize: 1024 * 1024 * 50, // 50MB
	AllowedMIMETypes: []string{
		"text/plain",
		"text/markdown",
		"text/html",
		"application/json",
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
	},
}

// validateTextResourceContent validates a text resource
func validateTextResourceContent(content TextResourceContents, opts ContentValidationOptions) error {
	// Validate URI
	if err := validateURI(content.URI); err != nil {
		return &ValidationError{Field: "uri", Message: err.Error()}
	}

	// Check text content
	if content.Text == "" {
		return &ValidationError{Field: "text", Message: "text content cannot be empty"}
	}

	// Check text size
	if int64(len(content.Text)) > opts.MaxTextSize {
		return &ValidationError{
			Field:   "text",
			Message: fmt.Sprintf("text content exceeds maximum size of %d bytes", opts.MaxTextSize),
		}
	}

	// Validate MIME type if provided
	if content.MimeType != "" {
		if err := validateMIMEType(content.MimeType, opts.AllowedMIMETypes); err != nil {
			return &ValidationError{Field: "mimeType", Message: err.Error()}
		}
	}

	return nil
}

// validateBlobResourceContent validates a blob resource
func validateBlobResourceContent(content BlobResourceContents, opts ContentValidationOptions) error {
	// Validate URI
	if err := validateURI(content.URI); err != nil {
		return &ValidationError{Field: "uri", Message: err.Error()}
	}

	// Check blob content
	if len(content.Blob) == 0 {
		return &ValidationError{Field: "blob", Message: "blob content cannot be empty"}
	}

	// Check blob size
	if int64(len(content.Blob)) > opts.MaxBlobSize {
		return &ValidationError{
			Field:   "blob",
			Message: fmt.Sprintf("blob content exceeds maximum size of %d bytes", opts.MaxBlobSize),
		}
	}

	// MIME type is required for blobs
	if content.MimeType == "" {
		return &ValidationError{Field: "mimeType", Message: "MIME type is required for blob content"}
	}

	// Validate MIME type
	if err := validateMIMEType(content.MimeType, opts.AllowedMIMETypes); err != nil {
		return &ValidationError{Field: "mimeType", Message: err.Error()}
	}

	return nil
}

// validateMIMEType validates a MIME type
func validateMIMEType(mimeType string, allowedTypes []string) error {
	// Parse MIME type
	mediatype, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return fmt.Errorf("invalid MIME type format: %w", err)
	}

	// If no allowed types specified, accept all valid MIME types
	if len(allowedTypes) == 0 {
		return nil
	}

	// Check against allowed types
	for _, allowed := range allowedTypes {
		if mediatype == allowed {
			return nil
		}
	}

	return fmt.Errorf("MIME type %q is not allowed", mimeType)
}

// validateURI validates a URI string
func validateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI cannot be empty")
	}

	// Parse the URI
	parsed, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid URI format: %w", err)
	}

	// Require scheme
	if parsed.Scheme == "" {
		return fmt.Errorf("URI must have a scheme")
	}

	// Additional scheme-specific validation could be added here
	switch parsed.Scheme {
	case "file":
		if parsed.Host != "" && parsed.Host != "localhost" {
			return fmt.Errorf("file URI must not have a host or must be localhost")
		}
	case "http", "https":
		if parsed.Host == "" {
			return fmt.Errorf("HTTP(S) URI must have a host")
		}
	}

	return nil
}

// generateSubscriptionID generates a unique subscription ID
func generateSubscriptionID() string {
	// Generate 16 random bytes
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("sub_%d", time.Now().UnixNano())
	}

	// Convert to hex string
	return fmt.Sprintf("sub_%s", hex.EncodeToString(bytes))
}
