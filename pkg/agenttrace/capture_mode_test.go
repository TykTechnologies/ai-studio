package agenttrace

import (
	"context"
	"testing"
)

// modeProcessor resolves every span to one mode.
type modeProcessor struct{ mode string }

func (p modeProcessor) Process(ev *Event)  { ev.ContentMode = p.mode; ev.Raw = nil }
func (p modeProcessor) Mode(*Event) string { return p.mode }

// CaptureRaw does not copy content the span's mode drops (jyx.34: the copy
// was most of the capture's allocation in the default off mode).
func TestCaptureRawSkipsContentTheModeDrops(t *testing.T) {
	withEnabled(t, true)
	t.Cleanup(func() { SetContentProcessor(nil) })
	for _, tc := range []struct {
		mode string
		kept int
	}{{ContentOff, 0}, {ContentMetadata, 1}, {ContentFull, 1}} {
		SetContentProcessor(modeProcessor{tc.mode})
		_, r := Begin(context.Background())
		r.CaptureRaw(PartRequest, "application/json", []byte(`{"a":1}`))
		if got := len(r.span.Raw); got != tc.kept {
			t.Errorf("mode %s: %d raw parts, want %d", tc.mode, got, tc.kept)
		}
	}
	// Without a processor nothing is decided up front.
	SetContentProcessor(nil)
	_, r := Begin(context.Background())
	r.CaptureRaw(PartRequest, "application/json", []byte(`{"a":1}`))
	if len(r.span.Raw) != 1 {
		t.Fatal("raw part dropped without a processor")
	}
}
