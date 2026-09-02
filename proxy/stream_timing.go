package proxy

import (
	"context"
	"sync/atomic"
	"time"
)

// streamTiming carries the per-request streaming latency landmarks needed by the
// OpenTelemetry GenAI metrics. The two values are recorded in different places:
// time-to-first-token is known in the chunk read loop, while time-per-output-token
// additionally needs the completion token count, which only becomes available in
// the analytics path (analyzeStreamingResponse -> AnalyzeCompletionResponse).
// The struct is passed between them on the request context.
//
// firstChunkAt is stored as Unix nanoseconds in an atomic so the value is safe to
// read from the analytics goroutine while the stream is still being drained.
type streamTiming struct {
	start        time.Time
	firstChunkAt atomic.Int64
}

type streamTimingKey struct{}

func newStreamTiming(start time.Time) *streamTiming {
	return &streamTiming{start: start}
}

func withStreamTiming(ctx context.Context, t *streamTiming) context.Context {
	return context.WithValue(ctx, streamTimingKey{}, t)
}

// streamTimingFrom returns the timing carried on ctx, or nil when the request is
// not a streaming one.
func streamTimingFrom(ctx context.Context) *streamTiming {
	t, _ := ctx.Value(streamTimingKey{}).(*streamTiming)
	return t
}

// markFirstChunk records the arrival of the first streamed chunk. Subsequent
// calls are ignored, so it is safe to call unconditionally inside the read loop.
func (t *streamTiming) markFirstChunk(at time.Time) {
	if t == nil {
		return
	}
	t.firstChunkAt.CompareAndSwap(0, at.UnixNano())
}

// timeToFirstToken reports the latency from request start to the first chunk.
// ok is false when no chunk ever arrived.
func (t *streamTiming) timeToFirstToken() (time.Duration, bool) {
	if t == nil {
		return 0, false
	}
	ns := t.firstChunkAt.Load()
	if ns == 0 {
		return 0, false
	}
	return time.Unix(0, ns).Sub(t.start), true
}

// sinceFirstToken reports the time spent generating everything after the first
// chunk. ok is false when no chunk ever arrived.
func (t *streamTiming) sinceFirstToken(end time.Time) (time.Duration, bool) {
	if t == nil {
		return 0, false
	}
	ns := t.firstChunkAt.Load()
	if ns == 0 {
		return 0, false
	}
	return end.Sub(time.Unix(0, ns)), true
}
