package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/corsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inspectorOrigin is the origin MCP Inspector v2 serves itself from.
const inspectorOrigin = "http://127.0.0.1:6274"

func corsTestHandler() (http.Handler, *bool) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	return toolCORSMiddleware(next), &reached
}

func TestToolCORSMiddleware_AnswersPreflightAheadOfAuth(t *testing.T) {
	t.Setenv(corsutil.EnvAllowedOrigins, "")

	for _, path := range []string{
		"/tools/currency-exchange-rates",
		"/tools/currency-exchange-rates/mcp",
		"/tools/currency-exchange-rates/mcp/sse",
		"/tools/currency-exchange-rates/mcp/message",
	} {
		t.Run(path, func(t *testing.T) {
			handler, reached := corsTestHandler()

			req := httptest.NewRequest(http.MethodOptions, path, nil)
			req.Header.Set("Origin", inspectorOrigin)
			req.Header.Set("Access-Control-Request-Method", "POST")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// The preflight carries no credentials by design, so it has to be
			// answered before the credential middleware turns it into a 401.
			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.False(t, *reached, "the preflight must not reach the authenticated handler")
			assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
			assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
		})
	}
}

func TestToolCORSMiddleware_AllowsMCPTransportHeaders(t *testing.T) {
	t.Setenv(corsutil.EnvAllowedOrigins, "")
	handler, _ := corsTestHandler()

	req := httptest.NewRequest(http.MethodOptions, "/tools/x/mcp", nil)
	req.Header.Set("Origin", inspectorOrigin)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	allow := w.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"Authorization", "Content-Type", "Mcp-Session-Id", "MCP-Protocol-Version", "Last-Event-ID"} {
		assert.Truef(t, strings.Contains(allow, header), "Access-Control-Allow-Headers must include %s, got %q", header, allow)
	}

	// The session id is minted on the initialize response; a browser client
	// cannot read it back unless it is exposed.
	expose := w.Header().Get("Access-Control-Expose-Headers")
	assert.Contains(t, expose, "Mcp-Session-Id")
	assert.Contains(t, expose, "WWW-Authenticate")
}

func TestToolCORSMiddleware_TagsActualRequests(t *testing.T) {
	t.Setenv(corsutil.EnvAllowedOrigins, "")
	handler, reached := corsTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/tools/x/mcp", strings.NewReader("{}"))
	req.Header.Set("Origin", inspectorOrigin)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.True(t, *reached, "a real request must still reach the authenticated handler")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"),
		"without this the browser discards the response even though the call succeeded")
}

func TestToolCORSMiddleware_HonoursAllowlist(t *testing.T) {
	t.Setenv(corsutil.EnvAllowedOrigins, inspectorOrigin)

	t.Run("allowlisted origin is echoed", func(t *testing.T) {
		handler, _ := corsTestHandler()
		req := httptest.NewRequest(http.MethodOptions, "/tools/x/mcp", nil)
		req.Header.Set("Origin", inspectorOrigin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, inspectorOrigin, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	})

	t.Run("other origins get no allow-origin header", func(t *testing.T) {
		handler, _ := corsTestHandler()
		req := httptest.NewRequest(http.MethodOptions, "/tools/x/mcp", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestToolCORSMiddleware_LeavesOtherPathsAlone(t *testing.T) {
	t.Setenv(corsutil.EnvAllowedOrigins, "")

	for _, path := range []string{"/llm/rest/openai/chat/completions", "/datasource/kb", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			handler, reached := corsTestHandler()
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Origin", inspectorOrigin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.True(t, *reached)
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}
