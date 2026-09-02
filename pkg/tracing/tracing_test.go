package tracing

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		raw          string
		wantEndpoint string
		wantInsecure bool
		wantErr      bool
	}{
		{raw: "localhost:4317", wantEndpoint: "localhost:4317", wantInsecure: true},
		{raw: "  otel-collector:4317  ", wantEndpoint: "otel-collector:4317", wantInsecure: true},
		{raw: "http://collector.observability:4317", wantEndpoint: "collector.observability:4317", wantInsecure: true},
		{raw: "https://collector.example.com:4317", wantEndpoint: "collector.example.com:4317", wantInsecure: false},
		{raw: "https://", wantErr: true},
	}

	for _, tc := range cases {
		endpoint, insecure, err := parseEndpoint(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseEndpoint(%q) expected an error, got %q", tc.raw, endpoint)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseEndpoint(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if endpoint != tc.wantEndpoint {
			t.Errorf("parseEndpoint(%q) endpoint = %q, want %q", tc.raw, endpoint, tc.wantEndpoint)
		}
		if insecure != tc.wantInsecure {
			t.Errorf("parseEndpoint(%q) insecure = %v, want %v", tc.raw, insecure, tc.wantInsecure)
		}
	}
}

func TestInitDisabledIsNoOpButInstallsPropagator(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("disabled Init should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil so callers can defer it unconditionally")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown should not error: %v", err)
	}

	// The propagator must be installed even with export off, so a caller's trace
	// survives the hop through the gateway.
	in := http.Header{}
	in.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	ctx := ExtractIncoming(context.Background(), in)

	out := http.Header{}
	InjectOutgoing(ctx, out)

	if got := out.Get("traceparent"); got != in.Get("traceparent") {
		t.Errorf("traceparent not propagated: got %q, want %q", got, in.Get("traceparent"))
	}
}

func TestInitEnabledWithoutEndpointFails(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Enabled: true, Endpoint: "   "})
	if err == nil {
		t.Fatal("expected an error when tracing is enabled with no endpoint")
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil even on the error path")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("fallback shutdown should not error: %v", err)
	}
}

func TestTracerIsUsableWhenDisabled(t *testing.T) {
	if _, err := Init(context.Background(), Config{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	// Span creation against the no-op provider must be safe on the request path.
	_, span := Tracer().Start(context.Background(), "chat gpt-4o")
	span.End()
}

// TestInitEnabledInstallsRealProvider covers the path that actually matters when
// the feature is switched on. The OTLP gRPC exporter connects lazily, so this
// needs no live collector — but it does prove Init wires a real provider rather
// than leaving the no-op in place.
func TestInitEnabledInstallsRealProvider(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{
		Enabled:        true,
		Endpoint:       "127.0.0.1:4317",
		ServiceName:    "tyk-microgateway",
		ServiceVersion: "test",
	})
	if err != nil {
		t.Fatalf("Init with a valid endpoint should succeed: %v", err)
	}
	t.Cleanup(func() {
		// Shutdown flushes to a collector that is not there; bound the wait so
		// the suite does not pay the full internal timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Logf("shutdown returned %v (expected when no collector is listening)", err)
		}
	})

	ctx, span := Tracer().Start(context.Background(), "chat gpt-4o")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Fatal("span context is invalid; a no-op provider is still installed")
	}
	if !span.SpanContext().IsSampled() {
		t.Error("span is not sampled; nothing would ever be exported")
	}

	// A real span must be injectable so the upstream continues the trace.
	out := http.Header{}
	InjectOutgoing(ctx, out)
	if out.Get("traceparent") == "" {
		t.Error("a live span produced no traceparent")
	}
}

// TestInitEnabledOverHTTPS exercises the TLS branch of endpoint parsing end to
// end, which the parseEndpoint unit test only covers in isolation.
func TestInitEnabledOverHTTPS(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{
		Enabled:     true,
		Endpoint:    "https://collector.example.com:4317",
		ServiceName: "tyk-ai-studio",
	})
	if err != nil {
		t.Fatalf("Init over https should succeed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = shutdown(ctx)
	})

	_, span := Tracer().Start(context.Background(), "chat gpt-4o")
	defer span.End()
	if !span.SpanContext().IsValid() {
		t.Error("span context is invalid; the https endpoint did not yield a real provider")
	}
}

// TestShutdownIsIdempotent guards the deferred-shutdown pattern both entrypoints
// use: a second call must not panic.
func TestShutdownIsIdempotent(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{
		Enabled:     true,
		Endpoint:    "127.0.0.1:4317",
		ServiceName: "tyk-microgateway",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = shutdown(ctx)
	_ = shutdown(ctx)
}
