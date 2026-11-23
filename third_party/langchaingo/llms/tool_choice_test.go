package llms

import (
	"errors"
	"testing"
)

// Tyk: tests for ResolveToolChoice (tool_choice.go, not in upstream).
func TestResolveToolChoice(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want ResolvedToolChoice
	}{
		{"nil", nil, ResolvedToolChoice{}},
		{"empty string", "", ResolvedToolChoice{}},
		{"auto", "auto", ResolvedToolChoice{Mode: ToolChoiceAuto}},
		{"none", "none", ResolvedToolChoice{Mode: ToolChoiceNone}},
		{"required", "required", ResolvedToolChoice{Mode: ToolChoiceRequired}},
		{"decoded JSON object", map[string]any{"type": "function", "function": map[string]any{"name": "get_time"}},
			ResolvedToolChoice{Mode: ToolChoiceFunction, Name: "get_time"}},
		{"ToolChoice", ToolChoice{Type: "function", Function: &FunctionReference{Name: "get_time"}},
			ResolvedToolChoice{Mode: ToolChoiceFunction, Name: "get_time"}},
		{"*ToolChoice", &ToolChoice{Type: "function", Function: &FunctionReference{Name: "get_time"}},
			ResolvedToolChoice{Mode: ToolChoiceFunction, Name: "get_time"}},
		{"nil *ToolChoice", (*ToolChoice)(nil), ResolvedToolChoice{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveToolChoice(tc.in)
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}

	for name, in := range map[string]any{
		"unknown word":            "any",
		"object without a name":   map[string]any{"type": "function", "function": map[string]any{}},
		"object of another type":  map[string]any{"type": "allowed_tools", "allowed_tools": map[string]any{}},
		"ToolChoice without name": ToolChoice{Type: "function"},
		"number":                  3,
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			got, err := ResolveToolChoice(in)
			if !errors.Is(err, ErrUnsupportedToolChoice) || got != (ResolvedToolChoice{}) {
				t.Fatalf("got %+v, %v; want ErrUnsupportedToolChoice", got, err)
			}
		})
	}
}
