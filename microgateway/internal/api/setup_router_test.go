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
type stubGateway struct {
	gotPath string
}

func (s *stubGateway) Start() error                               { return nil }
func (s *stubGateway) Stop(ctx context.Context) error             { return nil }
func (s *stubGateway) Reload() error                              { return nil }
func (s *stubGateway) AddResponseHook(proxy.ResponseHook)         {}
func (s *stubGateway) SetAuthHooks(*proxy.AuthHooks)              {}
func (s *stubGateway) SetPostAuthCallback(proxy.PostAuthCallback) {}
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

	stub := &stubGateway{}
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
