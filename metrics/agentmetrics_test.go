package metrics

import (
	"context"
	"sort"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func recordAgentSamples(ctx context.Context) {
	RecordAgentSessionClosed(ctx, 7, 42.5)
	RecordAgentTurnClosed(ctx, 7, 3, 2)
	RecordAgentCost(ctx, 7, "USD", 1500)
	RecordCorrelationLink(ctx, "strong", "mcp_name_args")
	RecordExportFailure(ctx, "tempo")
	RecordCoverage(7, true)
	RecordCoverage(7, false)
	SetActiveSessionsFn(func() (int64, bool) { return 4, true })
}

// Prometheus scrape shows the new metrics.
func TestAgentMetrics_PrometheusScrape(t *testing.T) {
	h := Init()
	t.Cleanup(func() { SetActiveSessionsFn(nil) })
	recordAgentSamples(context.Background())
	body := scrape(t, h)
	for _, name := range []string{
		"tyk_agent_session_duration_seconds_bucket",
		"tyk_agent_turn_llm_calls_bucket",
		"tyk_agent_turn_tool_calls_bucket",
		"tyk_agent_cost_total",
		"tyk_tracing_correlation_links_total",
		"tyk_tracing_export_failures_total",
		"tyk_agent_sessions_active",
		"tyk_tracing_coverage_ratio",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("scrape is missing %s", name)
		}
	}
}

// With an OTLP-style extra meter, an in-memory reader receives every metric,
// each with exactly its attribute set: never a session, trace, user or
// tool-call id (the cardinality guard).
func TestAgentMetrics_ReaderAndAttributeSets(t *testing.T) {
	Init()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := AddAgentMetricsMeter(provider.Meter("agenttracing")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetActiveSessionsFn(nil); SetAgentMetricsIncludeAppID(true) })
	recordAgentSamples(context.Background())

	got := collectKeys(t, reader)
	want := map[string][]string{
		"tyk_agent_session_duration":    {"tyk.app.id"},
		"tyk_agent_turn_llm_calls":      {"tyk.app.id"},
		"tyk_agent_turn_tool_calls":     {"tyk.app.id"},
		"tyk_agent_cost":                {"currency", "tyk.app.id"},
		"tyk_tracing_correlation_links": {"confidence", "rule"},
		"tyk_tracing_export_failures":   {"exporter"},
		"tyk_agent_sessions_active":     {},
		"tyk_tracing_coverage_ratio":    {"tyk.app.id"},
	}
	for name, keys := range want {
		if strings.Join(got[name], ",") != strings.Join(keys, ",") {
			t.Errorf("%s attributes: got %v, want %v", name, got[name], keys)
		}
	}
	forbidden := []string{"session", "trace", "user", "tool_call", "span"}
	for name, keys := range got {
		for _, k := range keys {
			for _, f := range forbidden {
				if strings.Contains(k, f) {
					t.Errorf("%s has a high-cardinality attribute %q", name, k)
				}
			}
		}
	}

	// tyk.app.id is switchable.
	SetAgentMetricsIncludeAppID(false)
	reader2 := sdkmetric.NewManualReader()
	p2 := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader2))
	if err := AddAgentMetricsMeter(p2.Meter("agenttracing")); err != nil {
		t.Fatal(err)
	}
	RecordAgentCost(context.Background(), 7, "EUR", 10)
	if keys := collectKeys(t, reader2)["tyk_agent_cost"]; strings.Join(keys, ",") != "currency" {
		t.Fatalf("with the app id off: %v", keys)
	}
}

// AGENT_TRACING_METRICS_ENABLED=false switches off the tyk_agent_* metrics;
// the tyk_tracing_* health metrics stay (jyx.110).
func TestAgentMetrics_Disabled(t *testing.T) {
	Init()
	SetAgentMetricsEnabled(false)
	t.Cleanup(func() { SetAgentMetricsEnabled(true); SetActiveSessionsFn(nil) })
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := AddAgentMetricsMeter(provider.Meter("agenttracing")); err != nil {
		t.Fatal(err)
	}
	recordAgentSamples(context.Background())

	got := collectKeys(t, reader)
	for _, name := range []string{"tyk_agent_session_duration", "tyk_agent_turn_llm_calls", "tyk_agent_cost", "tyk_agent_sessions_active"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s recorded with agent metrics off", name)
		}
	}
	for _, name := range []string{"tyk_tracing_correlation_links", "tyk_tracing_export_failures"} {
		if _, ok := got[name]; !ok {
			t.Errorf("health metric %s is missing", name)
		}
	}
}

// collectKeys returns, per metric name, the sorted attribute keys of its
// data points.
func collectKeys(t *testing.T, r *sdkmetric.ManualReader) map[string][]string {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			keys := map[string]bool{}
			switch d := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
				}
			case metricdata.Histogram[int64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
				}
			case metricdata.Sum[int64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
				}
			case metricdata.Gauge[int64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
				}
			case metricdata.Gauge[float64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
				}
			}
			list := []string{}
			for k := range keys {
				list = append(list, k)
			}
			sort.Strings(list)
			out[m.Name] = list
		}
	}
	return out
}
