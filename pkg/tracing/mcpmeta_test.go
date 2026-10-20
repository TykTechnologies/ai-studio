package tracing

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/propagation"
)

// Outbound headers never carry a caller's tracestate, and an allowlisted
// baggage member loses its properties (jyx.89).
func TestOutboundTraceContextHygiene(t *testing.T) {
	withAllowlist(t, "session.id")
	Use(nil, propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { Use(nil, nil) })

	ctx := ExtractIncoming(context.Background(), http.Header{
		"Traceparent": {"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		"Tracestate":  {"acme=c2VjcmV0LXRva2Vu,vendor=abc"},
		"Baggage":     {"session.id=s1;user.email=leak%40x;pii=yes"},
	})
	out := http.Header{"tracestate": {"raw=1"}}
	InjectOutgoing(ctx, out)
	if out.Get("Traceparent") == "" {
		t.Fatal("traceparent not propagated")
	}
	for k := range out {
		if http.CanonicalHeaderKey(k) == "Tracestate" {
			t.Fatalf("tracestate forwarded: %v", out)
		}
	}
	if got := out.Get("Baggage"); got != "session.id=s1" {
		t.Fatalf("baggage = %q", got)
	}
}

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

	// AGENT_TRACING_PROPAGATE_MCP=false: no trace keys leave, the client's
	// included.
	SetOutboundPropagation(true, true, false)
	t.Cleanup(func() { SetOutboundPropagation(true, true, true) })
	off := InjectMCPMeta(ctx, map[string]any{"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", "progressToken": "p1"})
	if _, ok := off["traceparent"]; ok || off["progressToken"] != "p1" {
		t.Fatalf("propagation off: %v", off)
	}
}
