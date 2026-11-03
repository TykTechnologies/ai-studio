package nativeapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
)

// checkJSONKeysMain is CheckJSONKeys as main had it before 53e.50, verbatim
// (json.Decoder.Token, a path per frame, no nesting limit): the reference
// for verdicts and error text. Up to encoding/json's nesting limit every
// verdict and text must match it; deeper bodies are now refused (53e.58).
func checkJSONKeysMain(b []byte, foldAt ...string) error {
	fold := make(map[string]bool, len(foldAt))
	for _, p := range foldAt {
		fold[p] = true
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	// One frame per open object or array.
	type frame struct {
		path    string
		isArray bool
		exact   map[string]bool
		folded  map[string]bool // nil: case variants are not checked here
		key     string          // the key whose value comes next
	}
	var stack []*frame
	childPath := func() string {
		if len(stack) == 0 {
			return ""
		}
		f := stack[len(stack)-1]
		if f.isArray {
			return f.path + "[]"
		}
		if f.path == "" {
			return f.key
		}
		return f.path + "." + f.key
	}
	started := false
	for {
		if started && len(stack) == 0 {
			if _, err := dec.Token(); err != io.EOF {
				return errors.New("trailing data after JSON value")
			}
			return nil
		}
		// In an object, a key comes before every value.
		if len(stack) > 0 && !stack[len(stack)-1].isArray && dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := tok.(string)
			if !ok {
				return errors.New("expected an object key")
			}
			f := stack[len(stack)-1]
			if f.exact[key] {
				return fmt.Errorf("%w %q", ErrDuplicateKey, key)
			}
			f.exact[key] = true
			if f.folded != nil {
				folded := FoldKey(key)
				if f.folded[folded] {
					return fmt.Errorf("%w %q", ErrDuplicateKey, key)
				}
				f.folded[folded] = true
			}
			f.key = key
		}
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF && !started {
				return errors.New("empty JSON body")
			}
			return err
		}
		started = true
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{':
				p := childPath()
				f := &frame{path: p, exact: map[string]bool{}}
				if fold[p] {
					f.folded = map[string]bool{}
				}
				stack = append(stack, f)
			case '[':
				stack = append(stack, &frame{path: childPath(), isArray: true})
			case '}', ']':
				stack = stack[:len(stack)-1]
			}
		}
	}
}

// sameVerdict reports whether CheckJSONKeys (and the bounded json.Decoder
// version it falls back to for wording) agree with main's on b: both accept
// it, or both refuse it with the same error. A body nested deeper than
// encoding/json's limit is the one allowed difference: it must be refused.
func sameVerdict(b []byte, foldAt ...string) (bool, error, error) {
	got := CheckJSONKeys(b, foldAt...)
	if nestingDepth(b) > maxNestingDepth {
		return got != nil, got, errors.New("refused: deeper than the nesting limit")
	}
	want := checkJSONKeysMain(b, foldAt...)
	for _, e := range []error{got, checkJSONKeysDecoder(b, foldAt...)} {
		if (e == nil) != (want == nil) {
			return false, e, want
		}
		if e != nil && (errors.Is(e, ErrDuplicateKey) != errors.Is(want, ErrDuplicateKey) || e.Error() != want.Error()) {
			return false, e, want
		}
	}
	return true, got, want
}

// nestingDepth is the deepest nesting of objects and arrays in b, strings
// skipped; for a malformed b, as far as it can tell.
func nestingDepth(b []byte) int {
	depth, max, inStr := 0, 0, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inStr && c == '\\':
			i++
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{' || c == '[':
			depth++
			if depth > max {
				max = depth
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return max
}

var keyCheckCorpus = []string{
	``, ` `, `{}`, `[]`, `"x"`, `1`, `-0`, `-0.5e+3`, `1E9`, `null`, `true`, `false`,
	`{"a":1}`, `{"a":1,"a":2}`, `{"a":1,"A":2}`, `{"messages":[],"meſſages":[]}`, `{"k":1,"\u212a":2}`,
	`{"model":"ok","\u006dodel":"evil"}`, `{"a\ud800":1,"a\ufffd":2}`, "{\"a\xff\":1,\"a\xfe\":2}",
	"{\"a\xff\":1,\"a\\ufffd\":2}", `{"a":"\u00"}`, `{"a":"\x"}`, "{\"a\":\"\x01\"}", `{"a":"\/\b\f\n\r\t\"\\"}`,
	`{"messages":[{"role":"user","content":"ok","content":"evil"}]}`,
	`{"messages":[{"role":"user","content":"ok","Content":"evil"}]}`,
	`{"tool_choice":{"type":"any","Type":"none"}}`, `{"tools":[{"input_schema":{"properties":{"id":{},"ID":{}}}}]}`,
	`{"a":{"x":1},"A":2}`, `[[{"a":1}],[{"b":1,"b":2}]]`, `{"a":1,}`, `{"a"}`, `{"a":1} {}`, `[1,2`, `[1,]`, `nope`,
	`{"a":1,"a"}`, `{"a":1,"a":`, `{,}`, `[,1]`, `01`, `1.`, `.5`, `-`, `1e`, `1e+`, `tru`, `nul`, `[1 2]`, `{"a" 1}`,
	`{"a":1}x`, ` {"a":[1,{"b":[]},"c"]} `, "\t[\r\n]", `["\u12G4"]`, `{"":1,"":2}`, `{"":1}`,
	`[[[[[[[[[[]]]]]]]]]]`,
	`{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"m","messages":[{"role":"user","content":"x"}]}}`,
	`{"custom_id":"a","body":{"model":"m"},"Body":{"model":"evil"}}`,
	`{"custom_id":"a","body":{"model":"m","Model":"evil"}}`,
	`{"body":{"messages":[{"role":"user","content":"x","Content":"y"}]}}`,
	`{"body":{"tools":[{"type":"function","function":{"name":"f","Name":"g"}}]}}`,
	`{"body":{"tools":[{"type":"function","function":{"name":"f","parameters":{"properties":{"id":{},"ID":{}}}}}]}}`,
	`{"body":{"response_format":{"type":"json_object","Type":"text"}}}`,
	`{"body":{"audio":{"voice":"a","VOICE":"b"},"stream_options":{"include_usage":true,"Include_Usage":false}}}`,
	`{"metadata":{"k":"a","K":"b"},"items":[{"type":"message","role":"user","content":"x"}]}`, `{"a":[{"b":{"c":[{"d":1,"D":2}]}}]}`, `{"messages":[{"a":[{"k":1,"K":2}]}]}`,
}

// Paths are strings, so keys holding "." or "[]" can spell a foldAt path,
// and the empty key at the root keeps the root's path: main folded those
// objects, and so must the bounded paths (53e.58).
var keyCheckPathEdges = []string{
	`{"messages[]":{"x":1,"X":2}}`,
	`{"body.tools[]":{"function":{"name":"f","Name":"g"}}}`,
	`{"body":{"tools[].function":{"name":"f","Name":"g"}}}`,
	`{"":{"a":1,"A":2}}`,
	`{"":{"":{"a":1,"A":2}}}`,
	`{"messages":[[{"a":1,"A":2}]]}`,
	`{"tool_choice":{"x":{"a":1,"A":2}}}`,
	`{"tool":{"a":1,"A":2},"tool_choicex":{"a":1,"A":2}}`,
}

var keyCheckFoldPaths = []string{"", "messages[]", "tool_choice"}

// The other callers' fold paths: OpenAI batch lines (vendors/openai
// batchLineFoldPaths) and conversations (the root only).
var (
	batchLineFoldPaths = []string{
		"", "body", "body.messages[]", "body.tools[]", "body.tools[].function",
		"body.response_format", "body.audio", "body.stream_options",
	}
	keyCheckFoldSets = [][]string{nil, {""}, keyCheckFoldPaths, batchLineFoldPaths}
)

// TestCheckJSONKeys_MatchesDecoder: the byte scanner accepts and refuses
// what the json.Decoder version did, duplicates as duplicates.
func TestCheckJSONKeys_MatchesDecoder(t *testing.T) {
	corpus := keyCheckCorpus
	// Up to encoding/json's nesting limit as main; deeper is refused (53e.58).
	for _, depth := range []int{9999, 10000, 10001, 20000} {
		corpus = append(corpus,
			strings.Repeat("[", depth)+"1"+strings.Repeat("]", depth),
			strings.Repeat("[", depth-1)+`{"a":1,"a":2}`+strings.Repeat("]", depth-1))
	}
	// Malformed bodies keep encoding/json's wording (53e.56).
	corpus = append(corpus, `{"a":"\x"}`, `{"a":01}`, `{"a":tru}`, `{`, `{"a":"unterminated`, "\x00", `{"a":1]`, `[1}`)
	corpus = append(corpus, keyCheckPathEdges...)
	for _, b := range corpus {
		for _, foldAt := range append(keyCheckFoldSets, []string{"a", "a[]", "a[].b.c[]"}) {
			if ok, got, want := sameVerdict([]byte(b), foldAt...); !ok {
				t.Errorf("%.80q foldAt %q: scanner %v, decoder %v", b, foldAt, got, want)
			}
		}
	}
}

func FuzzCheckJSONKeys(f *testing.F) {
	for _, b := range append(keyCheckCorpus, keyCheckPathEdges...) {
		f.Add([]byte(b))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, foldAt := range keyCheckFoldSets {
			if ok, got, want := sameVerdict(b, foldAt...); !ok {
				t.Fatalf("%q foldAt %q: scanner %v, decoder %v", b, foldAt, got, want)
			}
		}
	})
}

// TestCheckJSONKeys_AgentSizedBody: a duplicate after a long string, or deep
// in the body, is still found.
func TestCheckJSONKeys_AgentSizedBody(t *testing.T) {
	long := strings.Repeat("lorem ipsum \u00e9 \\n ", 20000)
	for _, b := range []string{
		fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"%s"}],"model":"evil"}`, long),
		fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"%s","Content":"x"}]}`, long),
		fmt.Sprintf(`{"messages":[{"role":"user","content":[{"type":"text","text":"%s","text":"evil"}]}]}`, long),
	} {
		if err := CheckJSONKeys([]byte(b), keyCheckFoldPaths...); !errors.Is(err, ErrDuplicateKey) {
			t.Errorf("err = %v, want ErrDuplicateKey", err)
		}
	}
	ok := fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"%s"}]}`, long)
	if err := CheckJSONKeys([]byte(ok), keyCheckFoldPaths...); err != nil {
		t.Errorf("valid body: %v", err)
	}
}

func BenchmarkCheckJSONKeys(b *testing.B) {
	content := strings.Repeat("lorem ipsum dolor sit amet ", 256*1024/27)
	body, _ := json.Marshal(map[string]any{"model": "claude-sonnet-4-5", "max_tokens": 10, "messages": []map[string]any{{"role": "user", "content": content}}})
	for _, impl := range []struct {
		name string
		fn   func([]byte, ...string) error
	}{{"scanner", CheckJSONKeys}, {"decoder", checkJSONKeysDecoder}} {
		b.Run(impl.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				if err := impl.fn(body, keyCheckFoldPaths...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestCheckJSONKeys_DoesNotDecodeValues: the check reads keys only, so an
// agent-sized body costs a few small allocations, not a decode of its
// values (the json.Decoder version made 96 and copied every string; 53e.50).
func TestCheckJSONKeys_DoesNotDecodeValues(t *testing.T) {
	content := strings.Repeat("lorem ipsum dolor sit amet ", 256*1024/27)
	body, err := json.Marshal(map[string]any{"model": "claude-sonnet-4-5", "max_tokens": 10, "messages": []map[string]any{{"role": "user", "content": content}}})
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(10, func() { _ = CheckJSONKeys(body, keyCheckFoldPaths...) })
	if allocs > 40 {
		t.Errorf("CheckJSONKeys of a %d-byte body made %v allocations, want at most 40", len(body), allocs)
	}
}

// TestCheckJSONKeys_DeepBodiesStayBounded: memory does not grow with the
// square of the nesting, or with the keys' length past the foldAt paths, and
// a body nested deeper than encoding/json's limit is refused (53e.58).
func TestCheckJSONKeys_DeepBodiesStayBounded(t *testing.T) {
	key := strings.Repeat("k", 100)
	for name, b := range map[string][]byte{
		"arrays":            []byte(strings.Repeat("[", maxNestingDepth) + "1" + strings.Repeat("]", maxNestingDepth)),
		"objects":           []byte(strings.Repeat(`{"a":`, maxNestingDepth) + "1" + strings.Repeat("}", maxNestingDepth)),
		"objects long keys": []byte(strings.Repeat(`{"`+key+`":`, maxNestingDepth) + "1" + strings.Repeat("}", maxNestingDepth)),
		"messages nest":     []byte(`{"messages":` + strings.Repeat(`[{"content":`, maxNestingDepth/2-1) + "1" + strings.Repeat("}]", maxNestingDepth/2-1) + "}"),
	} {
		for _, impl := range []struct {
			name string
			fn   func([]byte, ...string) error
		}{{"scanner", CheckJSONKeys}, {"decoder", checkJSONKeysDecoder}} {
			if mb := allocatedMB(func() {
				if err := impl.fn(b, keyCheckFoldPaths...); err != nil {
					t.Errorf("%s %s: %v", name, impl.name, err)
				}
			}); mb > 32 {
				t.Errorf("%s %s: %d MB allocated for a %d-byte body, want at most 32", name, impl.name, mb, len(b))
			}
			extra := maxNestingDepth + 1 - nestingDepth(b)
			deeper := []byte(strings.Repeat("[", extra) + string(b) + strings.Repeat("]", extra))
			if mb := allocatedMB(func() {
				if err := impl.fn(deeper, keyCheckFoldPaths...); err == nil || errors.Is(err, ErrDuplicateKey) {
					t.Errorf("%s %s nested one deeper than the limit: err = %v, want a syntax error", name, impl.name, err)
				}
			}); mb > 32 {
				t.Errorf("%s %s one deeper: %d MB allocated, want at most 32", name, impl.name, mb)
			}
		}
		if mb := allocatedMB(func() { _, _ = FindDuplicateKey(b) }); mb > 32 {
			t.Errorf("%s FindDuplicateKey: %d MB allocated, want at most 32", name, mb)
		}
	}
}

func allocatedMB(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) >> 20
}
