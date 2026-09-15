package chat_session

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEventsSession builds a minimal session in events mode with an in-memory
// queue and the fan-out running, without touching the database or an LLM.
func newEventsSession(t *testing.T) *ChatSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cs := &ChatSession{
		id:          "evt-test",
		input:       make(chan *models.UserMessage, 1),
		queue:       NewInMemoryQueue("evt-test", 64),
		stop:        make(chan struct{}),
		ctx:         ctx,
		ctxCancel:   cancel,
		tools:       map[string]models.Tool{},
		datasources: map[uint]*models.Datasource{},
		files:       map[string]string{},
		outputMode:  OutputModeEvents,
	}
	cs.startEventFanout()
	t.Cleanup(cs.Stop)
	return cs
}

func collect(t *testing.T, ch <-chan ChatEvent, n int) []ChatEvent {
	t.Helper()
	var out []ChatEvent
	deadline := time.After(2 * time.Second)
	for len(out) < n {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timed out after %d/%d events: %+v", len(out), n, out)
		}
	}
	return out
}

func TestEvents_RunLifecycleReachesSubscriber(t *testing.T) {
	cs := newEventsSession(t)
	events, unsub := cs.Subscribe("run-1", 16)
	defer unsub()

	cs.beginRun("run-1")
	cs.markStreamed()
	cs.emit(EventTextDelta, TextDeltaData{Delta: "hel"})
	cs.emit(EventTextDelta, TextDeltaData{Delta: "lo"})
	cs.sendStatus("Running governance filters")
	cs.sendError(errors.New("error calling tool operation [x]: boom"))
	cs.finishRun(FinishStop)

	got := collect(t, events, 6)
	kinds := []string{}
	for i, ev := range got {
		kinds = append(kinds, ev.Kind)
		assert.Equal(t, "run-1", ev.RunID)
		assert.Equal(t, uint64(i+1), ev.Seq, "sequence numbers are contiguous per run")
	}
	assert.Equal(t, []string{EventStart, EventTextDelta, EventTextDelta, EventStatus, EventError, EventFinish}, kinds)

	var errData ErrorData
	require.NoError(t, json.Unmarshal(got[4].Data, &errData))
	assert.Equal(t, ErrCodeTool, errData.Code)

	var fin FinishData
	require.NoError(t, json.Unmarshal(got[5].Data, &fin))
	assert.Equal(t, FinishStop, fin.Reason)
	assert.Equal(t, "", cs.ActiveRunID(), "finishRun clears the active run")
}

func TestEvents_CancelledRunFinishesAsCancelled(t *testing.T) {
	cs := newEventsSession(t)
	events, unsub := cs.Subscribe("run-2", 16)
	defer unsub()

	cs.beginRun("run-2")
	runCtx := cs.runCtx()
	assert.True(t, cs.CancelRun())
	assert.Error(t, runCtx.Err(), "cancel aborts the run context an LLM call would use")
	cs.finishRun(FinishStop)

	got := collect(t, events, 2)
	var fin FinishData
	require.NoError(t, json.Unmarshal(got[1].Data, &fin))
	assert.Equal(t, FinishCancelled, fin.Reason)
	assert.False(t, cs.CancelRun(), "nothing left to cancel")
}

func TestEvents_EventsWithoutSubscriberAreDropped(t *testing.T) {
	cs := newEventsSession(t)
	cs.beginRun("lonely")
	cs.emit(EventStatus, StatusData{Text: "nobody listening"})
	cs.finishRun(FinishStop)

	events, unsub := cs.Subscribe("later", 16)
	defer unsub()
	cs.beginRun("later")
	cs.finishRun(FinishStop)
	got := collect(t, events, 2)
	assert.Equal(t, "later", got[0].RunID, "a later subscriber never sees the earlier run")
}

func TestEvents_RawModeEmitsNothing(t *testing.T) {
	cs := newEventsSession(t)
	cs.outputMode = OutputModeRaw
	cs.beginRun("raw")
	cs.emit(EventTextDelta, TextDeltaData{Delta: "x"})
	cs.finishRun(FinishStop)
	select {
	case b := <-cs.queue.ConsumeStream(context.Background()):
		t.Fatalf("raw mode must not publish envelopes, got %s", b)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEvents_StopIsIdempotentAndNonBlocking(t *testing.T) {
	cs := newEventsSession(t)
	done := make(chan struct{})
	go func() {
		cs.Stop()
		cs.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked")
	}
	assert.True(t, cs.Stopped())
	assert.Error(t, cs.ctx.Err())
}

func TestDecodeChatEvent(t *testing.T) {
	_, ok := DecodeChatEvent([]byte("plain chunk"))
	assert.False(t, ok)
	_, ok = DecodeChatEvent([]byte(`{"role":"ai","text":"final"}`))
	assert.False(t, ok, "a stored MessageContent is not an envelope")
	ev, ok := DecodeChatEvent([]byte(`{"v":"aui/v1","run_id":"r","seq":3,"kind":"text-delta","data":{"delta":"x"}}`))
	require.True(t, ok)
	assert.Equal(t, EventTextDelta, ev.Kind)
	assert.Equal(t, uint64(3), ev.Seq)
}

func TestClassifyError(t *testing.T) {
	cases := map[string]string{
		"blocked by filter 'pii': no":                     ErrCodeFilter,
		"Response blocked: policy":                        ErrCodeFilter,
		"API returned unexpected status code: 429":        ErrCodeAPI,
		"Unauthorized":                                    ErrCodeAuth,
		"context deadline exceeded":                       ErrCodeConnection,
		"error calling tool operation [getPets]: 500":     ErrCodeTool,
		"failed to create message: anthropic bad request": ErrCodeLLMConfig,
		"something else entirely":                         ErrCodeInternal,
	}
	for msg, want := range cases {
		assert.Equal(t, want, ClassifyError(errors.New(msg)), msg)
	}
	assert.Equal(t, ErrCodeInternal, ClassifyError(nil))
}
