package metrics

import (
	"context"
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	initialized atomic.Bool
	handler     http.Handler

	// Counters
	requestsTotal    otelmetric.Int64Counter
	tokensTotal      otelmetric.Int64Counter
	costTotal        otelmetric.Float64Counter
	toolCallsTotal   otelmetric.Int64Counter
	policyBlockTotal otelmetric.Int64Counter

	// Histograms
	requestDuration  otelmetric.Float64Histogram
	toolExecDuration otelmetric.Float64Histogram

	// Gauges (UpDownCounter)
	inflightRequests otelmetric.Int64UpDownCounter
)

// Init creates the OTEL-to-Prometheus bridge, registers all instruments,
// and returns an http.Handler that serves the /metrics endpoint.
func Init() http.Handler {
	registry := prometheus.NewRegistry()

	exporter, err := otelprometheus.New(
		otelprometheus.WithRegisterer(registry),
	)
	if err != nil {
		panic("failed to create prometheus exporter: " + err.Error())
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	meter := provider.Meter("aistudio")

	// Register counters
	requestsTotal, err = meter.Int64Counter("aistudio_llm_requests_total",
		otelmetric.WithDescription("Total LLM proxy requests"),
	)
	if err != nil {
		panic("failed to create requestsTotal counter: " + err.Error())
	}

	tokensTotal, err = meter.Int64Counter("aistudio_llm_tokens_total",
		otelmetric.WithDescription("Tokens consumed"),
	)
	if err != nil {
		panic("failed to create tokensTotal counter: " + err.Error())
	}

	costTotal, err = meter.Float64Counter("aistudio_llm_cost_total",
		otelmetric.WithDescription("Cumulative LLM cost"),
	)
	if err != nil {
		panic("failed to create costTotal counter: " + err.Error())
	}

	toolCallsTotal, err = meter.Int64Counter("aistudio_tool_calls_total",
		otelmetric.WithDescription("MCP / tool invocations"),
	)
	if err != nil {
		panic("failed to create toolCallsTotal counter: " + err.Error())
	}

	policyBlockTotal, err = meter.Int64Counter("aistudio_policy_blocks_total",
		otelmetric.WithDescription("Requests blocked by policy"),
	)
	if err != nil {
		panic("failed to create policyBlockTotal counter: " + err.Error())
	}

	// Register histograms
	requestDuration, err = meter.Float64Histogram("aistudio_llm_request_duration_seconds",
		otelmetric.WithDescription("End-to-end LLM request latency"),
		otelmetric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120),
	)
	if err != nil {
		panic("failed to create requestDuration histogram: " + err.Error())
	}

	toolExecDuration, err = meter.Float64Histogram("aistudio_tool_execution_duration_seconds",
		otelmetric.WithDescription("Tool call latency"),
		otelmetric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		panic("failed to create toolExecDuration histogram: " + err.Error())
	}

	// Register gauges (UpDownCounter)
	inflightRequests, err = meter.Int64UpDownCounter("aistudio_llm_inflight_requests",
		otelmetric.WithDescription("Currently in-flight LLM requests"),
	)
	if err != nil {
		panic("failed to create inflightRequests gauge: " + err.Error())
	}

	handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	initialized.Store(true)

	return handler
}

// Handler returns the Prometheus HTTP handler. Returns nil if Init() has not been called.
func Handler() http.Handler {
	return handler
}

// RecordRequest increments the LLM requests counter.
func RecordRequest(ctx context.Context, appID, vendor, model string, statusCode int) {
	if !initialized.Load() {
		return
	}
	requestsTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("app_id", appID),
			attribute.String("vendor", vendor),
			attribute.String("model", model),
			attribute.Int("status_code", statusCode),
		),
	)
}

// RecordTokens increments the token counter for a given type.
// tokenType should be one of: "prompt", "completion", "cache_read", "cache_write".
func RecordTokens(ctx context.Context, vendor, model, tokenType string, count int) {
	if !initialized.Load() || count == 0 {
		return
	}
	tokensTotal.Add(ctx, int64(count),
		otelmetric.WithAttributes(
			attribute.String("vendor", vendor),
			attribute.String("model", model),
			attribute.String("type", tokenType),
		),
	)
}

// RecordCost increments the cumulative cost counter.
func RecordCost(ctx context.Context, vendor, model, appID string, cost float64) {
	if !initialized.Load() || cost == 0 {
		return
	}
	costTotal.Add(ctx, cost,
		otelmetric.WithAttributes(
			attribute.String("vendor", vendor),
			attribute.String("model", model),
			attribute.String("app_id", appID),
		),
	)
}

// RecordToolCall increments the tool calls counter.
func RecordToolCall(ctx context.Context, toolName, appID string) {
	if !initialized.Load() {
		return
	}
	toolCallsTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("tool_name", toolName),
			attribute.String("app_id", appID),
		),
	)
}

// RecordPolicyBlock increments the policy blocks counter.
// blockType should be one of: "filter", "firewall", "rate_limit".
func RecordPolicyBlock(ctx context.Context, ruleName, blockType string) {
	if !initialized.Load() {
		return
	}
	policyBlockTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("rule_name", ruleName),
			attribute.String("block_type", blockType),
		),
	)
}

// ObserveRequestDuration records an LLM request duration observation.
func ObserveRequestDuration(ctx context.Context, vendor, model string, streaming bool, durationSeconds float64) {
	if !initialized.Load() {
		return
	}
	streamingStr := "false"
	if streaming {
		streamingStr = "true"
	}
	requestDuration.Record(ctx, durationSeconds,
		otelmetric.WithAttributes(
			attribute.String("vendor", vendor),
			attribute.String("model", model),
			attribute.String("streaming", streamingStr),
		),
	)
}

// ObserveToolDuration records a tool execution duration observation.
func ObserveToolDuration(ctx context.Context, toolName string, durationSeconds float64) {
	if !initialized.Load() {
		return
	}
	toolExecDuration.Record(ctx, durationSeconds,
		otelmetric.WithAttributes(
			attribute.String("tool_name", toolName),
		),
	)
}

// IncrementInflight increments the in-flight request gauge.
func IncrementInflight(ctx context.Context, vendor string) {
	if !initialized.Load() {
		return
	}
	inflightRequests.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("vendor", vendor),
		),
	)
}

// DecrementInflight decrements the in-flight request gauge.
func DecrementInflight(ctx context.Context, vendor string) {
	if !initialized.Load() {
		return
	}
	inflightRequests.Add(ctx, -1,
		otelmetric.WithAttributes(
			attribute.String("vendor", vendor),
		),
	)
}
