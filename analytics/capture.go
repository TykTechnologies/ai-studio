package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// CaptureRecorder is implemented by handlers that stage Agent Call Tracing
// capture events (spec §4.1, D3). The embedded gateway reaches it through
// the agenttrace Sink that CaptureSink returns; it is only ever called while
// tracing is on. Implementations must never block the request.
type CaptureRecorder interface {
	RecordCaptureEvents(ctx context.Context, events []*agenttrace.Event)
}

// RecordCaptureEvents hands capture events to the global handler when it
// stages them. Other handlers ignore them.
func RecordCaptureEvents(ctx context.Context, events []*agenttrace.Event) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	if cr, ok := globalHandler.(CaptureRecorder); ok {
		cr.RecordCaptureEvents(ctx, events)
	}
}

type captureSink struct{}

func (captureSink) Enqueue(ev *agenttrace.Event) bool {
	RecordCaptureEvents(context.Background(), []*agenttrace.Event{ev})
	return true
}

// CaptureSink is the agenttrace.Sink for the hub's embedded gateway: finished
// captures go to the global analytics handler. Studio installs it with
// agenttrace.SetSink once the tracing start gate has passed.
func CaptureSink() agenttrace.Sink { return captureSink{} }

// Capture queue limits per node (spec §10: capture.queue_max_events and
// queue_max_bytes). Studio sets them from AGENT_TRACING_QUEUE_MAX_*.
var (
	captureMaxEvents atomic.Int64
	captureMaxBytes  atomic.Int64
)

const (
	defaultCaptureMaxEvents = 10000
	defaultCaptureMaxBytes  = 64 << 20
	// captureBatchSize and captureFlushInterval bound a staging write
	// (spec §10: batch <= 500, flush <= 1 s).
	captureBatchSize     = 500
	captureFlushInterval = time.Second
)

// SetCaptureLimits sets the capture queue caps; zero or less keeps the default.
func SetCaptureLimits(maxEvents int, maxBytes int64) {
	captureMaxEvents.Store(int64(maxEvents))
	captureMaxBytes.Store(maxBytes)
}

// Capture drop reasons (tyk_tracing_capture_dropped{reason}).
const (
	DropPayload            = "payload"              // payload bytes removed, span kept (§10 step 1)
	DropGatewayEventDetail = "gateway_event_detail" // event details removed (§10 step 2)
	DropSpan               = "span"                 // whole span dropped (§10 step 3)
	DropNoSealer           = "no_sealer"            // payload bytes removed: no key to seal them
	DropWriteError         = "write_error"          // the staging write failed
)

// captureQueue is the bounded queue between the request path and the
// analytics worker. Admission never blocks: when the caps are reached the
// incoming event is degraded in the §10 order until it fits, or dropped.
// Events count against the caps until the worker has written them.
type captureQueue struct {
	mu        sync.Mutex
	pending   []*agenttrace.Event
	sizes     []int64
	held      int64 // events pending or in a batch not yet written
	heldBytes int64
	maxEvents int64
	maxBytes  int64
	kick      chan struct{}

	dropped sync.Map // reason -> *atomic.Uint64
}

type captureBatch struct {
	events []*agenttrace.Event
	bytes  int64
}

func newCaptureQueue() *captureQueue {
	q := &captureQueue{
		maxEvents: captureMaxEvents.Load(),
		maxBytes:  captureMaxBytes.Load(),
		kick:      make(chan struct{}, 1),
	}
	if q.maxEvents <= 0 {
		q.maxEvents = defaultCaptureMaxEvents
	}
	if q.maxBytes <= 0 {
		q.maxBytes = defaultCaptureMaxBytes
	}
	return q
}

func (q *captureQueue) drop(reason string, n int64) {
	v, _ := q.dropped.LoadOrStore(reason, new(atomic.Uint64))
	v.(*atomic.Uint64).Add(uint64(n))
	metrics.RecordTracingCaptureDropped(context.Background(), reason, n)
}

// Dropped returns how many drops were counted for reason.
func (q *captureQueue) Dropped(reason string) uint64 {
	if v, ok := q.dropped.Load(reason); ok {
		return v.(*atomic.Uint64).Load()
	}
	return 0
}

// add admits events, degrading or dropping them to fit the caps.
func (q *captureQueue) add(events []*agenttrace.Event) {
	q.mu.Lock()
	for _, ev := range events {
		if ev == nil {
			continue
		}
		if q.held+1 > q.maxEvents {
			q.drop(DropSpan, 1)
			continue
		}
		size := int64(ev.ApproxSize())
		if q.heldBytes+size > q.maxBytes && agenttrace.StripPayloadData(ev) {
			q.drop(DropPayload, 1)
			size = int64(ev.ApproxSize())
		}
		if q.heldBytes+size > q.maxBytes && agenttrace.StripGatewayEventDetail(ev) {
			q.drop(DropGatewayEventDetail, 1)
			size = int64(ev.ApproxSize())
		}
		if q.heldBytes+size > q.maxBytes {
			q.drop(DropSpan, 1)
			continue
		}
		q.pending = append(q.pending, ev)
		q.sizes = append(q.sizes, size)
		q.held++
		q.heldBytes += size
	}
	full := len(q.pending) >= captureBatchSize
	q.mu.Unlock()
	if full {
		select {
		case q.kick <- struct{}{}:
		default:
		}
	}
}

// take removes up to captureBatchSize pending events. They stay counted
// against the caps until release.
func (q *captureQueue) take() captureBatch {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.pending)
	if n > captureBatchSize {
		n = captureBatchSize
	}
	b := captureBatch{events: make([]*agenttrace.Event, n)}
	copy(b.events, q.pending[:n])
	for _, s := range q.sizes[:n] {
		b.bytes += s
	}
	q.pending = append(q.pending[:0], q.pending[n:]...)
	q.sizes = append(q.sizes[:0], q.sizes[n:]...)
	return b
}

func (q *captureQueue) release(b captureBatch) {
	q.mu.Lock()
	q.held -= int64(len(b.events))
	q.heldBytes -= b.bytes
	q.mu.Unlock()
}

func (q *captureQueue) depth() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.held
}

// RecordCaptureEvents queues capture events for staging. The queue and its
// flusher start on first use, so a handler that never sees tracing traffic
// (tracing off, Community Edition) allocates nothing for it.
func (h *DatabaseHandler) RecordCaptureEvents(_ context.Context, events []*agenttrace.Event) {
	h.recMutex.RLock()
	started := h.recStarted
	h.recMutex.RUnlock()
	if !started || len(events) == 0 {
		return
	}
	h.captureOnce.Do(h.startCapture)
	h.capture.add(events)
}

func (h *DatabaseHandler) startCapture() {
	h.capture = newCaptureQueue()
	metrics.SetTracingQueueDepth(h.capture.depth)
	go h.flushCapture()
}

// flushCapture hands batches to the worker every captureFlushInterval, or as
// soon as a full batch is pending.
func (h *DatabaseHandler) flushCapture() {
	t := time.NewTicker(captureFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-t.C:
		case <-h.capture.kick:
		}
		for {
			b := h.capture.take()
			if len(b.events) == 0 {
				break
			}
			select {
			case h.captureBatchChan <- b:
			case <-h.ctx.Done():
				return
			}
			if len(b.events) < captureBatchSize {
				break
			}
		}
	}
}

// writeCaptureBatch stages one batch (on the analytics worker).
func (h *DatabaseHandler) writeCaptureBatch(b captureBatch) {
	defer h.capture.release(b)
	now := time.Now()
	rows := make([]models.AgentTraceCaptureEvent, 0, len(b.events))
	var noSealer int64
	for _, ev := range b.events {
		ev.SetIdempotencyKey(agenttrace.SourceEmbedded)
		blob, stripped, err := agenttrace.EncodeStaged(ev)
		if err != nil {
			logger.Warnf("Agent tracing: dropping a capture event that could not be encoded: %s", sanitizeError(err))
			h.capture.drop(DropWriteError, 1)
			continue
		}
		if stripped {
			noSealer++
		}
		rows = append(rows, models.AgentTraceCaptureEvent{
			IdempotencyKey: ev.IdempotencyKey,
			Source:         ev.Source,
			Kind:           string(ev.Kind),
			AppID:          ev.Identity.AppID,
			SpanID:         ev.SpanID.Bytes(),
			ReceivedAt:     now,
			Event:          blob,
			Size:           len(blob),
		})
	}
	if noSealer > 0 {
		h.capture.drop(DropNoSealer, noSealer)
	}
	if len(rows) == 0 {
		return
	}
	err := h.createRecordWithRetry(func() error {
		_, err := models.InsertAgentTraceCaptureEvents(h.db, rows)
		return err
	})
	if err != nil {
		logger.Warnf("Agent tracing: error staging %d capture events: %s", len(rows), sanitizeError(err))
		h.capture.drop(DropWriteError, int64(len(rows)))
	}
}

var _ CaptureRecorder = (*DatabaseHandler)(nil)
