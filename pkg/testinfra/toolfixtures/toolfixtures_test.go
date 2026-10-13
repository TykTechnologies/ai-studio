package toolfixtures

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The manifest must describe its bodies exactly, or every test built on it
// asserts the wrong thing.
func TestManifestMatchesBodies(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("no fixtures loaded")
	}
	seen := map[string]bool{}
	for _, f := range all {
		t.Run(f.Name, func(t *testing.T) {
			if seen[f.Name] {
				t.Fatalf("duplicate fixture name")
			}
			seen[f.Name] = true
			switch f.Status {
			case StatusInScope, StatusOpenQuestion, StatusControl:
			default:
				t.Errorf("unknown status %q", f.Status)
			}
			doc, err := Decode(f.Body)
			if err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}

			injectionPaths := map[string]bool{}
			for _, s := range f.Segments {
				v, ok := Lookup(doc, s.Path)
				if !ok {
					t.Errorf("segment %s does not resolve", s.Path)
					continue
				}
				text, ok := v.(string)
				if !ok {
					t.Errorf("segment %s is %T, want a string leaf", s.Path, v)
					continue
				}
				if !strings.Contains(text, s.Contains) {
					t.Errorf("segment %s = %q, want it to contain %q", s.Path, text, s.Contains)
				}
				if got := strings.Contains(text, CanaryPrefix); got != s.Injection {
					t.Errorf("segment %s: carries canary = %v, injection flag = %v", s.Path, got, s.Injection)
				}
				if s.PII && !strings.Contains(text, PIIEmail) {
					t.Errorf("segment %s is flagged pii but lacks %s", s.Path, PIIEmail)
				}
				if s.IsTool() {
					if s.ToolName == "" || s.Kind != "tool_result" {
						t.Errorf("tool segment %s needs tool_name and kind tool_result", s.Path)
					}
					if s.ToolCallID != "" && !strings.Contains(string(f.Body), strconv.Quote(s.ToolCallID)) {
						t.Errorf("tool segment %s: id %s not in the body", s.Path, s.ToolCallID)
					}
				} else if s.Injection {
					t.Errorf("segment %s: the canary may only sit in tool output", s.Path)
				}
				if s.Injection {
					injectionPaths[s.Path] = true
				}
			}
			for _, p := range append(append([]string{}, f.OptionalSegments...), f.NotSegments...) {
				v, ok := Lookup(doc, p)
				if !ok {
					t.Errorf("pointer %s does not resolve", p)
					continue
				}
				if _, ok := v.(string); !ok {
					t.Errorf("pointer %s is %T, want a string leaf", p, v)
				}
			}
			for _, p := range f.Preserve {
				if _, ok := Lookup(doc, p); !ok {
					t.Errorf("preserve pointer %s does not resolve", p)
				}
			}
			if len(f.InjectionSegments()) == 0 {
				t.Error("every fixture plants at least one canary")
			}

			// The canary appears nowhere but in the injection segments, so a
			// detection proves the guardrail read tool output.
			walkStrings("", doc, func(ptr, s string) {
				if strings.Contains(s, CanaryPrefix) && !injectionPaths[ptr] {
					t.Errorf("canary at %s, which is not an injection segment", ptr)
				}
			})
		})
	}
}

func walkStrings(ptr string, v any, fn func(ptr, s string)) {
	switch n := v.(type) {
	case map[string]any:
		for k, x := range n {
			walkStrings(ptr+"/"+escapeToken(k), x, fn)
		}
	case []any:
		for i, x := range n {
			walkStrings(ptr+"/"+strconv.Itoa(i), x, fn)
		}
	case string:
		fn(ptr, n)
	}
}

// jsonEscape is the JSON escape for one ASCII character, spelt so no tool
// along the way turns it back into the character.
func jsonEscape(c byte) string { return fmt.Sprintf("%c%c00%02x", 92, 'u', c) }

func TestDiffPaths(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		want          []string
	}{
		{"key order and whitespace", `{"a":1,"b":[1,2]}`, `{ "b": [1, 2], "a": 1 }`, nil},
		{"equal decimals", `{"x":1.50}`, `{"x":1.5}`, nil},
		{"html escaping is not a change", `{"s":"<b>&"}`, `{"s":"` + jsonEscape('<') + `b` + jsonEscape('>') + jsonEscape('&') + `"}`, nil},
		{"float64 precision loss", `{"seed":9007199254740993}`, `{"seed":9007199254740992}`, []string{"/seed"}},
		{"leaf rewrite", `{"m":[{"c":"x","id":"1"}]}`, `{"m":[{"c":"y","id":"1"}]}`, []string{"/m/0/c"}},
		{"dropped key", `{"m":[{"c":"x","id":"1"}]}`, `{"m":[{"c":"x"}]}`, []string{"/m/0/id"}},
		{"added key", `{"m":{}}`, `{"m":{"k":null}}`, []string{"/m/k"}},
		{"array length", `{"m":[1,2]}`, `{"m":[1]}`, []string{"/m"}},
		{"string to blocks", `{"c":"x"}`, `{"c":[{"type":"text","text":"x"}]}`, []string{"/c"}},
		{"null vs absent", `{"c":null}`, `{}`, []string{"/c"}},
		{"escaped pointer", `{"a/b":{"~":1}}`, `{"a/b":{"~":2}}`, []string{"/a~1b/~0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DiffPaths([]byte(c.before), []byte(c.after))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("DiffPaths() = %v, want %v", got, c.want)
			}
		})
	}
	if _, err := DiffPaths([]byte(`{}`), []byte(`{"broken`)); err == nil {
		t.Error("invalid JSON must be an error")
	}
}

func TestLookupPointers(t *testing.T) {
	doc, _ := Decode([]byte(`{"a":[{"b":"x"}],"c/d":"y","e~f":"z"}`))
	for ptr, want := range map[string]any{"/a/0/b": "x", "/c~1d": "y", "/e~0f": "z"} {
		if got, ok := Lookup(doc, ptr); !ok || got != want {
			t.Errorf("Lookup(%s) = %v, %v", ptr, got, ok)
		}
	}
	for _, ptr := range []string{"/a/1", "/a/x", "/nope", "a", "/a/0/b/c"} {
		if _, ok := Lookup(doc, ptr); ok {
			t.Errorf("Lookup(%s) resolved, want not found", ptr)
		}
	}
}

// Bodies are copies: one test mutating a fixture cannot break another.
func TestAllReturnsCopies(t *testing.T) {
	f := Get("openai_tool_string")
	f.Body[0] = 'X'
	if Get("openai_tool_string").Body[0] == 'X' {
		t.Fatal("All() shares body slices between calls")
	}
	if !json.Valid(Get("openai_tool_string").Body) {
		t.Fatal("fixture corrupted")
	}
}

// The helpers fail on a rewrite that drops a tool id and pass on a rewrite
// that only touches the flagged leaf.
func TestAssertHelpers(t *testing.T) {
	f := Get("openai_tool_string")
	doc, _ := Decode(f.Body)
	seg := f.InjectionSegments()[0]

	good := setLeaf(t, doc, seg.Path, "[content removed by policy]")
	AssertOnlyChanged(t, f.Body, good, seg.Path)
	AssertPreserved(t, f, good)

	doc, _ = Decode(f.Body)
	delete(doc.(map[string]any)["messages"].([]any)[3].(map[string]any), "tool_call_id")
	bad, _ := json.Marshal(doc)
	rec := &recorder{TB: t}
	AssertPreserved(rec, f, bad)
	AssertOnlyChanged(rec, f.Body, bad, seg.Path)
	if rec.errors != 2 {
		t.Errorf("helpers reported %d failures for a dropped tool_call_id, want 2", rec.errors)
	}
}

func setLeaf(t *testing.T, doc any, ptr, value string) []byte {
	t.Helper()
	i := strings.LastIndex(ptr, "/")
	parent, ok := Lookup(doc, ptr[:i])
	if !ok {
		t.Fatalf("parent of %s not found", ptr)
	}
	key := ptr[i+1:]
	switch p := parent.(type) {
	case map[string]any:
		p[key] = value
	case []any:
		n, _ := strconv.Atoi(key)
		p[n] = value
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type recorder struct {
	testing.TB
	errors int
}

func (r *recorder) Errorf(string, ...any) { r.errors++ }
func (r *recorder) Helper()               {}
