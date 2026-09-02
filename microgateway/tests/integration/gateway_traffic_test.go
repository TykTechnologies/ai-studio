// tests/integration/gateway_traffic_test.go
//
// End-to-end LLM traffic through the assembled microgateway: real config, real
// service container, real server listening on a TCP port (the /ai/ shim's
// internal /llm/call/ hop is a real HTTP call to 127.0.0.1:<port>, so a
// recorder is not enough), and a mockllm OpenAI-compatible upstream.
//
// Covered paths:
//
//	client -> /ai/{slug}/v1/chat/completions -> auth -> shim -> /llm/call/ -> mock upstream
//	client -> /v1/chat/completions (unified router) -> rewrite -> same chain
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/mockllm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const trafficTestToken = "e2e-traffic-test-token"

// setupTrafficTest boots the full microgateway on a free localhost port with a
// mock OpenAI upstream and one seeded LLM ("mock-openai") + app + API token.
// It returns the gateway base URL and the mock backend for request assertions.
func setupTrafficTest(t *testing.T) (string, *mockllm.MockLLMBackend) {
	t.Helper()
	return setupTrafficTestWithGateway(t, config.GatewayConfig{})
}

// setupTrafficTestWithGateway is setupTrafficTest with explicit gateway settings,
// so tests can boot the assembled microgateway with the unified router moved or
// switched off and verify the real mounted route table.
func setupTrafficTestWithGateway(t *testing.T, gwCfg config.GatewayConfig) (string, *mockllm.MockLLMBackend) {
	t.Helper()

	// Shared-cache named in-memory DB: the server handles requests concurrently,
	// so every pooled connection must see the same database.
	dsn := fmt.Sprintf("file:gwtraffic_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, mgwdb.Migrate(db))

	// Mock OpenAI-compatible upstream
	backend := mockllm.NewMockLLMBackend()
	t.Cleanup(backend.Close)

	// Reserve a free port for the gateway; the /ai/ shim dials 127.0.0.1:<port>
	// for its internal /llm/call/ hop, so the server must really listen there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         port,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: config.DatabaseConfig{Type: "sqlite"},
		Cache:    config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
			JWTSecret:     "test-secret-key",
		},
		Analytics: config.AnalyticsConfig{
			Enabled:       true,
			BufferSize:    1,
			FlushInterval: 100 * time.Millisecond,
		},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
		Gateway:       gwCfg,
	}

	serviceContainer, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)

	// Seed resources BEFORE server.New: the gateway loads its route table once
	// during construction (gateway.Reload()).
	apiKeyEnc, err := serviceContainer.Crypto.Encrypt("mock-upstream-key")
	require.NoError(t, err)

	llm := &mgwdb.LLM{
		Name:            "Mock OpenAI",
		Slug:            "mock-openai",
		Vendor:          "openai",
		Endpoint:        backend.URL,
		APIKeyEncrypted: apiKeyEnc,
		DefaultModel:    "gpt-4",
		IsActive:        true,
	}
	require.NoError(t, db.Create(llm).Error)

	// A second active LLM the test app is deliberately NOT associated with:
	// it must never appear in the app's /v1/models listing.
	hiddenLLM := &mgwdb.LLM{
		Name:         "Hidden LLM",
		Slug:         "hidden-llm",
		Vendor:       "openai",
		Endpoint:     backend.URL,
		DefaultModel: "gpt-secret",
		IsActive:     true,
	}
	require.NoError(t, db.Create(hiddenLLM).Error)

	app := &mgwdb.App{Name: "Traffic Test App", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error)

	require.NoError(t, db.Create(&mgwdb.APIToken{
		Token:    trafficTestToken,
		Name:     "traffic-test",
		AppID:    app.ID,
		IsActive: true,
	}).Error)

	srv, err := server.New(cfg, serviceContainer, "test", "test", "test")
	require.NoError(t, err)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			t.Logf("server exited: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealthy(t, baseURL)
	return baseURL, backend
}

func waitForHealthy(t *testing.T, baseURL string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "gateway never became healthy")
}

func gatewayPost(t *testing.T, url, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody
}

func gatewayGet(t *testing.T, url, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody
}

func TestGatewayTraffic_EndToEnd(t *testing.T) {
	baseURL, backend := setupTrafficTest(t)

	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	t.Run("per-route shim: /ai/{slug}/v1/chat/completions", func(t *testing.T) {
		status, body := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
		require.Equal(t, http.StatusOK, status, "body: %s", body)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &resp))
		choices, ok := resp["choices"].([]interface{})
		require.True(t, ok, "response missing choices: %s", body)
		require.NotEmpty(t, choices)

		assert.GreaterOrEqual(t, backend.GetRequestCount(), int64(1), "mock upstream never received the request")
	})

	t.Run("unified router: /v1/chat/completions with vendor/model", func(t *testing.T) {
		before := backend.GetRequestCount()

		status, body := gatewayPost(t, baseURL+"/v1/chat/completions", trafficTestToken,
			`{"model":"mock-openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
		require.Equal(t, http.StatusOK, status, "body: %s", body)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.NotEmpty(t, resp["choices"], "response missing choices: %s", body)

		require.Greater(t, backend.GetRequestCount(), before, "mock upstream never received the routed request")

		// The route prefix must be stripped before the request leaves the gateway.
		reqs := backend.GetRequests()
		last := reqs[len(reqs)-1]
		var upstreamReq map[string]interface{}
		require.NoError(t, json.Unmarshal(last.Body, &upstreamReq))
		assert.Equal(t, "gpt-4", upstreamReq["model"], "upstream saw a prefixed model name")
	})

	t.Run("unified router: prefixless model is rejected with format guidance", func(t *testing.T) {
		status, body := gatewayPost(t, baseURL+"/v1/chat/completions", trafficTestToken, chatBody)
		assert.Equal(t, http.StatusBadRequest, status)
		// Decode first: the JSON encoder escapes '<' as < in the raw bytes.
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(body, &errResp))
		assert.Contains(t, errResp.Error.Message, "<vendor>/<model>")
	})

	t.Run("unified router: unknown vendor 404s", func(t *testing.T) {
		status, body := gatewayPost(t, baseURL+"/v1/chat/completions", trafficTestToken,
			`{"model":"no-such-route/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Contains(t, string(body), "not found or not supported by your access rights")
	})

	t.Run("invalid token is rejected", func(t *testing.T) {
		before := backend.GetRequestCount()
		status, _ := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", "wrong-token", chatBody)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, before, backend.GetRequestCount(), "unauthenticated request must not reach the upstream")
	})

	t.Run("missing token is rejected on the unified router", func(t *testing.T) {
		before := backend.GetRequestCount()
		status, _ := gatewayPost(t, baseURL+"/v1/chat/completions", "",
			`{"model":"mock-openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, before, backend.GetRequestCount(), "unauthenticated request must not reach the upstream")
	})

	t.Run("GET /v1/models lists only the app's accessible routes", func(t *testing.T) {
		status, body := gatewayGet(t, baseURL+"/v1/models", trafficTestToken)
		require.Equal(t, http.StatusOK, status, "body: %s", body)

		var resp struct {
			Object string `json:"object"`
			Data   []struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				OwnedBy string `json:"owned_by"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		assert.Equal(t, "list", resp.Object)

		ids := make([]string, 0, len(resp.Data))
		for _, m := range resp.Data {
			ids = append(ids, m.ID)
		}
		assert.Contains(t, ids, "mock-openai/gpt-4")
		// The scoping requirement: an active LLM the app is not associated with
		// must be invisible.
		for _, id := range ids {
			assert.NotContains(t, id, "hidden-llm", "model list leaked a route the app cannot access")
			assert.NotContains(t, id, "gpt-secret", "model list leaked a model the app cannot access")
		}
	})

	t.Run("GET /v1/models without a token is rejected", func(t *testing.T) {
		status, body := gatewayGet(t, baseURL+"/v1/models", "")
		assert.NotEqual(t, http.StatusOK, status)
		assert.NotContains(t, string(body), "mock-openai")
	})
}

// TestGatewayTraffic_UnifiedRouterConfigurable drives real LLM traffic through a
// relocated unified endpoint, and confirms the default prefix is left free — the
// property an embedding host gateway (e.g. Tyk's API Gateway) depends on.
func TestGatewayTraffic_UnifiedRouterConfigurable(t *testing.T) {
	baseURL, backend := setupTrafficTestWithGateway(t, config.GatewayConfig{
		UnifiedRouterPath: "/ai-gateway/v1",
	})

	t.Run("traffic flows through the configured path", func(t *testing.T) {
		before := backend.GetRequestCount()

		status, body := gatewayPost(t, baseURL+"/ai-gateway/v1/chat/completions", trafficTestToken,
			`{"model":"mock-openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
		require.Equal(t, http.StatusOK, status, "body: %s", body)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.NotEmpty(t, resp["choices"], "response missing choices: %s", body)
		require.Greater(t, backend.GetRequestCount(), before, "mock upstream never received the routed request")

		// The vendor prefix is still stripped before the request leaves the gateway.
		reqs := backend.GetRequests()
		last := reqs[len(reqs)-1]
		var upstreamReq map[string]interface{}
		require.NoError(t, json.Unmarshal(last.Body, &upstreamReq))
		assert.Equal(t, "gpt-4", upstreamReq["model"], "upstream saw a prefixed model name")
	})

	t.Run("default path is free for the host gateway", func(t *testing.T) {
		status, _ := gatewayPost(t, baseURL+"/v1/chat/completions", trafficTestToken,
			`{"model":"mock-openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
		require.Equal(t, http.StatusNotFound, status, "the relocated ingress must not still answer on /v1")
	})
}

// TestGatewayTraffic_UnifiedRouterDisabled confirms the endpoint can be removed
// entirely without disturbing the per-route endpoints.
func TestGatewayTraffic_UnifiedRouterDisabled(t *testing.T) {
	baseURL, _ := setupTrafficTestWithGateway(t, config.GatewayConfig{
		UnifiedRouterDisabled: true,
	})

	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	status, _ := gatewayPost(t, baseURL+"/v1/chat/completions", trafficTestToken,
		`{"model":"mock-openai/gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	require.Equal(t, http.StatusNotFound, status)

	status, body := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
	require.Equal(t, http.StatusOK, status, "per-route endpoint broke with the unified ingress off: %s", body)
}
