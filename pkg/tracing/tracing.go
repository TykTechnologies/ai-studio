// Package tracing wires OpenTelemetry distributed tracing for the LLM proxy.
//
// ENABLE_TRACING and TRACING_ENDPOINT have been parsed into the gateway config
// for some time without anything behind them; this package is what they now
// drive. When tracing is disabled the global provider stays OpenTelemetry's
// no-op implementation, so span creation on the request path costs nothing and
// callers never need to branch on whether it is on.
package tracing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TracerName is the instrumentation scope for every span this package's callers
// create.
const TracerName = "aistudio/proxy"

// Config describes how to export spans. It mirrors the existing
// ObservabilityConfig fields rather than introducing new ones.
type Config struct {
	Enabled bool
	// Endpoint is the OTLP collector address. A bare "host:port" is assumed to
	// be an insecure gRPC endpoint; an http:// or https:// URL sets the
	// transport security from the scheme.
	Endpoint       string
	ServiceName    string
	ServiceVersion string
}

// Shutdown flushes and stops the exporter. It is always non-nil, so callers can
// defer it unconditionally.
type Shutdown func(context.Context) error

func noopShutdown(context.Context) error { return nil }

// Init installs a global TracerProvider and the W3C trace context propagator.
//
// The propagator is installed even when tracing is disabled: it costs nothing,
// and it means an incoming traceparent is still carried through to the upstream
// provider so a caller's own tracing stays connected across the gateway.
func Init(ctx context.Context, cfg Config) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled {
		return noopShutdown, nil
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return noopShutdown, fmt.Errorf("tracing is enabled but no endpoint is configured")
	}

	endpoint, insecure, err := parseEndpoint(cfg.Endpoint)
	if err != nil {
		return noopShutdown, err
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return noopShutdown, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	attrs := []attribute.KeyValue{attribute.String("service.name", cfg.ServiceName)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, attribute.String("service.version", cfg.ServiceVersion))
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attrs...)),
	)
	otel.SetTracerProvider(provider)

	return func(ctx context.Context) error {
		// Bound the flush so a wedged collector cannot hold up shutdown past the
		// gateway's own SHUTDOWN_TIMEOUT.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return provider.Shutdown(ctx)
	}, nil
}

// parseEndpoint normalises the configured endpoint to the host:port form the
// OTLP gRPC exporter expects, and reports whether to dial without TLS.
func parseEndpoint(raw string) (endpoint string, insecure bool, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		return raw, true, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("invalid tracing endpoint %q: %w", raw, err)
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("tracing endpoint %q has no host", raw)
	}
	return u.Host, u.Scheme != "https", nil
}

// Tracer returns the tracer every proxy span is created from.
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

// ExtractIncoming returns a context carrying the trace parent from the inbound
// request headers, so gateway spans become children of the caller's trace
// rather than starting a disconnected one.
func ExtractIncoming(ctx context.Context, header map[string][]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
}

// InjectOutgoing writes the current trace context into the upstream request
// headers so the provider (or an in-cluster model server) can continue the trace.
func InjectOutgoing(ctx context.Context, header map[string][]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
