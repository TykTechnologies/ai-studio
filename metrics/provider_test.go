package metrics

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// An embedding host passes its own MeterProvider; Studio's measurements must
// land there and Studio must not serve a /metrics handler of its own.
func TestInitWithProviderRecordsOnTheHostProvider(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	InitWithProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { Init() })

	RecordRequest(context.Background(), "app-1", "openai", "gpt-4o", 200)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		t.Fatal("no metrics recorded on the injected provider")
	}
	if Handler() != nil {
		t.Error("Studio should not serve metrics itself when the host provides the MeterProvider")
	}
	if Register() == nil {
		t.Error("Register should fail without a Prometheus registry")
	}
}
