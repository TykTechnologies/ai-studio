package chat_session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mute for side calls (title generation) must be scoped to that call's
// context: those calls run concurrently with the user's turn, and a
// session-wide flag used to swallow the opening of the next reply.
func TestStreamingMute_IsScopedToTheRequestContext(t *testing.T) {
	cs := newEventsSession(t)
	defer cs.Stop()
	cs.beginRun("run-mute")
	events, unsubscribe := cs.Subscribe("run-mute", 8)
	defer unsubscribe()

	// A chunk from a muted request is dropped ...
	require.NoError(t, cs.streamingFunc(mutedContext(context.Background()), []byte("Title")))
	// ... while the user's own turn keeps streaming at the same time.
	require.NoError(t, cs.streamingFunc(context.Background(), []byte("Hello")))

	// The fan-out delivers asynchronously: beginRun's start event, then
	// exactly one text delta.
	var deltas []string
	deadline := time.After(500 * time.Millisecond)
	for done := false; !done; {
		select {
		case ev := <-events:
			if ev.Kind == EventTextDelta {
				deltas = append(deltas, string(ev.Data))
			}
		case <-deadline:
			done = true
		}
	}
	require.Len(t, deltas, 1)
	assert.Contains(t, deltas[0], "Hello")

	assert.False(t, streamingMuted(context.Background()))
	assert.False(t, streamingMuted(nil))
	assert.True(t, streamingMuted(mutedContext(context.Background())))
}
