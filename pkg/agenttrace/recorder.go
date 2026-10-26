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
	// holds counts work still filling the capture after the handler
	// returned (Hold); finishWanted is a Finish that waits for it.
	holds        int
	finishWanted bool
	// discarded is set by Discard; From then returns nil, so every hook on
	// the request stops at its first check.
	discarded atomic.Bool
	// sink is the Sink installed when the capture began (nil: none was).
	// Finish hands the events to it rather than to the Sink installed by
	// then: work that outlives the request (Hold) finishes the capture late,
	// and must not land in a Sink installed after the request arrived (the
	// next test's, when tests swap the Sink). With none at Begin, Finish
	// uses the Sink installed when it runs.
	sink *sinkHolder
}

func noRelease() {}

// Hold keeps the capture open for work that completes after the request
// handler returns (the analytics goroutine): a Finish while held waits until
// every release has been called. The release is idempotent; on a nil
// Recorder Hold returns a no-op and allocates nothing.
func (r *Recorder) Hold() (release func()) {
	if r == nil {
		return noRelease
	}
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return noRelease
	}
	r.holds++
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			r.holds--
			fire := r.holds == 0 && r.finishWanted
			r.mu.Unlock()
			if fire {
				r.Finish()
			}
		})
	}
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
	r := &Recorder{sink: currentSink.Load()}
	r.span.SpanID = NewSpanID()
	r.span.StartTime = time.Now()
	if n := gatewayNode.Load(); n != nil {
		r.span.GatewayNode = *n
	}
	return context.WithValue(ctx, ctxKey{}, r), r
}

var gatewayNode atomic.Pointer[string]

// SetGatewayNode names the gateway every capture of this process comes
// from (spans.gateway_node): the hub's cluster node id, or the edge's id.
// The hub overwrites an edge event's value with the authenticated edge id
// at ingest.
func SetGatewayNode(id string) {
	if id == "" {
		gatewayNode.Store(nil)
		return
	}
	gatewayNode.Store(&id)
}

// Fork begins a capture of its own for work that outlives the request that
// carried it: an MCP tools/call on the legacy SSE transport is answered on
// the event stream after its POST already returned 202. The new Recorder
// continues the request's context (trace id, declared trace context and
// parent, caller identity, gateway node) under a fresh span id with
// StartTime now; the caller finishes it. With no Recorder on ctx (or the
// flag off) it returns ctx unchanged and nil.
func Fork(ctx context.Context) (context.Context, *Recorder) {
	parent := From(ctx)
	if parent == nil {
		return ctx, nil
	}
	r := &Recorder{sink: parent.sink}
	parent.mu.Lock()
	r.span.TraceID = parent.span.TraceID
	r.span.ParentSpanID = parent.span.ParentSpanID
	r.span.Declared = parent.span.Declared
	r.span.Identity = parent.span.Identity
	r.span.GatewayNode = parent.span.GatewayNode
	parent.mu.Unlock()
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
	if r != nil && r.discarded.Load() {
		return nil
	}
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

// MarkAttemptOf records that this request is one attempt of the outer
// logical call whose span id is outer (the inner hop of the /ai/ loopback).
func (r *Recorder) MarkAttemptOf(outer SpanID) {
	if r == nil || outer.IsZero() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.span.AttemptOf = outer
}

// LogicalSpanID is the span id of the logical call this request belongs to:
// the outer call's for an attempt, otherwise its own. proxy_logs rows carry
// it (trace_span_id), so every failover rung of one call shares it.
func (r *Recorder) LogicalSpanID() SpanID {
	if r == nil {
		return SpanID{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.span.AttemptOf.IsZero() {
		return r.span.AttemptOf
	}
	return r.span.SpanID
}

// SetTraceID sets the provisional trace id (the declared one, or one the
// gateway minted) when none is set yet.
func (r *Recorder) SetTraceID(t TraceID) {
	if r == nil || t.IsZero() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.span.TraceID.IsZero() {
		r.span.TraceID = t
	}
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

// Finish completes the capture and hands the events to the Sink installed
// when the capture began (else the one installed now): the primary
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
	if r.holds > 0 {
		r.finishWanted = true
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

	h := r.sink
	if h == nil {
		h = currentSink.Load()
	}
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
// App has tracing off). Later Finish calls do nothing, and From returns nil
// for the request from then on, so no hook does any further work.
func (r *Recorder) Discard() {
	if r == nil {
		return
	}
	r.discarded.Store(true)
	r.mu.Lock()
	r.finished = true
	r.extra = nil
	r.mu.Unlock()
}
