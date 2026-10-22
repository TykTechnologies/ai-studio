package segments

import (
	"encoding/json"
	"testing"
)

var fuzzFormats = []string{FormatOpenAI, FormatOllama, FormatAnthropic, FormatGoogleAI, FormatBedrock, FormatOpenAIResponses}

// assertUnitsMapBack: an unchanged unit text maps back to no edits, and a
// rewrite of every leaf maps back to one edit per leaf.
func assertUnitsMapBack(t *testing.T, units []Unit) {
	t.Helper()
	for _, u := range units {
		if u.whole == wholeNone {
			// Detection-only (the model's own Responses items): never
			// rewritten.
			if _, ok := u.MapRewrite(u.Text); ok || len(u.Leaves) != 0 {
				t.Fatalf("unit %d: a detection-only unit is rewritable", u.Index)
			}
			continue
		}
		if edits, ok := u.MapRewrite(u.Text); !ok || len(edits) != 0 {
			t.Fatalf("unit %d: identity rewrite gave %v, %v", u.Index, edits, ok)
		}
		if len(u.pieces) == 0 {
			if !u.Undecodable {
				t.Fatalf("unit %d has no text", u.Index)
			}
			continue
		}
		parts := make([]string, len(u.pieces))
		for i, p := range u.pieces {
			parts[i] = p.text
			if p.leaf >= 0 {
				parts[i] = "[q]" + p.text
			}
		}
		joined := parts[0]
		for _, p := range parts[1:] {
			joined += u.sep + p
		}
		edits, ok := u.MapRewrite(joined)
		if !ok || len(edits) != len(u.Leaves) {
			t.Fatalf("unit %d: rewriting every leaf gave %d edits (ok %v), want %d", u.Index, len(edits), ok, len(u.Leaves))
		}
	}
}

// FuzzExtractRewrite: Extract never panics, and rewriting every segment it
// returns yields valid JSON in which Extract finds the same segments with
// the new text.
func FuzzExtractRewrite(f *testing.F) {
	for _, fx := range fixtures {
		for i, format := range fuzzFormats {
			if format == fx.format {
				f.Add(uint8(i), []byte(fx.body))
			}
		}
	}
	f.Add(uint8(0), []byte(`{"messages":[{"role":"tool","content":"\ud800"}]}`))
	f.Add(uint8(3), []byte(`{"contents":[{"parts":[{"functionResponse":{"response":{"":"", "~":["x"]}}}]}]}`))

	f.Fuzz(func(t *testing.T, fi uint8, body []byte) {
		format := fuzzFormats[int(fi)%len(fuzzFormats)]
		// The rewriter helpers never panic, and a breakpoint's position is
		// one Cached can compare units against.
		_ = CheckToolPairing(format, body)
		_ = NativeContextManagement(format, body)
		_ = NativeContextManagement(FormatOpenAIResponses, body)
		_, _ = LastCacheBreakpoint(format, body)
		bps, bpErr := CacheBreakpoints(format, body)
		if units, err := ExtractUnits(format, body); err == nil && bpErr == nil {
			for _, bp := range bps {
				for _, u := range units {
					_ = u.Cached(bp)
				}
			}
		}
		if units, err := Generic(body); err == nil {
			assertUnitsMapBack(t, units)
		}
		units, err := ExtractUnits(format, body)
		if err != nil {
			return
		}
		assertUnitsMapBack(t, units)
		segs, err := Extract(format, body)
		if err != nil {
			t.Fatalf("ExtractUnits succeeded, Extract failed: %v", err)
		}
		edits := make(map[string]string, len(segs))
		for _, s := range segs {
			edits[s.Path] = "[q]" + s.Text
		}
		out, err := Rewrite(body, edits)
		if err != nil {
			t.Fatalf("Rewrite of extracted paths failed: %v", err)
		}
		if !json.Valid(out) {
			t.Fatalf("Rewrite produced invalid JSON: %s", out)
		}
		again, err := Extract(format, out)
		if err != nil {
			t.Fatalf("Extract after Rewrite: %v", err)
		}
		if len(again) != len(segs) {
			t.Fatalf("segment count %d -> %d", len(segs), len(again))
		}
		for i := range again {
			if again[i].Path != segs[i].Path || again[i].Text != edits[segs[i].Path] {
				t.Fatalf("segment %d: %+v, want path %s text %q", i, again[i], segs[i].Path, edits[segs[i].Path])
			}
		}
	})
}
