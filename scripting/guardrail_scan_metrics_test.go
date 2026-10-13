package scripting

import (
	"context"
	"strings"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
)

// segmentResults sums aistudio_guardrail_segments_total by result.
func segmentResults(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "aistudio_guardrail_segments_total" {
				continue
			}
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				result, _ := dp.Attributes.Value("result")
				out[result.AsString()] += dp.Value
			}
		}
	}
	return out
}

// A tool result quarantined because its call failed under fail closed was
// not scanned, and is counted so.
func TestGuardrailScan_FailedToolsCountedUnscanned(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	metrics.InitWithProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { metrics.Init() })
	freshVerdictCache(t, time.Minute)

	p := &scanFake{fail: func(in guardrails.Input) error {
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "BADTOOL") {
				return &guardrails.StatusError{Status: 503}
			}
		}
		return nil
	}}
	f := scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailClosed, "on_detect": guardrails.ActionBlock})
	body := chatBody(t,
		chatMsg{Role: "user", Content: "summarise it"},
		chatMsg{Role: "tool", Content: "page text BADTOOL", ToolCallID: "call_1"},
	)
	if out := runRequest(t, f, body, app(1)); out.Block {
		t.Fatalf("blocked: %v", eventTypes(out))
	}
	got := segmentResults(t, reader)
	if got["scanned"] != 1 || got["unscanned"] != 1 {
		t.Errorf("segments by result = %v, want the user text scanned and the failed tool result unscanned", got)
	}
}
