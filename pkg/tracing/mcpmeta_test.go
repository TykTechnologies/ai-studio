package tracing

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/propagation"
)

// Outbound _meta carries the current span context, replacing the client's
// trace keys, with baggage reduced to the allowlist; other keys are kept.
func TestInjectMCPMeta(t *testing.T) {
	withAllowlist(t, "session.id")
	Use(nil, propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { Use(nil, nil) })

	ctx := ExtractIncoming(context.Background(), http.Header{
		"Traceparent": {"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		"Baggage":     {"session.id=s1,user.email=leak"},
	})
	meta := map[string]any{
		"traceparent":   "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		"tracestate":    "client=1",
		"progressToken": "p1",
	}
	got := MCPMeta.InjectMeta(ctx, meta)
	if got["traceparent"] != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("traceparent = %v", got["traceparent"])
	}
	if _, ok := got["tracestate"]; ok {
		t.Fatalf("the client's tracestate must be replaced, got %v", got["tracestate"])
	}
	if got["baggage"] != "session.id=s1" {
		t.Fatalf("baggage = %v", got["baggage"])
	}
	if got["progressToken"] != "p1" {
		t.Fatal("other _meta keys are kept")
	}

	if fresh := InjectMCPMeta(ctx, nil); fresh["traceparent"] == nil {
		t.Fatal("a nil _meta gets a new map")
	}
}
