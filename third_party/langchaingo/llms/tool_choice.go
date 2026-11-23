package llms

import (
	"errors"
	"fmt"
)

// Tyk: this file is not in upstream langchaingo. CallOptions.ToolChoice is
// an untyped value in OpenAI's shape ("auto", "none", "required", or
// {"type":"function","function":{"name":...}}), which only the OpenAI driver
// sent on. ResolveToolChoice gives the other drivers (Anthropic, Google AI,
// Vertex) and the Bedrock translator one reading of it, so a forced tool, a
// required tool call and "none" reach every vendor.

// ToolChoiceMode is what a caller asked for with tool_choice.
type ToolChoiceMode string

const (
	// ToolChoiceUnset: no tool_choice; the vendor's default applies.
	ToolChoiceUnset ToolChoiceMode = ""
	// ToolChoiceAuto: the model decides whether to call a tool.
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceNone: the model must not call a tool.
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceRequired: the model must call some tool.
	ToolChoiceRequired ToolChoiceMode = "required"
	// ToolChoiceFunction: the model must call the named function.
	ToolChoiceFunction ToolChoiceMode = "function"
)

// ResolvedToolChoice is a tool_choice in one shape.
type ResolvedToolChoice struct {
	Mode ToolChoiceMode
	// Name is the function to call when Mode is ToolChoiceFunction.
	Name string
}

// ErrUnsupportedToolChoice is wrapped by the error ResolveToolChoice returns
// for a value it does not understand.
var ErrUnsupportedToolChoice = errors.New("unsupported tool_choice")

// ResolveToolChoice reads a CallOptions.ToolChoice value: nil, one of the
// strings "auto", "none" and "required", a ToolChoice (or pointer), or the
// JSON-decoded object {"type":"function","function":{"name":"..."}}. Anything
// else is an error wrapping ErrUnsupportedToolChoice: it is never ignored.
func ResolveToolChoice(choice any) (ResolvedToolChoice, error) {
	switch c := choice.(type) {
	case nil:
		return ResolvedToolChoice{}, nil
	case string:
		switch ToolChoiceMode(c) {
		case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
			return ResolvedToolChoice{Mode: ToolChoiceMode(c)}, nil
		case ToolChoiceUnset:
			return ResolvedToolChoice{}, nil
		}
		return ResolvedToolChoice{}, fmt.Errorf("%w: %q (use \"auto\", \"none\", \"required\" or a function)", ErrUnsupportedToolChoice, c)
	case *ToolChoice:
		if c == nil {
			return ResolvedToolChoice{}, nil
		}
		return ResolveToolChoice(*c)
	case ToolChoice:
		if c.Type != "function" || c.Function == nil || c.Function.Name == "" {
			return ResolvedToolChoice{}, fmt.Errorf("%w: only {\"type\":\"function\",\"function\":{\"name\":...}} names a tool", ErrUnsupportedToolChoice)
		}
		return ResolvedToolChoice{Mode: ToolChoiceFunction, Name: c.Function.Name}, nil
	case map[string]any:
		typ, _ := c["type"].(string)
		fn, _ := c["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if typ != "function" || name == "" {
			return ResolvedToolChoice{}, fmt.Errorf("%w: only {\"type\":\"function\",\"function\":{\"name\":...}} names a tool", ErrUnsupportedToolChoice)
		}
		return ResolvedToolChoice{Mode: ToolChoiceFunction, Name: name}, nil
	}
	return ResolvedToolChoice{}, fmt.Errorf("%w: unexpected value of type %T", ErrUnsupportedToolChoice, choice)
}
