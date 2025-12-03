package proxy

import (
	"context"
	"encoding/json"
)

// ExampleResponseHook demonstrates how to create a custom response hook
type ExampleResponseHook struct {
	name string
}

// NewExampleResponseHook creates a new example response hook
func NewExampleResponseHook(name string) *ExampleResponseHook {
	return &ExampleResponseHook{name: name}
}

// GetName returns the hook name
func (h *ExampleResponseHook) GetName() string {
	return h.name
}

// OnBeforeWriteHeaders adds custom headers to responses
func (h *ExampleResponseHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	// Copy original headers
	modifiedHeaders := make(map[string]string)
	for key, value := range req.Headers {
		modifiedHeaders[key] = value
	}
	
	// Add custom headers
	modifiedHeaders["X-Gateway-Version"] = "v2.0"
	modifiedHeaders["X-Hook-Applied"] = h.name
	modifiedHeaders["X-Request-ID"] = req.Context.RequestID
	
	return &HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

// OnBeforeWrite modifies response body content
func (h *ExampleResponseHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	// Try to parse as JSON and add metadata
	var response map[string]interface{}
	if err := json.Unmarshal(req.Body, &response); err != nil {
		// If not JSON, just return unchanged
		return &ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}
	
	// Add metadata to JSON response
	if response["metadata"] == nil {
		response["metadata"] = make(map[string]interface{})
	}
	
	if metadata, ok := response["metadata"].(map[string]interface{}); ok {
		metadata["processed_by"] = h.name
		metadata["request_id"] = req.Context.RequestID
		metadata["llm_slug"] = req.Context.LLMSlug
		metadata["app_id"] = req.Context.AppID
	}
	
	// Marshal back to JSON
	modifiedBody, err := json.Marshal(response)
	if err != nil {
		// If marshaling fails, return original
		return &ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}
	
	return &ResponseWriteResponse{
		Modified: true,
		Body:     modifiedBody,
		Headers:  req.Headers,
	}, nil
}

// OnStreamComplete is called after a streaming response finishes
func (h *ExampleResponseHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	// Example hook doesn't need to do anything special for stream completion
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}

// CORSResponseHook adds CORS headers to responses
type CORSResponseHook struct{}

func NewCORSResponseHook() *CORSResponseHook {
	return &CORSResponseHook{}
}

func (h *CORSResponseHook) GetName() string {
	return "cors-hook"
}

func (h *CORSResponseHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	modifiedHeaders := make(map[string]string)
	for key, value := range req.Headers {
		modifiedHeaders[key] = value
	}
	
	// Add CORS headers
	modifiedHeaders["Access-Control-Allow-Origin"] = "*"
	modifiedHeaders["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, OPTIONS"
	modifiedHeaders["Access-Control-Allow-Headers"] = "Origin, Content-Type, Accept, Authorization"
	
	return &HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

func (h *CORSResponseHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	// CORS hook only modifies headers, not body
	return &ResponseWriteResponse{
		Modified: false,
		Body:     req.Body,
		Headers:  req.Headers,
	}, nil
}

// OnStreamComplete is called after a streaming response finishes
func (h *CORSResponseHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	// CORS hook doesn't need to do anything special for stream completion
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}

// ContentFilterHook demonstrates content filtering
type ContentFilterHook struct {
	blockedWords []string
}

func NewContentFilterHook(blockedWords []string) *ContentFilterHook {
	return &ContentFilterHook{blockedWords: blockedWords}
}

func (h *ContentFilterHook) GetName() string {
	return "content-filter-hook"
}

func (h *ContentFilterHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	// Content filter doesn't modify headers
	return &HeadersResponse{
		Modified: false,
		Headers:  req.Headers,
	}, nil
}

func (h *ContentFilterHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	bodyStr := string(req.Body)
	modified := false
	
	// Filter out blocked words
	for _, word := range h.blockedWords {
		if contains := containsWord(bodyStr, word); contains {
			bodyStr = replaceWord(bodyStr, word, "[FILTERED]")
			modified = true
		}
	}
	
	if !modified {
		return &ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}
	
	return &ResponseWriteResponse{
		Modified: true,
		Body:     []byte(bodyStr),
		Headers:  req.Headers,
	}, nil
}

// OnStreamComplete is called after a streaming response finishes
func (h *ContentFilterHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	// Content filter doesn't need to do anything special for stream completion
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}

// Helper functions for content filtering
func containsWord(text, word string) bool {
	// Simple contains check - in production would use proper word boundary matching
	return len(word) > 0 && len(text) > 0 && 
		   (text == word || 
		    json.Valid([]byte(text))) // Basic check for JSON content
}

func replaceWord(text, word, replacement string) string {
	// Simple replacement - in production would use regex with word boundaries
	if containsWord(text, word) {
		// For JSON content, try to parse and replace in values
		var jsonObj map[string]interface{}
		if err := json.Unmarshal([]byte(text), &jsonObj); err == nil {
			replaceInJSONValue(jsonObj, word, replacement)
			if modified, err := json.Marshal(jsonObj); err == nil {
				return string(modified)
			}
		}
	}
	return text
}

func replaceInJSONValue(obj interface{}, word, replacement string) {
	switch v := obj.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if str, ok := value.(string); ok && containsWord(str, word) {
				v[key] = replacement
			} else {
				replaceInJSONValue(value, word, replacement)
			}
		}
	case []interface{}:
		for i, item := range v {
			if str, ok := item.(string); ok && containsWord(str, word) {
				v[i] = replacement
			} else {
				replaceInJSONValue(item, word, replacement)
			}
		}
	}
}