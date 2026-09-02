package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/gin-gonic/gin"
)

// stubGateway is a minimal aigateway.Gateway that records the paths its handler
// receives, so route-table wiring can be verified through the REAL SetupRouter.
// unifiedBasePath is what the gateway reports as its unified-router ingress
// prefix ("" meaning the ingress is disabled), which is what SetupRouter mounts.
type stubGateway struct {
	gotPath         string
	unifiedBasePath string
}

func (s *stubGateway) Start() error                               { return nil }
func (s *stubGateway) Stop(ctx context.Context) error             { return nil }
func (s *stubGateway) Reload() error                              { return nil }
func (s *stubGateway) AddResponseHook(proxy.ResponseHook)         {}
func (s *stubGateway) SetAuthHooks(*proxy.AuthHooks)              {}
func (s *stubGateway) SetPostAuthCallback(proxy.PostAuthCallback) {}
func (s *stubGateway) UnifiedRouterBasePath() string              { return s.unifiedBasePath }
func (s *stubGateway) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
}

// TestSetupRouter_UnifiedRouterRoute runs the real SetupRouter — including the
// Model Router registration that shares the route tree — to prove that the fixed
// /v1/ unified-router prefix registers without gin conflicts, forwards to the
// gateway handler, and does not shadow the management API under /api/v1.
func TestSetupRouter_UnifiedRouterRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &stubGateway{unifiedBasePath: "/v1"}
	router := SetupRouter(&RouterConfig{
		Gateway: stub,
		// Non-nil so the Enterprise Model Router routes (/router/{slug}/...) are
		// registered alongside /v1/ — this is the conflict the feature must avoid.
		ModelRouterService: &services.ModelRouterService{},
	})

	t.Run("v1 chat completions forwards to gateway handler", func(t *testing.T) {
		stub.gotPath = ""
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		router.ServeHTTP(w, req)

		if stub.gotPath != "/v1/chat/completions" {
			t.Fatalf("expected forward to gateway handler with /v1/chat/completions, got %q (status %d)", stub.gotPath, w.Code)
		}
	})

	t.Run("v1 completions forwards to gateway handler", func(t *testing.T) {
		stub.gotPath = ""
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{}`)))

		if stub.gotPath != "/v1/completions" {
			t.Fatalf("expected forward to gateway handler with /v1/completions, got %q", stub.gotPath)
		}
	})

	t.Run("model router keeps its own /router/ dispatch", func(t *testing.T) {
		// A /router/{slug}/ request must be handled by the Model Router handler
		// (here: its own 404, since no routers are loaded), never fall through to
		// the gateway handler where the unified /v1/ router lives.
		stub.gotPath = ""
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/router/my-router/v1/chat/completions",
			strings.NewReader(`{"model":"gpt-4o"}`)))

		if stub.gotPath != "" {
			t.Fatalf("model router request leaked to gateway handler: %q", stub.gotPath)
		}
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "Router not found") {
			t.Fatalf("expected Model Router's own 'Router not found' response, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("management API under /api/v1 is not forwarded to the gateway", func(t *testing.T) {
		stub.gotPath = ""
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/llms", nil))

		if stub.gotPath != "" {
			t.Fatalf("management API request leaked to gateway handler: %q", stub.gotPath)
		}
		if w.Code == http.StatusNotFound {
			t.Fatalf("management API route lost: got 404 for /api/v1/llms")
		}
	})
}

// TestSetupRouter_UnifiedRouterPathIsConfigurable proves the data plane mounts
// whatever prefix the gateway reports rather than a hardcoded /v1, so an embedder
// can move the single endpoint off a path the host already owns.
func TestSetupRouter_UnifiedRouterPathIsConfigurable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &stubGateway{unifiedBasePath: "/ai-gateway/v1"}
	router := SetupRouter(&RouterConfig{
		Gateway:            stub,
		ModelRouterService: &services.ModelRouterService{},
	})

	t.Run("configured prefix forwards to gateway handler", func(t *testing.T) {
		stub.gotPath = ""
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ai-gateway/v1/chat/completions",
			strings.NewReader(`{}`)))

		if stub.gotPath != "/ai-gateway/v1/chat/completions" {
			t.Fatalf("expected forward with /ai-gateway/v1/chat/completions, got %q (status %d)", stub.gotPath, w.Code)
		}
	})

	t.Run("default prefix is left to the host", func(t *testing.T) {
		stub.gotPath = ""
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))

		if stub.gotPath != "" {
			t.Fatalf("/v1 must not be mounted when the ingress moved, but reached the gateway handler: %q", stub.gotPath)
		}
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for the unmounted default prefix, got %d", w.Code)
		}
	})
}

// TestSetupRouter_UnifiedRouterDisabled proves that a gateway reporting no ingress
// path causes nothing to be mounted, leaving the prefix free for the host gateway.
func TestSetupRouter_UnifiedRouterDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &stubGateway{unifiedBasePath: ""}
	router := SetupRouter(&RouterConfig{
		Gateway:            stub,
		ModelRouterService: &services.ModelRouterService{},
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))

	if stub.gotPath != "" {
		t.Fatalf("disabled ingress still forwarded to the gateway handler: %q", stub.gotPath)
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with the ingress disabled, got %d", w.Code)
	}

	// Per-route endpoints keep working with the unified ingress off.
	stub.gotPath = ""
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ai/openai/v1/chat/completions", strings.NewReader(`{}`)))
	if stub.gotPath != "/ai/openai/v1/chat/completions" {
		t.Fatalf("per-route endpoint stopped forwarding: %q (status %d)", stub.gotPath, w.Code)
	}
}
