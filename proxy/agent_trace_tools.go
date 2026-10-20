package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// toolCapture is what a tool path knows about one executed tool call.
type toolCapture struct {
	kind      agenttrace.Kind // tool_call or datasource_call
	resource  agenttrace.Resource
	toolName  string // the name the model calls it by (operation id for Studio tools)
	operation string // mcp.method.name, or "execute_tool" for REST
	args      []byte // the call's arguments as sent (JSON)
	// fpArgs, when set, is what args_fp is computed over instead of args (the
	// REST envelope reshaped like the MCP shim's flat arguments).
	fpArgs      []byte
	result      []byte // the result as returned (JSON or text)
	toolError   bool
	status      int
	toolCallID  string // gen_ai.tool.call.id when the request carried one
	mcpMethod   string // Mcp-Method header (MCP 2026-07-28), when sent
	mcpSession  string // the MCP session id, a weak session hint
	protocolVer string
}

// captureTool completes the request's tool_call or datasource_call capture
// (spec §3.2, §4.4 R2). Fingerprints are computed on the analytics goroutine
// with the same functions the LLM side uses (args flattened like the
// OpenAPI-to-MCP shim, results as text), so R2 compares like with like.
func (p *Proxy) captureTool(ctx context.Context, c toolCapture) {
	rec := agenttrace.From(ctx)
	if rec == nil {
		return
	}
	app, _ := ctx.Value("app").(*models.App)
	p.analyzeTool(rec, app, c)
}

// analyzeTool is captureTool's work on the analytics goroutine, kept apart
// so c escapes to the heap only while tracing is on.
func (p *Proxy) analyzeTool(rec *agenttrace.Recorder, app *models.App, c toolCapture) {
	release := rec.Hold()
	p.goAnalyze(func() {
		defer release()
		fp := agenttrace.CurrentFingerprinter()
		fpArgs := c.fpArgs
		if fpArgs == nil {
			fpArgs = c.args
		}
		var argsFP agenttrace.Fingerprint
		if len(fpArgs) > 0 {
			argsFP, _ = extract.ArgsFingerprint(fp, fpArgs)
		}
		var resultFP agenttrace.Fingerprint
		if len(c.result) > 0 {
			resultFP = extract.ResultFingerprint(fp, string(c.result))
		}
		rec.Update(func(ev *agenttrace.Event) {
			ev.Kind = c.kind
			ev.Resource = c.resource
			ev.ToolName = c.toolName
			ev.Operation = c.operation
			ev.Provider = "tyk.studio"
			fillAppIdentity(ev, app)
			ev.EndTime = time.Now()
			if c.status != 0 {
				ev.HTTPStatus = c.status
			}
			ev.Extracts.ArgsFP, ev.Extracts.ResultFP = argsFP, resultFP
			ev.Extracts.ToolError = c.toolError
			ev.Extracts.ToolCallID = c.toolCallID
			if c.toolError || c.status >= http.StatusBadRequest {
				ev.Status = agenttrace.StatusError
				if ev.ErrorType == "" {
					ev.ErrorType = "tool_error"
				}
			}
			if c.mcpSession != "" {
				ev.Declared.MCPSessionID = c.mcpSession
			}
			if c.mcpMethod != "" || c.protocolVer != "" {
				if ev.Attributes == nil {
					ev.Attributes = map[string]string{}
				}
				if c.mcpMethod != "" {
					ev.Attributes["mcp.method.name"] = c.mcpMethod
				}
				if c.protocolVer != "" {
					ev.Attributes["mcp.protocol.version"] = c.protocolVer
				}
			}
		})
		rec.CaptureRaw(agenttrace.PartToolArgs, "application/json", c.args)
		rec.CaptureRaw(agenttrace.PartToolResult, "application/json", c.result)
	})
}

// mcpDetachedKey marks a request whose MCP messages are answered after the
// HTTP request returned (the legacy SSE transport's POST /message, which
// replies 202 and answers on the event stream).
type mcpDetachedKey struct{}

// markMCPDetached marks r as an SSE message POST while tracing is on.
func markMCPDetached(r *http.Request) *http.Request {
	if agenttrace.From(r.Context()) == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), mcpDetachedKey{}, true))
}

// beginMCPToolCapture prepares the capture of one MCP tools/call.
//
// On the SSE transport the POST's capture finishes when it returns 202,
// before the call runs, so the call gets a capture of its own continuing the
// POST's context (agenttrace.Fork); the returned finish completes it (a no-op
// otherwise).
//
// A params._meta.traceparent is the declared parent, preferred over the HTTP
// header for MCP (SEP-414): the capture takes its trace, and the tool
// upstream sees the gateway span as the child of it. With tracing off ctx is
// returned unchanged.
func beginMCPToolCapture(ctx context.Context, request mcp.CallToolRequest) (context.Context, func()) {
	rec := agenttrace.From(ctx)
	if rec == nil {
		return ctx, noFinish
	}
	finish := noFinish
	sc := trace.SpanContextFromContext(ctx)
	changed := false
	if detached, _ := ctx.Value(mcpDetachedKey{}).(bool); detached {
		ctx, rec = agenttrace.Fork(ctx)
		finish = rec.Finish
		sc = sc.WithSpanID(trace.SpanID(rec.SpanID()))
		changed = true
	}
	if ci := mcpClientInfo(request); ci != "" {
		rec.Update(func(ev *agenttrace.Event) { ev.Declared.ClientInfo = ci })
	}
	if parent := mcpMetaParent(request); parent.IsValid() {
		traceID := agenttrace.TraceID(parent.TraceID())
		rec.Update(func(ev *agenttrace.Event) {
			ev.TraceID = traceID
			ev.Declared.TraceID = traceID
			ev.Declared.ParentSpanID = agenttrace.SpanID(parent.SpanID())
			ev.Declared.TraceState = parent.TraceState().String()
			ev.Declared.Sampled = parent.IsSampled()
			ev.Declared.TraceSource = "mcp_meta"
		})
		sc = trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    parent.TraceID(),
			SpanID:     trace.SpanID(rec.SpanID()),
			TraceFlags: parent.TraceFlags(), // the caller's sampling decision
			Remote:     true,
		})
		changed = true
	}
	if changed && sc.IsValid() {
		ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	}
	return ctx, finish
}

func noFinish() {}

// mcpClientInfo is params._meta io.modelcontextprotocol/clientInfo as
// "name/version" (the §4.2 priority 4 discriminator), parsed and bounded by
// the declared-context extractor.
func mcpClientInfo(request mcp.CallToolRequest) string {
	meta := request.Params.Meta
	if meta == nil {
		return ""
	}
	ci, ok := meta.AdditionalFields["io.modelcontextprotocol/clientInfo"]
	if !ok {
		return ""
	}
	body, err := json.Marshal(map[string]any{"params": map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/clientInfo": ci}}})
	if err != nil {
		return ""
	}
	return declared.Default().Headers(http.Header{}, declared.BodyMCP, body).Declared.ClientInfo
}

// mcpMetaParent parses params._meta.traceparent. It is W3C trace context by
// definition, whatever propagator the gateway is configured with.
func mcpMetaParent(request mcp.CallToolRequest) trace.SpanContext {
	meta := request.Params.Meta
	if meta == nil {
		return trace.SpanContext{}
	}
	tp, _ := meta.AdditionalFields["traceparent"].(string)
	if tp == "" {
		return trace.SpanContext{}
	}
	carrier := propagation.MapCarrier{"traceparent": tp}
	if ts, ok := meta.AdditionalFields["tracestate"].(string); ok {
		carrier["tracestate"] = ts
	}
	return trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), carrier))
}

// withMCPToolCapture wraps an MCP tool handler so that, with tracing on,
// each tools/call is captured as a tool_call whichever way it ends. With
// tracing off it calls h and allocates nothing.
func (p *Proxy) withMCPToolCapture(tool *models.Tool, h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if !capturing(ctx) {
			return h(ctx, request)
		}
		ctx, finish := beginMCPToolCapture(ctx, request)
		defer finish()
		res, err := h(ctx, request)
		p.captureTool(ctx, mcpToolCapture(ctx, request, tool, res))
		return res, err
	}
}

// restCapture holds what a REST tool call's capture needs until it ends.
type restCapture struct {
	sent   scripting.ToolCallArgs // the arguments as the client sent them
	result interface{}            // the result returned, once there is one
}

// finishRESTCapture completes a REST tool call's capture (deferred by
// handleToolRequest while tracing is on), with the status the client got.
func (p *Proxy) finishRESTCapture(ctx context.Context, w http.ResponseWriter, tool *models.Tool, c *restCapture) {
	p.captureTool(ctx, restToolCapture(ctx, tool, c.sent, c.result, writtenStatus(w)))
}

// mcpToolCapture describes one MCP tools/call for captureTool. args are the
// raw params.arguments, not the rebuilt ToolCallArgs, so args_fp matches the
// model's tool call.
func mcpToolCapture(ctx context.Context, request mcp.CallToolRequest, tool *models.Tool, res *mcp.CallToolResult) toolCapture {
	c := toolCapture{
		kind:      agenttrace.KindToolCall,
		resource:  agenttrace.Resource{Type: agenttrace.ResourceTool, ID: uintString(tool.ID), Name: tool.Name},
		toolName:  request.Params.Name,
		operation: string(mcp.MethodToolsCall),
		args:      toolJSON(ctx, request.Params.Arguments),
	}
	if res != nil {
		c.toolError = res.IsError
		c.result = mcpResultText(res)
	}
	if h := request.Header; h != nil {
		c.mcpMethod = h.Get("Mcp-Method")
		c.protocolVer = h.Get("Mcp-Protocol-Version")
		c.mcpSession = h.Get("Mcp-Session-Id")
	}
	if c.mcpSession == "" {
		if s := server.ClientSessionFromContext(ctx); s != nil {
			c.mcpSession = s.SessionID()
		}
	}
	return c
}

// mcpResultText is an MCP result's text content items, joined in order.
func mcpResultText(res *mcp.CallToolResult) []byte {
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := mcp.AsTextContent(c); ok {
			b.WriteString(t.Text)
		}
	}
	if b.Len() == 0 {
		return nil
	}
	return []byte(b.String())
}

// restToolCapture describes one REST tool call for captureTool. args are the
// call's envelope as the client sent it, without its headers (they carry the
// upstream's credentials); args_fp is computed over the same arguments in the
// MCP shim's shape (single-valued parameters as scalars, the payload as
// body), so a model's tool call matches whichever transport the client used.
func restToolCapture(ctx context.Context, tool *models.Tool, in scripting.ToolCallArgs, result interface{}, status int) toolCapture {
	in.Headers = nil
	c := toolCapture{
		kind:      agenttrace.KindToolCall,
		resource:  agenttrace.Resource{Type: agenttrace.ResourceTool, ID: uintString(tool.ID), Name: tool.Name},
		toolName:  in.OperationID,
		operation: "execute_tool",
		args:      toolJSON(ctx, in),
		status:    status,
		toolError: status >= http.StatusBadRequest,
	}
	flat := make(map[string]interface{}, len(in.Parameters)+1)
	for k, v := range in.Parameters {
		if len(v) == 1 {
			flat[k] = v[0]
		} else {
			flat[k] = v
		}
	}
	if len(in.Payload) > 0 {
		flat["body"] = in.Payload
	}
	c.fpArgs, _ = json.Marshal(flat)
	if result != nil {
		if body, err := toolResultBody(result); err == nil {
			c.result = []byte(body)
		}
	}
	return c
}

// datasourceCapture describes one datasource call for captureTool. op names
// the route (search, vector_search, metadata_query, embeddings).
func datasourceCapture(ds *models.Datasource, op string, args, result []byte, status int) toolCapture {
	return toolCapture{
		kind:      agenttrace.KindDatasourceCall,
		resource:  agenttrace.Resource{Type: agenttrace.ResourceDatasource, ID: uintString(ds.ID), Name: ds.Name},
		toolName:  op,
		operation: "retrieval",
		args:      args,
		result:    result,
		status:    status,
	}
}

// noteDatasourceCall gives the request's capture its kind and resource once
// the datasource is resolved, so a call that then fails is still recorded as
// a datasource_call (finishCapture keeps the status the client got).
func noteDatasourceCall(ctx context.Context, ds *models.Datasource, op string) {
	rec := agenttrace.From(ctx)
	if rec == nil {
		return
	}
	rec.Update(func(ev *agenttrace.Event) {
		ev.Kind = agenttrace.KindDatasourceCall
		ev.Resource = agenttrace.Resource{Type: agenttrace.ResourceDatasource, ID: uintString(ds.ID), Name: ds.Name}
		ev.ToolName = op
		ev.Operation = "retrieval"
		ev.Provider = "tyk.studio"
	})
}

// toolJSON renders a value for capture; nil when tracing is off.
func toolJSON(ctx context.Context, v interface{}) []byte {
	if agenttrace.From(ctx) == nil || v == nil {
		return nil
	}
	if b, ok := v.([]byte); ok {
		return b
	}
	if s, ok := v.(string); ok {
		return []byte(s)
	}
	b, _ := json.Marshal(v)
	return b
}

// capturing reports whether the request is being captured (agent tracing on).
func capturing(ctx context.Context) bool { return agenttrace.From(ctx) != nil }

// PluginEndpointCapture is the capture of one microgateway /plugins/{slug}/
// request: a generic plugin_endpoint span with the plugin, the caller's App
// when authenticated, the status and the timing. The body is not parsed. The
// zero value (agent tracing off) does nothing.
type PluginEndpointCapture struct{ rec *agenttrace.Recorder }

// BeginPluginEndpointCapture starts the capture of a plugin endpoint request
// and returns the request carrying it (the trace context of
// beginRequestCapture, so a plugin calling out continues the trace). With
// tracing off it returns r unchanged and allocates nothing.
func BeginPluginEndpointCapture(r *http.Request, pluginID uint, slug string) (*http.Request, PluginEndpointCapture) {
	r, rec := beginRequestCapture(r)
	if rec == nil {
		return r, PluginEndpointCapture{}
	}
	method := r.Method
	rec.Update(func(ev *agenttrace.Event) {
		ev.Kind = agenttrace.KindPluginEndpoint
		ev.Resource = agenttrace.Resource{Type: agenttrace.ResourcePlugin, ID: uintString(pluginID), Name: slug}
		ev.Provider = "tyk.studio"
		if ev.Attributes == nil {
			ev.Attributes = map[string]string{}
		}
		ev.Attributes["http.request.method"] = method
	})
	return r, PluginEndpointCapture{rec: rec}
}

// SetApp records the App the request authenticated as.
func (c PluginEndpointCapture) SetApp(appID uint) {
	if c.rec == nil || appID == 0 {
		return
	}
	c.rec.Update(func(ev *agenttrace.Event) { ev.Identity.AppID = appID })
}

// Finish completes the capture with the status the client got.
func (c PluginEndpointCapture) Finish(status int) {
	if c.rec == nil {
		return
	}
	c.rec.Update(func(ev *agenttrace.Event) {
		ev.HTTPStatus = status
		if status >= http.StatusBadRequest {
			ev.Status = agenttrace.StatusError
			ev.ErrorType = strconv.Itoa(status)
		}
	})
	c.rec.Finish()
}
