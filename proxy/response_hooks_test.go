package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponseHook is a simple test hook
type TestResponseHook struct {
	name string
}

func (h *TestResponseHook) GetName() string {
	return h.name
}

func (h *TestResponseHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	// Add a test header
	modifiedHeaders := make(map[string]string)
	for k, v := range req.Headers {
		modifiedHeaders[k] = v
	}
	modifiedHeaders["X-Test-Hook"] = "header-modified"
	
	return &HeadersResponse{
		Modified: true,
		Headers:  modifiedHeaders,
	}, nil
}

func (h *TestResponseHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	// Modify the response body
	modifiedBody := append(req.Body, []byte(" [MODIFIED]")...)

	return &ResponseWriteResponse{
		Modified: true,
		Body:     modifiedBody,
		Headers:  req.Headers,
	}, nil
}

func (h *TestResponseHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{
		Handled: false,
		Cached:  false,
	}, nil
}

// TestBufferedResponseCapture tests the buffered response capture for hooks
func TestBufferedResponseCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	bufferedCapture := newBufferedResponseCapture(rec)

	// Write response but don't write to client yet
	bufferedCapture.Header().Set("Content-Type", "application/json")
	bufferedCapture.WriteHeader(http.StatusOK)
	bufferedCapture.Write([]byte(`{"test": true}`))

	// Note: httptest.ResponseRecorder may record status immediately, but the key is that
	// buffered capture doesn't write to real client until WriteToClient() is called
	// The important test is that data is available for modification
	
	// Captured data should be available for hooks
	assert.Equal(t, http.StatusOK, bufferedCapture.statusCode)
	assert.Equal(t, `{"test": true}`, string(bufferedCapture.CapturedBody()))
	assert.Equal(t, "application/json", bufferedCapture.header.Get("Content-Type"))
	assert.False(t, bufferedCapture.written) // Should not be marked as written yet

	// Now write to client
	bufferedCapture.WriteToClient()
	assert.True(t, bufferedCapture.written) // Should now be marked as written
	
	// Response should now be available in recorder
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `{"test": true}`, rec.Body.String())
}

// TestResponseHookManager tests the hook manager functionality
func TestResponseHookManager(t *testing.T) {
	manager := NewDefaultResponseHookManager()
	hook := &TestResponseHook{name: "test-hook"}
	manager.AddHook(hook)

	// Test header hooks
	headerReq := &HeadersRequest{
		Headers: map[string]string{"Content-Type": "application/json"},
		Context: &PluginContext{RequestID: "test-123"},
	}
	
	headerResp, err := manager.ExecuteOnBeforeWriteHeaders(context.Background(), headerReq)
	require.NoError(t, err)
	assert.True(t, headerResp.Modified)
	assert.Equal(t, "header-modified", headerResp.Headers["X-Test-Hook"])
	assert.Equal(t, "application/json", headerResp.Headers["Content-Type"])

	// Test body hooks
	writeReq := &ResponseWriteRequest{
		Body:    []byte(`{"original": true}`),
		Headers: headerResp.Headers,
		Context: &PluginContext{RequestID: "test-123"},
	}
	
	writeResp, err := manager.ExecuteOnBeforeWrite(context.Background(), writeReq)
	require.NoError(t, err)
	assert.True(t, writeResp.Modified)
	assert.Equal(t, `{"original": true} [MODIFIED]`, string(writeResp.Body))
}

// TestBufferedResponseModification tests the modification methods
func TestBufferedResponseModification(t *testing.T) {
	rec := httptest.NewRecorder()
	bufferedCapture := newBufferedResponseCapture(rec)

	// Set initial response
	bufferedCapture.Header().Set("Content-Type", "application/json")
	bufferedCapture.WriteHeader(http.StatusOK)
	bufferedCapture.Write([]byte(`{"original": true}`))

	// Modify headers
	newHeaders := map[string]string{
		"Content-Type":   "application/json",
		"X-Hook-Applied": "true",
	}
	bufferedCapture.ModifyHeaders(newHeaders)

	// Modify body
	bufferedCapture.ModifyBody([]byte(`{"modified": true}`))

	// Modify status
	bufferedCapture.ModifyStatusCode(http.StatusCreated)

	// Write to client and verify
	bufferedCapture.WriteToClient()

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, `{"modified": true}`, rec.Body.String())
	assert.Equal(t, "true", rec.Header().Get("X-Hook-Applied"))
}

// TestBufferedResponseDoubleWrite tests that double write is safe
func TestBufferedResponseDoubleWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	bufferedCapture := newBufferedResponseCapture(rec)

	bufferedCapture.Header().Set("Content-Type", "text/plain")
	bufferedCapture.WriteHeader(http.StatusOK)
	bufferedCapture.Write([]byte("test"))

	// First write to client
	bufferedCapture.WriteToClient()
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())

	// Modifications after write should be ignored
	bufferedCapture.ModifyBody([]byte("modified"))
	bufferedCapture.ModifyStatusCode(http.StatusBadRequest)

	// Second write should be safe and not change anything
	assert.NotPanics(t, func() {
		bufferedCapture.WriteToClient()
	})
	
	// Response should remain unchanged
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())
}