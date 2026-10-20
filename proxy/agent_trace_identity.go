package proxy

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
)

// hdrInternalSpan carries the outer /ai/ call's gateway span id across the
// loopback hop, so the inner request (and each failover rung) is recorded as
// an attempt of that call. It is trusted only with the hop token
// (IsInternalHop) and never leaves for a vendor (stripFailoverHeaders).
const hdrInternalSpan = "X-Tyk-Internal-Span"

// beginRequestCapture gives the request its canonical gateway span id when
// agent tracing is on (spec §4.1): it attaches the capture recorder, takes
// the trace id from the client's traceparent or mints one, and makes the
// gateway span the current span context, so upstream calls carry it as
// their parent. With tracing off it returns r unchanged and allocates
// nothing.
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

	ctx = tracing.ExtractIncoming(ctx, r.Header)
	parent := trace.SpanContextFromContext(ctx)
	traceID := agenttrace.TraceID(parent.TraceID())
	if !parent.IsValid() {
		traceID = agenttrace.NewTraceID()
	}
	rec.SetTraceID(traceID)
	if parent.IsValid() {
		parentID := agenttrace.SpanID(parent.SpanID())
		rec.Update(func(ev *agenttrace.Event) {
			ev.Declared.TraceID = traceID
			ev.Declared.ParentSpanID = parentID
			ev.Declared.TraceState = parent.TraceState().String()
			ev.Declared.Sampled = parent.IsSampled()
			ev.Declared.TraceSource = "traceparent"
		})
	}
	gateway := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID(traceID),
		SpanID:     trace.SpanID(rec.SpanID()),
		TraceFlags: parent.TraceFlags() | trace.FlagsSampled,
		TraceState: parent.TraceState(),
		Remote:     true,
	})
	return r.WithContext(trace.ContextWithRemoteSpanContext(ctx, gateway)), rec
}

// traceSpanIDFor is the proxy_logs.trace_span_id for a request: the logical
// call's gateway span id, shared by every failover rung of an /ai/ call.
// Empty with tracing off.
func traceSpanIDFor(ctx context.Context) string {
	return agenttrace.From(ctx).LogicalSpanID().String()
}

// injectUpstreamTrace continues the trace into an LLM upstream (traceparent,
// tracestate, and baggage reduced to the allowlist), unless the LLM opted out
// of propagation, in which case it removes all three (decision Q11).
func injectUpstreamTrace(ctx context.Context, h http.Header, llm *models.LLM) {
	if llm != nil && llm.TracingPropagationOptOut {
		tracing.StripTraceContext(h)
		return
	}
	tracing.InjectOutgoing(ctx, h)
}

// contextToolCaller is implemented by gateway services whose tool calls take
// the request context (services.Service, the microgateway adapter).
type contextToolCaller interface {
	CallToolOperationContext(ctx context.Context, toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error)
}

// callToolOperation calls a tool operation bound to ctx, so the upstream
// call is cancelled with the request and carries its traceparent. Services
// without the context-aware method get the old call.
func (p *Proxy) callToolOperation(ctx context.Context, toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	if c, ok := p.gatewayService.(contextToolCaller); ok {
		return c.CallToolOperationContext(ctx, toolID, operationID, params, payload, headers)
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
