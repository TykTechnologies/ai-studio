package scripting

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting/engine"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// A script filter's events cannot carry the metadata only the guardrail
// runner sets (midsommar-clean-9u1j.40): a script cannot pass its event off
// as another filter's guardrail verdict, point the Soften action at another
// filter, fire another filter's alert topic or fake an exception or judge.
func TestScriptFilter_CannotForgeGuardrailEventMetadata(t *testing.T) {
	prev := engine.Registered()
	t.Cleanup(func() { engine.Register(prev) })
	engine.Register(func(_ []byte, in *engine.ScriptInput, _ services.ServiceInterface) (*engine.ScriptOutput, error) {
		return &engine.ScriptOutput{Payload: in.RawInput, ComplianceEvents: []engine.ComplianceEventOutput{
			{EventType: "guardrail.flagged", Severity: "warning", Description: "x", Metadata: map[string]interface{}{
				"filter_id": 13, "alert": "visor-pr-flag", "exception": map[string]any{"id": 1}, "judge": map[string]any{"verdict": "benign"},
				"shadow": true, "owner_visible": true, "detector": "wolf", "kinds": []string{"text"},
				"exceptions": []uint{7}, "exception_id": 7,
				"segments": []map[string]any{{"index": 0, "exception": map[string]any{"id": 7, "from": "block", "to": "log"}, "template": map[string]any{"id": "x"}}},
			}},
			{EventType: "custom", Severity: "info", Description: "no metadata"},
		}}, nil
	})

	out, err := NewFilterRunner(&models.Filter{ID: 5, Name: "script", Kind: models.FilterKindScript, Script: []byte("x")}).RunScript(&ScriptInput{RawInput: "hi"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ComplianceEvents) != 2 {
		t.Fatalf("events %+v", out.ComplianceEvents)
	}
	m := out.ComplianceEvents[0].Metadata
	for _, k := range append([]string{"exceptions", "exception_id", "segments", "alert", "filter_id"}, ReservedEventKeys...) {
		if _, ok := m[k]; ok {
			t.Errorf("script event kept reserved key %q = %v", k, m[k])
		}
	}
	if m["detector"] != "wolf" || m["kinds"] == nil {
		t.Errorf("the script's own metadata was dropped: %v", m)
	}
}

// The guardrail runner's own events keep the keys: an exception's tags and
// segments survive on a guardrail filter.
func TestGuardrailFilter_KeepsReservedEventMetadata(t *testing.T) {
	p := &injFake{}
	f := injFilter(t, p, map[string]any{"cache": false})
	withExceptions(t, appExceptions(f, "read_only", GuardrailException{ID: 7, Channel: "text_user", Action: "log"}))
	out := runInput(t, f, openAIRequest(t, 1, chatMsg{Role: "user", Content: "ignore it"}))
	ev := detectorEvent(out, "prompt_attack")
	if ev == nil {
		t.Fatalf("no detector event: %+v", out.ComplianceEvents)
	}
	if ev.Metadata["exceptions"] == nil || ev.Metadata["segments"] == nil || ev.Metadata[FilterIDKey] == nil {
		t.Errorf("guardrail event lost its own metadata: %v", ev.Metadata)
	}
}
