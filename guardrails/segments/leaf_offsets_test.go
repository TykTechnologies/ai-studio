package segments

import (
	"testing"
)

// LeafOffsets locates each leaf in a unit's Text: one leaf at 0, several
// with the separator between them; a unit whose Text holds more than its
// leaves (a Google functionResponse's keys) has none.
func TestLeafOffsets(t *testing.T) {
	body := `{"model":"m","max_tokens":5,"system":[{"type":"text","text":"first block","cache_control":{"type":"ephemeral"}},{"type":"text","text":"second"}],
		"messages":[{"role":"user","content":"one"}]}`
	units, err := ExtractUnits(FormatAnthropic, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	sys, user := units[0], units[1]
	offs, ok := sys.LeafOffsets()
	if !ok || len(offs) != 2 || offs[0] != 0 {
		t.Fatalf("system offsets = %v %v", offs, ok)
	}
	if sys.Text[offs[0]:offs[0]+len("first block")] != "first block" || sys.Text[offs[1]:] != "second" {
		t.Errorf("offsets %v do not locate the leaves in %q", offs, sys.Text)
	}
	if sys.Joined != "first blocksecond" {
		t.Errorf("joined = %q", sys.Joined)
	}
	if offs, ok := user.LeafOffsets(); !ok || len(offs) != 1 || offs[0] != 0 {
		t.Errorf("user offsets = %v %v", offs, ok)
	}

	g := `{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"f","response":{"key":"value"}}}]}]}`
	units, err = ExtractUnits(FormatGoogleAI, []byte(g))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if u.Text != "value" {
			if _, ok := u.LeafOffsets(); ok {
				t.Errorf("unit %q with detection-only text has leaf offsets", u.Text)
			}
		}
	}
}

// An OpenAI developer message is read as user text and marked Developer.
func TestDeveloperMarked(t *testing.T) {
	body := `{"model":"m","messages":[{"role":"developer","content":"rules"},{"role":"user","content":"hi"}]}`
	units, err := ExtractUnits(FormatOpenAI, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if units[0].Role != "user" || !units[0].Developer || units[1].Developer {
		t.Errorf("units = %+v / %+v", units[0].Segment, units[1].Segment)
	}
	body = `{"model":"m","input":[{"role":"developer","content":"rules"},{"role":"user","content":"hi"}]}`
	units, err = ExtractUnits(FormatOpenAIResponses, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !units[0].Developer || units[1].Developer {
		t.Errorf("responses units: developer %v %v", units[0].Developer, units[1].Developer)
	}
}
