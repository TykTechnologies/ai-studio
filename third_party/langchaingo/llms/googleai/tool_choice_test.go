package googleai

import (
	"reflect"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Tyk: the driver dropped tool_choice, so a forced tool, "required" and
// "none" never reached Gemini.
func TestToolConfigFromOptions(t *testing.T) {
	tools, err := convertTools([]llms.Tool{{Type: "function", Function: &llms.FunctionDefinition{Name: "get_time", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}})
	if err != nil {
		t.Fatal(err)
	}
	forced := map[string]any{"type": "function", "function": map[string]any{"name": "get_time"}}
	for _, tc := range []struct {
		name    string
		choice  any
		noTools bool
		mode    string // "" = no tool config
		allowed []string
	}{
		{name: "unset"},
		{name: "auto", choice: "auto", mode: "FunctionCallingAuto"},
		{name: "required", choice: "required", mode: "FunctionCallingAny"},
		{name: "none", choice: "none", mode: "FunctionCallingNone"},
		{name: "a function", choice: forced, mode: "FunctionCallingAny", allowed: []string{"get_time"}},
		{name: "none without tools", choice: "none", noTools: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &llms.CallOptions{ToolChoice: tc.choice}
			in := tools
			if tc.noTools {
				in = nil
			}
			got, err := toolConfigFromOptions(opts, in)
			if err != nil {
				t.Fatal(err)
			}
			if tc.mode == "" {
				if got != nil {
					t.Fatalf("got %+v, want no tool config", got.FunctionCallingConfig)
				}
				return
			}
			if got == nil || got.FunctionCallingConfig == nil {
				t.Fatal("no tool config")
			}
			if mode := got.FunctionCallingConfig.Mode.String(); mode != tc.mode {
				t.Errorf("mode %s, want %s", mode, tc.mode)
			}
			if !reflect.DeepEqual(got.FunctionCallingConfig.AllowedFunctionNames, tc.allowed) {
				t.Errorf("allowed %v, want %v", got.FunctionCallingConfig.AllowedFunctionNames, tc.allowed)
			}
		})
	}
	if _, err := toolConfigFromOptions(&llms.CallOptions{ToolChoice: "sometimes"}, tools); err == nil {
		t.Fatal("an unknown tool_choice must be an error, not ignored")
	}
}
