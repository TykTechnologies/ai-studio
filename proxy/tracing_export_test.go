package proxy

import (
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// While the agent-trace OTel exporter runs, a captured request's assembled
// span is exported under the gateway span id; the proxy then leaves out its
// live span so one call is one span (jyx.32). Uncaptured requests, and any
// request while export is off, keep the live span.
func TestProxySpan_LeftOutWhileAgentTraceExportRuns(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		agenttrace.SetEnabled(false)
		agenttrace.SetExportActive(false)
	})
	llm := &models.LLM{Name: "gpt", DefaultModel: "gpt-4.1", Vendor: models.OPENAI}
	call := func(captured bool) {
		r := httptest.NewRequest("POST", "/llm/rest/gpt/v1/chat/completions", nil)
		if captured {
			ctx, rec := agenttrace.Begin(r.Context())
			if rec == nil {
				t.Fatal("no recorder")
			}
			r = r.WithContext(ctx)
		}
		_, span := startProxySpan(r, llm)
		endProxySpan(span, 200)
	}

	agenttrace.SetEnabled(true)
	call(true)
	if n := len(exp.GetSpans()); n != 1 {
		t.Fatalf("export off: want the live span, got %d", n)
	}
	exp.Reset()

	agenttrace.SetExportActive(true)
	call(true)
	if n := len(exp.GetSpans()); n != 0 {
		t.Fatalf("export on, captured: want no live span, got %d", n)
	}
	agenttrace.SetEnabled(false)
	call(false)
	if n := len(exp.GetSpans()); n != 1 {
		t.Fatalf("export on, not captured: want the live span, got %d", n)
	}
}
