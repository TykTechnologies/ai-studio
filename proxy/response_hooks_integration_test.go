package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRESTOnlyResponseHookIntegration tests complete REST hook integration
func TestRESTOnlyResponseHookIntegration(t *testing.T) {
	// Create a test hook that adds headers and modifies body
	hook := &IntegrationTestHook{}
	
	// Create proxy with hook manager
	manager := NewDefaultResponseHookManager()
	manager.AddHook(hook)

	// Create mock LLM and app
	llm := &models.LLM{
		ID:     1,
		Name:   "test-llm",
		Vendor: models.OPENAI,
	}
	app := &models.App{
		ID:     1,
		UserID: 1,
		Name:   "test-app",
	}

	// Test the complete flow with buffered response
	rec := httptest.NewRecorder()
	bufferedCapture := newBufferedResponseCapture(rec)

	// Simulate what reverse proxy does - write response to buffered capture
	bufferedCapture.Header().Set("Content-Type", "application/json")
	bufferedCapture.WriteHeader(http.StatusOK)
	bufferedCapture.Write([]byte(`{"message": "hello"}`))

	// Execute hooks (simulating what executeBufferedResponseHooks does)
	ctx := context.Background()
	pluginCtx := &PluginContext{
		RequestID: "test-req-123",
		LLMSlug:   llm.Name,
		LLMID:     llm.ID,
		AppID:     app.ID,
		UserID:    app.UserID,
		Metadata:  make(map[string]string),
	}

	// Test header hooks
	headerReq := &HeadersRequest{
		Headers: map[string]string{"Content-Type": "application/json"},
		Context: pluginCtx,
	}
	
	headerResp, err := manager.ExecuteOnBeforeWriteHeaders(ctx, headerReq)
	require.NoError(t, err)
	
	if headerResp.Modified {
		bufferedCapture.ModifyHeaders(headerResp.Headers)
	}

	// Test body hooks
	writeReq := &ResponseWriteRequest{
		Body:    bufferedCapture.CapturedBody(),
		Headers: headerResp.Headers,
		Context: pluginCtx,
	}
	
	writeResp, err := manager.ExecuteOnBeforeWrite(ctx, writeReq)
	require.NoError(t, err)
	
	if writeResp.Modified {
		bufferedCapture.ModifyBody(writeResp.Body)
		bufferedCapture.ModifyHeaders(writeResp.Headers)
	}

	// Write the modified response to client
	bufferedCapture.WriteToClient()

	// Verify the hook modifications were applied
	assert.True(t, headerResp.Modified)
	assert.True(t, writeResp.Modified)
	
	// Check the final response includes hook modifications
	finalBody := bufferedCapture.CapturedBody()
	var response map[string]interface{}
	err = json.Unmarshal(finalBody, &response)
	require.NoError(t, err)
	
	assert.Equal(t, "hello", response["message"])
	assert.True(t, response["hook_modified"].(bool))
	assert.Equal(t, "integration-test", response["modified_by"])
}

// TestResponseHookChain tests multiple hooks in sequence
func TestResponseHookChain(t *testing.T) {
	manager := NewDefaultResponseHookManager()
	
	// Add multiple hooks
	manager.AddHook(&IntegrationTestHook{})
	manager.AddHook(&SecondTestHook{})

	ctx := context.Background()
	pluginCtx := &PluginContext{RequestID: "chain-test"}

	// Test header hook chain
	headerReq := &HeadersRequest{
		Headers: map[string]string{"Content-Type": "application/json"},
		Context: pluginCtx,
	}
	
	headerResp, err := manager.ExecuteOnBeforeWriteHeaders(ctx, headerReq)
	require.NoError(t, err)
	
	// Both hooks should have modified headers
	assert.True(t, headerResp.Modified)
	assert.Equal(t, "integration-test", headerResp.Headers["X-Hook-1"])
	assert.Equal(t, "second-test", headerResp.Headers["X-Hook-2"])

	// Test body hook chain
	writeReq := &ResponseWriteRequest{
		Body:    []byte(`{"test": true}`),
		Headers: headerResp.Headers,
		Context: pluginCtx,
	}
	
	writeResp, err := manager.ExecuteOnBeforeWrite(ctx, writeReq)
	require.NoError(t, err)
	
	// Body should be modified by both hooks
	assert.True(t, writeResp.Modified)
	bodyStr := string(writeResp.Body)
	assert.Contains(t, bodyStr, "hook_modified")
	assert.Contains(t, bodyStr, "second_modified")
}

// Test hooks
type IntegrationTestHook struct{}

func (h *IntegrationTestHook) GetName() string {
	return "integration-test-hook"
}

func (h *IntegrationTestHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	modifiedHeaders := make(map[string]string)
	for k, v := range req.Headers {
		modifiedHeaders[k] = v
	}
	modifiedHeaders["X-Hook-1"] = "integration-test"
	modifiedHeaders["X-Request-ID"] = req.Context.RequestID
	
	return &HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

func (h *IntegrationTestHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	var response map[string]interface{}
	if err := json.Unmarshal(req.Body, &response); err != nil {
		// If not JSON, just append text
		modifiedBody := append(req.Body, []byte(" [HOOK-MODIFIED]")...)
		return &ResponseWriteResponse{
			Modified: true,
			Body:     modifiedBody,
			Headers:  req.Headers,
		}, nil
	}

	// Modify JSON response
	response["hook_modified"] = true
	response["modified_by"] = "integration-test"
	response["request_id"] = req.Context.RequestID

	modifiedBody, err := json.Marshal(response)
	if err != nil {
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

func (h *IntegrationTestHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}

// SecondTestHook for testing hook chaining
type SecondTestHook struct{}

func (h *SecondTestHook) GetName() string {
	return "second-test-hook"
}

func (h *SecondTestHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	modifiedHeaders := make(map[string]string)
	for k, v := range req.Headers {
		modifiedHeaders[k] = v
	}
	modifiedHeaders["X-Hook-2"] = "second-test"
	
	return &HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

func (h *SecondTestHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	var response map[string]interface{}
	if err := json.Unmarshal(req.Body, &response); err != nil {
		return &ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}

	// Add second modification
	response["second_modified"] = true

	modifiedBody, err := json.Marshal(response)
	if err != nil {
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

func (h *SecondTestHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}