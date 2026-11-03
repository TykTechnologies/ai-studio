package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Edges get no LLM privacy scores, so a judge scored below an LLM the filter
// guards is left out of the config they receive (midsommar-clean-9u1j.46).
func TestEdgeJudgeConfig_PrivacyRule(t *testing.T) {
	cfg := func() models.JSONMap {
		return models.JSONMap{"provider": "builtin", "detectors": []any{map[string]any{"name": "injection"}}, "on_detect": "block",
			"judge": map[string]any{"mode": "record", "llm_id": 70}}
	}
	privacy := map[uint]int{70: 50, 1: 30, 2: 50, 3: 90}

	for _, c := range []struct {
		name   string
		llms   []uint32
		strips bool
	}{
		{"judge above every LLM", []uint32{1}, false},
		{"judge equal", []uint32{1, 2}, false},
		{"one LLM above the judge", []uint32{1, 3}, true},
		{"not attached", nil, false},
		{"LLM not in the snapshot", []uint32{99}, false},
	} {
		in := cfg()
		out := edgeJudgeConfig(4, in, c.llms, privacy)
		_, has := out["judge"]
		if has == c.strips {
			t.Errorf("%s: judge kept = %v", c.name, has)
		}
		if _, still := in["judge"]; !still {
			t.Errorf("%s: the stored config was modified", c.name)
		}
	}

	// A judge LLM the snapshot does not hold, or a judge that is off, is
	// left as it is.
	off := cfg()
	off["judge"] = map[string]any{"mode": "off", "llm_id": 70}
	if _, has := edgeJudgeConfig(4, off, []uint32{3}, privacy)["judge"]; !has {
		t.Error("an off judge was touched")
	}
	missing := cfg()
	missing["judge"] = map[string]any{"mode": "record", "llm_id": 71}
	if _, has := edgeJudgeConfig(4, missing, []uint32{3}, privacy)["judge"]; !has {
		t.Error("a judge the snapshot does not hold was touched")
	}
}
