package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// mountGatewayRoutes mirrors the `if config.Gateway != nil` block in SetupRouter so the
// gateway route table can be exercised without building the full router/config (same
// approach as registerTestMetricsRoute in metrics_endpoint_test.go). Keep in sync with
// SetupRouter.
func mountGatewayRoutes(router *gin.Engine, h http.Handler) {
	gateway := router.Group("/")
	gateway.Any("/.well-known/*path", gin.WrapH(h))
	gateway.Any("/llm/*path", gin.WrapH(h))
	gateway.Any("/tools/*path", gin.WrapH(h))
	gateway.Any("/datasource/*path", gin.WrapH(h))
	gateway.Any("/ai/*path", gin.WrapH(h))
	gateway.Any("/anthropic/*path", gin.WrapH(h))
	gateway.Any("/v1/*path", gin.WrapH(h))
}

// TestGatewayRoutes_ForwardAnthropicBridge verifies the microgateway forwards the
// Anthropic Messages bridge path prefix to the gateway handler (which owns routing,
// auth, and translation), alongside the existing /ai and /llm prefixes.
func TestGatewayRoutes_ForwardAnthropicBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	router := gin.New()
	mountGatewayRoutes(router, stub)

	cases := []struct {
		name        string
		path        string
		wantStatus  int
		wantForward bool
	}{
		{"anthropic bridge", "/anthropic/aws-bedrock/v1/messages", http.StatusOK, true},
		{"openai-compatible", "/ai/aws-bedrock/v1/chat/completions", http.StatusOK, true},
		{"llm passthrough", "/llm/call/aws-bedrock/v1/messages", http.StatusOK, true},
		{"unified router chat", "/v1/chat/completions", http.StatusOK, true},
		{"unified router completions", "/v1/completions", http.StatusOK, true},
		{"unmounted path", "/nope/x", http.StatusNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPath = ""
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, nil))

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.wantForward && gotPath != tc.path {
				t.Errorf("expected forward to handler with path %q, got %q", tc.path, gotPath)
			}
			if !tc.wantForward && gotPath != "" {
				t.Errorf("did not expect forward, but handler saw %q", gotPath)
			}
		})
	}
}

// TestGatewayRoutes_ServeOAuthProtectedResourceMetadata covers the discovery
// document an unauthenticated MCP request advertises in its WWW-Authenticate
// header. Before it was mounted, the edge pointed clients at a URL that gin
// answered with a 404, so a browser client could complete no handshake.
func TestGatewayRoutes_ServeOAuthProtectedResourceMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	router := gin.New()
	mountGatewayRoutes(router, stub)

	cases := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantForward bool
	}{
		{"metadata", http.MethodGet, "/.well-known/oauth-protected-resource", http.StatusOK, true},
		{"metadata preflight", http.MethodOptions, "/.well-known/oauth-protected-resource", http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPath = ""
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.wantForward && gotPath != tc.path {
				t.Errorf("expected forward to handler with path %q, got %q", tc.path, gotPath)
			}
		})
	}
}
