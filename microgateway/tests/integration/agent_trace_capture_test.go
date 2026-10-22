// tests/integration/agent_trace_capture_test.go
//
// Agent tracing capture through the assembled microgateway (pkg/aigateway):
// with capture on, LLM calls on /llm/rest/ and /ai/ produce llm_inference
// events that reach the analytics pulse's capture outbox and leave in
// capture pulses to the hub (decision D3).
package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	mgwplugins "github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/mockllm"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// pulseHub stands in for the hub's pulse endpoint and keeps the capture
// events it received.
type pulseHub struct {
	pb.ConfigurationSyncServiceClient
	mu     sync.Mutex
	events []*agenttrace.Event
}

func (h *pulseHub) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range pulse.CaptureEvents {
		h.events = append(h.events, agenttrace.FromProto(p))
	}
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func (h *pulseHub) llmEvents() []*agenttrace.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*agenttrace.Event
	for _, e := range h.events {
		if e.Kind == agenttrace.KindLLMInference {
			out = append(out, e)
		}
	}
	return out
}

type pulseEdge struct{ hub *pulseHub }

func (e pulseEdge) GetGRPCClient() pb.ConfigurationSyncServiceClient { return e.hub }
func (e pulseEdge) GetEdgeID() string                                { return "edge-trace" }
func (e pulseEdge) GetEdgeNamespace() string                         { return "" }

// setupCaptureGateway boots the microgateway as setupTrafficTest does, with
// the analytics pulse loaded and agent tracing capture on (what the
// Enterprise edge's start gate does once it passes).
func setupCaptureGateway(t *testing.T, appTracing ...string) (string, *pulseHub, uint) {
	t.Helper()
	dsn := fmt.Sprintf("file:gwcapture_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, mgwdb.Migrate(db))

	backend := mockllm.NewMockLLMBackend()
	t.Cleanup(backend.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: port, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second},
		Database: config.DatabaseConfig{Type: "sqlite"},
		Cache:    config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security: config.SecurityConfig{EncryptionKey: "12345678901234567890123456789012", JWTSecret: "test-secret-key"},
		Analytics: config.AnalyticsConfig{
			Enabled: true, BufferSize: 1, FlushInterval: 100 * time.Millisecond,
		},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
	}
	sc, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)

	apiKeyEnc, err := sc.Crypto.Encrypt("mock-upstream-key")
	require.NoError(t, err)
	llm := &mgwdb.LLM{Name: "Mock OpenAI", Slug: "mock-openai", Vendor: "openai", Endpoint: backend.URL,
		APIKeyEncrypted: apiKeyEnc, DefaultModel: "gpt-4", IsActive: true}
	require.NoError(t, db.Create(llm).Error)
	app := &mgwdb.App{Name: "Capture App", UserID: 5, IsActive: true}
	if len(appTracing) > 0 {
		app.Tracing = mgwdb.JSON(appTracing[0]) // the hub App's tracing settings, as synced
	}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error)
	require.NoError(t, db.Create(&mgwdb.APIToken{Token: trafficTestToken, Name: "capture", AppID: app.ID, IsActive: true}).Error)

	hub := &pulseHub{}
	sc.PluginManager.SetEdgeClient(pulseEdge{hub})
	require.NoError(t, sc.PluginManager.LoadDeferredBuiltinPlugins([]plugins.DataCollectionPluginConfig{{
		Name: "analytics_pulse", Enabled: true, HookTypes: []string{"analytics"},
		Config: map[string]interface{}{"interval_seconds": 3600, "timeout_seconds": 2},
	}}))
	require.True(t, sc.PluginManager.EnableAgentTracingCapture(mgwplugins.CaptureSettings{
		MaxEvents: 1000, MaxBytes: 1 << 20, FlushInterval: 50 * time.Millisecond, FlushMaxEvents: 1,
	}))
	agenttrace.SetSink(analytics.CaptureSink())
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 1, Key: []byte("k")})
	agenttrace.SetEnabled(true)
	t.Cleanup(func() {
		agenttrace.SetEnabled(false)
		agenttrace.SetSink(nil)
		agenttrace.SetFingerprinter(nil)
	})

	srv, err := server.New(cfg, sc, "test", "test", "test")
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
	return baseURL, hub, app.ID
}

func assertEdgeLLMEvent(t *testing.T, ev *agenttrace.Event, appID uint) {
	t.Helper()
	assert.Equal(t, "openai", ev.Provider)
	assert.Equal(t, "gpt-4", ev.RequestModel)
	assert.Equal(t, appID, ev.Identity.AppID)
	assert.Equal(t, uint(5), ev.Identity.UserID)
	assert.Equal(t, agenttrace.ResourceLLM, ev.Resource.Type)
	assert.Equal(t, http.StatusOK, ev.HTTPStatus)
	assert.Equal(t, agenttrace.StatusOK, ev.Status)
	assert.Positive(t, ev.Usage.InputTokens+ev.Usage.OutputTokens, "usage is captured")
	assert.False(t, ev.SpanID.IsZero())
	assert.False(t, ev.TraceID.IsZero())
}

// One /llm/rest/ call and one /ai/ call through the microgateway each reach
// the hub as llm_inference capture events; the /ai/ call is its outer event
// plus the inner hop's attempt of it.
func TestAgentTraceCapture_EdgeLLMCallsReachThePulse(t *testing.T) {
	baseURL, hub, appID := setupCaptureGateway(t)
	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	status, body := gatewayPost(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
	require.Equal(t, http.StatusOK, status, "body: %s", body)

	var rest *agenttrace.Event
	require.Eventually(t, func() bool {
		evs := hub.llmEvents()
		if len(evs) == 1 {
			rest = evs[0]
		}
		return rest != nil
	}, 10*time.Second, 25*time.Millisecond, "the /llm/rest/ capture event never reached the hub")
	assertEdgeLLMEvent(t, rest, appID)
	assert.True(t, rest.AttemptOf.IsZero())

	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
	require.Equal(t, http.StatusOK, status, "body: %s", body)

	var ai []*agenttrace.Event
	require.Eventually(t, func() bool {
		ai = hub.llmEvents()[1:]
		return len(ai) >= 2
	}, 10*time.Second, 25*time.Millisecond, "the /ai/ capture events never reached the hub")
	time.Sleep(200 * time.Millisecond)
	ai = hub.llmEvents()[1:]
	require.Len(t, ai, 2, "an /ai/ call is the outer event and one attempt")

	var outer, attempt *agenttrace.Event
	for _, e := range ai {
		if e.AttemptOf.IsZero() {
			outer = e
		} else {
			attempt = e
		}
	}
	require.NotNil(t, outer, "the outer /ai/ event")
	require.NotNil(t, attempt, "the inner hop's attempt")
	assert.Equal(t, outer.SpanID, attempt.AttemptOf, "the attempt points at its logical call")
	assert.Equal(t, outer.TraceID, attempt.TraceID, "both hops share the trace")
	assert.Equal(t, appID, outer.Identity.AppID)
	assertEdgeLLMEvent(t, attempt, appID)
}

// An App the hub switched tracing off for is not captured on the edge: the
// calls are served and nothing reaches the hub's capture (jyx.112).
func TestAgentTraceCapture_EdgeAppTracingOff(t *testing.T) {
	baseURL, hub, _ := setupCaptureGateway(t, `{"enabled":false}`)
	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	status, body := gatewayPost(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
	require.Equal(t, http.StatusOK, status, "body: %s", body)
	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
	require.Equal(t, http.StatusOK, status, "body: %s", body)

	time.Sleep(500 * time.Millisecond) // several capture flush intervals
	hub.mu.Lock()
	defer hub.mu.Unlock()
	assert.Empty(t, hub.events, "an App with tracing off leaves no capture")
}

// postWithHeaders is gatewayPost with extra request headers.
func postWithHeaders(t *testing.T, url, body string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+trafficTestToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// The context a client declares reaches the hub from the edge (jyx.113):
// X-Tyk-* headers, a coding harness's session and turn, and
// X-Tyk-Trace-Capture.
func TestAgentTraceCapture_EdgeDeclaredContext(t *testing.T) {
	declared.SetDefault(declared.New(declared.Options{
		Harness: []string{declared.HarnessClaudeCode, declared.HarnessCodexCLI, declared.HarnessOpenCode},
	}))
	t.Cleanup(func() { declared.SetDefault(nil) })
	baseURL, hub, _ := setupCaptureGateway(t)
	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	require.Equal(t, http.StatusOK, postWithHeaders(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", chatBody, map[string]string{
		"X-Tyk-Session-Id": "edge-s1", "X-Tyk-Agent-Name": "planner", "X-Tyk-Trace-Capture": "off",
	}))
	require.Equal(t, http.StatusOK, postWithHeaders(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", chatBody, map[string]string{
		"User-Agent":            "codex_exec/0.160.1 (Mac OS)",
		"X-Codex-Turn-Metadata": `{"session_id":"01a158d5-cb58-7a20-9c50-a33ff56877da","turn_id":"01a158d5-cc44-78c1-84bd-67edbc4e3b88"}`,
	}))

	var evs []*agenttrace.Event
	require.Eventually(t, func() bool { evs = hub.llmEvents(); return len(evs) == 2 }, 10*time.Second, 25*time.Millisecond)
	bySession := map[string]agenttrace.DeclaredContext{}
	for _, e := range evs {
		bySession[e.Declared.SessionID] = e.Declared
	}
	tyk := bySession["edge-s1"]
	assert.Equal(t, "planner", tyk.AgentName)
	assert.Equal(t, "off", tyk.CaptureMode)
	codex := bySession["01a158d5-cb58-7a20-9c50-a33ff56877da"]
	assert.Equal(t, declared.HarnessCodexCLI, codex.Harness)
	assert.Equal(t, "01a158d5-cc44-78c1-84bd-67edbc4e3b88", codex.TurnID)
}
