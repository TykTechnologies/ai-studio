package proxy

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
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
	model := requestModelName(r, llm)

	ctx, span := tracing.Tracer().Start(
		tracing.ExtractIncoming(r.Context(), r.Header),
		// The GenAI conventions name a span "{operation} {model}".
		metrics.OperationChat+" "+model,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", metrics.OperationChat),
			attribute.String("gen_ai.provider.name", metrics.GenAIProviderName(string(llm.Vendor))),
			attribute.String("gen_ai.request.model", model),
		),
	)
	return r.WithContext(ctx), span
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
