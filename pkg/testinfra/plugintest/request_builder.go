package plugintest

import (
	"encoding/json"
	"strconv"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// Message represents a chat message for building requests.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestBuilder builds EnrichedRequest proto messages for testing.
type RequestBuilder struct {
	body          []byte
	headers       map[string]string
	path          string
	method        string
	vendor        string
	model         string
	userID        string
	appID         string
	authClaims    map[string]string
	authenticated bool

	// Context fields for PluginContext
	requestID    string
	llmID        uint32
	llmSlug      string
	userIDInt    uint32
	appIDInt     uint32
	metadata     map[string]string
	traceContext map[string]string
}

// NewRequestBuilder creates a new request builder.
func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		headers:       make(map[string]string),
		authClaims:    make(map[string]string),
		metadata:      make(map[string]string),
		traceContext:  make(map[string]string),
		method:        "POST",
		path:          "/v1/chat/completions",
		authenticated: true,
		requestID:     "test-request-id",
	}
}

// WithChatCompletion sets the request body to a chat completion request.
func (b *RequestBuilder) WithChatCompletion(messages []Message) *RequestBuilder {
	body := map[string]interface{}{
		"messages": messages,
	}
	if b.model != "" {
		body["model"] = b.model
	}
	b.body, _ = json.Marshal(body)
	return b
}

// WithModel sets the model for the request.
func (b *RequestBuilder) WithModel(model string) *RequestBuilder {
	b.model = model

	// Also update body if already set
	if len(b.body) > 0 {
		var body map[string]interface{}
		if err := json.Unmarshal(b.body, &body); err == nil {
			body["model"] = model
			b.body, _ = json.Marshal(body)
		}
	}
	return b
}

// WithVendor sets the vendor for the request.
func (b *RequestBuilder) WithVendor(vendor string) *RequestBuilder {
	b.vendor = vendor
	return b
}

// WithHeader adds a header to the request.
func (b *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	b.headers[key] = value
	return b
}

// WithUserID sets the user ID.
func (b *RequestBuilder) WithUserID(userID string) *RequestBuilder {
	b.userID = userID
	return b
}

// WithAppID sets the app ID.
func (b *RequestBuilder) WithAppID(appID string) *RequestBuilder {
	b.appID = appID
	return b
}

// WithUserIDInt sets the user ID from an integer.
func (b *RequestBuilder) WithUserIDInt(userID uint32) *RequestBuilder {
	b.userID = strconv.FormatUint(uint64(userID), 10)
	b.userIDInt = userID
	return b
}

// WithAppIDInt sets the app ID from an integer.
func (b *RequestBuilder) WithAppIDInt(appID uint32) *RequestBuilder {
	b.appID = strconv.FormatUint(uint64(appID), 10)
	b.appIDInt = appID
	return b
}

// WithRequestID sets the request ID for the plugin context.
func (b *RequestBuilder) WithRequestID(requestID string) *RequestBuilder {
	b.requestID = requestID
	return b
}

// WithLLMID sets the LLM ID for the plugin context.
func (b *RequestBuilder) WithLLMID(llmID uint32) *RequestBuilder {
	b.llmID = llmID
	return b
}

// WithLLMSlug sets the LLM slug for the plugin context.
func (b *RequestBuilder) WithLLMSlug(llmSlug string) *RequestBuilder {
	b.llmSlug = llmSlug
	return b
}

// WithMetadata adds metadata to the plugin context.
func (b *RequestBuilder) WithMetadata(key, value string) *RequestBuilder {
	b.metadata[key] = value
	return b
}

// WithTraceContext adds trace context to the plugin context.
func (b *RequestBuilder) WithTraceContext(key, value string) *RequestBuilder {
	b.traceContext[key] = value
	return b
}

// WithAuthClaim adds an auth claim.
func (b *RequestBuilder) WithAuthClaim(key, value string) *RequestBuilder {
	b.authClaims[key] = value
	return b
}

// WithStreamingEnabled enables streaming in the request.
func (b *RequestBuilder) WithStreamingEnabled() *RequestBuilder {
	if len(b.body) > 0 {
		var body map[string]interface{}
		if err := json.Unmarshal(b.body, &body); err == nil {
			body["stream"] = true
			b.body, _ = json.Marshal(body)
		}
	}
	return b
}

// WithPath sets the request path.
func (b *RequestBuilder) WithPath(path string) *RequestBuilder {
	b.path = path
	return b
}

// WithMethod sets the HTTP method.
func (b *RequestBuilder) WithMethod(method string) *RequestBuilder {
	b.method = method
	return b
}

// WithBody sets the raw request body.
func (b *RequestBuilder) WithBody(body []byte) *RequestBuilder {
	b.body = body
	return b
}

// WithJSONBody sets the request body from a Go value.
func (b *RequestBuilder) WithJSONBody(v interface{}) *RequestBuilder {
	b.body, _ = json.Marshal(v)
	return b
}

// Authenticated sets whether the request is authenticated.
func (b *RequestBuilder) Authenticated(authenticated bool) *RequestBuilder {
	b.authenticated = authenticated
	return b
}

// Build creates the EnrichedRequest proto message.
func (b *RequestBuilder) Build() *pb.EnrichedRequest {
	return &pb.EnrichedRequest{
		Request: &pb.PluginRequest{
			Body:    b.body,
			Headers: b.headers,
			Path:    b.path,
			Method:  b.method,
			Context: &pb.PluginContext{
				RequestId:    b.requestID,
				Vendor:       b.vendor,
				LlmId:        b.llmID,
				LlmSlug:      b.llmSlug,
				AppId:        b.appIDInt,
				UserId:       b.userIDInt,
				Metadata:     b.metadata,
				TraceContext: b.traceContext,
			},
		},
		UserId:        b.userID,
		AppId:         b.appID,
		AuthClaims:    b.authClaims,
		Authenticated: b.authenticated,
	}
}

// ResponseBuilder builds ResponseWriteRequest proto messages for testing.
type ResponseBuilder struct {
	body          []byte
	headers       map[string]string
	isStreamChunk bool

	// Context fields for PluginContext
	requestID    string
	vendor       string
	llmID        uint32
	llmSlug      string
	appID        uint32
	userID       uint32
	metadata     map[string]string
	traceContext map[string]string
}

// NewResponseBuilder creates a new response builder.
func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{
		headers:      make(map[string]string),
		metadata:     make(map[string]string),
		traceContext: make(map[string]string),
		requestID:    "test-request-id",
	}
}

// WithChatCompletion sets the response body to a chat completion response.
func (r *ResponseBuilder) WithChatCompletion(content string) *ResponseBuilder {
	body := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"index":         0,
				"finish_reason": "stop",
			},
		},
	}
	r.body, _ = json.Marshal(body)
	r.headers["Content-Type"] = "application/json"
	return r
}

// WithBody sets the raw response body.
func (r *ResponseBuilder) WithBody(body []byte) *ResponseBuilder {
	r.body = body
	return r
}

// WithJSONBody sets the response body from a Go value.
func (r *ResponseBuilder) WithJSONBody(v interface{}) *ResponseBuilder {
	r.body, _ = json.Marshal(v)
	r.headers["Content-Type"] = "application/json"
	return r
}

// WithHeader adds a header to the response.
func (r *ResponseBuilder) WithHeader(key, value string) *ResponseBuilder {
	r.headers[key] = value
	return r
}

// WithStreamChunk marks this as a streaming chunk.
func (r *ResponseBuilder) WithStreamChunk(isChunk bool) *ResponseBuilder {
	r.isStreamChunk = isChunk
	return r
}

// WithRequestID sets the request ID for the plugin context.
func (r *ResponseBuilder) WithRequestID(requestID string) *ResponseBuilder {
	r.requestID = requestID
	return r
}

// WithVendor sets the vendor for the plugin context.
func (r *ResponseBuilder) WithVendor(vendor string) *ResponseBuilder {
	r.vendor = vendor
	return r
}

// WithLLMID sets the LLM ID for the plugin context.
func (r *ResponseBuilder) WithLLMID(llmID uint32) *ResponseBuilder {
	r.llmID = llmID
	return r
}

// WithLLMSlug sets the LLM slug for the plugin context.
func (r *ResponseBuilder) WithLLMSlug(llmSlug string) *ResponseBuilder {
	r.llmSlug = llmSlug
	return r
}

// WithAppID sets the app ID for the plugin context.
func (r *ResponseBuilder) WithAppID(appID uint32) *ResponseBuilder {
	r.appID = appID
	return r
}

// WithUserID sets the user ID for the plugin context.
func (r *ResponseBuilder) WithUserID(userID uint32) *ResponseBuilder {
	r.userID = userID
	return r
}

// Build creates the ResponseWriteRequest proto message.
func (r *ResponseBuilder) Build() *pb.ResponseWriteRequest {
	return &pb.ResponseWriteRequest{
		Body:          r.body,
		Headers:       r.headers,
		IsStreamChunk: r.isStreamChunk,
		Context: &pb.PluginContext{
			RequestId:    r.requestID,
			Vendor:       r.vendor,
			LlmId:        r.llmID,
			LlmSlug:      r.llmSlug,
			AppId:        r.appID,
			UserId:       r.userID,
			Metadata:     r.metadata,
			TraceContext: r.traceContext,
		},
	}
}

// StreamCompleteBuilder builds StreamCompleteRequest proto messages for testing.
type StreamCompleteBuilder struct {
	accumulatedResponse []byte
	headers             map[string]string
	statusCode          int32
	chunkCount          int32
	requestBody         []byte

	// Context fields for PluginContext
	requestID    string
	vendor       string
	llmID        uint32
	llmSlug      string
	appID        uint32
	userID       uint32
	metadata     map[string]string
	traceContext map[string]string
}

// NewStreamCompleteBuilder creates a new stream complete builder.
func NewStreamCompleteBuilder() *StreamCompleteBuilder {
	return &StreamCompleteBuilder{
		headers:      make(map[string]string),
		metadata:     make(map[string]string),
		traceContext: make(map[string]string),
		requestID:    "test-request-id",
		statusCode:   200,
		chunkCount:   1,
	}
}

// WithAccumulatedResponse sets the accumulated SSE response body.
func (s *StreamCompleteBuilder) WithAccumulatedResponse(data []byte) *StreamCompleteBuilder {
	s.accumulatedResponse = data
	return s
}

// WithStatusCode sets the HTTP status code.
func (s *StreamCompleteBuilder) WithStatusCode(code int32) *StreamCompleteBuilder {
	s.statusCode = code
	return s
}

// WithChunkCount sets the number of chunks received.
func (s *StreamCompleteBuilder) WithChunkCount(count int32) *StreamCompleteBuilder {
	s.chunkCount = count
	return s
}

// WithRequestBody sets the original request body.
func (s *StreamCompleteBuilder) WithRequestBody(body []byte) *StreamCompleteBuilder {
	s.requestBody = body
	return s
}

// WithHeader adds a header to the response.
func (s *StreamCompleteBuilder) WithHeader(key, value string) *StreamCompleteBuilder {
	s.headers[key] = value
	return s
}

// WithRequestID sets the request ID for the plugin context.
func (s *StreamCompleteBuilder) WithRequestID(requestID string) *StreamCompleteBuilder {
	s.requestID = requestID
	return s
}

// WithVendor sets the vendor for the plugin context.
func (s *StreamCompleteBuilder) WithVendor(vendor string) *StreamCompleteBuilder {
	s.vendor = vendor
	return s
}

// WithLLMID sets the LLM ID for the plugin context.
func (s *StreamCompleteBuilder) WithLLMID(llmID uint32) *StreamCompleteBuilder {
	s.llmID = llmID
	return s
}

// WithLLMSlug sets the LLM slug for the plugin context.
func (s *StreamCompleteBuilder) WithLLMSlug(llmSlug string) *StreamCompleteBuilder {
	s.llmSlug = llmSlug
	return s
}

// WithAppID sets the app ID for the plugin context.
func (s *StreamCompleteBuilder) WithAppID(appID uint32) *StreamCompleteBuilder {
	s.appID = appID
	return s
}

// WithUserID sets the user ID for the plugin context.
func (s *StreamCompleteBuilder) WithUserID(userID uint32) *StreamCompleteBuilder {
	s.userID = userID
	return s
}

// Build creates the StreamCompleteRequest proto message.
func (s *StreamCompleteBuilder) Build() *pb.StreamCompleteRequest {
	return &pb.StreamCompleteRequest{
		AccumulatedResponse: s.accumulatedResponse,
		Headers:             s.headers,
		StatusCode:          s.statusCode,
		ChunkCount:          s.chunkCount,
		RequestBody:         s.requestBody,
		Context: &pb.PluginContext{
			RequestId:    s.requestID,
			Vendor:       s.vendor,
			LlmId:        s.llmID,
			LlmSlug:      s.llmSlug,
			AppId:        s.appID,
			UserId:       s.userID,
			Metadata:     s.metadata,
			TraceContext: s.traceContext,
		},
	}
}

// ContextBuilder builds test context configurations.
type ContextBuilder struct {
	requestID    string
	appID        uint32
	userID       uint32
	llmID        uint32
	llmSlug      string
	vendor       string
	metadata     map[string]string
	traceContext map[string]string
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{
		requestID:    "test-request-id",
		metadata:     make(map[string]string),
		traceContext: make(map[string]string),
	}
}

// WithRequestID sets the request ID.
func (c *ContextBuilder) WithRequestID(id string) *ContextBuilder {
	c.requestID = id
	return c
}

// WithAppID sets the app ID.
func (c *ContextBuilder) WithAppID(id uint32) *ContextBuilder {
	c.appID = id
	return c
}

// WithUserID sets the user ID.
func (c *ContextBuilder) WithUserID(id uint32) *ContextBuilder {
	c.userID = id
	return c
}

// WithLLMID sets the LLM ID.
func (c *ContextBuilder) WithLLMID(id uint32) *ContextBuilder {
	c.llmID = id
	return c
}

// WithLLMSlug sets the LLM slug.
func (c *ContextBuilder) WithLLMSlug(slug string) *ContextBuilder {
	c.llmSlug = slug
	return c
}

// WithVendor sets the vendor.
func (c *ContextBuilder) WithVendor(vendor string) *ContextBuilder {
	c.vendor = vendor
	return c
}

// WithMetadata adds metadata.
func (c *ContextBuilder) WithMetadata(key, value string) *ContextBuilder {
	c.metadata[key] = value
	return c
}

// WithTraceContext adds trace context.
func (c *ContextBuilder) WithTraceContext(key, value string) *ContextBuilder {
	c.traceContext[key] = value
	return c
}

// BuildConfig returns the context configuration as a map.
// This can be passed to plugin initialization.
func (c *ContextBuilder) BuildConfig() map[string]string {
	config := map[string]string{
		"request_id": c.requestID,
		"app_id":     strconv.FormatUint(uint64(c.appID), 10),
		"user_id":    strconv.FormatUint(uint64(c.userID), 10),
		"llm_id":     strconv.FormatUint(uint64(c.llmID), 10),
		"llm_slug":   c.llmSlug,
		"vendor":     c.vendor,
	}

	for k, v := range c.metadata {
		config["meta_"+k] = v
	}

	return config
}
