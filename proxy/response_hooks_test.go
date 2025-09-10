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

// TestResponseCaptureBuffering tests the new buffering behavior
func TestResponseCaptureBuffering(t *testing.T) {
	rec := httptest.NewRecorder()
	capture := newResponseCapture(rec)

	// Write response but don't flush yet
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"test": true}`))

	// Response should not be written to client yet (captured data available)
	// Note: httptest.ResponseRecorder may write status immediately, but real response writer won't
	// The key test is that capture.written = false and data is available for modification
	
	// Captured data should be available
	assert.Equal(t, http.StatusOK, capture.statusCode)
	assert.Equal(t, `{"test": true}`, string(capture.CapturedBody()))
	assert.Equal(t, "application/json", capture.header.Get("Content-Type"))
	assert.False(t, capture.written) // Should not be marked as written yet

	// Now flush to client
	capture.Flush()
	assert.True(t, capture.written) // Should now be marked as written
	
	// Response should be available in recorder (httptest behavior)
	assert.Equal(t, `{"test": true}`, string(capture.CapturedBody()))
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

// TestResponseCaptureModification tests the modification methods
func TestResponseCaptureModification(t *testing.T) {
	rec := httptest.NewRecorder()
	capture := newResponseCapture(rec)

	// Set initial response
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"original": true}`))

	// Modify headers
	newHeaders := map[string]string{
		"Content-Type":   "application/json",
		"X-Hook-Applied": "true",
	}
	capture.ModifyHeaders(newHeaders)

	// Modify body
	capture.ModifyBody([]byte(`{"modified": true}`))

	// Modify status
	capture.ModifyStatusCode(http.StatusCreated)

	// Flush and verify
	capture.Flush()

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, `{"modified": true}`, rec.Body.String())
	assert.Equal(t, "true", rec.Header().Get("X-Hook-Applied"))
}

// TestResponseCaptureDoubleFlush tests that double flush is safe
func TestResponseCaptureDoubleFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	capture := newResponseCapture(rec)

	capture.Header().Set("Content-Type", "text/plain")
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte("test"))

	// First flush
	capture.Flush()
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())

	// Modifications after flush should be ignored
	capture.ModifyBody([]byte("modified"))
	capture.ModifyStatusCode(http.StatusBadRequest)

	// Second flush should be safe and not change anything
	assert.NotPanics(t, func() {
		capture.Flush()
	})
	
	// Response should remain unchanged
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test", rec.Body.String())
}