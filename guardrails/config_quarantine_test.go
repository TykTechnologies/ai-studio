package guardrails

import (
	"errors"
	"strings"
	"testing"
)

func quarantineConfig(placeholder *string) Config {
	return Config{
		Provider:   ProviderBuiltin,
		Detectors:  []DetectorConfig{{Name: CategoryInjection}},
		OnDetect:   ActionQuarantine,
		Quarantine: Quarantine{Placeholder: placeholder},
	}
}

func TestNormalizeQuarantine(t *testing.T) {
	cfg, err := Normalize(quarantineConfig(nil), false)
	if err != nil {
		t.Fatalf("quarantine on a request filter: %v", err)
	}
	if cfg.Quarantine.Placeholder == nil || *cfg.Quarantine.Placeholder != DefaultQuarantinePlaceholder {
		t.Errorf("placeholder = %v, want the default", cfg.Quarantine.Placeholder)
	}

	custom := "[removed: {{source}}]"
	if cfg, err := Normalize(quarantineConfig(&custom), false); err != nil || *cfg.Quarantine.Placeholder != custom {
		t.Errorf("custom placeholder: %v, %v", cfg.Quarantine.Placeholder, err)
	}

	empty, blank, long := "", "   ", strings.Repeat("x", MaxQuarantinePlaceholder+1)
	for name, p := range map[string]*string{"empty": &empty, "blank": &blank, "too long": &long} {
		if _, err := Normalize(quarantineConfig(p), false); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s placeholder: err %v, want ErrInvalidConfig", name, err)
		}
	}

	if _, err := Normalize(quarantineConfig(nil), true); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("quarantine on a response filter: err %v, want ErrInvalidConfig", err)
	}
}

// The placeholder survives the trip to the edge, and a stored config
// without one gets the default there too.
func TestQuarantineConfigRoundTripsToTheEdge(t *testing.T) {
	custom := "[gone]"
	for name, raw := range map[string]map[string]any{
		"custom": {"provider": ProviderBuiltin, "detectors": []any{map[string]any{"name": CategoryInjection}}, "on_detect": ActionQuarantine, "quarantine": map[string]any{"placeholder": custom}},
		"absent": {"provider": ProviderBuiltin, "detectors": []any{map[string]any{"name": CategoryInjection}}, "on_detect": ActionQuarantine},
	} {
		js, err := ConfigJSONForEdge(raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		m, err := ConfigFromJSON(js)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := ParseConfig(m)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err = Normalize(cfg, false)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := DefaultQuarantinePlaceholder
		if name == "custom" {
			want = custom
		}
		if *cfg.Quarantine.Placeholder != want {
			t.Errorf("%s: placeholder %q, want %q", name, *cfg.Quarantine.Placeholder, want)
		}
	}
}
