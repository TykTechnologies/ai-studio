package guardrails

import (
	"fmt"
	"strings"
)

// Guardrail exceptions (features/GuardrailExceptions.md) soften the action a
// filter takes on a verdict for one app, filter, detector and channel. They
// change the consequence of a verdict, never the detection, and how far they
// may soften is bounded by the app's attested Capability: the floor table
// below. The hub enforces it when an exception is saved and the runtime
// (hub and edge) again when it applies one, since the app's capability can
// change after the exception was approved.

// Channel is where a flagged segment sits, as an exception scopes it.
type Channel string

const (
	// ChannelTextUser is the client's own user (and assistant) text.
	ChannelTextUser Channel = "text_user"
	// ChannelTextSystem is the system prompt.
	ChannelTextSystem Channel = "text_system"
	// ChannelToolResult is the output of a tool the client ran.
	ChannelToolResult Channel = "tool_result"
	// ChannelToolCall is a tool call's arguments.
	ChannelToolCall Channel = "tool_call"
	// ChannelToolDefinition is a tool the request offers the model.
	ChannelToolDefinition Channel = "tool_definition"
)

// Channels lists every channel, in the order the UI shows them.
var Channels = []Channel{ChannelTextUser, ChannelTextSystem, ChannelToolResult, ChannelToolCall, ChannelToolDefinition}

// ParseChannel checks a stored or submitted channel.
func ParseChannel(s string) (Channel, error) {
	for _, c := range Channels {
		if string(c) == s {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown channel %q (want one of %s)", s, joinChannels())
}

func joinChannels() string {
	out := make([]string, len(Channels))
	for i, c := range Channels {
		out[i] = string(c)
	}
	return strings.Join(out, ", ")
}

// IsTool reports a tool channel: only these scope an exception by tool name.
func (c Channel) IsTool() bool {
	return c == ChannelToolResult || c == ChannelToolCall || c == ChannelToolDefinition
}

// ActionRank orders actions by strength: log < redact < quarantine = remove
// < block. An unknown action ranks 0.
func ActionRank(a string) int {
	switch a {
	case ActionLog, ActionAlert:
		return 1
	case ActionRedact:
		return 2
	case ActionQuarantine, ActionRemove:
		return 3
	case ActionBlock:
		return 4
	}
	return 0
}

// Softer is the weaker of two actions (a on a tie).
func Softer(a, b string) string {
	if ActionRank(b) < ActionRank(a) {
		return b
	}
	return a
}

// exceptionActions are the actions an exception may soften to, per channel:
// what the runtime can do on that channel. Text and tool results can be
// logged or have the flagged segment replaced (quarantine); a tool call can
// only be logged; a tool definition logged or removed from the request.
// Alert (log and raise an event) ranks as log wherever log is allowed.
var exceptionActions = map[Channel][]string{
	ChannelTextUser:       {ActionLog, ActionAlert, ActionQuarantine},
	ChannelTextSystem:     {ActionLog, ActionAlert, ActionQuarantine},
	ChannelToolResult:     {ActionLog, ActionAlert, ActionQuarantine},
	ChannelToolCall:       {ActionLog, ActionAlert},
	ChannelToolDefinition: {ActionLog, ActionAlert, ActionRemove},
}

// ExceptionActions lists the actions an exception on the channel may soften
// to, softest first.
func ExceptionActions(ch Channel) []string {
	return exceptionActions[ch]
}

// floors is the fixed floor table: the softest action an exception may
// reach, by effective capability and channel. "" means no downgrade at all.
// It is not configurable.
var floors = map[Capability]map[Channel]string{
	CapabilityReadOnly: {
		ChannelTextUser: ActionLog, ChannelTextSystem: ActionLog,
		ChannelToolResult: ActionLog,
		ChannelToolCall:   ActionLog, ChannelToolDefinition: ActionLog,
	},
	CapabilityReadTools: {
		ChannelTextUser: ActionLog, ChannelTextSystem: ActionLog,
		ChannelToolResult: ActionQuarantine,
		ChannelToolCall:   ActionLog, ChannelToolDefinition: ActionLog,
	},
	CapabilityActing: {
		ChannelTextUser: ActionQuarantine, ChannelTextSystem: ActionQuarantine,
		ChannelToolResult: "",
		ChannelToolCall:   "", ChannelToolDefinition: "",
	},
}

// Floor is the softest action an exception may reach for an app of the
// capability on the channel; "" means the channel cannot be softened. An
// unattested capability is acting.
func Floor(c Capability, ch Channel) string {
	return floors[c.Effective()][ch]
}

// CheckException reports why an exception softening to action on the
// channel is not allowed for an app of the capability, nil when it is.
func CheckException(c Capability, ch Channel, action string) error {
	if _, err := ParseChannel(string(ch)); err != nil {
		return err
	}
	valid := false
	for _, a := range exceptionActions[ch] {
		if a == action {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("an exception on %s can soften to %s, not %q", ch, strings.Join(exceptionActions[ch], " or "), action)
	}
	eff := c.Effective()
	floor := Floor(c, ch)
	if floor == "" {
		return fmt.Errorf("an app whose capability is %s cannot soften verdicts on %s; attest a lower capability if its tools only read", describeCapability(c, eff), ch)
	}
	if ActionRank(action) < ActionRank(floor) {
		return fmt.Errorf("an app whose capability is %s can soften verdicts on %s down to %s at most, not %s", describeCapability(c, eff), ch, floor, action)
	}
	return nil
}

func describeCapability(c, eff Capability) string {
	if c == CapabilityUnattested {
		return "not attested (treated as acting)"
	}
	return string(eff)
}

// Allowed reports whether an exception softening to action on the channel
// is within the floor for an app of the capability.
func Allowed(c Capability, ch Channel, action string) bool {
	return CheckException(c, ch, action) == nil
}

// SoftenedAction is what an exception turns the configured action into: the
// softer of the two, when the exception is allowed for the app's capability
// now. An exception outside the floor (the capability changed since it was
// approved) changes nothing; ok is then false so the caller can report it.
func SoftenedAction(configured, target string, c Capability, ch Channel) (action string, ok bool) {
	if !Allowed(c, ch, target) {
		return configured, false
	}
	return Softer(configured, target), true
}

// Exception is an approved guardrail exception as the runtime applies it
// (scripting.ExceptionSource).
type Exception struct {
	ID       uint
	FilterID uint
	// Detector is the finding detector it covers; "" is any.
	Detector string
	Channel  Channel
	// ToolName narrows a tool channel to one tool; "" is any.
	ToolName string
	// Action is what it softens to.
	Action string
	// AlertEvent names the event raised when Action is alert.
	AlertEvent string
}

// AppExceptions are an app's approved exceptions and its guardrail
// capability, which bounds them.
type AppExceptions struct {
	Capability Capability
	Exceptions []Exception
}
