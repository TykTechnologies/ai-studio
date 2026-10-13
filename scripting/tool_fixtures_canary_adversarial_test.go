//go:build enterprise

package scripting

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// builtinProvider builds the shipped built-in provider for one category.
func builtinProvider(t *testing.T, category string) (guardrails.Config, guardrails.Provider) {
	t.Helper()
	cfg, err := guardrails.ParseConfig(map[string]any{
		"provider":  "builtin",
		"detectors": []any{map[string]any{"name": category}},
		"on_detect": guardrails.ActionLog,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = guardrails.Normalize(cfg, false); err != nil {
		t.Fatal(err)
	}
	p, err := guardrails.NewProvider(cfg)
	if err != nil {
		t.Fatalf("builtin provider: %v", err)
	}
	return cfg, p
}

// The tool-request fixtures are only useful if their canaries trip the
// shipped built-in injection patterns and nothing else in them does: then a
// detection in a tool-result test proves the guardrail read the tool output,
// and a miss proves it did not. This checks the fixtures, not the feature,
// and passes before any 4uc bead lands.
func TestToolFixtures_CanariesTripBuiltinPatterns(t *testing.T) {
	injCfg, inj := builtinProvider(t, guardrails.CategoryInjection)
	piiCfg, pii := builtinProvider(t, guardrails.CategoryPII)

	classify := func(cfg guardrails.Config, p guardrails.Provider, text string) guardrails.Verdict {
		v, err := guardrails.Classify(context.Background(), cfg, p, guardrails.Input{
			Segments:  []guardrails.Segment{{Index: 0, Role: "user", Text: text}},
			Direction: guardrails.DirectionRequest,
		})
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		return v
	}

	for _, f := range toolfixtures.All() {
		t.Run(f.Name, func(t *testing.T) {
			doc, err := toolfixtures.Decode(f.Body)
			if err != nil {
				t.Fatal(err)
			}
			text := func(ptr string) string {
				v, _ := toolfixtures.Lookup(doc, ptr)
				s, _ := v.(string)
				return s
			}
			for _, s := range f.Segments {
				got := classify(injCfg, inj, text(s.Path))
				if got.Flagged != s.Injection {
					t.Errorf("%s: builtin injection flagged = %v, want %v (findings %+v)", s.Path, got.Flagged, s.Injection, got.Findings)
				}
				if s.PII {
					if v := classify(piiCfg, pii, text(s.Path)); !v.Flagged {
						t.Errorf("%s: builtin pii did not flag %s", s.Path, toolfixtures.PIIEmail)
					}
				}
			}
			for _, p := range f.OptionalSegments {
				if got := classify(injCfg, inj, text(p)); got.Flagged {
					t.Errorf("optional segment %s trips the injection patterns: %+v", p, got.Findings)
				}
			}
			// Binary leaves must stay out of the segment list; several would
			// trip injection.base64_blob if a segmenter scanned them.
			for _, p := range f.NotSegments {
				if got := classify(injCfg, inj, text(p)); got.Flagged {
					t.Logf("not-segment %s would trip %s if scanned", p, got.Findings[0].Detector)
				}
			}
		})
	}
}
