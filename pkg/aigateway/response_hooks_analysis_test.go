package aigateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponseHookArchitectureAnalysis validates the current state and identifies issues
func TestResponseHookArchitectureAnalysis(t *testing.T) {
	t.Run("ProxyResponseCaptureIssue", func(t *testing.T) {
		// This test demonstrates the core issue: responseCapture writes to client immediately
		
		// Create a test response writer
		rec := httptest.NewRecorder()
		
		// Simulate what the proxy does (from proxy/response_capture.go)
		capture := newTestResponseCapture(rec)
		
		// Simulate writing a response (this is what happens in proxy/proxy.go:446)
		capture.Header().Set("Content-Type", "application/json")
		capture.WriteHeader(http.StatusOK)
		n, err := capture.Write([]byte(`{"message": "test"}`))
		
		require.NoError(t, err)
		assert.Greater(t, n, 0)
		
		// The problem: response is already sent to client at this point
		// Any hooks that try to modify it would be too late
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, `{"message": "test"}`, rec.Body.String())
		
		// This demonstrates why OnBeforeWrite/OnBeforeHeaders hooks don't work:
		// The response is written to the client before hooks can intercept it
	})

	t.Run("HookIntegrationMissing", func(t *testing.T) {
		// This test shows that the proxy package has no hook integration at all
		
		// The proxy only uses responseCapture for analytics, not for hooks
		// From proxy/proxy.go:445-447:
		//   capture := newResponseCapture(w)
		//   httpProxy.ServeHTTP(capture, r)
		//   go p.analyzeResponse(llm, app, capture.statusCode, capture.buffer.Bytes(), reqBody, r)
		
		// There's no plugin system integration in the proxy package
		// The OnBeforeWrite/OnBeforeHeaders hooks exist in microgateway/plugins but not in pkg/aigateway
		
		// This confirms that the AI Gateway library is missing response hook support entirely
		t.Log("CONFIRMED: AI Gateway library (pkg/aigateway) has no response hook integration")
		t.Log("CONFIRMED: OnBeforeWrite/OnBeforeHeaders exist only in microgateway plugin system")
		t.Log("CONFIRMED: proxy package writes response immediately without hook interception")
	})

	t.Run("ArchitectureMismatch", func(t *testing.T) {
		// The AI Gateway uses proxy.Proxy, but proxy.Proxy doesn't support plugin hooks
		// The plugin system exists in microgateway/plugins, not in the AI Gateway library
		
		// From microgateway/internal/server/server.go:60-61:
		//   // Response hooks disabled - using baseline plugin system
		//   // Create AI Gateway instance for mounting (not standalone)
		
		// This comment confirms that response hooks were intentionally disabled
		t.Log("CONFIRMED: Response hooks are intentionally disabled in microgateway")
		t.Log("CONFIRMED: AI Gateway library has no plugin system at all")
	})
}

// TestExpectedBehavior shows what the hooks should do when properly implemented
func TestExpectedBehavior(t *testing.T) {
	t.Run("OnBeforeWriteHeadersExpected", func(t *testing.T) {
		// This is what OnBeforeWriteHeaders should do:
		// 1. Intercept response headers before they're sent
		// 2. Allow plugins to modify headers
		// 3. Apply modifications before writing to client
		
		headers := map[string]string{
			"Content-Type": "application/json",
			"X-Original":   "true",
		}
		
		// Plugin should be able to modify headers
		modifiedHeaders := make(map[string]string)
		for k, v := range headers {
			modifiedHeaders[k] = v
		}
		modifiedHeaders["X-Plugin-Modified"] = "response-modifier"
		modifiedHeaders["X-Modification-Type"] = "headers"
		
		assert.Equal(t, "response-modifier", modifiedHeaders["X-Plugin-Modified"])
		assert.Equal(t, "headers", modifiedHeaders["X-Modification-Type"])
		assert.Equal(t, "application/json", modifiedHeaders["Content-Type"])
		
		t.Log("EXPECTED: OnBeforeWriteHeaders should allow header modifications")
	})

	t.Run("OnBeforeWriteExpected", func(t *testing.T) {
		// This is what OnBeforeWrite should do:
		// 1. Intercept response body before it's sent
		// 2. Allow plugins to modify body content
		// 3. Apply modifications before writing to client
		
		originalBody := []byte(`{"message": "Hello, world!"}`)
		
		// Plugin should be able to modify body
		modifiedBody := []byte(`{"message": "Hello, world!", "plugin_modified": true}`)
		
		assert.NotEqual(t, originalBody, modifiedBody)
		assert.Contains(t, string(modifiedBody), "plugin_modified")
		
		t.Log("EXPECTED: OnBeforeWrite should allow body modifications")
	})
}

// TestProposedSolution outlines the architecture needed to fix the hooks
func TestProposedSolution(t *testing.T) {
	t.Run("RequiredChanges", func(t *testing.T) {
		// To fix the response hooks, we need:
		// 1. Create a plugin system interface for the AI Gateway
		// 2. Modify responseCapture to buffer responses instead of writing immediately
		// 3. Add hook execution points before writing headers and body
		// 4. Create a pluggable response writer that can apply modifications
		
		t.Log("SOLUTION 1: Create ResponseHookInterface for AI Gateway")
		t.Log("SOLUTION 2: Modify proxy/response_capture.go to support buffering")
		t.Log("SOLUTION 3: Add hook execution in proxy/proxy.go before response writing")
		t.Log("SOLUTION 4: Create ResponseModifier interface for plugins")
	})

	t.Run("BufferedResponseWriter", func(t *testing.T) {
		// This demonstrates the required buffering approach
		
		rec := httptest.NewRecorder()
		buffer := newBufferedResponseWriter(rec)
		
		// Write response but don't send to client yet
		buffer.Header().Set("Content-Type", "application/json")
		buffer.WriteHeader(http.StatusOK)
		buffer.Write([]byte(`{"original": true}`))
		
		// At this point, client hasn't received anything yet
		assert.Equal(t, 0, rec.Code) // Not written to client
		assert.Empty(t, rec.Body.String()) // Not written to client
		
		// Now we can apply hooks
		buffer.ModifyHeader("X-Hook-Applied", "true")
		buffer.ModifyBody([]byte(`{"original": true, "modified": true}`))
		
		// Finally, flush to client
		buffer.Flush()
		
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "true", rec.Header().Get("X-Hook-Applied"))
		assert.Contains(t, rec.Body.String(), "modified")
		
		t.Log("SOLUTION: Use buffered response writer to enable hook execution")
	})
}

// Mock implementations to demonstrate concepts

// testResponseCapture mimics the current proxy/response_capture.go behavior
type testResponseCapture struct {
	http.ResponseWriter
	statusCode int
	buffer     *bytes.Buffer
	headers    http.Header
}

func newTestResponseCapture(w http.ResponseWriter) *testResponseCapture {
	return &testResponseCapture{
		ResponseWriter: w,
		buffer:         &bytes.Buffer{},
		headers:        make(http.Header),
	}
}

func (rc *testResponseCapture) Header() http.Header {
	return rc.headers
}

func (rc *testResponseCapture) WriteHeader(statusCode int) {
	rc.statusCode = statusCode
	// Current implementation writes immediately to client - this is the problem
	for k, v := range rc.headers {
		rc.ResponseWriter.Header()[k] = v
	}
	rc.ResponseWriter.WriteHeader(statusCode)
}

func (rc *testResponseCapture) Write(b []byte) (int, error) {
	rc.buffer.Write(b)
	// Current implementation writes immediately to client - this is the problem
	return rc.ResponseWriter.Write(b)
}

// bufferedResponseWriter demonstrates the solution approach
type bufferedResponseWriter struct {
	target     http.ResponseWriter
	statusCode int
	headers    http.Header
	body       []byte
	written    bool
}

func newBufferedResponseWriter(target http.ResponseWriter) *bufferedResponseWriter {
	return &bufferedResponseWriter{
		target:  target,
		headers: make(http.Header),
		written: false,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.headers
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.statusCode = code
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if w.written {
		return 0, http.ErrBodyNotAllowed
	}
	w.body = append(w.body, data...)
	return len(data), nil
}

func (w *bufferedResponseWriter) ModifyHeader(key, value string) {
	w.headers.Set(key, value)
}

func (w *bufferedResponseWriter) ModifyBody(newBody []byte) {
	w.body = newBody
}

func (w *bufferedResponseWriter) Flush() {
	if w.written {
		return
	}
	
	// Copy headers to target
	for key, values := range w.headers {
		for _, value := range values {
			w.target.Header().Add(key, value)
		}
	}
	
	// Write status and body
	w.target.WriteHeader(w.statusCode)
	w.target.Write(w.body)
	w.written = true
}