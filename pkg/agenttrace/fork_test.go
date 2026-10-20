package agenttrace

import (
	"context"
	"testing"
)

// Fork continues the request's context under a span of its own, finished on
// its own; the request's capture is untouched.
func TestFork(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	ctx, parent := Begin(context.Background())
	trace := NewTraceID()
	parent.SetTraceID(trace)
	parent.Update(func(ev *Event) {
		ev.Identity.AppID = 7
		ev.Declared.TraceSource = "traceparent"
		ev.GatewayNode = "edge-1"
	})
	parent.Finish() // the SSE POST returned 202

	fctx, child := Fork(ctx)
	if child == nil || From(fctx) != child {
		t.Fatal("Fork attaches a new Recorder")
	}
	if child.SpanID() == parent.SpanID() || child.SpanID().IsZero() {
		t.Fatal("the fork has its own span id")
	}
	child.Update(func(ev *Event) { ev.Kind = KindToolCall })
	child.Finish()

	if len(sink.events) != 1 {
		t.Fatalf("got %d events, want the fork's only", len(sink.events))
	}
	ev := sink.events[0]
	if ev.TraceID != trace || ev.Identity.AppID != 7 || ev.Declared.TraceSource != "traceparent" || ev.GatewayNode != "edge-1" {
		t.Fatalf("context not carried: %+v", ev)
	}
}

func TestForkOff(t *testing.T) {
	withEnabled(t, false)
	ctx := context.Background()
	if got, r := Fork(ctx); r != nil || got != ctx {
		t.Fatal("Fork with tracing off is a no-op")
	}
	if n := testing.AllocsPerRun(100, func() { Fork(ctx) }); n != 0 {
		t.Fatalf("Fork off allocates %v", n)
	}
}
