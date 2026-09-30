package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Tee sends every record to a primary handler (Studio's database handler:
// budgets and spend are computed from it) and to extra sinks, for a host
// that ships analytics into its own pipeline. Sinks never slow a request:
// each has a bounded queue drained by its own goroutine, and records that
// do not fit are dropped and counted. Sinks receive copies, so they never
// share a record with the primary handler (which writes IDs back into it).
type Tee struct {
	primary AnalyticsHandler
	sinks   []*teeSink
	// mu guards sends to the sink queues against Stop closing them.
	mu      sync.RWMutex
	stopped bool
}

type teeSink struct {
	h       AnalyticsHandler
	queue   chan func(AnalyticsHandler)
	dropped atomic.Uint64
	done    chan struct{}
}

// TeeQueueSize is the per-sink queue length.
var TeeQueueSize = 10000

// NewTee returns a Tee over primary and sinks. Stop it to drain the sinks.
func NewTee(primary AnalyticsHandler, sinks ...AnalyticsHandler) *Tee {
	t := &Tee{primary: primary}
	for _, h := range sinks {
		if h == nil {
			continue
		}
		s := &teeSink{h: h, queue: make(chan func(AnalyticsHandler), TeeQueueSize), done: make(chan struct{})}
		go s.run()
		t.sinks = append(t.sinks, s)
	}
	return t
}

func (s *teeSink) run() {
	defer close(s.done)
	for fn := range s.queue {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Analytics sink panicked: %v", r)
				}
			}()
			fn(s.h)
		}()
	}
}

func (t *Tee) toSinks(fn func(AnalyticsHandler)) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stopped {
		return
	}
	for _, s := range t.sinks {
		select {
		case s.queue <- fn:
		default:
			if n := s.dropped.Add(1); n == 1 || n%1000 == 0 {
				logger.Warnf("Analytics sink is falling behind; %d record(s) not sent to it", n)
			}
		}
	}
}

// Dropped reports, per sink, how many records did not fit its queue.
func (t *Tee) Dropped() []uint64 {
	out := make([]uint64, len(t.sinks))
	for i, s := range t.sinks {
		out[i] = s.dropped.Load()
	}
	return out
}

// Stop waits up to timeout for the sinks to take what is queued. Records
// sent after Stop are lost to the sinks (the primary still gets them).
func (t *Tee) Stop(timeout time.Duration) {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	t.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range t.sinks {
		close(s.queue)
		wg.Add(1)
		go func(s *teeSink) { defer wg.Done(); <-s.done }(s)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warn("Analytics sinks did not finish in time; some queued records were not sent")
	}
}

func copyChatRecord(r *models.LLMChatRecord) *models.LLMChatRecord {
	if r == nil {
		return nil
	}
	c := *r
	if r.TeamID != nil {
		id := *r.TeamID
		c.TeamID = &id
	}
	return &c
}

func copyProxyLog(l *models.ProxyLog) *models.ProxyLog {
	if l == nil {
		return nil
	}
	c := *l
	return &c
}

func (t *Tee) RecordChatRecord(ctx context.Context, record *models.LLMChatRecord) {
	c := copyChatRecord(record)
	t.primary.RecordChatRecord(ctx, record)
	t.toSinks(func(h AnalyticsHandler) { h.RecordChatRecord(context.Background(), c) })
}

func (t *Tee) RecordChatLogEntry(ctx context.Context, log *models.LLMChatLogEntry) {
	var c *models.LLMChatLogEntry
	if log != nil {
		cc := *log
		c = &cc
	}
	t.primary.RecordChatLogEntry(ctx, log)
	t.toSinks(func(h AnalyticsHandler) { h.RecordChatLogEntry(context.Background(), c) })
}

func (t *Tee) RecordProxyLog(ctx context.Context, log *models.ProxyLog) {
	c := copyProxyLog(log)
	t.primary.RecordProxyLog(ctx, log)
	t.toSinks(func(h AnalyticsHandler) { h.RecordProxyLog(context.Background(), c) })
}

func (t *Tee) RecordToolCall(ctx context.Context, name string, timestamp time.Time, execTime int, toolID uint) {
	t.primary.RecordToolCall(ctx, name, timestamp, execTime, toolID)
	t.toSinks(func(h AnalyticsHandler) { h.RecordToolCall(context.Background(), name, timestamp, execTime, toolID) })
}

// SetAsGlobalHandler makes the Tee the process's analytics handler.
func (t *Tee) SetAsGlobalHandler() { SetHandler(t) }

func (t *Tee) RecordChatRecordsBatch(ctx context.Context, records []*models.LLMChatRecord) {
	c := make([]*models.LLMChatRecord, len(records))
	for i, r := range records {
		c[i] = copyChatRecord(r)
	}
	t.primary.RecordChatRecordsBatch(ctx, records)
	t.toSinks(func(h AnalyticsHandler) { h.RecordChatRecordsBatch(context.Background(), c) })
}

func (t *Tee) RecordProxyLogsBatch(ctx context.Context, logs []*models.ProxyLog) {
	c := make([]*models.ProxyLog, len(logs))
	for i, l := range logs {
		c[i] = copyProxyLog(l)
	}
	t.primary.RecordProxyLogsBatch(ctx, logs)
	t.toSinks(func(h AnalyticsHandler) { h.RecordProxyLogsBatch(context.Background(), c) })
}

func (t *Tee) RecordComplianceEvents(ctx context.Context, events []*models.ComplianceEvent) {
	c := make([]*models.ComplianceEvent, len(events))
	for i, e := range events {
		if e != nil {
			ce := *e
			c[i] = &ce
		}
	}
	t.primary.RecordComplianceEvents(ctx, events)
	t.toSinks(func(h AnalyticsHandler) { h.RecordComplianceEvents(context.Background(), c) })
}

// RecordExchange keeps a proxied request's log and chat record together for
// handlers that store them as one (ExchangeRecorder) and splits them for the
// others, as the package-level RecordExchange does.
func (t *Tee) RecordExchange(ctx context.Context, log *models.ProxyLog, rec *models.LLMChatRecord) {
	cl, cr := copyProxyLog(log), copyChatRecord(rec)
	recordExchangeWith(ctx, t.primary, log, rec)
	t.toSinks(func(h AnalyticsHandler) { recordExchangeWith(context.Background(), h, cl, cr) })
}

func recordExchangeWith(ctx context.Context, h AnalyticsHandler, log *models.ProxyLog, rec *models.LLMChatRecord) {
	if er, ok := h.(ExchangeRecorder); ok {
		er.RecordExchange(ctx, log, rec)
		return
	}
	h.RecordProxyLog(ctx, log)
	if rec != nil {
		h.RecordChatRecord(ctx, rec)
	}
}

var _ AnalyticsHandler = (*Tee)(nil)
var _ ExchangeRecorder = (*Tee)(nil)
