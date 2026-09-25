package services

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultWriterQueueSize     = 10000
	defaultWriterBatchSize     = 500
	defaultWriterFlushInterval = 100 * time.Millisecond
	writerBusyRetries          = 3
	// writerMaxRowFailures ends a row-by-row retry of a failed batch when
	// no row gets through.
	writerMaxRowFailures = 10
)

// AnalyticsWriter is the gateway's single writer for per-request data. It
// takes analytics events from a bounded queue and writes them in batches, one
// transaction per batch, together with the budget ledger's accumulated usage.
//
// SQLite has one writer at a time and pays for every transaction with whole
// page writes for the table and each index. One transaction per request
// capped throughput and kept the write lock permanently contended; a batch
// of hundreds of rows costs about as much as one.
//
// Enqueue never blocks. When the queue is full the event is dropped and
// counted: the analytics pulse and the budget ledger are fed before the
// queue, so a dropped row only loses the gateway's local copy.
type AnalyticsWriter struct {
	db        *gorm.DB
	queue     chan *database.AnalyticsEvent
	batchSize int
	interval  time.Duration
	ledger    *BudgetLedger

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	started  atomic.Bool

	written     atomic.Uint64
	dropped     atomic.Uint64
	failed      atomic.Uint64
	batches     atomic.Uint64
	commitNanos atomic.Uint64
}

// NewAnalyticsWriter returns a writer on db (the gateway's writer handle).
// Zero or negative sizes and interval take the defaults.
func NewAnalyticsWriter(db *gorm.DB, queueSize, batchSize int, interval time.Duration) *AnalyticsWriter {
	if queueSize <= 0 {
		queueSize = defaultWriterQueueSize
	}
	if batchSize <= 0 {
		batchSize = defaultWriterBatchSize
	}
	if interval <= 0 {
		interval = defaultWriterFlushInterval
	}
	return &AnalyticsWriter{
		db:        db,
		queue:     make(chan *database.AnalyticsEvent, queueSize),
		batchSize: batchSize,
		interval:  interval,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// SetLedger makes each batch also write the ledger's accumulated budget usage.
// Call it before Start.
func (w *AnalyticsWriter) SetLedger(l *BudgetLedger) { w.ledger = l }

// Start runs the writer until Stop.
func (w *AnalyticsWriter) Start() {
	if w.started.Swap(true) {
		return
	}
	go w.run()
}

// Enqueue queues an event for writing. It reports false when the queue was
// full and the event was dropped.
func (w *AnalyticsWriter) Enqueue(e *database.AnalyticsEvent) bool {
	select {
	case w.queue <- e:
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

// Stop writes what is queued and stops the writer. Events enqueued after Stop
// are not written.
func (w *AnalyticsWriter) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	if w.started.Load() {
		<-w.done
	}
}

func (w *AnalyticsWriter) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	batch := make([]*database.AnalyticsEvent, 0, w.batchSize)
	lastRefresh := time.Now()
	for {
		select {
		case e := <-w.queue:
			batch = append(batch, e)
			if len(batch) >= w.batchSize {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			w.flush(batch)
			batch = batch[:0]
			if w.ledger != nil && time.Since(lastRefresh) >= ledgerRefreshInterval {
				w.ledger.refresh()
				lastRefresh = time.Now()
			}
		case <-w.stop:
			for drained := false; !drained; {
				select {
				case e := <-w.queue:
					batch = append(batch, e)
					if len(batch) >= w.batchSize {
						w.flush(batch)
						batch = batch[:0]
					}
				default:
					drained = true
				}
			}
			w.flush(batch)
			return
		}
	}
}

// flush writes the events and the ledger's pending usage in one transaction.
// Busy errors are retried. Any other error is taken to be a bad row (a
// foreign key to a configuration row deleted meanwhile, on databases that
// enforce them): the usage and then each event are written on their own, so
// only the bad rows are lost. Failed events are counted; failed usage stays
// in the ledger for the next flush.
func (w *AnalyticsWriter) flush(events []*database.AnalyticsEvent) {
	var usage *ledgerFlush
	if w.ledger != nil {
		usage = w.ledger.take()
	}
	if len(events) == 0 && usage.empty() {
		return
	}

	start := time.Now()
	err := w.withBusyRetry(func(tx *gorm.DB) error {
		if err := insertEvents(tx, events); err != nil {
			return err
		}
		return usage.write(tx)
	})
	written, failed := len(events), 0
	if err != nil && !isBusyError(err) && len(events) > 0 {
		log.Warn().Err(err).Int("events", len(events)).Msg("Analytics writer: batch failed, writing its rows one by one")
		err = w.withBusyRetry(usage.write)
		written = 0
		for i, e := range events {
			rowErr := w.withBusyRetry(func(tx *gorm.DB) error { return insertEvents(tx, []*database.AnalyticsEvent{e}) })
			if rowErr == nil {
				written++
				continue
			}
			failed++
			log.Error().Err(rowErr).Str("request_id", e.RequestID).Msg("Analytics writer: event dropped")
			if written == 0 && failed >= writerMaxRowFailures {
				// Every row fails: the database, not a row, is the
				// problem. Drop the rest of the batch.
				failed += len(events) - i - 1
				break
			}
		}
	} else if err != nil {
		written, failed = 0, len(events)
	}
	w.commitNanos.Add(uint64(time.Since(start)))
	w.batches.Add(1)

	if w.ledger != nil && usage != nil {
		w.ledger.finish(usage, err == nil)
	}
	w.written.Add(uint64(written))
	w.failed.Add(uint64(failed))
	if err != nil {
		log.Error().Err(err).Int("events", failed).Msg("Analytics writer: flush failed; events dropped, budget usage kept for the next flush")
	}
}

// withBusyRetry runs fn in a transaction, retrying while the database is
// locked by another writer.
func (w *AnalyticsWriter) withBusyRetry(fn func(tx *gorm.DB) error) error {
	var err error
	for attempt := 0; attempt <= writerBusyRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 50 * time.Millisecond)
		}
		err = w.db.Transaction(fn)
		if err == nil || !isBusyError(err) {
			return err
		}
	}
	return err
}

// insertEvents inserts analytics rows. A request ID that is already stored is
// skipped rather than failing the batch.
func insertEvents(tx *gorm.DB, events []*database.AnalyticsEvent) error {
	if len(events) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(events, 100).Error
}

func isBusyError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database table is locked")
}

// AnalyticsWriterStats is a snapshot of the writer's counters.
type AnalyticsWriterStats struct {
	Queued, Written, Dropped, Failed, Batches uint64
	CommitSeconds                             float64
}

// Stats returns the writer's counters.
func (w *AnalyticsWriter) Stats() AnalyticsWriterStats {
	return AnalyticsWriterStats{
		Queued:        uint64(len(w.queue)),
		Written:       w.written.Load(),
		Dropped:       w.dropped.Load(),
		Failed:        w.failed.Load(),
		Batches:       w.batches.Load(),
		CommitSeconds: float64(w.commitNanos.Load()) / 1e9,
	}
}
