package segments

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// errText is an error's message, "" for none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// FuzzDocMatchesFunctions: a Doc's methods answer exactly as the functions
// that parse the body themselves, before and after Apply, for leaf edits
// (parsed back without validation) and for value edits.
func FuzzDocMatchesFunctions(f *testing.F) {
	for _, fx := range fixtures {
		for i, format := range fuzzFormats {
			if format == fx.format {
				f.Add(uint8(i), []byte(fx.body), false)
				f.Add(uint8(i), []byte(fx.body), true)
			}
		}
	}
	f.Add(uint8(0), []byte(`{"messages":[{"role":"tool","content":"\ud800"}]}`), false)
	f.Add(uint8(0), []byte(`{"messages":[{"role":"tool","content":"a"},{"role":"tool","content":"a"}],"messages":1}`), false)
	f.Add(uint8(0), []byte(`[1]`), false)

	f.Fuzz(func(t *testing.T, fi uint8, body []byte, value bool) {
		format := fuzzFormats[int(fi)%len(fuzzFormats)]
		doc, err := Parse(format, body)
		units, uErr := ExtractUnits(format, body)
		if err != nil {
			if uErr == nil || err.Error() != uErr.Error() {
				t.Fatalf("Parse: %v, ExtractUnits: %v", err, uErr)
			}
			return
		}
		same := func(d *Doc, b []byte) {
			t.Helper()
			want, wErr := ExtractUnits(format, b)
			got, gErr := d.Units(Options{})
			if errText(gErr) != errText(wErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("Units differ: %v / %v", gErr, wErr)
			}
			wantAll, wErr := ExtractUnitsWith(format, b, Options{ToolCalls: true, ToolDefinitions: true})
			gotAll, gErr := d.Units(Options{ToolCalls: true, ToolDefinitions: true})
			if errText(gErr) != errText(wErr) || !reflect.DeepEqual(gotAll, wantAll) {
				t.Fatalf("Units with options differ: %v / %v", gErr, wErr)
			}
			if d.NativeContextManagement() != NativeContextManagement(format, b) {
				t.Fatal("NativeContextManagement differs")
			}
			wb, wErr := CacheBreakpoints(format, b)
			gb, gErr := d.CacheBreakpoints()
			if errText(gErr) != errText(wErr) || !reflect.DeepEqual(gb, wb) {
				t.Fatalf("CacheBreakpoints differ: %v / %v", gErr, wErr)
			}
			if errText(d.CheckToolPairing()) != errText(CheckToolPairing(format, b)) {
				t.Fatal("CheckToolPairing differs")
			}
		}
		same(doc, body)
		if uErr != nil {
			return
		}

		var edits []Edit
		for _, u := range units {
			for _, l := range u.Leaves {
				e := Edit{Path: l.Path, Old: l.Text, New: "[q]" + l.Text[:len(l.Text)/2]}
				if value && len(edits) == 0 {
					e = Edit{Path: l.Path, Value: json.RawMessage(`{"replaced":["x"]}`)}
				}
				edits = append(edits, e)
			}
		}
		want, wErr := Apply(body, edits)
		out, gErr := doc.Apply(edits)
		if errText(gErr) != errText(wErr) {
			t.Fatalf("Apply: %v, Doc.Apply: %v", wErr, gErr)
		}
		if wErr != nil {
			return
		}
		if !bytes.Equal(out.Body(), want) || !json.Valid(want) {
			t.Fatal("Doc.Apply wrote other bytes")
		}
		same(out, want)
	})
}

// FuzzDecodeStringMatchesJSON: decodeString reads every string token
// json.Valid accepts exactly as encoding/json does.
func FuzzDecodeStringMatchesJSON(f *testing.F) {
	for _, s := range []string{`""`, `"plain"`, `"a\nb\t\"q\"\\\/"`, `"\u00e9\u20ac"`, `"\ud83d\ude00"`, `"\ud800"`, `"\udc00x"`,
		`"\ud800\u0041"`, `"\ud800\ud800\udc00"`, "\"\xff\xfe\"", "\"caf\xc3\xa9 \xe2\x82\"", `"\u001b[32mok\u001b[0m"`, `"\uD83D\uDE00"`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		// A token as the parser spans it: no space around it
		if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' || !json.Valid(raw) {
			return
		}
		var want string
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("json.Unmarshal of a valid token: %v", err)
		}
		if got := decodeString(raw); got != want {
			t.Fatalf("decodeString(%q) = %q, want %q", raw, got, want)
		}
	})
}

// Raw is the value's bytes as the body holds them; As reads the same tree
// in another format.
func TestDocRawAndAs(t *testing.T) {
	body := []byte(`{"tools": [ {"name":"a"} ],"toolConfig":{"tools":[1]},"messages":[{"role":"user","content":"hi"}]}`)
	d, err := Parse(FormatOpenAI, body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(d.Raw("/tools")); got != `[ {"name":"a"} ]` {
		t.Errorf("Raw(/tools) = %s", got)
	}
	if got := string(d.Raw("/toolConfig/tools")); got != `[1]` {
		t.Errorf("Raw(/toolConfig/tools) = %s", got)
	}
	if d.Raw("/missing") != nil || d.Raw("/tools/5") != nil {
		t.Error("Raw found a missing value")
	}
	a, err := d.As(FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ExtractUnits(FormatAnthropic, body)
	got, _ := a.Units(Options{})
	if !reflect.DeepEqual(got, want) {
		t.Error("As read differently from ExtractUnits")
	}
	if _, err := d.As("huggingface"); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("As an unsupported format: %v", err)
	}
}

func TestDocApplyWithoutEditsIsTheDoc(t *testing.T) {
	d, err := Parse(FormatOpenAI, []byte(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := d.Apply(nil); err != nil || out != d {
		t.Fatalf("%v %v", out, err)
	}
}
