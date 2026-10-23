package guardrails

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails/internal/v222"
)

// During a rolling upgrade a new hub pushes configs to edges on the previous
// release. Those edges must load every config and apply an action at least
// as strong as the new one: an action they do not know reaches them as
// block, never as a rejected config (which fails open or closed) and never
// as log. v222 is the guardrails package as released in v2.2.2.

func quarantineRaw() map[string]any {
	return map[string]any{
		"provider":   ProviderBuiltin,
		"detectors":  []any{map[string]any{"name": CategoryInjection}},
		"on_detect":  ActionQuarantine,
		"quarantine": map[string]any{"placeholder": "[gone]"},
		"cache":      false,
		"connection": map[string]any{},
	}
}

func TestEdgeSnapshot_OldEdgeBlocksOnQuarantine(t *testing.T) {
	js, err := ConfigJSONForEdge(quarantineRaw(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := v222.ConfigFromJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	old, err := v222.ParseConfig(raw)
	if err != nil {
		t.Fatalf("a v2.2.2 edge cannot parse the snapshot: %v", err)
	}
	old, err = v222.Normalize(old, false)
	if err != nil {
		t.Fatalf("a v2.2.2 edge rejects the snapshot: %v", err)
	}
	if old.OnDetect != v222.ActionBlock {
		t.Errorf("a v2.2.2 edge applies %q, want block", old.OnDetect)
	}
}

func TestEdgeSnapshot_NewEdgeQuarantines(t *testing.T) {
	js, err := ConfigJSONForEdge(quarantineRaw(), nil, nil)
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
	if cfg.OnDetect != ActionQuarantine || cfg.OnDetectExt != "" || *cfg.Quarantine.Placeholder != "[gone]" {
		t.Errorf("new edge config = on_detect %q ext %q placeholder %q", cfg.OnDetect, cfg.OnDetectExt, *cfg.Quarantine.Placeholder)
	}
}

// An action newer than the edge (a hub ahead of this build) leaves the
// on_detect the hub sent, block, in force.
func TestEdgeSnapshot_UnknownExtendedActionKeepsBlock(t *testing.T) {
	raw := quarantineRaw()
	raw["on_detect"] = ActionBlock
	raw["on_detect_ext"] = "some_future_action"
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OnDetect != ActionBlock {
		t.Errorf("on_detect = %q, want block", cfg.OnDetect)
	}
}

// The encoding exists only on the way to the edge: the stored config, and
// what the API and the UI read back, keep the real action.
func TestEdgeSnapshot_StoredConfigKeepsQuarantine(t *testing.T) {
	cfg, err := ParseConfig(quarantineRaw())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Normalize(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := cfg.ToMap()
	if err != nil {
		t.Fatal(err)
	}
	if m["on_detect"] != ActionQuarantine {
		t.Errorf("stored on_detect = %v", m["on_detect"])
	}
	if _, ok := m["on_detect_ext"]; ok {
		t.Error("on_detect_ext is stored")
	}
}

// Actions every edge knows go out as they are.
func TestEdgeSnapshot_LegacyActionsUnchanged(t *testing.T) {
	for _, action := range []string{ActionBlock, ActionRedact, ActionLog} {
		raw := quarantineRaw()
		raw["on_detect"] = action
		js, err := ConfigJSONForEdge(raw, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		m, _ := ConfigFromJSON(js)
		if m["on_detect"] != action || m["on_detect_ext"] != nil {
			t.Errorf("%s: snapshot on_detect %v ext %v", action, m["on_detect"], m["on_detect_ext"])
		}
		old, err := v222.ParseConfig(m)
		if err != nil {
			t.Fatal(err)
		}
		if old, err = v222.Normalize(old, false); err != nil || old.OnDetect != action {
			t.Errorf("%s: v2.2.2 edge loads %q, %v", action, old.OnDetect, err)
		}
	}
}
