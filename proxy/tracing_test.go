package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
)

// withSpanRecorder installs a real (non-noop) tracer provider so span content
// can be asserted, and restores the previous one afterwards.
func withSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	// Both entrypoints call tracing.Init before serving, which is what installs
	// the W3C propagator. Without it ExtractIncoming/InjectOutgoing are silently
	// no-ops, so the test harness must mirror startup here.
	if _, err := tracing.Init(context.Background(), tracing.Config{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	return recorder
}

func spanAttr(t *testing.T, attrs []attribute.KeyValue, key string) string {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func TestStartProxySpanAttributes(t *testing.T) {
	recorder := withSpanRecorder(t)

	llm := &models.LLM{Name: "Bedrock Claude", Vendor: models.BEDROCK, DefaultModel: "claude-sonnet-4"}
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/bedrock-claude/v1/chat/completions", nil)

	_, span := startProxySpan(r, llm)
	endProxySpan(span, http.StatusOK)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	got := spans[0]

	// The conventions name a span "{operation} {model}".
	if got.Name() != "chat claude-sonnet-4" {
		t.Errorf("span name = %q, want %q", got.Name(), "chat claude-sonnet-4")
	}
	if v := spanAttr(t, got.Attributes(), "gen_ai.operation.name"); v != "chat" {
		t.Errorf("gen_ai.operation.name = %q, want chat", v)
	}
	// The vendor must be mapped to the registered provider value, not passed raw.
	if v := spanAttr(t, got.Attributes(), "gen_ai.provider.name"); v != "aws.bedrock" {
		t.Errorf("gen_ai.provider.name = %q, want aws.bedrock", v)
	}
	if v := spanAttr(t, got.Attributes(), "gen_ai.request.model"); v != "claude-sonnet-4" {
		t.Errorf("gen_ai.request.model = %q, want claude-sonnet-4", v)
	}
	if got.Status().Code == codes.Error {
		t.Error("a 200 response must not mark the span as an error")
	}
}

func TestEndProxySpanRecordsFailures(t *testing.T) {
	recorder := withSpanRecorder(t)

	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, DefaultModel: "gpt-4o"}
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/openai/v1/chat/completions", nil)

	_, span := startProxySpan(r, llm)
	endProxySpan(span, http.StatusTooManyRequests)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("status code = %v, want Error for a 429", spans[0].Status().Code)
	}

	var found bool
	for _, a := range spans[0].Attributes() {
		if string(a.Key) == "http.response.status_code" && a.Value.AsInt64() == 429 {
			found = true
		}
	}
	if !found {
		t.Error("missing http.response.status_code=429 on the failed span")
	}
}

// TestStartProxySpanJoinsInboundTrace is the point of extracting the inbound
// headers: the gateway must continue the caller's trace, not start a new one.
func TestStartProxySpanJoinsInboundTrace(t *testing.T) {
	recorder := withSpanRecorder(t)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/openai/v1/chat/completions", nil)
	r.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")

	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, DefaultModel: "gpt-4o"}
	_, span := startProxySpan(r, llm)
	endProxySpan(span, http.StatusOK)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != traceID {
		t.Errorf("trace ID = %s, want %s — the gateway started a new trace instead of joining", got, traceID)
	}
	if !spans[0].Parent().IsValid() {
		t.Error("span has no parent; the inbound traceparent was not extracted")
	}
}

// TestProxySpanContextIsInjectable covers the other half of propagation: the
// context returned by startProxySpan must carry a trace the director can inject
// into the upstream request.
func TestProxySpanContextIsInjectable(t *testing.T) {
	withSpanRecorder(t)

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/openai/v1/chat/completions", nil)
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, DefaultModel: "gpt-4o"}

	r, span := startProxySpan(r, llm)
	defer endProxySpan(span, http.StatusOK)

	upstream := http.Header{}
	tracing.InjectOutgoing(r.Context(), upstream)

	if upstream.Get("traceparent") == "" {
		t.Fatal("no traceparent injected into the upstream request headers")
	}
}

func TestRequestModelNameFallbackChain(t *testing.T) {
	base := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)

	t.Run("prefers the model parsed from the request body", func(t *testing.T) {
		r := base.WithContext(context.WithValue(base.Context(), "model_name", "gpt-4o-mini"))
		llm := &models.LLM{Name: "OpenAI", DefaultModel: "gpt-4o"}
		if got := requestModelName(r, llm); got != "gpt-4o-mini" {
			t.Errorf("got %q, want the context model", got)
		}
	})

	t.Run("falls back to the LLM default model", func(t *testing.T) {
		llm := &models.LLM{Name: "OpenAI", DefaultModel: "gpt-4o"}
		if got := requestModelName(base, llm); got != "gpt-4o" {
			t.Errorf("got %q, want the default model", got)
		}
	})

	t.Run("falls back to the LLM name when there is no default", func(t *testing.T) {
		llm := &models.LLM{Name: "Custom Pool"}
		if got := requestModelName(base, llm); got != "Custom Pool" {
			t.Errorf("got %q, want the LLM name", got)
		}
	})

	t.Run("ignores an empty or wrongly typed context value", func(t *testing.T) {
		llm := &models.LLM{Name: "OpenAI", DefaultModel: "gpt-4o"}

		empty := base.WithContext(context.WithValue(base.Context(), "model_name", ""))
		if got := requestModelName(empty, llm); got != "gpt-4o" {
			t.Errorf("empty context model: got %q, want the default", got)
		}

		wrongType := base.WithContext(context.WithValue(base.Context(), "model_name", 42))
		if got := requestModelName(wrongType, llm); got != "gpt-4o" {
			t.Errorf("non-string context model: got %q, want the default", got)
		}
	})
}
