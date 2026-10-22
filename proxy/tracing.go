package proxy

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
)

// startProxySpan opens the span covering one LLM proxy hop and returns the
// request rebound to the span's context, so everything downstream — including
// the upstream call — is a child of it.
//
// When tracing is disabled the global provider is OpenTelemetry's no-op, so this
// costs a couple of allocations and nothing leaves the process. The inbound
// traceparent is extracted regardless, which is what makes the gateway span join
// the caller's trace rather than starting an unrelated one.
func startProxySpan(r *http.Request, llm *models.LLM) (*http.Request, trace.Span) {
	// With agent-trace OTel export running, the assembled span is exported
	// under the gateway span id the upstream already sees as its parent; a
	// live span here would describe the same call twice (jyx.32).
	if agenttrace.ExportActive() && agenttrace.From(r.Context()) != nil {
		if !agenttrace.KeepLiveSpan() {
			return r, trace.SpanFromContext(r.Context())
		}
		// An edge that keeps its live span (jyx.176, its own collector): the
		// span gets its own id and a link to the gateway span id, and the
		// request keeps the gateway span as current, so the upstream's
		// parent stays the id the assembled span is exported under.
		gateway := trace.SpanContextFromContext(r.Context())
		_, span := tracing.Tracer().Start(r.Context(), metrics.OperationChat+" "+requestModelName(r, llm),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithLinks(trace.Link{SpanContext: gateway}),
			trace.WithAttributes(liveSpanAttributes(r, llm)...),
		)
		return r, span
	}
	model := requestModelName(r, llm)

	// With agent tracing on, the request already carries the gateway span
	// context (beginRequestCapture), itself a child of the caller's.
	parent := r.Context()
	if !trace.SpanContextFromContext(parent).IsValid() {
		parent = tracing.ExtractIncoming(parent, r.Header)
	}
	ctx, span := tracing.Tracer().Start(
		parent,
		// The GenAI conventions name a span "{operation} {model}".
		metrics.OperationChat+" "+model,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(liveSpanAttributes(r, llm)...),
	)
	return r.WithContext(ctx), span
}

// liveSpanAttributes are the live span's metadata attributes; it never
// carries content.
func liveSpanAttributes(r *http.Request, llm *models.LLM) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("gen_ai.operation.name", metrics.OperationChat),
		attribute.String("gen_ai.provider.name", metrics.GenAIProviderName(string(llm.Vendor))),
		attribute.String("gen_ai.request.model", requestModelName(r, llm)),
	}
}

// endProxySpan records the outcome and closes the span.
func endProxySpan(span trace.Span, statusCode int) {
	if statusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(statusCode))
		span.SetAttributes(attribute.Int("http.response.status_code", statusCode))
	}
	span.End()
}

// requestModelName resolves the model this request targets. The validation
// middleware puts the parsed body model on the context; the LLM's own default is
// the fallback for routes that skip it.
func requestModelName(r *http.Request, llm *models.LLM) string {
	if v := r.Context().Value("model_name"); v != nil {
		if model, ok := v.(string); ok && model != "" {
			return model
		}
	}
	if llm.DefaultModel != "" {
		return llm.DefaultModel
	}
	return llm.Name
}
