package metrics

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Native API extension instruments (pkg/nativeapi): the SDK-specific
// operations on the native /llm routes, such as Anthropic Message Batches.
var (
	nativeRequestsTotal     otelmetric.Int64Counter
	nativeRequestDuration   otelmetric.Float64Histogram
	nativeResultsBytesTotal otelmetric.Int64Counter
	nativeReconcileOutcomes otelmetric.Int64Counter
)

func initNative(meter otelmetric.Meter) {
	var err error
	nativeRequestsTotal, err = meter.Int64Counter("aistudio_native_requests_total",
		otelmetric.WithDescription("Native API extension operations served (batch create, retrieve, list, results...), by vendor, operation and status code"),
	)
	if err != nil {
		panic("failed to create nativeRequestsTotal counter: " + err.Error())
	}
	nativeRequestDuration, err = meter.Float64Histogram("aistudio_native_request_duration_seconds",
		otelmetric.WithDescription("Native API extension operation latency, by vendor, operation and status code"),
		otelmetric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 300, 1800),
	)
	if err != nil {
		panic("failed to create nativeRequestDuration histogram: " + err.Error())
	}
	nativeResultsBytesTotal, err = meter.Int64Counter("aistudio_native_results_bytes_total",
		otelmetric.WithDescription("Bytes of native results downloads (batch results files) streamed to clients"),
	)
	if err != nil {
		panic("failed to create nativeResultsBytesTotal counter: " + err.Error())
	}
	nativeReconcileOutcomes, err = meter.Int64Counter("aistudio_native_reconcile_outcomes_total",
		otelmetric.WithDescription("Native resources the reconciler looked at, by vendor, resource type and outcome (accounted, settled, not_ended, unaccountable, lost, failed)"),
	)
	if err != nil {
		panic("failed to create nativeReconcileOutcomes counter: " + err.Error())
	}
}

// RecordNativeRequest records one native API operation: its count and its
// duration. operation is the extension's operation name
// ("anthropic.message_batches.create"), or "method_not_allowed" for a native
// path asked with an undeclared method.
func RecordNativeRequest(ctx context.Context, vendor, operation string, statusCode int, durationSeconds float64) {
	if !initialized.Load() {
		return
	}
	attrs := otelmetric.WithAttributes(
		attribute.String("vendor", vendor),
		attribute.String("operation", operation),
		attribute.String("status_code", strconv.Itoa(statusCode)),
	)
	nativeRequestsTotal.Add(ctx, 1, attrs)
	nativeRequestDuration.Record(ctx, durationSeconds, attrs)
}

// RecordNativeResultsBytes counts bytes of a results download sent to the
// client.
func RecordNativeResultsBytes(ctx context.Context, vendor, operation string, n int64) {
	if !initialized.Load() || n <= 0 {
		return
	}
	nativeResultsBytesTotal.Add(ctx, n, otelmetric.WithAttributes(
		attribute.String("vendor", vendor),
		attribute.String("operation", operation),
	))
}

// RecordNativeReconcile counts one reconciler pass over a native resource,
// by its outcome.
func RecordNativeReconcile(ctx context.Context, vendor, resourceType, outcome string) {
	if !initialized.Load() {
		return
	}
	nativeReconcileOutcomes.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("vendor", vendor),
		attribute.String("resource_type", resourceType),
		attribute.String("outcome", outcome),
	))
}
