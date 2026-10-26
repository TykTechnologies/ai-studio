package agenttrace

import (
	"context"
	"testing"
)

// A capture finished after the Sink was replaced (work held past the
// request: Hold) goes to the Sink installed when it began, and so does a
// capture forked from it. A capture begun with no Sink is dropped.
// midsommar-clean-vrc.19.
func TestFinishUsesTheSinkInstalledAtBegin(t *testing.T) {
	withEnabled(t, true)
	atBegin := withSink(t)

	ctx, rec := Begin(context.Background())
	rec.Update(func(ev *Event) { ev.Kind = KindLLMInference })
	release := rec.Hold()
	rec.Finish() // waits for the hold
	_, forked := Fork(ctx)
	forked.Update(func(ev *Event) { ev.Kind = KindToolCall })

	later := &captureSink{}
	SetSink(later) // the next test installs its own
	release()
	forked.Finish()

	if len(atBegin.events) != 2 || len(later.events) != 0 {
		t.Fatalf("sink at Begin got %d events, later sink %d; want 2 and 0", len(atBegin.events), len(later.events))
	}

	// Begun with no Sink installed: dropped, even if a Sink is installed
	// before it finishes (a test without a sink must not feed the next
	// test's).
	SetSink(nil)
	dropped := DroppedNoSink()
	_, unbound := Begin(context.Background())
	unbound.Update(func(ev *Event) { ev.Kind = KindLLMInference })
	SetSink(later)
	unbound.Finish()
	if len(later.events) != 0 || DroppedNoSink() != dropped+1 {
		t.Fatalf("a capture begun with no Sink is dropped: later sink got %d events, dropped %d", len(later.events), DroppedNoSink()-dropped)
	}
}
