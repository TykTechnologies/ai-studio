package guardrails

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/internal/v222"
)

func toolResultsRaw(toolResults map[string]any) map[string]any {
	raw := map[string]any{
		"provider":  ProviderBuiltin,
		"detectors": []any{map[string]any{"name": CategoryInjection}},
		"on_detect": ActionBlock,
	}
	if toolResults != nil {
		raw["tool_results"] = toolResults
	}
	return raw
}

func TestNormalizeToolResults(t *testing.T) {
	for name, tr := range map[string]map[string]any{
		"absent":           nil,
		"scan false":       {"scan": false},
		"quarantine":       {"on_detect": ActionQuarantine},
		"redact (builtin)": {"on_detect": ActionRedact},
		"log":              {"on_detect": ActionLog},
	} {
		cfg, err := ParseConfig(toolResultsRaw(tr))
		if err == nil {
			_, err = Normalize(cfg, false)
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	bad := map[string]struct {
		tr       map[string]any
		provider string
		response bool
	}{
		"unknown action":                 {tr: map[string]any{"on_detect": "drop"}},
		"redact without a redacting one": {tr: map[string]any{"on_detect": ActionRedact}, provider: ProviderAzureContentSafety},
		"quarantine on a response":       {tr: map[string]any{"on_detect": ActionQuarantine}, response: true},
	}
	for name, c := range bad {
		raw := toolResultsRaw(c.tr)
		if c.provider != "" {
			raw["provider"] = c.provider
			raw["detectors"] = []any{map[string]any{"name": "prompt_attack"}}
			raw["connection"] = map[string]any{"api_key": "k", "endpoint": "https://e"}
		}
		cfg, err := ParseConfig(raw)
		if err == nil {
			_, err = Normalize(cfg, c.response)
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err %v, want ErrInvalidConfig", name, err)
		}
	}
}

func TestActionFor(t *testing.T) {
	cases := []struct {
		onDetect, toolOnDetect, kind, want string
	}{
		{ActionBlock, "", KindText, ActionBlock},
		{ActionBlock, "", KindToolResult, ActionQuarantine},
		{ActionBlock, ActionBlock, KindToolResult, ActionBlock},
		{ActionRedact, "", KindToolResult, ActionRedact},
		{ActionLog, "", KindToolResult, ActionLog},
		{ActionLog, ActionQuarantine, KindToolResult, ActionQuarantine},
		{ActionLog, ActionQuarantine, KindText, ActionLog},
	}
	for _, c := range cases {
		cfg := Config{OnDetect: c.onDetect, ToolResults: ToolResults{OnDetect: c.toolOnDetect}}
		if got := cfg.ActionFor(c.kind); got != c.want {
			t.Errorf("on_detect %s, tool %q, %s: %s, want %s", c.onDetect, c.toolOnDetect, c.kind, got, c.want)
		}
	}
}

// "Absent" survives the trip to the edge, so stored configs from before
// tool results were scanned scan them on edges too; an explicit false stays
// false. Edges on v2.2 ignore the key and keep scanning as they did.
func TestToolResultsScanSurvivesTheEdgeSnapshot(t *testing.T) {
	for name, c := range map[string]struct {
		tr   map[string]any
		want bool
	}{
		"absent": {nil, true},
		"true":   {map[string]any{"scan": true}, true},
		"false":  {map[string]any{"scan": false}, false},
	} {
		js, err := ConfigJSONForEdge(toolResultsRaw(c.tr), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := ConfigFromJSON(js)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := ParseConfig(raw)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err = Normalize(cfg, false)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ScansToolResults() != c.want {
			t.Errorf("%s: ScansToolResults %v after the snapshot, want %v", name, cfg.ScansToolResults(), c.want)
		}
		if _, set := raw["tool_results"].(map[string]any)["scan"]; set != (name != "absent") {
			t.Errorf("%s: snapshot %s", name, js)
		}

		old, err := v222.ParseConfig(raw)
		if err == nil {
			_, err = v222.Normalize(old, false)
		}
		if err != nil {
			t.Errorf("%s: a v2.2.2 edge rejects the snapshot: %v", name, err)
		}
	}
}
