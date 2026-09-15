package chat_session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session reloaded by a v1 mutation endpoint starts raw; the v2 client
// takes it over when idle, and the fan-out starts exactly once.
func TestSwitchOutputMode(t *testing.T) {
	cs := newEventsSession(t)
	defer cs.Stop()
	cs.SetOutputMode(OutputModeRaw)

	assert.True(t, cs.SwitchOutputMode(OutputModeRaw), "same mode is a no-op")
	assert.Equal(t, OutputModeRaw, cs.OutputMode())

	// Busy sessions are never switched.
	require.True(t, cs.TryLockRun())
	assert.False(t, cs.SwitchOutputMode(OutputModeEvents))
	assert.Equal(t, OutputModeRaw, cs.OutputMode())
	cs.UnlockRun()

	assert.True(t, cs.SwitchOutputMode(OutputModeEvents))
	assert.Equal(t, OutputModeEvents, cs.OutputMode())
	assert.True(t, cs.fanoutStarted)
	assert.True(t, cs.SwitchOutputMode(OutputModeEvents), "idempotent")

	// The fan-out owns the stream once started: no way back to raw.
	assert.False(t, cs.SwitchOutputMode(OutputModeRaw))
	assert.Equal(t, OutputModeEvents, cs.OutputMode())

	// Events keep flowing to a subscriber after the switch.
	events, unsubscribe := cs.Subscribe("run-switch", 8)
	defer unsubscribe()
	cs.beginRun("run-switch")
	cs.emit(EventTextDelta, TextDeltaData{Delta: "hi"})
	assert.Equal(t, EventStart, (<-events).Kind)
	assert.Equal(t, EventTextDelta, (<-events).Kind)
}
