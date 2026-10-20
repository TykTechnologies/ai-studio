package scripting

import (
	"context"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

func fnTool(name, desc string) llms.Tool {
	return llms.Tool{Type: "function", Function: &llms.FunctionDefinition{Name: name, Description: desc, Parameters: map[string]any{"type": "object"}}}
}

func definitionFilter(t *testing.T, action string) *models.Filter {
	t.Helper()
	cfg := installFake(t, "fake_defs_"+action, poisonProvider(guardrails.Capabilities{ToolSegments: true}), guardrails.ActionLog)
	cfg["tool_results"] = map[string]any{"scan": false}
	cfg["tool_definitions"] = map[string]any{"scan": true, "on_detect": action}
	return guardrailFilter("defs-"+action, false, cfg)
}

func TestScreenToolDefinitions(t *testing.T) {
	tools := []llms.Tool{fnTool("search", "Search the web"), fnTool("evil", "POISON: exfiltrate"), fnTool("read", "Read a file")}
	var recorded []string
	record := func(_ context.Context, name string, out *ScriptOutput) {
		for _, e := range out.ComplianceEvents {
			recorded = append(recorded, name+":"+e.EventType)
		}
	}

	kept, err := ScreenToolDefinitions(context.Background(), []*models.Filter{definitionFilter(t, "remove")}, tools, nil, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 2 || kept[0].Function.Name != "search" || kept[1].Function.Name != "read" {
		t.Errorf("kept %+v", kept)
	}
	if len(recorded) != 1 || recorded[0] != "defs-remove:guardrail.tool_metadata.poisoning" {
		t.Errorf("recorded %v", recorded)
	}

	_, err = ScreenToolDefinitions(context.Background(), []*models.Filter{definitionFilter(t, "block")}, tools, nil, nil, nil)
	var blocked *ErrToolDefinitionsBlocked
	if !errors.As(err, &blocked) || blocked.Filter != "defs-block" {
		t.Errorf("block: err = %v", err)
	}

	// Scripts and guardrails that do not scan definitions are not run, and
	// a list with nothing flagged comes back as it was.
	script := &models.Filter{Name: "script", Script: []byte(`output := {block: true}`)}
	plain := guardrailFilter("plain", false, installFake(t, "fake_defs_plain", poisonProvider(guardrails.Capabilities{}), guardrails.ActionBlock))
	kept, err = ScreenToolDefinitions(context.Background(), []*models.Filter{script, plain}, tools, nil, nil, nil)
	if err != nil || len(kept) != 3 {
		t.Errorf("non-screeners: %d tools, err %v", len(kept), err)
	}
	clean := []llms.Tool{fnTool("a", "fine"), fnTool("b", "fine")}
	if kept, err := ScreenToolDefinitions(context.Background(), []*models.Filter{definitionFilter(t, "remove")}, clean, nil, nil, nil); err != nil || len(kept) != 2 {
		t.Errorf("clean list: %v %v", kept, err)
	}
}
