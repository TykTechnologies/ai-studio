package metrics

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	initialized atomic.Bool
	handler     http.Handler
	registry    *prometheus.Registry

	// Counters
	requestsTotal    otelmetric.Int64Counter
	tokensTotal      otelmetric.Int64Counter
	costTotal        otelmetric.Float64Counter
	toolCallsTotal   otelmetric.Int64Counter
	policyBlockTotal      otelmetric.Int64Counter
	complianceEventsTotal otelmetric.Int64Counter
	failoverTotal         otelmetric.Int64Counter
	guardrailErrorsTotal  otelmetric.Int64Counter
	guardrailSegmentsTotal otelmetric.Int64Counter
	guardrailCacheTotal    otelmetric.Int64Counter
	guardrailAbandonedTotal otelmetric.Int64Counter
	guardrailShadowDropped  otelmetric.Int64Counter
	guardrailTemplatesTotal otelmetric.Int64Counter
	goroutinePanicsTotal  otelmetric.Int64Counter
	decisionQuestionsTotal otelmetric.Int64Counter

	// Histograms
	requestDuration  otelmetric.Float64Histogram
	toolExecDuration otelmetric.Float64Histogram
	guardrailLatency otelmetric.Float64Histogram
	guardrailTemplateExempt otelmetric.Float64Histogram

	// Gauges (UpDownCounter)
	inflightRequests otelmetric.Int64UpDownCounter
)

// Init creates the OTEL-to-Prometheus bridge, registers all instruments,
// and returns an http.Handler that serves the /metrics endpoint.
func Init() http.Handler {
	registry = prometheus.NewRegistry()
	// Standard Go runtime and process metrics (goroutines, heap, RSS, open
	// FDs, CPU), so capacity and soak runs can watch for leaks.
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	exporter, err := otelprometheus.New(
		otelprometheus.WithRegisterer(registry),
	)
	if err != nil {
		panic("failed to create prometheus exporter: " + err.Error())
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	registerInstruments(provider.Meter("aistudio"))

	handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	initialized.Store(true)

	return handler
}

// InitWithProvider registers Studio's instruments on provider, which an
// embedding host exports however it likes. Unlike Init it neither creates a
// Prometheus registry nor sets the global MeterProvider: Handler stays nil and
// Register returns an error.
func InitWithProvider(provider otelmetric.MeterProvider) {
	registry, handler = nil, nil
	registerInstruments(provider.Meter("aistudio"))
	initialized.Store(true)
}

// registerInstruments creates every instrument on meter.
func registerInstruments(meter otelmetric.Meter) {
	var err error

	// Register the OpenTelemetry GenAI semantic convention instruments (genai.go)
	// before the aistudio_* ones, so legacyNames is set for every recorder below.
	initGenAI(meter)
	initNative(meter)
	initAgentTracing(meter)
	initAgentMetrics(meter)

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

	complianceEventsTotal, err = meter.Int64Counter("aistudio_compliance_events_total",
		otelmetric.WithDescription("Compliance events reported by filter scripts"),
	)
	if err != nil {
		panic("failed to create complianceEventsTotal counter: " + err.Error())
	}

	failoverTotal, err = meter.Int64Counter("aistudio_llm_failover_total",
		otelmetric.WithDescription("LLM requests re-routed to a failover target"),
	)
	if err != nil {
		panic("failed to create failoverTotal counter: " + err.Error())
	}

	guardrailErrorsTotal, err = meter.Int64Counter("aistudio_guardrail_errors_total",
		otelmetric.WithDescription("Guardrail provider calls that failed or timed out, by provider and the fail mode applied"),
	)
	if err != nil {
		panic("failed to create guardrailErrorsTotal counter: " + err.Error())
	}

	guardrailSegmentsTotal, err = meter.Int64Counter("aistudio_guardrail_segments_total",
		otelmetric.WithDescription("Request segments a guardrail screened, by provider, segment kind and result (scanned, cached, unscanned)"),
	)
	if err != nil {
		panic("failed to create guardrailSegmentsTotal counter: " + err.Error())
	}

	guardrailCacheTotal, err = meter.Int64Counter("aistudio_guardrail_cache_total",
		otelmetric.WithDescription("Guardrail verdict cache lookups, by provider and outcome (hit or miss)"),
	)
	if err != nil {
		panic("failed to create guardrailCacheTotal counter: " + err.Error())
	}

	guardrailAbandonedTotal, err = meter.Int64Counter("aistudio_guardrail_calls_abandoned_total",
		otelmetric.WithDescription("Guardrail provider calls given up on after their timeout because the provider did not return, by provider"),
	)
	if err != nil {
		panic("failed to create guardrailAbandonedTotal counter: " + err.Error())
	}

	guardrailShadowDropped, err = meter.Int64Counter("aistudio_guardrail_shadow_dropped_total",
		otelmetric.WithDescription("Shadow guardrail evaluations dropped instead of queued, by provider and reason (queue_full, tenant_share, shutdown)"),
	)
	if err != nil {
		panic("failed to create guardrailShadowDropped counter: " + err.Error())
	}

	guardrailTemplatesTotal, err = meter.Int64Counter("aistudio_guardrail_templates_total",
		otelmetric.WithDescription("Request messages a guardrail tried against the app's trusted prompt templates, by provider and result (hit, miss, mimicry, backstop_only)"),
	)
	if err != nil {
		panic("failed to create guardrailTemplatesTotal counter: " + err.Error())
	}

	guardrailTemplateExempt, err = meter.Float64Histogram("aistudio_guardrail_template_exempt_share",
		otelmetric.WithDescription("Share of a matched message's characters a trusted prompt template left unscored"),
		otelmetric.WithExplicitBucketBoundaries(0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6),
	)
	if err != nil {
		panic("failed to create guardrailTemplateExempt histogram: " + err.Error())
	}

	goroutinePanicsTotal, err = meter.Int64Counter("aistudio_goroutine_panics_total",
		otelmetric.WithDescription("Panics recovered in Studio's background work and gRPC handlers, by goroutine"),
	)
	if err != nil {
		panic("failed to create goroutinePanicsTotal counter: " + err.Error())
	}

	decisionQuestionsTotal, err = meter.Int64Counter("aistudio_decision_questions_total",
		otelmetric.WithDescription("Questions put to decision models, by vendor, model and outcome (answered or refused)"),
	)
	if err != nil {
		panic("failed to create decisionQuestionsTotal counter: " + err.Error())
	}

	guardrailLatency, err = meter.Float64Histogram("aistudio_guardrail_latency_seconds",
		otelmetric.WithDescription("Guardrail provider call latency"),
		otelmetric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5),
	)
	if err != nil {
		panic("failed to create guardrailLatency histogram: " + err.Error())
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
}

// Register adds collectors to the registry Init created, for metrics that
// come from outside this package (connection pools, file sizes). It returns
// an error when Init has not been called.
func Register(cs ...prometheus.Collector) error {
	if !initialized.Load() || registry == nil {
		return errors.New("metrics not initialized with a Prometheus registry")
	}
	for _, c := range cs {
		if err := registry.Register(c); err != nil {
			return err
		}
	}
	return nil
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

// RecordTokens records token consumption for a given type.
// tokenType should be one of: "prompt", "completion", "cache_read", "cache_write".
func RecordTokens(ctx context.Context, vendor, model, tokenType string, count int) {
	if !initialized.Load() || count == 0 {
		return
	}
	recordGenAITokens(ctx, vendor, model, tokenType, count)
	if !legacyNames {
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

// RecordDecisionQuestions counts a decision call's questions by outcome.
func RecordDecisionQuestions(ctx context.Context, vendor, model string, answered, refused int) {
	if !initialized.Load() {
		return
	}
	for outcome, n := range map[string]int{"answered": answered, "refused": refused} {
		if n <= 0 {
			continue
		}
		decisionQuestionsTotal.Add(ctx, int64(n),
			otelmetric.WithAttributes(
				attribute.String("vendor", vendor),
				attribute.String("model", model),
				attribute.String("outcome", outcome),
			),
		)
	}
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

// RecordFailover counts one hop down an LLM's failover waterfall. reason is one
// of a small fixed set (status_<code>, timeout, connection_error, driver_error)
// so the label stays bounded.
func RecordFailover(ctx context.Context, fromLLM, toLLM, reason string) {
	if !initialized.Load() {
		return
	}
	failoverTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("from_llm", fromLLM),
			attribute.String("to_llm", toLLM),
			attribute.String("reason", reason),
		),
	)
}

// RecordComplianceEvent increments the compliance events counter.
func RecordComplianceEvent(ctx context.Context, eventType, severity, filterName string) {
	if !initialized.Load() {
		return
	}
	complianceEventsTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("event_type", eventType),
			attribute.String("severity", severity),
			attribute.String("filter_name", filterName),
		),
	)
}

// RecordGuardrail records one guardrail provider call. outcome is "flagged",
// "clean" or "error", action the filter's configured action and
// segmentKind the kind of text in the call ("text", "tool_result" or
// "mixed"), so the series stays bounded.
func RecordGuardrail(ctx context.Context, provider, outcome, action, segmentKind string, durationSeconds float64) {
	if !initialized.Load() {
		return
	}
	guardrailLatency.Record(ctx, durationSeconds,
		otelmetric.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("outcome", outcome),
			attribute.String("action", action),
			attribute.String("segment_kind", segmentKind),
		),
	)
}

// RecordGuardrailSegments counts request segments a guardrail screened.
// result is "scanned" (sent to the provider), "cached" (answered by the
// verdict cache) or "unscanned" (over a budget, or its call failed).
func RecordGuardrailSegments(ctx context.Context, provider, segmentKind, result string, n int64) {
	if !initialized.Load() {
		return
	}
	guardrailSegmentsTotal.Add(ctx, n,
		otelmetric.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("segment_kind", segmentKind),
			attribute.String("result", result),
		),
	)
}

// RecordGuardrailCache counts a verdict cache lookup; outcome is "hit" or
// "miss".
func RecordGuardrailCache(ctx context.Context, provider, outcome string) {
	if !initialized.Load() {
		return
	}
	guardrailCacheTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("outcome", outcome),
		),
	)
}

// RecordGuardrailAbandoned counts a guardrail provider call the runner gave
// up on because it did not return after its timeout; its goroutine is still
// running. A steady rate means a provider that ignores its context.
func RecordGuardrailAbandoned(ctx context.Context, provider string) {
	if !initialized.Load() {
		return
	}
	guardrailAbandonedTotal.Add(ctx, 1,
		otelmetric.WithAttributes(attribute.String("provider", provider)),
	)
}

// RecordGuardrailShadowDropped counts a shadow guardrail evaluation that was
// dropped rather than queued (the request was never waited on).
func RecordGuardrailShadowDropped(ctx context.Context, provider, reason string) {
	if !initialized.Load() {
		return
	}
	guardrailShadowDropped.Add(ctx, 1,
		otelmetric.WithAttributes(attribute.String("provider", provider), attribute.String("reason", reason)),
	)
}

// RecordGuardrailTemplate counts a request message a guardrail tried against
// the app's trusted prompt templates. result is "hit" (a template matched),
// "miss" (a template's placement fits but none matched), "mimicry" (template
// text where it does not belong) or "backstop_only" (the full-text backstop
// found what the residual did not). Per-app detail is in the compliance
// events; the labels stay low in cardinality.
func RecordGuardrailTemplate(ctx context.Context, provider, result string) {
	if !initialized.Load() {
		return
	}
	guardrailTemplatesTotal.Add(ctx, 1,
		otelmetric.WithAttributes(attribute.String("provider", provider), attribute.String("result", result)),
	)
}

// RecordGuardrailTemplateExempt records the share of a matched message's
// characters its template left unscored.
func RecordGuardrailTemplateExempt(ctx context.Context, provider string, share float64) {
	if !initialized.Load() {
		return
	}
	guardrailTemplateExempt.Record(ctx, share, otelmetric.WithAttributes(attribute.String("provider", provider)))
}

// RecordGuardrailError counts a guardrail provider failure and the fail mode
// that decided the request's fate ("open" or "closed").
func RecordGuardrailError(ctx context.Context, provider, failMode string) {
	if !initialized.Load() {
		return
	}
	guardrailErrorsTotal.Add(ctx, 1,
		otelmetric.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("fail_mode", failMode),
		),
	)
}

// RecordGoroutinePanic counts a panic recovered in the named background
// goroutine or gRPC handler (pkg/safe).
func RecordGoroutinePanic(ctx context.Context, goroutine string) {
	if !initialized.Load() {
		return
	}
	goroutinePanicsTotal.Add(ctx, 1,
		otelmetric.WithAttributes(attribute.String("goroutine", goroutine)),
	)
}

// ObserveRequestDuration records an LLM request duration observation.
// statusCode is the response status the caller received; it populates the
// error.type attribute on the GenAI instrument and may be 0 when unknown.
func ObserveRequestDuration(ctx context.Context, vendor, model string, streaming bool, durationSeconds float64, statusCode int) {
	if !initialized.Load() {
		return
	}
	recordGenAIRequestDuration(ctx, vendor, model, durationSeconds, statusCode)
	if !legacyNames {
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
	recordGenAIToolDuration(ctx, toolName, durationSeconds)
	if !legacyNames {
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
