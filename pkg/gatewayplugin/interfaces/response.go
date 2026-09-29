// plugins/interfaces/response.go
package interfaces

import "context"

// Legacy ResponseData for internal use (kept for compatibility with existing code)
type ResponseData struct {
	RequestID   string            `json:"request_id"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body"`
	Context     *PluginContext    `json:"context"`
	LatencyMs   int64             `json:"latency_ms"`
}

// New clean response hook types
type HeadersRequest struct {
	Headers map[string]string `json:"headers"`
	Context *PluginContext    `json:"context"`
}

type HeadersResponse struct {
	Modified bool              `json:"modified"`
	Headers  map[string]string `json:"headers"`
}

type ResponseWriteRequest struct {
	Body         []byte            `json:"body"`
	Headers      map[string]string `json:"headers"`
	IsStreamChunk bool             `json:"is_stream_chunk"`
	Context      *PluginContext    `json:"context"`
}

type ResponseWriteResponse struct {
	Modified bool              `json:"modified"`
	Body     []byte            `json:"body"`
	Headers  map[string]string `json:"headers"`
}

// ResponsePlugin defines the interface for response processing plugins (new clean interface)
// These plugins execute before responses are sent to clients and can modify:
// - Response headers (fast path via OnBeforeWriteHeaders)
// - Response body and headers (full path via OnBeforeWrite)
type ResponsePlugin interface {
	BasePlugin
	
	// OnBeforeWriteHeaders is called before response headers are written (fast path)
	// Use this for header-only modifications without processing the body
	OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest, pluginCtx *PluginContext) (*HeadersResponse, error)
	
	// OnBeforeWrite is called before response body is written (full path)
	// Use this for body modifications or combined header+body modifications
	// isStreamChunk indicates if this is a streaming chunk (true) or complete response (false)
	OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest, pluginCtx *PluginContext) (*ResponseWriteResponse, error)
}