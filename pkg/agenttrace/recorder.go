package agenttrace

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Sink receives finished events. The embedded transport (CaptureRecorder on
// the analytics handler) and the edge transport (pulse sub-buffer) implement
// it. Enqueue must never block the request (§10): it returns false when the
// event was dropped.
type Sink interface {
	Enqueue(ev *Event) bool
}

type sinkHolder struct{ s Sink }

var (
	currentSink   atomic.Pointer[sinkHolder]
	droppedNoSink atomic.Uint64
)

// SetSink installs the process-wide Sink. Nil removes it.
func SetSink(s Sink) {
	if s == nil {
		currentSink.Store(nil)
		return
	}
	currentSink.Store(&sinkHolder{s: s})
}

// DroppedNoSink counts events finished while no Sink was installed.
func DroppedNoSink() uint64 { return droppedNoSink.Load() }

type ctxKey struct{}

// Recorder collects the capture for one gateway request: its primary span
// and any extra events. It is safe for concurrent use (the streaming parser
// runs in its own goroutine). Every method is a no-op on a nil Recorder, and
// every write after Finish or Discard is ignored.
type Recorder struct {
	mu       sync.Mutex
	span     Event
	extra    []*Event
	finished bool
}

// Begin attaches a Recorder to ctx and returns both. With the flag off it
// returns ctx unchanged and a nil Recorder, allocating nothing. If ctx
// already carries a Recorder (nested handlers), that one is returned.
//
// The primary span gets a fresh span id and StartTime now.
func Begin(ctx context.Context) (context.Context, *Recorder) {
	if !Enabled() {
		return ctx, nil
	}
	if r, ok := ctx.Value(ctxKey{}).(*Recorder); ok && r != nil {
		return ctx, r
	}
	r := &Recorder{}
	r.span.SpanID = NewSpanID()
	r.span.StartTime = time.Now()
	return context.WithValue(ctx, ctxKey{}, r), r
}

// From returns the request's Recorder, or nil when the flag is off or none
// was begun. It allocates nothing.
func From(ctx context.Context) *Recorder {
	if ctx == nil || !Enabled() {
		return nil
	}
	r, _ := ctx.Value(ctxKey{}).(*Recorder)
	return r
}

// Emit adds an extra event to the request's Recorder (for example a
// gateway_event span with no parent span of its own). No-op when there is no
// Recorder. The Recorder owns ev afterwards.
func Emit(ctx context.Context, ev *Event) { From(ctx).Emit(ev) }

// AddPart adds a payload part to the request's primary span.
func AddPart(ctx context.Context, p PayloadPart) { From(ctx).AddPart(p) }

// AddGatewayEvent adds a gateway event to the request's primary span.
func AddGatewayEvent(ctx context.Context, ge GatewayEvent) { From(ctx).AddGatewayEvent(ge) }

// SpanID returns the primary span id, or the zero id on a nil Recorder.
func (r *Recorder) SpanID() SpanID {
	if r == nil {
		return SpanID{}
	}
	return r.span.SpanID // set once in Begin, never written again
}

// TraceID returns the primary span's provisional trace id.
func (r *Recorder) TraceID() TraceID {
	if r == nil {
		return TraceID{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.span.TraceID
}

// Update runs fn on the primary span under the Recorder's lock. fn must not
// retain the pointer or call back into the Recorder. Ignored after Finish.
func (r *Recorder) Update(fn func(ev *Event)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	fn(&r.span)
}

// AddPart adds a payload part to the primary span.
func (r *Recorder) AddPart(p PayloadPart) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.span.Parts = append(r.span.Parts, p)
}

// AddGatewayEvent adds a gateway event to the primary span. A zero At is
// stamped with the current time.
func (r *Recorder) AddGatewayEvent(ge GatewayEvent) {
	if r == nil {
		return
	}
	if ge.At.IsZero() {
		ge.At = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.span.GatewayEvents = append(r.span.GatewayEvents, ge)
}

// Emit adds an extra event. A zero span id gets a fresh one and a zero
// StartTime is stamped now.
func (r *Recorder) Emit(ev *Event) {
	if r == nil || ev == nil {
		return
	}
	if ev.SpanID.IsZero() {
		ev.SpanID = NewSpanID()
	}
	if ev.StartTime.IsZero() {
		ev.StartTime = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.extra = append(r.extra, ev)
}

// Finish completes the capture and hands the events to the Sink: the primary
// span when it has a Kind, then the extra events. A zero EndTime is stamped
// now and an empty Status becomes ok. Only the first call has an effect.
func (r *Recorder) Finish() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	r.finished = true
	now := time.Now()
	var primary *Event
	if r.span.Kind != "" {
		primary = &r.span
		if primary.EndTime.IsZero() {
			primary.EndTime = now
		}
		if primary.Status == "" {
			primary.Status = StatusOK
		}
	}
	extra := r.extra
	r.extra = nil
	r.mu.Unlock()

	for _, ev := range extra {
		if ev.EndTime.IsZero() {
			ev.EndTime = now
		}
		if ev.Status == "" {
			ev.Status = StatusOK
		}
	}

	h := currentSink.Load()
	if h == nil {
		n := uint64(len(extra))
		if primary != nil {
			n++
		}
		droppedNoSink.Add(n)
		return
	}
	if primary != nil {
		h.s.Enqueue(primary)
	}
	for _, ev := range extra {
		h.s.Enqueue(ev)
	}
}

// Discard ends the capture without emitting anything (for example when the
// App has tracing off). Later Finish calls do nothing.
func (r *Recorder) Discard() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.finished = true
	r.extra = nil
	r.mu.Unlock()
}
