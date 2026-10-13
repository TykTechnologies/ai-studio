package scripting

import (
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

var registerPanicking sync.Once

// PanickingGuardrail returns a guardrail filter whose provider factory
// panics with text that must never reach an event or an error.
func PanickingGuardrail(failMode string, response bool) *models.Filter {
	registerPanicking.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{
			Name: "test_panicking", DisplayName: "panics",
			Detectors: []guardrails.DetectorSpec{{Name: "x", Category: "other"}},
		})
		guardrails.RegisterFactory("test_panicking", func(guardrails.Config) (guardrails.Provider, error) {
			panic("boom: secret-tool-output")
		})
	})
	cfg := models.JSONMap{"provider": "test_panicking", "detectors": []any{map[string]any{"name": "x"}}, "on_detect": guardrails.ActionLog}
	if failMode != "" {
		cfg["fail_mode"] = failMode
	}
	return &models.Filter{Name: "panicking", Kind: models.FilterKindGuardrail, ResponseFilter: response, Config: cfg}
}

// A guardrail that panics never takes the process down, and a request it
// was screening is blocked whatever the fail mode: nothing about it is known
// to be clean. The panic text reaches no event or error.
func TestGuardrailRunner_PanicBlocksTheRequest(t *testing.T) {
	out, err := NewFilterRunner(PanickingGuardrail(guardrails.FailOpen, false)).RunScript(&ScriptInput{RawInput: toolOnlyBodies["openai"], Format: "openai"}, nil)
	if err != nil {
		t.Fatalf("a panic surfaced as an error: %v", err)
	}
	if !out.Block {
		t.Fatal("a request whose guardrail panicked was let through")
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["error_class"] != "panic" {
		t.Errorf("events %+v", out.ComplianceEvents)
	}
	for _, e := range out.ComplianceEvents {
		if strings.Contains(e.Description, "secret-tool-output") || strings.Contains(out.Message, "secret-tool-output") {
			t.Error("the panic value leaked")
		}
	}

	// On the response side the fail mode decides.
	out, _ = NewFilterRunner(PanickingGuardrail("", true)).RunScript(&ScriptInput{RawInput: "answer", IsResponse: true}, nil)
	if out.Block || out.Payload != "answer" {
		t.Errorf("response, fail open: block %v payload %q", out.Block, out.Payload)
	}
	out, _ = NewFilterRunner(PanickingGuardrail(guardrails.FailClosed, true)).RunScript(&ScriptInput{RawInput: "answer", IsResponse: true}, nil)
	if !out.Block {
		t.Error("response, fail closed: not blocked")
	}
}
