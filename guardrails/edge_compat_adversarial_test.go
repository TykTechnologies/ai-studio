package guardrails

// Adversarial test for 4uc.5 (tests agent): a v2.2.2 edge receiving a
// snapshot that uses every key added in v2.3 at once (tool_results, budget,
// cache, quarantine, top-level quarantine) still loads it, and never applies
// an action weaker than block where the new one is quarantine or block.

import (
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/internal/v222"
)

func TestAdversarial_V222EdgeLoadsEveryNewKeyAtOnce(t *testing.T) {
	actions := []string{ActionBlock, ActionQuarantine, ActionRedact, ActionLog}
	toolActions := []any{nil, ActionBlock, ActionQuarantine, ActionRedact, ActionLog}
	scans := []any{nil, true, false}
	for _, top := range actions {
		for _, toolAction := range toolActions {
			for _, scan := range scans {
				name := fmt.Sprintf("%s/tool=%v/scan=%v", top, toolAction, scan)
				t.Run(name, func(t *testing.T) {
					tr := map[string]any{}
					if scan != nil {
						tr["scan"] = scan
					}
					if toolAction != nil {
						tr["on_detect"] = toolAction
					}
					raw := map[string]any{
						"provider":     ProviderBuiltin,
						"detectors":    []any{map[string]any{"name": CategoryInjection}, map[string]any{"name": CategoryPII}},
						"on_detect":    top,
						"scope":        ScopeLastUser,
						"tool_results": tr,
						"budget":       map[string]any{"max_scan_chars": 100000, "max_pieces": 64, "time_ms": 9000, "single_call_chars": 20000},
						"cache":        false,
						"quarantine":   map[string]any{"placeholder": "[removed: {{detector}} in {{source}}]"},
						"connection":   map[string]any{},
					}
					// The hub validates before it stores; skip combinations it
					// refuses itself.
					cfg, err := ParseConfig(raw)
					if err == nil {
						_, err = Normalize(cfg, false)
					}
					if err != nil {
						t.Skipf("the hub refuses this config: %v", err)
					}
					js, err := ConfigJSONForEdge(raw, nil)
					if err != nil {
						t.Fatal(err)
					}
					oldRaw, err := v222.ConfigFromJSON(js)
					if err != nil {
						t.Fatal(err)
					}
					old, err := v222.ParseConfig(oldRaw)
					if err == nil {
						old, err = v222.Normalize(old, false)
					}
					if err != nil {
						t.Fatalf("a v2.2.2 edge rejects the snapshot %s: %v", js, err)
					}
					switch top {
					case ActionBlock, ActionQuarantine:
						if old.OnDetect != v222.ActionBlock {
							t.Errorf("a v2.2.2 edge applies %q for a %s filter, want block", old.OnDetect, top)
						}
					default:
						if old.OnDetect != top {
							t.Errorf("a v2.2.2 edge applies %q, want %q", old.OnDetect, top)
						}
					}
				})
			}
		}
	}
}
