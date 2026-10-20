package proxy

import (
	"context"
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
)

// hdrInternalSpan carries the outer /ai/ call's gateway span id across the
// loopback hop, so the inner request (and each failover rung) is recorded as
// an attempt of that call. It is trusted only with the hop token
// (IsInternalHop) and never leaves for a vendor (stripFailoverHeaders).
const hdrInternalSpan = "X-Tyk-Internal-Span"

// beginRequestCapture gives the request its canonical gateway span id when
// agent tracing is on (spec §4.1): it attaches the capture recorder, reads
// the context the client declared in its headers (declared.Default: trace
// context, X-Tyk-* headers, compat families, baggage, coding-harness session
// ids, X-Tyk-Trace-Capture), takes the trace id from it or mints one, and
// makes the gateway span the current span context, so upstream calls carry
// it as their parent. It runs before StripTracingHeaders removes those
// headers. With tracing off it returns r unchanged and allocates nothing.
func beginRequestCapture(r *http.Request) (*http.Request, *agenttrace.Recorder) {
	ctx, rec := agenttrace.Begin(r.Context())
	if rec == nil {
		return r, nil
	}
	if IsInternalHop(r) {
		if outer, ok := agenttrace.ParseSpanID(r.Header.Get(hdrInternalSpan)); ok {
			rec.MarkAttemptOf(outer)
		}
	}

	// Headers only: an LLM body's fields are read on the analytics
	// goroutine (applyDeclaredBody), never on the request path.
	res := declared.Default().Headers(r.Header, "", nil)
	ctx = context.WithValue(ctx, declaredKey{}, &res)

	ctx = tracing.ExtractIncoming(ctx, r.Header)
	parent := trace.SpanContextFromContext(ctx)
	traceID := res.Declared.TraceID
	if traceID.IsZero() && parent.IsValid() {
		// A trace context the configured propagator read and the W3C
		// parser did not (a host's own propagator).
		traceID = agenttrace.TraceID(parent.TraceID())
		res.Declared.TraceID = traceID
		res.Declared.ParentSpanID = agenttrace.SpanID(parent.SpanID())
		res.Declared.TraceState = parent.TraceState().String()
		res.Declared.Sampled = parent.IsSampled()
		res.Declared.TraceSource = "traceparent"
	}
	if traceID.IsZero() {
		traceID = agenttrace.NewTraceID()
	}
	rec.SetTraceID(traceID)
	d := res.Declared
	rec.Update(func(ev *agenttrace.Event) { ev.Declared = d })
	// The caller's sampling decision stands; a trace the gateway minted (or
	// took from a header without flags) is sampled. The caller's tracestate
	// is not carried on: it is recorded as declared, never forwarded.
	flags := trace.FlagsSampled
	if res.Declared.TraceSource == "traceparent" && !res.Declared.Sampled {
		flags = 0
	}
	gateway := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID(traceID),
		SpanID:     trace.SpanID(rec.SpanID()),
		TraceFlags: flags,
		Remote:     true,
	})
	return r.WithContext(trace.ContextWithRemoteSpanContext(ctx, gateway)), rec
}

// declaredKey holds the request's declared.Result while tracing is on, for
// the body fields read after the response (applyDeclaredBody).
type declaredKey struct{}

// applyDeclaredBody adds an LLM request body's declared fields (§4.2
// priority 4: conversation, metadata.session_id, user, and Claude Code's
// metadata.user_id session) to the capture, below every header source. It
// runs on the analytics goroutine.
func applyDeclaredBody(ctx context.Context, rec *agenttrace.Recorder, body []byte) {
	stored, _ := ctx.Value(declaredKey{}).(*declared.Result)
	if rec == nil || stored == nil || len(body) == 0 {
		return
	}
	res := *stored
	declared.Default().ApplyBody(&res, body)
	d := res.Declared
	rec.Update(func(ev *agenttrace.Event) {
		// Fields a later stage set (an MCP session hint) are kept.
		if d.MCPSessionID == "" {
			d.MCPSessionID = ev.Declared.MCPSessionID
		}
		ev.Declared = d
	})
}

// traceSpanIDFor is the proxy_logs.trace_span_id for a request: the logical
// call's gateway span id, shared by every failover rung of an /ai/ call.
// Empty with tracing off.
func traceSpanIDFor(ctx context.Context) string {
	return agenttrace.From(ctx).LogicalSpanID().String()
}

// injectUpstreamTrace continues the trace into an LLM upstream (traceparent,
// tracestate, and baggage reduced to the allowlist), unless the LLM opted out
// of propagation or AGENT_TRACING_PROPAGATE_LLM_PROVIDERS is off, in which
// case it removes all three (decision Q11). With agent tracing on, an OpenAI
// upstream also gets the gateway span id as X-Client-Request-Id, so the
// provider's logs can be matched with the span
// (AGENT_TRACING_OPENAI_CLIENT_REQUEST_ID), unless the LLM opted out.
func injectUpstreamTrace(ctx context.Context, h http.Header, llm *models.LLM) {
	if llm != nil && llm.Vendor == models.OPENAI && !llm.TracingPropagationOptOut &&
		openAIClientRequestID.Load() && h.Get(hdrClientRequestID) == "" {
		if id := agenttrace.From(ctx).LogicalSpanID(); !id.IsZero() {
			h.Set(hdrClientRequestID, id.String())
		}
	}
	if (llm != nil && llm.TracingPropagationOptOut) || !tracing.PropagateToLLMProviders() {
		tracing.StripTraceContext(h)
		return
	}
	tracing.InjectOutgoing(ctx, h)
}

// hdrClientRequestID is OpenAI's client-supplied request id header.
const hdrClientRequestID = "X-Client-Request-Id"

var openAIClientRequestID atomic.Bool

func init() { openAIClientRequestID.Store(true) }

// SetOpenAIClientRequestID switches the X-Client-Request-Id an OpenAI
// upstream gets while agent tracing is on (default on).
func SetOpenAIClientRequestID(on bool) { openAIClientRequestID.Store(on) }

// callToolOperation calls a tool operation with the request's trace context
// in the call's headers, so the tool upstream continues the trace. It always
// goes through the service's CallToolOperation: a type assertion for a
// context-aware variant would pick a method promoted from an embedded
// service and bypass a wrapper's override of CallToolOperation.
func (p *Proxy) callToolOperation(ctx context.Context, toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	if trace.SpanContextFromContext(ctx).IsValid() && tracing.PropagateToHTTPUpstreams() {
		h := make(map[string][]string, len(headers)+2)
		for k, v := range headers {
			h[k] = v
		}
		tracing.InjectOutgoing(ctx, h)
		headers = h
	}
	return p.gatewayService.CallToolOperation(toolID, operationID, params, payload, headers)
}

// traceLoopbackHeaders adds the outer request's trace context to the
// headers of an /ai/ loopback attempt. The attempt runs on a context detached
// from the request's, so the SDK's call would otherwise carry no traceparent
// and the inner hop would start a trace of its own. With agent tracing on it
// also passes the outer call's gateway span id (hdrInternalSpan). With no
// trace context and tracing off it changes nothing.
func traceLoopbackHeaders(ctx context.Context, h http.Header) http.Header {
	rec := agenttrace.From(ctx)
	if rec == nil && !trace.SpanContextFromContext(ctx).IsValid() {
		return h
	}
	if h == nil {
		h = http.Header{}
	}
	if rec != nil {
		h.Set(hdrInternalSpan, rec.SpanID().String())
	}
	tracing.InjectOutgoing(ctx, h)
	return h
}
