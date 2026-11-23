package openaiclient

import (
	"encoding/json"
	"testing"
)

// Tyk: ExtraBody adds top-level fields to the request body and never
// overrides one the request already carries.
func TestChatRequestExtraBody(t *testing.T) {
	req := ChatRequest{
		Model: "gpt-4o", Temperature: 0.2, TopP: 0.9, StopWords: []string{"x"},
		ExtraBody: map[string]any{
			"service_tier": "flex", "store": true, "top_logprobs": 2,
			"model": "attacker-model", "stop": []string{"overridden"}, "top_p": 0.1,
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{
		"service_tier": `"flex"`, "store": `true`, "top_logprobs": `2`,
		"model": `"gpt-4o"`, "stop": `["x"]`, "top_p": `0.9`, "temperature": `0.2`,
	} {
		if string(got[field]) != want {
			t.Errorf("%s = %s, want %s", field, got[field], want)
		}
	}

	// Without extra fields the body is what it was.
	plain, err := json.Marshal(ChatRequest{Model: "gpt-4o", Temperature: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != `{"model":"gpt-4o","messages":null,"temperature":0.2}` {
		t.Errorf("plain body changed: %s", plain)
	}
}
