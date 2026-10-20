package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
)

const (
	metaTraceID     = "0af7651916cd43dd8448eb211c80319c"
	metaParentSpan  = "b7ad6b7169203331"
	metaTraceparent = "00-" + metaTraceID + "-" + metaParentSpan + "-01"
)

func (s *captureSink) ofKind(kind agenttrace.Kind) []*agenttrace.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*agenttrace.Event
	for _, e := range s.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// waitKindEvent waits for exactly one finished event of kind, then a little
// longer to catch a second one that should not be there.
func waitKindEvent(t *testing.T, sink *captureSink, kind agenttrace.Kind) *agenttrace.Event {
	t.Helper()
	require.Eventually(t, func() bool { return len(sink.ofKind(kind)) >= 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	evs := sink.ofKind(kind)
	require.Len(t, evs, 1, "one %s event per call", kind)
	return evs[0]
}

// modelArgsFP is the G3 fingerprint the LLM side computes for a model's tool
// call arguments (nested REST shape, as a model emits them).
func modelArgsFP(t *testing.T, args string) agenttrace.Fingerprint {
	t.Helper()
	fp, err := extract.ArgsFingerprint(agenttrace.CurrentFingerprinter(), []byte(args))
	require.NoError(t, err)
	require.NotEmpty(t, fp)
	return fp
}

// postMCP sends one JSON-RPC message to the tool's Streamable HTTP endpoint
// with extra headers.
func (s *mcpSession) postWith(t *testing.T, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/tools/"+testToolSlug+"/mcp", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+s.fixture.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	s.fixture.router.ServeHTTP(rr, req)
	return rr
}

func assertToolIdentity(t *testing.T, ev *agenttrace.Event, f *toolFilterFixture) {
	t.Helper()
	assert.NotZero(t, ev.Identity.AppID, "the App is on the event")
	assert.NotEmpty(t, ev.Identity.CredentialID, "the credential is on the event")
	assert.NotZero(t, ev.Identity.UserID, "the App owner is on the event")
	assert.Equal(t, agenttrace.Resource{Type: agenttrace.ResourceTool, ID: uintString(f.tool.ID), Name: f.tool.Name}, ev.Resource)
}

// An MCP tools/call over Streamable HTTP is one tool_call event: the raw
// arguments fingerprint like the model's tool call (G3), the result is
// fingerprinted, and the MCP session id is kept as a hint.
func TestAgentTraceTool_MCPStreamableCapture(t *testing.T) {
	sink := withCapture(t)
	withW3CPropagation(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{"body_name": "x"})
	require.False(t, isErr)

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assertToolIdentity(t, ev, f)
	assert.Equal(t, "getTestData", ev.ToolName)
	assert.Equal(t, "tools/call", ev.Operation)
	assert.Equal(t, agenttrace.StatusOK, ev.Status)
	assert.Equal(t, modelArgsFP(t, `{"body":{"name":"x"}}`), ev.Extracts.ArgsFP,
		"args_fp equals the G3 fingerprint of the model's tool call")
	assert.Equal(t, extract.ResultFingerprint(agenttrace.CurrentFingerprinter(), `{"message":"ok"}`), ev.Extracts.ResultFP)
	assert.False(t, ev.Extracts.ToolError)
	assert.Equal(t, session.sessionID, ev.Declared.MCPSessionID)
	assert.Empty(t, sink.ofKind(agenttrace.KindLLMInference))

	tp := f.lastUpstreamHeaders(t).Get("Traceparent")
	assert.Contains(t, tp, ev.SpanID.String(), "the tool upstream's parent is the tool_call span")
}

// params._meta.traceparent is the declared parent, preferred over the HTTP
// header (fixture mcp_meta_traceparent); the upstream continues that trace.
func TestAgentTraceTool_MCPMetaTraceparent(t *testing.T) {
	sink := withCapture(t)
	withW3CPropagation(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	rr := session.postWith(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{},"_meta":{"traceparent":"`+metaTraceparent+`"}}}`,
		map[string]string{"Traceparent": clientTraceparent})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.Equal(t, "mcp_meta", ev.Declared.TraceSource)
	assert.Equal(t, metaTraceID, ev.Declared.TraceID.String())
	assert.Equal(t, metaParentSpan, ev.Declared.ParentSpanID.String())
	assert.Equal(t, metaTraceID, ev.TraceID.String(), "_meta wins over the HTTP traceparent")

	tp := f.lastUpstreamHeaders(t).Get("Traceparent")
	assert.Equal(t, metaTraceID, traceIDOf(tp))
	assert.Contains(t, tp, ev.SpanID.String())
}

// MCP 2026-07-28 stateless requests name the method in a header and carry no
// session (fixture mcp_2026_07_28_stateless); the header lands on the span.
func TestAgentTraceTool_MCPStatelessHeaders(t *testing.T) {
	sink := withCapture(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	rr := session.postWith(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{}}}`,
		map[string]string{"Mcp-Method": "tools/call", "Mcp-Name": "getTestData", "Mcp-Protocol-Version": "2026-07-28"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.Equal(t, "tools/call", ev.Attributes["mcp.method.name"])
	assert.Equal(t, "2026-07-28", ev.Attributes["mcp.protocol.version"])
	assert.Equal(t, "getTestData", ev.ToolName)
}

// A tool result flagged isError (the upstream failed) is a tool_error span.
func TestAgentTraceTool_MCPToolError(t *testing.T) {
	sink := withCapture(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()
	f.upstream.ResponseStatus = http.StatusInternalServerError

	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{})
	require.True(t, isErr)

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.True(t, ev.Extracts.ToolError)
	assert.Equal(t, agenttrace.StatusError, ev.Status)
	assert.Equal(t, "tool_error", ev.ErrorType)
}

// sseClient drives the legacy SSE transport: GET opens the event stream, the
// endpoint event names the message URL, and replies arrive on the stream.
type sseClient struct {
	srv      *httptest.Server
	apiKey   string
	endpoint string
	events   chan string
	cancel   func()
}

func newSSEClient(t *testing.T, f *toolFilterFixture) *sseClient {
	t.Helper()
	srv := httptest.NewServer(f.router)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/tools/"+testToolSlug+"/mcp/sse", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	c := &sseClient{srv: srv, apiKey: f.apiKey, events: make(chan string, 16)}
	c.cancel = func() { cancel(); resp.Body.Close(); srv.Close() }
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		event := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				c.events <- event + " " + strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		close(c.events)
	}()
	ep := c.next(t)
	require.True(t, strings.HasPrefix(ep, "endpoint "), ep)
	c.endpoint = strings.TrimPrefix(ep, "endpoint ")
	if !strings.HasPrefix(c.endpoint, "http") {
		c.endpoint = srv.URL + c.endpoint
	}
	return c
}

func (c *sseClient) next(t *testing.T) string {
	t.Helper()
	select {
	case e := <-c.events:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("no SSE event")
		return ""
	}
}

func (c *sseClient) send(t *testing.T, body string, hdr map[string]string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.endpoint, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// On the SSE transport the POST returns 202 before the call runs, and the
// answer travels on the event stream: the call still makes one tool_call
// event, continuing the POST's trace, with the App and the upstream parented
// on it.
func TestAgentTraceTool_MCPSSECapture(t *testing.T) {
	sink := withCapture(t)
	withW3CPropagation(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	c := newSSEClient(t, f)
	defer c.cancel()
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, nil)
	require.Contains(t, c.next(t), `"id":1`)
	c.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	c.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{"body_name":"x"}}}`,
		map[string]string{"Traceparent": clientTraceparent})
	reply := c.next(t)
	require.Contains(t, reply, `"id":2`)
	require.Contains(t, reply, "ok")

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assertToolIdentity(t, ev, f)
	assert.Equal(t, clientTraceID, ev.TraceID.String(), "the call continues the POST's trace")
	assert.Equal(t, "traceparent", ev.Declared.TraceSource)
	assert.Equal(t, modelArgsFP(t, `{"body":{"name":"x"}}`), ev.Extracts.ArgsFP)
	assert.NotEmpty(t, ev.Extracts.ResultFP)
	assert.NotEmpty(t, ev.Declared.MCPSessionID)
	assert.Equal(t, agenttrace.StatusOK, ev.Status)

	tp := f.lastUpstreamHeaders(t).Get("Traceparent")
	assert.Equal(t, clientTraceID, traceIDOf(tp))
	assert.Contains(t, tp, ev.SpanID.String(), "the upstream's parent is the call's own span")
}

// A REST tool call is the same tool_call event; its arguments fingerprint as
// the MCP shim's flat arguments do, so either transport matches the model.
func TestAgentTraceTool_RESTCapture(t *testing.T) {
	sink := withCapture(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	rr := f.call(t, "submitTestData", map[string]interface{}{"name": "x"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assertToolIdentity(t, ev, f)
	assert.Equal(t, "submitTestData", ev.ToolName)
	assert.Equal(t, "execute_tool", ev.Operation)
	assert.Equal(t, http.StatusOK, ev.HTTPStatus)
	assert.Equal(t, modelArgsFP(t, `{"body":{"name":"x"}}`), ev.Extracts.ArgsFP)
	assert.Equal(t, extract.ResultFingerprint(agenttrace.CurrentFingerprinter(), `{"message":"ok"}`), ev.Extracts.ResultFP)
}

// A REST tool call the upstream fails is an errored tool_call.
func TestAgentTraceTool_RESTUpstreamError(t *testing.T) {
	sink := withCapture(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()
	f.upstream.ResponseStatus = http.StatusInternalServerError

	rr := f.call(t, "submitTestData", map[string]interface{}{"name": "x"})
	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.Equal(t, http.StatusInternalServerError, ev.HTTPStatus)
	assert.True(t, ev.Extracts.ToolError)
	assert.Equal(t, agenttrace.StatusError, ev.Status)
	assert.Equal(t, "tool_error", ev.ErrorType)
}

// Each datasource route records a datasource_call for the App, also when the
// call then fails.
func TestAgentTraceTool_DatasourceRoutes(t *testing.T) {
	ds := &models.Datasource{Name: "Test DS", Active: true, DBSourceType: "pinecone"}
	ds.ID = 1
	app := testAppWithDatasource(ds)
	app.ID, app.UserID = 1, 9
	vars := map[string]string{"dsSlug": "test-ds"}

	for _, tc := range []struct {
		op      string
		handler func(p *Proxy) http.HandlerFunc
		body    interface{}
	}{
		{"search", func(p *Proxy) http.HandlerFunc { return p.handleDatasourceRequest }, SearchQuery{Query: "q", N: 1}},
		{"vector_search", func(p *Proxy) http.HandlerFunc { return p.handleDatasourceVectorSearch }, VectorSearchQuery{}},
		{"metadata_query", func(p *Proxy) http.HandlerFunc { return p.handleDatasourceMetadataQuery }, MetadataQuery{}},
		{"embeddings", func(p *Proxy) http.HandlerFunc { return p.handleDatasourceGenerateEmbedding }, EmbeddingRequest{}},
	} {
		t.Run(tc.op, func(t *testing.T) {
			sink := withCapture(t)
			p := newTestProxyWithDatasource(t, ds)

			bodyBytes, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/datasource/test-ds", bytes.NewReader(bodyBytes))
			req, rec := beginRequestCapture(req)
			require.NotNil(t, rec)
			captureApp(req.Context(), app)
			req = req.WithContext(context.WithValue(req.Context(), "app", app))
			w := httptest.NewRecorder()
			sw := &captureStatusWriter{ResponseWriter: w}
			tc.handler(p)(sw, mux.SetURLVars(req, vars))
			finishCapture(rec, req, sw.status)
			rec.Finish()

			ev := waitKindEvent(t, sink, agenttrace.KindDatasourceCall)
			assert.Equal(t, agenttrace.Resource{Type: agenttrace.ResourceDatasource, ID: "1", Name: "Test DS"}, ev.Resource)
			assert.Equal(t, tc.op, ev.ToolName)
			assert.Equal(t, uint(1), ev.Identity.AppID)
			assert.Equal(t, uint(9), ev.Identity.UserID)
			assert.Equal(t, w.Code, ev.HTTPStatus)
			assert.Equal(t, agenttrace.StatusError, ev.Status, "these calls fail without a vector store")
		})
	}
}

// A successful datasource call carries its arguments' and result's
// fingerprints (the success path needs a vector store, so captureTool is
// driven directly with what the handler passes).
func TestAgentTraceTool_DatasourceSuccessCapture(t *testing.T) {
	sink := withCapture(t)
	ds := &models.Datasource{Name: "Test DS"}
	ds.ID = 1
	p := newTestProxyWithDatasource(t, ds)
	app := testAppWithDatasource(ds)
	app.ID = 1

	ctx, rec := agenttrace.Begin(context.Background())
	ctx = context.WithValue(ctx, "app", app)
	body, result := []byte(`{"query":"q","n":1}`), []byte(`{"documents":[]}`)
	p.captureTool(ctx, datasourceCapture(ds, "search", body, result, http.StatusOK))
	p.waitForAnalyzers()
	rec.Finish()

	ev := waitKindEvent(t, sink, agenttrace.KindDatasourceCall)
	assert.Equal(t, uint(1), ev.Identity.AppID)
	assert.Equal(t, modelArgsFP(t, string(body)), ev.Extracts.ArgsFP)
	assert.NotEmpty(t, ev.Extracts.ResultFP)
	assert.Equal(t, agenttrace.StatusOK, ev.Status)
}

// With agent tracing off, the tool, MCP, datasource and plugin endpoint hooks
// allocate nothing.
func TestAgentTraceTool_OffAllocatesNothing(t *testing.T) {
	withAgentTracing(t, false)
	p := &Proxy{}
	ctx := context.Background()
	r := httptest.NewRequest(http.MethodPost, "/plugins/x/y", nil)
	w := httptest.NewRecorder()
	ds := &models.Datasource{}
	req := mcp.CallToolRequest{}
	args := map[string]string{}
	allocs := testing.AllocsPerRun(100, func() {
		if capturing(ctx) {
			t.Fatal("capturing with tracing off")
		}
		_, finish := beginMCPToolCapture(ctx, req)
		finish()
		_ = markMCPDetached(r)
		noteDatasourceCall(ctx, ds, "search")
		p.captureTool(ctx, toolCapture{})
		_ = toolJSON(ctx, args)
		_ = writtenStatus(w)
		r2, c := BeginPluginEndpointCapture(r, 1, "x")
		c.SetApp(1)
		c.Finish(http.StatusOK)
		_ = r2
	})
	assert.Zero(t, allocs)
}
