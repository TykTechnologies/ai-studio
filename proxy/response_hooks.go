package proxy

import (
	"context"
	"log"
)

// ResponseHookManager defines the interface for managing response hooks
type ResponseHookManager interface {
	// ExecuteOnBeforeWriteHeaders is called before response headers are written
	ExecuteOnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error)

	// ExecuteOnBeforeWrite is called before response body is written (REST-only)
	ExecuteOnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error)

	// ExecuteOnStreamComplete is called after a streaming response finishes (streaming-only)
	ExecuteOnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error)
}

// HeadersRequest represents a request to modify response headers
type HeadersRequest struct {
	Headers map[string]string `json:"headers"`
	Context *PluginContext    `json:"context"`
}

// HeadersResponse represents the response from header modification hooks
type HeadersResponse struct {
	Modified bool              `json:"modified"`
	Headers  map[string]string `json:"headers"`
}

// ResponseWriteRequest represents a request to modify response body (REST-only)
type ResponseWriteRequest struct {
	Body    []byte            `json:"body"`
	Headers map[string]string `json:"headers"`
	Context *PluginContext    `json:"context"`
	// Note: IsStreamChunk is always false for REST-only implementation
}

// ResponseWriteResponse represents the response from body modification hooks
type ResponseWriteResponse struct {
	Modified bool              `json:"modified"`
	Body     []byte            `json:"body"`
	Headers  map[string]string `json:"headers"`
}

// StreamCompleteRequest represents a request to process a completed streaming response
type StreamCompleteRequest struct {
	AccumulatedResponse []byte            `json:"accumulated_response"` // Full SSE response (all chunks concatenated)
	Headers             map[string]string `json:"headers"`              // Response headers from upstream
	StatusCode          int               `json:"status_code"`          // HTTP status code
	Context             *PluginContext    `json:"context"`              // Plugin context with request metadata
	ChunkCount          int               `json:"chunk_count"`          // Number of chunks received
	RequestBody         []byte            `json:"request_body"`         // Original request body (for cache key generation)
}

// StreamCompleteResponse represents the response from stream complete hooks
type StreamCompleteResponse struct {
	Handled      bool   `json:"handled"`       // Plugin processed the response
	Cached       bool   `json:"cached"`        // Response was cached (for metrics/logging)
	ErrorMessage string `json:"error_message"` // Error description if any
}

// PluginContext provides context information for hooks
type PluginContext struct {
	RequestID string            `json:"request_id"`
	LLMSlug   string            `json:"llm_slug"`
	LLMID     uint              `json:"llm_id"`
	AppID     uint              `json:"app_id"`
	UserID    uint              `json:"user_id"`
	Metadata  map[string]string `json:"metadata"`
}

// ResponseHook defines the interface that response hook implementations must satisfy
type ResponseHook interface {
	// OnBeforeWriteHeaders is called before response headers are written
	OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error)

	// OnBeforeWrite is called before response body is written (REST-only)
	OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error)

	// OnStreamComplete is called after a streaming response finishes (streaming-only)
	OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error)

	// GetName returns the name of this response hook
	GetName() string
}

// DefaultResponseHookManager provides a basic implementation of ResponseHookManager
type DefaultResponseHookManager struct {
	hooks []ResponseHook
}

// HasHooks returns true if there are any hooks configured
func (m *DefaultResponseHookManager) HasHooks() bool {
	return len(m.hooks) > 0
}

// NewDefaultResponseHookManager creates a new default response hook manager
func NewDefaultResponseHookManager() *DefaultResponseHookManager {
	return &DefaultResponseHookManager{
		hooks: make([]ResponseHook, 0),
	}
}

// AddHook adds a response hook to the manager
func (m *DefaultResponseHookManager) AddHook(hook ResponseHook) {
	m.hooks = append(m.hooks, hook)
}

// ExecuteOnBeforeWriteHeaders executes all registered header hooks
func (m *DefaultResponseHookManager) ExecuteOnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	// Start with original headers
	result := &HeadersResponse{
		Modified: false,
		Headers:  make(map[string]string),
	}
	
	// Copy original headers
	for key, value := range req.Headers {
		result.Headers[key] = value
	}
	
	// Execute each hook in sequence
	for _, hook := range m.hooks {
		hookReq := &HeadersRequest{
			Headers: result.Headers,
			Context: req.Context,
		}
		
		hookResp, err := hook.OnBeforeWriteHeaders(ctx, hookReq)
		if err != nil {
			// Log error but continue with other hooks
			log.Printf("Response header hook %s failed: %v", hook.GetName(), err)
			continue
		}
		
		if hookResp.Modified {
			result.Modified = true
			result.Headers = hookResp.Headers
		}
	}
	
	return result, nil
}

// ExecuteOnBeforeWrite executes all registered body hooks
func (m *DefaultResponseHookManager) ExecuteOnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	// Start with original response
	result := &ResponseWriteResponse{
		Modified: false,
		Body:     make([]byte, len(req.Body)),
		Headers:  make(map[string]string),
	}
	
	// Copy original body and headers
	copy(result.Body, req.Body)
	for key, value := range req.Headers {
		result.Headers[key] = value
	}
	
	// Execute each hook in sequence
	for _, hook := range m.hooks {
		hookReq := &ResponseWriteRequest{
			Body:    result.Body,
			Headers: result.Headers,
			Context: req.Context,
		}
		
		hookResp, err := hook.OnBeforeWrite(ctx, hookReq)
		if err != nil {
			// Log error but continue with other hooks
			log.Printf("Response body hook %s failed: %v", hook.GetName(), err)
			continue
		}
		
		if hookResp.Modified {
			result.Modified = true
			result.Body = hookResp.Body
			result.Headers = hookResp.Headers
		}
	}

	return result, nil
}

// ExecuteOnStreamComplete executes all registered stream complete hooks
func (m *DefaultResponseHookManager) ExecuteOnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	result := &StreamCompleteResponse{
		Handled: false,
	}

	// Execute each hook in sequence
	for _, hook := range m.hooks {
		hookResp, err := hook.OnStreamComplete(ctx, req)
		if err != nil {
			// Log error but continue with other hooks
			log.Printf("Stream complete hook %s failed: %v", hook.GetName(), err)
			continue
		}

		if hookResp.Handled {
			result.Handled = true
		}
		if hookResp.Cached {
			result.Cached = true
		}
		if hookResp.ErrorMessage != "" && result.ErrorMessage == "" {
			result.ErrorMessage = hookResp.ErrorMessage
		}
	}

	return result, nil
}