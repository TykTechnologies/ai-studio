package analytics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// CaptureRecorder is implemented by handlers that stage Agent Call Tracing
// capture events (spec §4.1, D3): the hub's DatabaseHandler writes them to
// the staging table, the microgateway's handler sends them on the pulse.
// It is only ever called while tracing is on. Implementations must never
// block the request.
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

// CaptureSink is the agenttrace.Sink that sends finished captures to the
// global analytics handler. The hub installs it with agenttrace.SetSink once
// the tracing start gate has passed; so does the Enterprise microgateway.
func CaptureSink() agenttrace.Sink { return captureSink{} }

// Capture queue limits per node (spec §10: capture.queue_max_events and
// queue_max_bytes). Studio sets them from AGENT_TRACING_QUEUE_MAX_*.
var (
	captureMaxEvents atomic.Int64
	captureMaxBytes  atomic.Int64
)

// captureBatchSize and captureFlushInterval bound a staging write (spec §10:
// batch <= 500, flush <= 1 s).
const (
	captureBatchSize     = 500
	captureFlushInterval = time.Second
)

// SetCaptureLimits sets the capture queue caps; zero or less keeps the
// default (10,000 events, 64 MiB). Call it before the first capture.
func SetCaptureLimits(maxEvents int, maxBytes int64) {
	captureMaxEvents.Store(int64(maxEvents))
	captureMaxBytes.Store(maxBytes)
}

func recordCaptureDrop(reason string, n int64) {
	metrics.RecordTracingCaptureDropped(context.Background(), reason, n)
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
	if h.capture.Add(events...) {
		select {
		case h.captureKick <- struct{}{}:
		default:
		}
	}
}

func (h *DatabaseHandler) startCapture() {
	h.capture = agenttrace.NewQueue(int(captureMaxEvents.Load()), captureMaxBytes.Load(), captureBatchSize, recordCaptureDrop)
	h.captureKick = make(chan struct{}, 1)
	metrics.SetTracingQueueDepth(h.capture.Depth)
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
		case <-h.captureKick:
		}
		for {
			b := h.capture.Take()
			if len(b.Events) == 0 {
				break
			}
			select {
			case h.captureBatchChan <- b:
			case <-h.ctx.Done():
				return
			}
			if len(b.Events) < captureBatchSize {
				break
			}
		}
	}
}

// StageCaptureEvents writes events received from source (an edge id) to the
// staging table synchronously, skipping duplicates, and returns how many rows
// were inserted. The hub's pulse handler uses it so that a failed write fails
// the pulse and the edge retries; the idempotency key makes retries safe.
func StageCaptureEvents(db *gorm.DB, source string, events []*agenttrace.Event) (int64, error) {
	rows := EncodeCaptureRows(source, events, recordCaptureDrop)
	if len(rows) == 0 {
		return 0, nil
	}
	return models.InsertAgentTraceCaptureEvents(db, rows)
}

// EncodeCaptureRows turns events into staging rows for source
// (SourceEmbedded or an edge id). The idempotency key is always rebuilt from
// source, so an edge cannot write keys in another source's space.
func EncodeCaptureRows(source string, events []*agenttrace.Event, onDrop func(reason string, n int64)) []models.AgentTraceCaptureEvent {
	now := time.Now()
	rows := make([]models.AgentTraceCaptureEvent, 0, len(events))
	var noSealer int64
	for _, ev := range events {
		switch ev.Kind {
		case agenttrace.KindLLMInference, agenttrace.KindToolCall, agenttrace.KindDatasourceCall,
			agenttrace.KindPluginEndpoint, agenttrace.KindGatewayEvent:
		default:
			// Only known kinds are staged: an unknown (or over-long) kind
			// would overflow the staging row's kind column on Postgres and
			// fail the whole insert (jyx.126).
			onDrop("unknown_kind", 1)
			continue
		}
		ev.SetIdempotencyKey(source)
		blob, stripped, err := agenttrace.EncodeStaged(ev)
		if err != nil {
			logger.Warnf("Agent tracing: dropping a capture event that could not be encoded: %s", sanitizeError(err))
			onDrop(agenttrace.DropWriteError, 1)
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
		onDrop(agenttrace.DropNoSealer, noSealer)
	}
	return rows
}

// writeCaptureBatch stages one batch (on the analytics worker).
func (h *DatabaseHandler) writeCaptureBatch(b agenttrace.Batch) {
	defer h.capture.Release(b)
	rows := EncodeCaptureRows(agenttrace.SourceEmbedded, b.Events, h.capture.Drop)
	if len(rows) == 0 {
		return
	}
	err := h.createRecordWithRetry(func() error {
		_, err := models.InsertAgentTraceCaptureEvents(h.db, rows)
		return err
	})
	if err != nil {
		logger.Warnf("Agent tracing: error staging %d capture events: %s", len(rows), sanitizeError(err))
		h.capture.Drop(agenttrace.DropWriteError, int64(len(rows)))
	}
}

var _ CaptureRecorder = (*DatabaseHandler)(nil)
