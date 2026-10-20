package metrics

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Agent Call Tracing capture instruments (spec §10): drops by reason and the
// depth of the capture queue on this node. Prometheus names:
// tyk_tracing_capture_dropped_total{reason} and tyk_tracing_queue_depth.
var (
	tracingCaptureDropped otelmetric.Int64Counter
	tracingQueueDepthFn   atomic.Pointer[func() int64]
)

func initAgentTracing(meter otelmetric.Meter) {
	var err error
	tracingCaptureDropped, err = meter.Int64Counter("tyk_tracing_capture_dropped",
		otelmetric.WithDescription("Agent tracing capture data dropped, by reason (payload, gateway_event_detail, span, no_sealer, write_error)"),
	)
	if err != nil {
		panic("failed to create tracingCaptureDropped counter: " + err.Error())
	}
	_, err = meter.Int64ObservableGauge("tyk_tracing_queue_depth",
		otelmetric.WithDescription("Agent tracing capture events waiting to be written on this node"),
		otelmetric.WithInt64Callback(func(_ context.Context, o otelmetric.Int64Observer) error {
			if fn := tracingQueueDepthFn.Load(); fn != nil {
				o.Observe((*fn)())
			}
			return nil
		}),
	)
	if err != nil {
		panic("failed to create tracingQueueDepth gauge: " + err.Error())
	}
}

// RecordTracingCaptureDropped counts n drops for reason.
func RecordTracingCaptureDropped(ctx context.Context, reason string, n int64) {
	if !initialized.Load() || n <= 0 {
		return
	}
	tracingCaptureDropped.Add(ctx, n, otelmetric.WithAttributes(attribute.String("reason", reason)))
}

// SetTracingQueueDepth sets the function the queue depth gauge reads.
func SetTracingQueueDepth(fn func() int64) {
	if fn == nil {
		tracingQueueDepthFn.Store(nil)
		return
	}
	tracingQueueDepthFn.Store(&fn)
}
