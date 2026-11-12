package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// Database hold instruments (pkg/dbhold): statements held across a database
// failover.
var (
	dbWriteRetriesTotal         otelmetric.Int64Counter
	dbWriteHoldSeconds          otelmetric.Float64Histogram
	dbWritesHeld                otelmetric.Int64UpDownCounter
	dbUnavailableResponsesTotal otelmetric.Int64Counter
)

func initDatabaseHold(meter otelmetric.Meter) {
	var err error
	dbWriteRetriesTotal, err = meter.Int64Counter("aistudio_db_write_retries_total",
		otelmetric.WithDescription("Database statements tried again because they did not reach a writable server (a failover), by outcome of the hold (succeeded, gave_up, cancelled, failed)"),
	)
	if err != nil {
		panic("failed to create dbWriteRetriesTotal counter: " + err.Error())
	}
	dbWriteHoldSeconds, err = meter.Float64Histogram("aistudio_db_write_hold_seconds",
		otelmetric.WithDescription("Time a database statement was held waiting for a writable server, by outcome of the hold"),
		otelmetric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 30, 60),
	)
	if err != nil {
		panic("failed to create dbWriteHoldSeconds histogram: " + err.Error())
	}
	dbWritesHeld, err = meter.Int64UpDownCounter("aistudio_db_writes_held",
		otelmetric.WithDescription("Database statements held now, waiting for a writable server (at most pkg/dbhold.MaxHeld, 512; beyond it a statement fails at once)"),
	)
	if err != nil {
		panic("failed to create dbWritesHeld gauge: " + err.Error())
	}
	dbUnavailableResponsesTotal, err = meter.Int64Counter("aistudio_db_unavailable_responses_total",
		otelmetric.WithDescription("Management API 500s answered as 503 database_unavailable because a statement found the database unavailable while the request ran (linked by time, not by request)"),
	)
	if err != nil {
		panic("failed to create dbUnavailableResponsesTotal counter: " + err.Error())
	}
}

// AddDBWritesHeld moves the count of statements held now by delta.
func AddDBWritesHeld(ctx context.Context, delta int64) {
	if !initialized.Load() {
		return
	}
	dbWritesHeld.Add(ctx, delta)
}

// RecordDBUnavailableResponse counts one 500 answered as 503
// database_unavailable.
func RecordDBUnavailableResponse(ctx context.Context) {
	if !initialized.Load() {
		return
	}
	dbUnavailableResponsesTotal.Add(ctx, 1)
}

// RecordDBWriteHold records one held statement: the retries it took and
// the time it was held. outcome is succeeded, gave_up (the window ran out),
// cancelled (the request ended), failed (a later error that is not a
// failover's) or rejected (MaxHeld statements were held already).
func RecordDBWriteHold(ctx context.Context, outcome string, retries int, seconds float64) {
	if !initialized.Load() {
		return
	}
	attrs := otelmetric.WithAttributes(attribute.String("outcome", outcome))
	if retries > 0 {
		dbWriteRetriesTotal.Add(ctx, int64(retries), attrs)
	}
	dbWriteHoldSeconds.Record(ctx, seconds, attrs)
}
