//go:build enterprise
// +build enterprise

package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/assembler"
	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/content"
	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/correlate"
	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/jsonv1"
	"github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/keys"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	mgwplugins "github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	aiconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/traceschema"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// The edge half of the MVP exit check (jyx.37, jyx.144): a demo agent drives
// the REAL microgateway with capture on; its content pipeline runs on the
// edge; the capture pulse carries the events (as protos) to a hub that
// stages them as the control server does, holds edge-processed content to
// its own resolved mode, assembles them with the correlation rules and
// exports the session. Assertions are on what an operator sees.

const (
	edgeE2EToken  = "edge-e2e-token"
	edgeE2EID     = "edge-e2e"
	edgeE2ESecret = "AKIAIOSFODNN7EXAMPLE" // AWS's documented example key
)

// protoHub is the hub's pulse endpoint: it keeps the capture protos.
type protoHub struct {
	pb.ConfigurationSyncServiceClient
	mu     sync.Mutex
	events []*pb.CaptureEventProto
}

func (h *protoHub) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, pulse.CaptureEvents...)
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func (h *protoHub) received() []*pb.CaptureEventProto {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*pb.CaptureEventProto(nil), h.events...)
}

// summary lists the received events' kinds, sessions and statuses.
func (h *protoHub) summary() string {
	var parts []string
	for _, p := range h.received() {
		ev := agenttrace.FromProto(p)
		parts = append(parts, fmt.Sprintf("%s/%s/%d", ev.Kind, ev.Declared.SessionID, ev.HTTPStatus))
	}
	return strings.Join(parts, " ")
}

type protoEdge struct{ hub *protoHub }

func (e protoEdge) GetGRPCClient() pb.ConfigurationSyncServiceClient { return e.hub }
func (e protoEdge) GetEdgeID() string                                { return edgeE2EID }
func (e protoEdge) GetEdgeNamespace() string                         { return "" }

// edgeVendor answers like OpenAI, scripted by the request.
type edgeVendor struct {
	mu    sync.Mutex
	calls []http.Header
	body  []string
}

func (v *edgeVendor) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	v.mu.Lock()
	v.calls = append(v.calls, r.Header.Clone())
	v.body = append(v.body, string(b))
	v.mu.Unlock()
	s := string(b)
	usage := `"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}`
	switch {
	case strings.Contains(s, `"stream":true`):
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for _, d := range []string{
			`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"It is "}}]}`,
			`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"sunny."}}]}`,
			`{"id":"c2","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],` + usage + `}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", d)
			f.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	case strings.Contains(s, "weather"):
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[`+
			`{"id":"call_mcp","type":"function","function":{"name":"getTestData","arguments":"{}"}},`+
			`{"id":"call_local","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"notes.txt\"}"}}]},"finish_reason":"tool_calls"}],`+usage+`}`)
	default:
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c3","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],`+usage+`}`)
	}
}

type edgeE2E struct {
	t        *testing.T
	baseURL  string
	hub      *protoHub
	vendor   *edgeVendor
	toolHits atomic.Int32
	toolHdrs []http.Header
	toolMu   sync.Mutex
	appID    uint
	cfg      aiconfig.AgentTracingConfig
	keys     *keys.Provider
}

// newEdgeE2E boots the microgateway with an LLM, a guarded LLM whose
// request filter blocks everything, and a Studio tool, all granted to one
// App; capture on, with the edge content pipeline the hub's snapshot
// configures (redacted, header ceiling redacted) and the hub's fingerprint
// key (decision D4).
func newEdgeE2E(t *testing.T) *edgeE2E {
	t.Helper()
	e := &edgeE2E{t: t, hub: &protoHub{}, vendor: &edgeVendor{}}
	e.cfg = aiconfig.DefaultAgentTracingConfig()
	e.cfg.Key = "edge-e2e-agent-tracing-root-key-0123456789"
	e.cfg.ContentDefaultMode = agenttrace.ContentRedacted
	e.cfg.ContentMaxRequestOverride = agenttrace.ContentRedacted
	k, err := keys.New(e.cfg)
	require.NoError(t, err)
	e.keys = k

	vendorSrv := httptest.NewServer(http.HandlerFunc(e.vendor.serve))
	t.Cleanup(vendorSrv.Close)
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.toolHits.Add(1)
		e.toolMu.Lock()
		e.toolHdrs = append(e.toolHdrs, r.Header.Clone())
		e.toolMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"forecast":"sunny"}`)
	}))
	t.Cleanup(toolSrv.Close)

	dsn := fmt.Sprintf("file:edgee2e_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, mgwdb.Migrate(db))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	cfg := &config.Config{
		Server:        config.ServerConfig{Host: "127.0.0.1", Port: port, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second},
		Database:      config.DatabaseConfig{Type: "sqlite"},
		Cache:         config.CacheConfig{Enabled: true, MaxSize: 100, TTL: 5 * time.Minute},
		Security:      config.SecurityConfig{EncryptionKey: "12345678901234567890123456789012", JWTSecret: "test-secret-key"},
		Analytics:     config.AnalyticsConfig{Enabled: true, BufferSize: 1, FlushInterval: 100 * time.Millisecond},
		Observability: config.ObservabilityConfig{LogLevel: "error", LogFormat: "json"},
	}
	sc, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)
	keyEnc, err := sc.Crypto.Encrypt("vendor-key")
	require.NoError(t, err)

	llm := &mgwdb.LLM{Name: "Primary", Slug: "primary", Vendor: "openai", Endpoint: vendorSrv.URL,
		APIKeyEncrypted: keyEnc, DefaultModel: "gpt-4", IsActive: true}
	require.NoError(t, db.Create(llm).Error)
	guarded := &mgwdb.LLM{Name: "Guarded", Slug: "guarded", Vendor: "openai", Endpoint: vendorSrv.URL,
		APIKeyEncrypted: keyEnc, DefaultModel: "gpt-4", IsActive: true}
	require.NoError(t, db.Create(guarded).Error)
	filter := &mgwdb.Filter{Name: "guard", Script: `output := {block: true, message: "policy says no"}`, IsActive: true}
	require.NoError(t, db.Create(filter).Error)
	require.NoError(t, db.Create(&mgwdb.LLMFilter{LLMID: guarded.ID, FilterID: filter.ID, IsActive: true}).Error)

	spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"Test Tool","version":"1.0.0"},"servers":[{"url":"%s"}],"paths":{"/test":{"get":{"operationId":"getTestData","summary":"Get Test Data","responses":{"200":{"description":"Success"}}}}}}`, toolSrv.URL)
	tool := &mgwdb.Tool{Name: "Test Mock Tool", Slug: "test-mock-tool", ToolType: "REST",
		OASSpec: base64.StdEncoding.EncodeToString([]byte(spec)), AvailableOperations: "getTestData", Active: true}
	require.NoError(t, db.Create(tool).Error)

	app := &mgwdb.App{Name: "Demo Agent", UserID: 5, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	e.appID = app.ID
	require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: llm.ID, IsActive: true}).Error)
	require.NoError(t, db.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: guarded.ID, IsActive: true}).Error)
	require.NoError(t, db.Create(&mgwdb.AppTool{AppID: app.ID, ToolID: tool.ID}).Error)
	require.NoError(t, db.Create(&mgwdb.APIToken{Token: edgeE2EToken, Name: "e2e", AppID: app.ID, IsActive: true}).Error)

	sc.PluginManager.SetEdgeClient(protoEdge{e.hub})
	require.NoError(t, sc.PluginManager.LoadDeferredBuiltinPlugins([]plugins.DataCollectionPluginConfig{{
		Name: "analytics_pulse", Enabled: true, HookTypes: []string{"analytics"},
		Config: map[string]interface{}{"interval_seconds": 3600, "timeout_seconds": 2},
	}}))
	require.True(t, sc.PluginManager.EnableAgentTracingCapture(mgwplugins.CaptureSettings{
		MaxEvents: 1000, MaxBytes: 1 << 20, FlushInterval: 50 * time.Millisecond, FlushMaxEvents: 1,
	}))

	// What the edge's start gate and an applied snapshot install.
	kid, key := k.EdgeKey()
	agenttrace.InstallEdgeKey(kid, key)
	globals := agenttrace.ContentGlobals{DefaultMode: agenttrace.ContentRedacted, MaxRequestOverride: agenttrace.ContentRedacted, StoreCap: agenttrace.ContentFull}
	p := agenttrace.NewContentProcessor(globals, func(uint, agenttrace.Resource) agenttrace.ContentOverrides { return agenttrace.ContentOverrides{} })
	require.NotNil(t, p, "the Enterprise content pipeline is registered")
	agenttrace.SetContentProcessor(p)
	declared.SetDefault(declared.New(declared.Options{
		CompatFamilies: []string{declared.FamilyHelicone, declared.FamilyMCPSession},
		Harness:        []string{declared.HarnessClaudeCode, declared.HarnessCodexCLI, declared.HarnessOpenCode},
	}))
	agenttrace.SetGatewayNode(edgeE2EID)
	agenttrace.SetSink(analytics.CaptureSink())
	agenttrace.SetEnabled(true)
	t.Cleanup(func() {
		agenttrace.SetGatewayNode("")
		agenttrace.SetEnabled(false)
		agenttrace.SetSink(nil)
		agenttrace.SetFingerprinter(nil)
		agenttrace.SetContentProcessor(nil)
		agenttrace.SetSealer(nil)
		agenttrace.SetEdgeContentGlobals(nil)
		declared.SetDefault(nil)
	})

	srv, err := server.New(cfg, sc, "test", "test", "test")
	require.NoError(t, err)
	go func() { _ = srv.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	e.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealthy(t, e.baseURL)
	return e
}

func (e *edgeE2E) post(path, body string, headers ...string) (int, []byte, http.Header) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.baseURL+path, bytes.NewBufferString(body))
	require.NoError(e.t, err)
	req.Header.Set("Authorization", "Bearer "+edgeE2EToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(e.t, err)
	return resp.StatusCode, b, resp.Header
}

// hubSide stages what the pulse carried as the control server does, then
// assembles and exports on a hub database.
type hubSide struct {
	t      *testing.T
	db     *gorm.DB
	keys   *keys.Provider
	asm    *assembler.Assembler
	offset atomic.Int64
}

func (e *edgeE2E) toHub(wantEvents int) *hubSide {
	t := e.t
	t.Helper()
	require.Eventually(t, func() bool { return len(e.hub.received()) >= wantEvents }, 10*time.Second, 25*time.Millisecond,
		"the pulse carried %d of %d capture events: %s", len(e.hub.received()), wantEvents, e.hub.summary())

	dsn := fmt.Sprintf("file:edgee2ehub_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	require.NoError(t, db.AutoMigrate(models.AgentTraceModels()...))
	require.NoError(t, (&models.App{ID: e.appID, Name: "Demo Agent"}).Create(db))

	h := &hubSide{t: t, db: db, keys: e.keys}
	// The hub's tracing as Studio starts it (features/agenttracing
	// Service.Start): its keys, sealer and content pipeline, which holds
	// edge-processed content to the hub's resolved mode. The service itself
	// is not linked: it pulls in the OTLP exporters this module does not
	// otherwise need.
	agenttrace.SetFingerprinter(e.keys)
	agenttrace.SetSealer(e.keys.Sealer())
	agenttrace.SetContentProcessor(content.NewProcessor(content.ConfigFrom(e.cfg), nil))

	var evs []*agenttrace.Event
	for _, p := range e.hub.received() {
		evs = append(evs, agenttrace.FromProto(p))
	}
	n, err := analytics.StageCaptureEvents(db, edgeE2EID, evs)
	require.NoError(t, err)
	require.EqualValues(t, len(evs), n)

	h.asm = assembler.New(assembler.Options{DB: db, Config: e.cfg, Keys: e.keys, NodeID: "hub", Correlator: correlate.New(e.cfg),
		Now: func() time.Time { return time.Now().Add(time.Duration(h.offset.Load())) }})
	ctx := context.Background()
	h.offset.Store(int64(31 * time.Second))
	for {
		n, err := h.asm.RunOnce(ctx)
		require.NoError(t, err)
		if n == 0 {
			break
		}
	}
	h.offset.Store(int64(e.cfg.SessionIdleTimeout + e.cfg.TurnIdleTimeout + time.Minute))
	require.NoError(t, h.asm.CloseIdle(ctx))
	return h
}

func (h *hubSide) export(appID uint, external string) (*jsonv1.Document, []byte) {
	h.t.Helper()
	var sess models.AgentTraceSession
	require.NoError(h.t, h.db.Where("app_id = ? AND external_id = ?", appID, external).First(&sess).Error, "a session for %q", external)
	// Read with content as the session export reads it (exportapi.go).
	doc, err := jsonv1.LoadSession(h.db, appID, sess.ID, jsonv1.Options{IncludeContent: true,
		Content: func(p *models.AgentTracePayload) ([]byte, error) {
			if strings.HasPrefix(p.Part, content.PreviewPrefix) {
				return nil, nil
			}
			plain, err := content.Open(p, h.keys, 1<<20)
			if err != nil {
				return nil, nil
			}
			return plain, nil
		}})
	require.NoError(h.t, err)
	for i := range doc.Spans { // the export leaves the preview rows out
		var kept []jsonv1.Part
		for _, p := range doc.Spans[i].Parts {
			if !strings.HasPrefix(p.Part, content.PreviewPrefix) {
				kept = append(kept, p)
			}
		}
		doc.Spans[i].Parts = kept
	}
	raw, err := jsonv1.Marshal(doc)
	require.NoError(h.t, err)
	require.NoError(h.t, traceschema.ValidateV1(raw))
	return doc, raw
}

func edgeSpans(doc *jsonv1.Document, kind string) []jsonv1.Span {
	var out []jsonv1.Span
	for _, s := range doc.Spans {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

func edgePart(s jsonv1.Span, name string) *jsonv1.Part {
	for i := range s.Parts {
		if s.Parts[i].Part == name {
			return &s.Parts[i]
		}
	}
	return nil
}

func TestAgentTraceE2E_EdgeDemoAgent(t *testing.T) {
	e := newEdgeE2E(t)
	sess := []string{"X-Tyk-Session-Id", "edge-demo", "X-Tyk-Agent-Name", "orchestrator"}
	ask := `{"model":"gpt-4","messages":[{"role":"user","content":"What is the weather? my key is ` + edgeE2ESecret + `"}]}`

	code, body, _ := e.post("/llm/rest/primary/v1/chat/completions", ask, sess...)
	require.Equal(t, http.StatusOK, code, string(body))

	// The MCP tool through the edge, in the same session.
	mcpHdr := []string{"X-Tyk-Session-Id", "edge-demo", "X-Tyk-Agent-Name", "orchestrator"}
	code, body, hdr := e.post("/tools/test-mock-tool/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"demo-agent","version":"1"}}}`, mcpHdr...)
	require.Equal(t, http.StatusOK, code, string(body))
	mcpSess := hdr.Get("Mcp-Session-Id")
	require.NotEmpty(t, mcpSess)
	withSess := append(append([]string(nil), mcpHdr...), "Mcp-Session-Id", mcpSess)
	e.post("/tools/test-mock-tool/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, withSess...)
	code, body, _ = e.post("/tools/test-mock-tool/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{}}}`, withSess...)
	require.Equal(t, http.StatusOK, code, string(body))
	require.Contains(t, string(body), "sunny")

	code, body, _ = e.post("/llm/stream/primary/v1/chat/completions", `{"model":"gpt-4","stream":true,"messages":[`+
		`{"role":"user","content":"What is the weather? my key is `+edgeE2ESecret+`"},`+
		`{"role":"assistant","content":null,"tool_calls":[{"id":"call_mcp","type":"function","function":{"name":"getTestData","arguments":"{}"}},`+
		`{"id":"call_local","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"notes.txt\"}"}}]},`+
		`{"role":"tool","tool_call_id":"call_mcp","content":"{\"forecast\":\"sunny\"}"},`+
		`{"role":"tool","tool_call_id":"call_local","content":"remember the umbrella"}]}`, sess...)
	require.Equal(t, http.StatusOK, code, string(body))
	assert.Contains(t, string(body), "sunny.")

	code, body, _ = e.post("/llm/rest/primary/v1/chat/completions",
		`{"model":"gpt-4","messages":[{"role":"user","content":"research umbrellas"}]}`,
		"X-Tyk-Session-Id", "edge-demo", "X-Tyk-Agent-Path", "/orchestrator/researcher")
	require.Equal(t, http.StatusOK, code, string(body))

	code, body, _ = e.post("/llm/rest/guarded/v1/chat/completions",
		`{"model":"gpt-4","messages":[{"role":"user","content":"do the forbidden thing"}]}`, sess...)
	require.Equal(t, http.StatusBadRequest, code, string(body))

	// A Claude Code session and a request that turns capture off.
	code, body, _ = e.post("/llm/rest/primary/v1/chat/completions", `{"model":"gpt-4","messages":[{"role":"user","content":"cc hello"}]}`,
		"User-Agent", "claude-cli/2.1.304 (external, cli)", "X-Claude-Code-Session-Id", "5d0c7a4e-3f7b-4f2a-9b51-0f0a8f2c1d11")
	require.Equal(t, http.StatusOK, code, string(body))
	code, body, _ = e.post("/llm/rest/primary/v1/chat/completions", `{"model":"gpt-4","messages":[{"role":"user","content":"private words"}]}`,
		"X-Tyk-Session-Id", "edge-off", "X-Tyk-Trace-Capture", "off")
	require.Equal(t, http.StatusOK, code, string(body))

	h := e.toHub(7) // six LLM calls and the MCP call
	doc, raw := h.export(e.appID, "edge-demo")

	assert.Equal(t, agenttrace.ContentRedacted, doc.Session.ContentMode)
	assert.Equal(t, 4, doc.Session.Totals.LLMCallCount)
	assert.Equal(t, 2, doc.Session.Totals.ToolCallCount, "the MCP call and the inferred client tool")
	assert.Equal(t, 1, doc.Session.Totals.ErrorCount)
	assert.NotContains(t, string(raw), edgeE2ESecret, "the edge redacted the key")

	agents := map[string]jsonv1.Agent{}
	for _, a := range doc.Agents {
		agents[a.Name] = a
	}
	require.Contains(t, agents, "researcher")
	assert.Equal(t, agents["orchestrator"].ID, agents["researcher"].ParentAgentID)

	llm := edgeSpans(doc, "llm_inference")
	require.Len(t, llm, 4)
	asked := llm[0]
	assert.Equal(t, []string{"call_mcp", "call_local"}, asked.ToolCallIDs)
	t.Run("the span names the edge that served it (jyx.150)", func(t *testing.T) {
		assert.Equal(t, edgeE2EID, asked.GatewayNode)
	})
	if p := edgePart(asked, agenttrace.PartRequest); assert.NotNil(t, p) {
		assert.Equal(t, agenttrace.StateRedacted, p.ContentState)
		assert.Positive(t, p.RedactionCount)
	}

	// R2 across the edge: the MCP call executes call_mcp, in the turn.
	tools := edgeSpans(doc, "tool_call")
	require.Len(t, tools, 1)
	assert.Equal(t, asked.SpanID, tools[0].ParentSpanID)
	assert.Equal(t, asked.TraceID, tools[0].TraceID)
	assert.Equal(t, []string{"call_mcp"}, tools[0].ToolCallIDs)
	client := edgeSpans(doc, "inferred_client_tool")
	require.Len(t, client, 1)
	assert.Equal(t, "read_file", client[0].ToolName)

	// The streamed step stays in the turn.
	assert.Equal(t, asked.TraceID, llm[1].TraceID)
	assert.Equal(t, []string{"stop"}, llm[1].FinishReasons)
	t.Run("streamed answer is in the transcript (jyx.146)", func(t *testing.T) {
		p := edgePart(llm[1], agenttrace.PartResponse)
		require.NotNil(t, p)
		require.NotNil(t, p.Content)
		assert.Contains(t, *p.Content, "sunny.")
	})
	assert.Equal(t, agents["researcher"].ID, llm[2].AgentID)

	events := edgeSpans(doc, "gateway_event")
	require.Len(t, events, 1)
	assert.Equal(t, "request_filter", events[0].GatewayEvent.Name)

	doc, _ = h.export(e.appID, "5d0c7a4e-3f7b-4f2a-9b51-0f0a8f2c1d11")
	assert.Equal(t, declared.HarnessClaudeCode, doc.Session.ExternalSource)

	doc, raw = h.export(e.appID, "edge-off")
	require.Len(t, doc.Spans, 1)
	assert.Empty(t, doc.Spans[0].Parts)
	assert.NotContains(t, string(raw), "private words", "X-Tyk-Trace-Capture: off on the edge")

	// No tracing header reached the vendor or the tool upstream; the
	// blocked prompt never reached the vendor.
	e.vendor.mu.Lock()
	for i, hd := range e.vendor.calls {
		assert.NotContains(t, e.vendor.body[i], "forbidden")
		for name := range hd {
			assert.False(t, strings.HasPrefix(strings.ToLower(name), "x-tyk-"), "vendor got %s", name)
		}
	}
	e.vendor.mu.Unlock()
	e.toolMu.Lock()
	require.NotEmpty(t, e.toolHdrs)
	for _, hd := range e.toolHdrs {
		for name := range hd {
			assert.False(t, strings.HasPrefix(strings.ToLower(name), "x-tyk-"), "tool upstream got %s", name)
		}
	}
	e.toolMu.Unlock()
}
