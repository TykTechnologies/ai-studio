package agenttrace

import (
	"context"
	"sync"
	"testing"
)

// A Finish while held waits for the last release; updates made meanwhile
// are kept.
func TestRecorderHoldDefersFinish(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	_, r := Begin(context.Background())
	release := r.Hold()
	r.Finish() // the handler returned
	if len(sink.events) != 0 {
		t.Fatal("finished while held")
	}
	r.Update(func(ev *Event) { ev.Kind = KindLLMInference; ev.Provider = "aws.bedrock" })
	release()
	release() // idempotent
	if len(sink.events) != 1 || sink.events[0].Provider != "aws.bedrock" {
		t.Fatalf("got %+v", sink.events)
	}
}

func TestRecorderHoldWithoutFinishDoesNotEmit(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	_, r := Begin(context.Background())
	r.Update(func(ev *Event) { ev.Kind = KindLLMInference })
	r.Hold()()
	if len(sink.events) != 0 {
		t.Fatal("a release without a pending Finish must not finish")
	}
	r.Finish()
	if len(sink.events) != 1 {
		t.Fatal("Finish after release")
	}
}

func TestRecorderHoldConcurrent(t *testing.T) {
	withEnabled(t, true)
	sink := withSink(t)
	_, r := Begin(context.Background())
	r.Update(func(ev *Event) { ev.Kind = KindLLMInference })
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		release := r.Hold()
		wg.Add(1)
		go func() { defer wg.Done(); r.Update(func(ev *Event) { ev.Attempt++ }); release() }()
	}
	r.Finish()
	wg.Wait()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 || sink.events[0].Attempt != 16 {
		t.Fatalf("got %d events", len(sink.events))
	}
}

func TestHoldOffAllocatesNothing(t *testing.T) {
	var r *Recorder
	if n := testing.AllocsPerRun(100, func() { r.Hold()() }); n != 0 {
		t.Fatalf("allocates %.0f", n)
	}
}
