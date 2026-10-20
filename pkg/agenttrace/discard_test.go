package agenttrace

import (
	"context"
	"testing"
)

// After Discard the request has no recorder as far as the hooks can tell:
// From returns nil, so each one stops at its first check (jyx.112).
func TestDiscardHidesTheRecorder(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	ctx, r := Begin(context.Background())
	if From(ctx) != r {
		t.Fatal("From returns the request's recorder")
	}
	r.Discard()
	if From(ctx) != nil {
		t.Fatal("From must return nil once the capture is discarded")
	}
	if release := r.Hold(); release == nil {
		t.Fatal("Hold on a discarded recorder still returns a release")
	}
	AddGatewayEvent(ctx, GatewayEvent{Type: GatewayEventBudget})
	r.Finish()
	if len(sink.events) != 0 {
		t.Fatalf("a discarded capture emitted %d events", len(sink.events))
	}
}
