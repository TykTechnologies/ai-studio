package guardrails

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExceptionFloors checks every capability x channel x action against
// the epic's floor table (features/GuardrailExceptions.md).
func TestExceptionFloors(t *testing.T) {
	actions := []string{ActionLog, ActionAlert, ActionRedact, ActionQuarantine, ActionRemove, ActionBlock, ""}
	// allowed[capability][channel] = the actions an exception may soften to.
	allowed := map[Capability]map[Channel][]string{
		CapabilityReadOnly: {
			ChannelTextUser:       {ActionLog, ActionAlert, ActionQuarantine},
			ChannelTextSystem:     {ActionLog, ActionAlert, ActionQuarantine},
			ChannelToolResult:     {ActionLog, ActionAlert, ActionQuarantine},
			ChannelToolCall:       {ActionLog, ActionAlert},
			ChannelToolDefinition: {ActionLog, ActionAlert, ActionRemove},
		},
		CapabilityReadTools: {
			ChannelTextUser:       {ActionLog, ActionAlert, ActionQuarantine},
			ChannelTextSystem:     {ActionLog, ActionAlert, ActionQuarantine},
			ChannelToolResult:     {ActionQuarantine},
			ChannelToolCall:       {ActionLog, ActionAlert},
			ChannelToolDefinition: {ActionLog, ActionAlert, ActionRemove},
		},
		CapabilityActing: {
			ChannelTextUser:   {ActionQuarantine},
			ChannelTextSystem: {ActionQuarantine},
		},
	}
	allowed[CapabilityUnattested] = allowed[CapabilityActing]

	for _, c := range []Capability{CapabilityUnattested, CapabilityReadOnly, CapabilityReadTools, CapabilityActing} {
		for _, ch := range Channels {
			want := map[string]bool{}
			for _, a := range allowed[c][ch] {
				want[a] = true
			}
			for _, a := range actions {
				t.Run(fmt.Sprintf("%q/%s/%q", c, ch, a), func(t *testing.T) {
					err := CheckException(c, ch, a)
					assert.Equal(t, want[a], err == nil, "error: %v", err)
					assert.Equal(t, want[a], Allowed(c, ch, a))
				})
			}
		}
	}
}

func TestFloor(t *testing.T) {
	assert.Equal(t, ActionLog, Floor(CapabilityReadOnly, ChannelToolResult))
	assert.Equal(t, ActionQuarantine, Floor(CapabilityReadTools, ChannelToolResult))
	assert.Equal(t, ActionQuarantine, Floor(CapabilityActing, ChannelTextUser))
	assert.Equal(t, "", Floor(CapabilityActing, ChannelToolCall), "no downgrade")
	assert.Equal(t, Floor(CapabilityActing, ChannelTextSystem), Floor(CapabilityUnattested, ChannelTextSystem), "unattested is acting")
}

func TestCheckException_ClearErrors(t *testing.T) {
	err := CheckException(CapabilityUnattested, ChannelToolResult, ActionLog)
	assert.ErrorContains(t, err, "not attested (treated as acting)")
	assert.ErrorContains(t, err, "cannot soften")

	err = CheckException(CapabilityActing, ChannelTextUser, ActionLog)
	assert.ErrorContains(t, err, "down to quarantine at most")

	err = CheckException(CapabilityReadOnly, ChannelToolCall, ActionQuarantine)
	assert.ErrorContains(t, err, "can soften to log")

	assert.ErrorContains(t, CheckException(CapabilityReadOnly, "assistant", ActionLog), "unknown channel")
}

func TestSoftenedAction(t *testing.T) {
	a, ok := SoftenedAction(ActionBlock, ActionLog, CapabilityReadOnly, ChannelTextUser)
	assert.True(t, ok)
	assert.Equal(t, ActionLog, a)

	// Never stronger than the configured action.
	a, ok = SoftenedAction(ActionLog, ActionQuarantine, CapabilityReadOnly, ChannelTextUser)
	assert.True(t, ok)
	assert.Equal(t, ActionLog, a)

	// Outside the floor now (the capability changed): no change.
	a, ok = SoftenedAction(ActionQuarantine, ActionLog, CapabilityReadTools, ChannelToolResult)
	assert.False(t, ok)
	assert.Equal(t, ActionQuarantine, a)
}

func TestSofterAndRank(t *testing.T) {
	assert.Equal(t, ActionLog, Softer(ActionBlock, ActionLog))
	assert.Equal(t, ActionLog, Softer(ActionLog, ActionBlock))
	assert.Equal(t, ActionQuarantine, Softer(ActionQuarantine, ActionRemove), "a tie keeps the first")
	assert.Less(t, ActionRank(ActionLog), ActionRank(ActionRedact))
	assert.Less(t, ActionRank(ActionRedact), ActionRank(ActionQuarantine))
	assert.Equal(t, ActionRank(ActionQuarantine), ActionRank(ActionRemove))
	assert.Less(t, ActionRank(ActionRemove), ActionRank(ActionBlock))
}

func TestParseChannel(t *testing.T) {
	for _, ch := range Channels {
		got, err := ParseChannel(string(ch))
		assert.NoError(t, err)
		assert.Equal(t, ch, got)
	}
	_, err := ParseChannel("")
	assert.Error(t, err)
	assert.True(t, ChannelToolResult.IsTool())
	assert.False(t, ChannelTextUser.IsTool())
}
