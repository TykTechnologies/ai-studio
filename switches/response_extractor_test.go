package switches

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A buffered OpenAI Responses API answer has output items, not choices. Its
// text (output_text, and a refusal) must reach response filters, in order;
// tool calls and reasoning are not text.
func TestExtractResponseText_OpenAIResponses(t *testing.T) {
	body := `{"id":"resp_1","object":"response","status":"completed","output":[
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}]},
		{"type":"message","id":"msg_1","role":"assistant","content":[
			{"type":"output_text","text":"First part. ","annotations":[]},
			{"type":"output_text","text":"Second part.","annotations":[]}]},
		{"type":"function_call","id":"fc_1","call_id":"c1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},
		{"type":"message","id":"msg_2","role":"assistant","content":[{"type":"refusal","refusal":"I can't help with that."}]}]}`
	for _, vendor := range []models.Vendor{models.OPENAI, models.OLLAMA} {
		got, err := ExtractResponseText(vendor, []byte(body))
		if err != nil {
			t.Fatalf("%s: %v", vendor, err)
		}
		if want := "First part. Second part.\nI can't help with that."; got != want {
			t.Errorf("%s: got %q, want %q", vendor, got, want)
		}
	}

	// Only a tool call: no text, and no error (the tool call is screened on
	// its own).
	got, err := ExtractResponseText(models.OPENAI, []byte(`{"object":"response","output":[{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"}]}`))
	if err != nil || got != "" {
		t.Errorf("tool call only: %q %v", got, err)
	}

	// Chat completions are unchanged, and a body with neither shape is still
	// an error.
	got, err = ExtractResponseText(models.OPENAI, []byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	if err != nil || got != "hello" {
		t.Errorf("chat completion: %q %v", got, err)
	}
	if _, err := ExtractResponseText(models.OPENAI, []byte(`{"id":"x"}`)); err == nil {
		t.Error("a body with no choices and no output was accepted")
	}
}
