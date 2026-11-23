package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/anthropic/internal/anthropicclient"
)

// Tyk: the driver dropped tool_choice, so a forced tool, "required" and
// "none" never reached Anthropic.
func TestToolChoiceFromOptions(t *testing.T) {
	forced := map[string]any{"type": "function", "function": map[string]any{"name": "get_time"}}
	for _, tc := range []struct {
		name   string
		choice any
		tools  int
		want   string // JSON of the tool_choice sent, "" for none
	}{
		{"unset", nil, 2, ""},
		{"auto", "auto", 2, `{"type":"auto"}`},
		{"required", "required", 2, `{"type":"any"}`},
		{"none", "none", 2, `{"type":"none"}`},
		{"a function", forced, 2, `{"type":"tool","name":"get_time"}`},
		{"none without tools sends nothing", "none", 0, ""},
		{"auto without tools sends nothing", "auto", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toolChoiceFromOptions(callOptions(llms.WithToolChoice(tc.choice)), tc.tools)
			if err != nil {
				t.Fatal(err)
			}
			// As it goes on the wire, in a request with nothing else set.
			body, err := json.Marshal(&anthropicclient.MessageRequest{Model: "m", ToolChoice: got})
			if err != nil {
				t.Fatal(err)
			}
			var sent map[string]json.RawMessage
			if err := json.Unmarshal(body, &sent); err != nil {
				t.Fatal(err)
			}
			if string(sent["tool_choice"]) != tc.want {
				t.Fatalf("tool_choice sent: %s, want %s (body %s)", sent["tool_choice"], tc.want, body)
			}
		})
	}

	if _, err := toolChoiceFromOptions(callOptions(llms.WithToolChoice("sometimes")), 1); err == nil {
		t.Fatal("an unknown tool_choice must be an error, not ignored")
	}
}
