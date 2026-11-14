package metrics

import (
	"context"
	"sync/atomic"

	otelmetric "go.opentelemetry.io/otel/metric"
)

// DatabaseState is what the database instruments read: whether this
// replica's database takes writes, and how many times the writable server
// changed since the replica started.
type DatabaseState func() (writable bool, failovers int64)

// databaseStateFn is set by pkg/studio for a PostgreSQL database (it has a
// dbstate.Tracker, which this package cannot import); with SQLite it stays
// nil and neither instrument reports.
var databaseStateFn atomic.Pointer[DatabaseState]

// initDatabase registers aistudio_db_writable (1 while this replica's
// database takes writes, 0 while it is read-only or unreachable) and
// aistudio_db_failovers_total (failovers of the database seen by this
// replica since it started).
func initDatabase(meter otelmetric.Meter) {
	_, err := meter.Int64ObservableGauge("aistudio_db_writable",
		otelmetric.WithDescription("1 while this replica's database takes writes, 0 while it is read-only or unreachable (PostgreSQL only)"),
		otelmetric.WithInt64Callback(func(_ context.Context, o otelmetric.Int64Observer) error {
			if fn := databaseStateFn.Load(); fn != nil {
				writable, _ := (*fn)()
				var v int64
				if writable {
					v = 1
				}
				o.Observe(v)
			}
			return nil
		}),
	)
	if err != nil {
		panic("failed to create aistudio_db_writable gauge: " + err.Error())
	}
	_, err = meter.Int64ObservableCounter("aistudio_db_failovers_total",
		otelmetric.WithDescription("Times this replica saw the writable database server change (another node, or a promotion) since it started (PostgreSQL only)"),
		otelmetric.WithInt64Callback(func(_ context.Context, o otelmetric.Int64Observer) error {
			if fn := databaseStateFn.Load(); fn != nil {
				_, failovers := (*fn)()
				o.Observe(failovers)
			}
			return nil
		}),
	)
	if err != nil {
		panic("failed to create aistudio_db_failovers_total counter: " + err.Error())
	}
}

// SetDatabaseState sets the function the database instruments read; nil
// stops them reporting.
func SetDatabaseState(fn DatabaseState) {
	if fn == nil {
		databaseStateFn.Store(nil)
		return
	}
	databaseStateFn.Store(&fn)
}
