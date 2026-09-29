package anthropicclient

import (
	"encoding/json"
	"strings"
	"testing"
)

// Anthropic rejects `temperature` outright on models that have deprecated it,
// so a caller who never set one must not have a zero value sent on their
// behalf.
func TestMessagePayloadOmitsUnsetTemperature(t *testing.T) {
	body, err := json.Marshal(&messagePayload{
		Model:    "claude-sonnet-5",
		Messages: []ChatMessage{{Role: "user", Content: "Say this is a test!"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "temperature") {
		t.Errorf("unset temperature must be omitted, got: %s", body)
	}
}

// An explicit temperature still has to reach the wire, including 0, which is
// a meaningful sampling value and not a stand-in for "unset".
func TestMessagePayloadKeepsExplicitTemperature(t *testing.T) {
	for _, temperature := range []float64{0, 0.7} {
		value := temperature
		body, err := json.Marshal(&messagePayload{
			Model:       "claude-sonnet-4-5-20250929",
			Messages:    []ChatMessage{{Role: "user", Content: "hi"}},
			Temperature: &value,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got, ok := decoded["temperature"]
		if !ok {
			t.Errorf("explicit temperature %v was dropped: %s", temperature, body)
			continue
		}
		if got != temperature {
			t.Errorf("temperature = %v, want %v", got, temperature)
		}
	}
}

func TestCompletionPayloadOmitsUnsetTemperature(t *testing.T) {
	body, err := json.Marshal(&completionPayload{Model: "claude-2", Prompt: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "temperature") {
		t.Errorf("unset temperature must be omitted, got: %s", body)
	}
}
