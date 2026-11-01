package guardrails

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestNormalizeTemplateBackstop(t *testing.T) {
	base := Config{Provider: ProviderBuiltin, Detectors: []DetectorConfig{{Name: CategoryInjection}}, OnDetect: ActionBlock}
	for _, mode := range []string{"", BackstopOff, BackstopLog, BackstopEnforce} {
		c := base
		c.TemplateBackstop = mode
		got, err := Normalize(c, false)
		if err != nil {
			t.Fatalf("%q: %v", mode, err)
		}
		if got.TemplateBackstop != mode {
			t.Errorf("%q is stored as %q: unset must stay unset, so stored configs do not change", mode, got.TemplateBackstop)
		}
	}
	if got := base.Backstop(); got != BackstopLog {
		t.Errorf("unset backstop = %q, want log", got)
	}
	c := base
	c.TemplateBackstop = "always"
	if _, err := Normalize(c, false); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("unknown mode: err %v, want ErrInvalidConfig", err)
	}

	// Unset, the field is not in the stored or synced config at all.
	m, err := base.ToMap()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["template_backstop"]; ok {
		t.Errorf("an unset template_backstop is written: %v", m)
	}
}

// template_backstop_min_score is 0 to 1, 0.9 when unset, and stays unset in
// the stored config when not given.
func TestNormalizeTemplateBackstopMinScore(t *testing.T) {
	base := Config{Provider: ProviderBuiltin, Detectors: []DetectorConfig{{Name: CategoryInjection}}, OnDetect: ActionBlock}
	if got := base.BackstopMinScore(); got != 0.9 {
		t.Errorf("unset min score = %v, want 0.9", got)
	}
	for _, v := range []float64{0, 0.5, 1} {
		c := base
		c.TemplateBackstopMinScore = &v
		got, err := Normalize(c, false)
		if err != nil {
			t.Fatalf("%v: %v", v, err)
		}
		if got.BackstopMinScore() != v {
			t.Errorf("%v is read as %v", v, got.BackstopMinScore())
		}
	}
	for _, v := range []float64{-0.1, 1.01, math.NaN()} {
		c := base
		c.TemplateBackstopMinScore = &v
		if _, err := Normalize(c, false); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%v: err %v, want ErrInvalidConfig", v, err)
		}
	}
	m, err := base.ToMap()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["template_backstop_min_score"]; ok {
		t.Errorf("an unset template_backstop_min_score is written: %v", m)
	}

	// On the wire it is absent unless the backstop was forced.
	b, _ := json.Marshal(SegmentTemplate{ID: "t", Version: 1, FullText: "f", Backstop: BackstopEnforce, BackstopMinScore: 0.9})
	if want := `{"id":"t","version":1,"full_text":"f","exempt":null,"backstop":"enforce","backstop_min_score":0.9}`; string(b) != want {
		t.Errorf("forced template = %s\nwant %s", b, want)
	}
}

// The segment and finding fields are additive on the wire: absent unless
// set, so a classifier that predates them sees what it always saw.
func TestTemplateWireFieldsAreAdditive(t *testing.T) {
	b, _ := json.Marshal(Segment{Index: 0, Role: RoleUser, Text: "hi"})
	if string(b) != `{"index":0,"role":"user","text":"hi"}` {
		t.Errorf("segment without a template = %s", b)
	}
	b, _ = json.Marshal(Segment{Index: 0, Role: RoleUser, Text: "[app template] diff", Template: &SegmentTemplate{
		ID: "pr-review", Version: 3, FullText: "You review. diff", Exempt: []Span{{Start: 0, End: 11}}, Backstop: BackstopLog,
	}})
	want := `{"index":0,"role":"user","text":"[app template] diff","template":{"id":"pr-review","version":3,"full_text":"You review. diff","exempt":[{"start":0,"end":11}],"backstop":"log"}}`
	if string(b) != want {
		t.Errorf("segment with a template = %s\nwant %s", b, want)
	}
	var f Finding
	if err := json.Unmarshal([]byte(`{"detector":"prompt_attack","segment_index":0,"backstop":true}`), &f); err != nil || !f.Backstop {
		t.Errorf("backstop finding: %+v %v", f, err)
	}

	seg := Segment{Index: 2, Role: RoleUser, Text: "residual", Template: &SegmentTemplate{ID: "t", Version: 1, FullText: "full"}}
	if got := seg.ForProvider(0, "residual", Capabilities{}); got.Template != nil {
		t.Errorf("a provider that does not read templates was sent one")
	}
	if got := seg.ForProvider(0, "resid", Capabilities{Templates: true}); got.Template != nil {
		t.Errorf("a piece of a segment was sent with its template")
	}
	got := seg.ForProvider(0, "residual", Capabilities{Templates: true})
	if got.Template == nil || got.Template == seg.Template || got.Template.FullText != "full" {
		t.Errorf("a template-aware provider gets a copy of the template: %+v", got.Template)
	}
}
