package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// EdgeBatch is the records of one analytics pulse from an edge, to be stored
// together and at most once.
//
// An edge sends a batch again when it did not get the acknowledgement,
// typically because the control plane that stored it went away before
// answering and the edge moved to another one (features/ActiveActive.md,
// Phase 2). The other Record functions cannot make that safe: they queue
// records for a background writer and return, so nothing ties "this batch was
// stored" to the records. RecordEdgeBatch writes the records and the batch's
// key in one transaction and returns when it has committed.
type EdgeBatch struct {
	// Key identifies the batch: models.EdgeAnalyticsBatchKey.
	Key string
	// EdgeID is the edge that sent it.
	EdgeID string

	ProxyLogs        []*models.ProxyLog
	ChatRecords      []*models.LLMChatRecord
	ComplianceEvents []*models.ComplianceEvent
	ToolCalls        []*models.ToolCallRecord
}

// Records is the number of records in the batch.
func (b *EdgeBatch) Records() int {
	return len(b.ProxyLogs) + len(b.ChatRecords) + len(b.ComplianceEvents) + len(b.ToolCalls)
}

// EdgeBatchRecorder is implemented by handlers that can store an edge's
// batch once. It is optional, as MCPCallRecorder is: a host's own
// AnalyticsHandler keeps compiling without it, and an edge's batches are
// then recorded as before, without de-duplication.
type EdgeBatchRecorder interface {
	// RecordEdgeBatch stores the batch and returns once it is committed.
	// stored is false, with no error, when a batch with this key was stored
	// before: nothing was written. An error means nothing was stored and
	// the edge should send the batch again; ErrEdgeBatchUnsupported means
	// this handler cannot store batches this way at all.
	RecordEdgeBatch(ctx context.Context, b *EdgeBatch) (stored bool, err error)
}

// ErrEdgeBatchUnsupported is returned by RecordEdgeBatch when the handler in
// use cannot store a batch once. The caller records the batch's records the
// usual way.
var ErrEdgeBatchUnsupported = errors.New("analytics handler cannot store an edge batch once")

// RecordEdgeBatch stores an edge's batch once through the handler for ctx.
// See EdgeBatchRecorder for what it returns.
func RecordEdgeBatch(ctx context.Context, b *EdgeBatch) (stored bool, err error) {
	h, release := handlerFor(ctx)
	defer release()
	r, ok := h.(EdgeBatchRecorder)
	if !ok {
		return false, ErrEdgeBatchUnsupported
	}
	return r.RecordEdgeBatch(ctx, b)
}

// markOwnerVisible sets OwnerVisible on the events whose metadata marks them
// for the App's owners, as RecordComplianceEvents does.
func markOwnerVisible(events []*models.ComplianceEvent) {
	for _, e := range events {
		if e != nil && !e.OwnerVisible && strings.Contains(e.Metadata, `"owner_visible":true`) {
			var m map[string]any
			if json.Unmarshal([]byte(e.Metadata), &m) == nil {
				e.OwnerVisible, _ = m["owner_visible"].(bool)
			}
		}
	}
}

// edgeBatchPruneInterval is how often a handler that stores batches deletes
// the batch records past their retention.
const edgeBatchPruneInterval = time.Hour

// lastEdgeBatchPrune is when this process last pruned, in Unix seconds.
var lastEdgeBatchPrune atomic.Int64

// RecordEdgeBatch implements EdgeBatchRecorder. One transaction records the
// batch's key and writes its records; a key that is already there ends it
// with nothing written. A row the database refuses on its own (an edge value
// over a column's size on Postgres) is left out and logged, as the background
// writer does for proxy logs, so one bad record cannot keep an edge sending
// the same batch for a day; any other failure fails the whole batch.
func (h *DatabaseHandler) RecordEdgeBatch(ctx context.Context, b *EdgeBatch) (bool, error) {
	if b == nil || b.Key == "" {
		return false, errors.New("edge batch needs a key")
	}
	h.recMutex.RLock()
	started := h.recStarted
	h.recMutex.RUnlock()
	if !started {
		return false, errors.New("analytics recording is not started")
	}

	h.teams.stampBatch(b.ChatRecords)
	markOwnerVisible(b.ComplianceEvents)

	now := time.Now()
	stored := false
	refused := 0
	err := h.createRecordWithRetry(func() error {
		stored, refused = false, 0
		// An attempt that was rolled back may have assigned ids; let the
		// database assign them again.
		for _, l := range b.ProxyLogs {
			l.ID = 0
		}
		for _, r := range b.ChatRecords {
			r.ID = 0
		}
		for _, e := range b.ComplianceEvents {
			e.ID = 0
		}
		for _, c := range b.ToolCalls {
			c.ID = 0
		}
		return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			claimed, err := models.ClaimEdgeAnalyticsBatch(tx, b.Key, b.EdgeID, now)
			if err != nil || !claimed {
				return err
			}
			refused += createRows(tx, b.ProxyLogs, func(l *models.ProxyLog) { l.ID = 0 })
			refused += createRows(tx, b.ChatRecords, func(r *models.LLMChatRecord) { r.ID = 0 })
			refused += createRows(tx, b.ComplianceEvents, func(e *models.ComplianceEvent) { e.ID = 0 })
			refused += createRows(tx, b.ToolCalls, func(c *models.ToolCallRecord) { c.ID = 0 })
			stored = true
			return nil
		})
	})
	if err != nil {
		return false, err
	}
	if !stored {
		return false, nil
	}
	if refused > 0 {
		logger.Log.Warn().
			Str("edge_id", b.EdgeID).
			Int("refused", refused).
			Int("records", b.Records()).
			Msg("Edge analytics batch: rows refused by the database were left out; the rest of the batch was stored")
	}

	complianceEventsStored(b.ComplianceEvents)
	for _, e := range b.ComplianceEvents {
		metrics.RecordComplianceEvent(ctx, e.EventType, e.Severity, e.FilterName)
	}
	for _, c := range b.ToolCalls {
		metrics.RecordToolCall(ctx, c.Name, "")
	}
	h.pruneEdgeBatches(now)
	return true, nil
}

// createRows inserts rows inside tx and returns how many the database
// refused. The batch is tried whole first, in a savepoint; if that fails the
// rows are written one by one, each in a savepoint, so only the refused rows
// are lost. When the failure is the connection's and not a row's, every
// later statement fails as well, the commit included, and the batch as a
// whole is reported as not stored.
func createRows[T any](tx *gorm.DB, rows []*T, resetID func(*T)) (refused int) {
	if len(rows) == 0 {
		return 0
	}
	err := tx.Transaction(func(sp *gorm.DB) error { return sp.CreateInBatches(rows, 100).Error })
	if err == nil {
		return 0
	}
	for _, row := range rows {
		resetID(row)
		row := row
		if rowErr := tx.Transaction(func(sp *gorm.DB) error { return sp.Create(row).Error }); rowErr != nil {
			refused++
			logger.Log.Warn().Str("error", sanitizeError(rowErr)).Msg("Edge analytics batch: row refused by the database")
		}
	}
	return refused
}

// pruneEdgeBatches deletes the batch records past their retention, at most
// once per edgeBatchPruneInterval in this process. Every replica that stores
// batches does this; the delete is idempotent.
func (h *DatabaseHandler) pruneEdgeBatches(now time.Time) {
	last := lastEdgeBatchPrune.Load()
	if now.Unix()-last < int64(edgeBatchPruneInterval/time.Second) || !lastEdgeBatchPrune.CompareAndSwap(last, now.Unix()) {
		return
	}
	n, err := models.PruneEdgeAnalyticsBatches(h.db, now)
	if err != nil {
		logger.Warnf("Could not prune stored edge analytics batch records: %s", sanitizeError(err))
		return
	}
	if n > 0 {
		logger.Debugf("Pruned %d edge analytics batch record(s) older than %s", n, models.EdgeAnalyticsBatchRetention)
	}
}

// RecordEdgeBatch implements EdgeBatchRecorder: the primary stores the batch
// once, and the sinks get copies of its records only when it was stored, so
// they do not see a repeated batch either.
func (t *Tee) RecordEdgeBatch(ctx context.Context, b *EdgeBatch) (bool, error) {
	r, ok := t.primary.(EdgeBatchRecorder)
	if !ok {
		return false, ErrEdgeBatchUnsupported
	}
	if len(t.sinks) == 0 {
		return r.RecordEdgeBatch(ctx, b)
	}
	logs := make([]*models.ProxyLog, len(b.ProxyLogs))
	for i, l := range b.ProxyLogs {
		logs[i] = copyProxyLog(l)
	}
	recs := make([]*models.LLMChatRecord, len(b.ChatRecords))
	for i, rec := range b.ChatRecords {
		recs[i] = copyChatRecord(rec)
	}
	events := make([]*models.ComplianceEvent, len(b.ComplianceEvents))
	for i, e := range b.ComplianceEvents {
		if e != nil {
			ce := *e
			events[i] = &ce
		}
	}
	calls := make([]models.ToolCallRecord, len(b.ToolCalls))
	for i, c := range b.ToolCalls {
		if c != nil {
			calls[i] = *c
		}
	}
	stored, err := r.RecordEdgeBatch(ctx, b)
	if err != nil || !stored {
		return stored, err
	}
	t.toSinks(func(h AnalyticsHandler) {
		bg := context.Background()
		if len(logs) > 0 {
			h.RecordProxyLogsBatch(bg, logs)
		}
		if len(recs) > 0 {
			h.RecordChatRecordsBatch(bg, recs)
		}
		if len(events) > 0 {
			h.RecordComplianceEvents(bg, events)
		}
		for _, c := range calls {
			h.RecordToolCall(bg, c.Name, c.TimeStamp, c.ExecTime, c.ToolID)
		}
	})
	return true, nil
}
