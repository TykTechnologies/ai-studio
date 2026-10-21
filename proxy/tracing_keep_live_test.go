package proxy

import (
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// An edge that keeps its live span (AGENT_TRACING_KEEP_LIVE_SPAN, jyx.176)
// emits it for a captured request while export runs: with its own span id,
// a link to the gateway span id the assembled span uses, and the request
// left on the gateway span so the upstream's parent does not change.
func TestProxySpan_KeptWithOwnIDAndLinkWhenOptedIn(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		agenttrace.SetEnabled(false)
		agenttrace.SetExportActive(false)
		agenttrace.SetKeepLiveSpan(false)
	})
	llm := &models.LLM{Name: "gpt", DefaultModel: "gpt-4.1", Vendor: models.OPENAI}
	agenttrace.SetEnabled(true)
	agenttrace.SetExportActive(true)
	agenttrace.SetKeepLiveSpan(true)

	r, _ := beginRequestCapture(httptest.NewRequest("POST", "/llm/rest/gpt/v1/chat/completions", nil))
	gateway := trace.SpanContextFromContext(r.Context())
	if !gateway.IsValid() {
		t.Fatal("no gateway span context")
	}
	out, span := startProxySpan(r, llm)
	endProxySpan(span, 200)

	if !trace.SpanContextFromContext(out.Context()).Equal(gateway) {
		t.Fatal("the request must stay on the gateway span (the upstream's parent)")
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("want the live span, got %d", len(spans))
	}
	live := spans[0]
	if live.SpanContext.SpanID() == gateway.SpanID() {
		t.Fatal("the live span needs its own span id")
	}
	if len(live.Links) != 1 || live.Links[0].SpanContext.SpanID() != gateway.SpanID() || live.Links[0].SpanContext.TraceID() != gateway.TraceID() {
		t.Fatalf("want a link to the gateway span, got %+v", live.Links)
	}
	for _, kv := range live.Attributes {
		if k := string(kv.Key); k == "gen_ai.input.messages" || k == "gen_ai.output.messages" {
			t.Fatalf("the live span carries no content: %s", k)
		}
	}

	// Off again: the live span is left out, as before.
	exp.Reset()
	agenttrace.SetKeepLiveSpan(false)
	r2, _ := beginRequestCapture(httptest.NewRequest("POST", "/llm/rest/gpt/v1/chat/completions", nil))
	_, span = startProxySpan(r2, llm)
	endProxySpan(span, 200)
	if n := len(exp.GetSpans()); n != 0 {
		t.Fatalf("opt-in off: want no live span, got %d", n)
	}
}
