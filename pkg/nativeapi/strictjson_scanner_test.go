package nativeapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// checkJSONKeysDecoder is CheckJSONKeys as it was before 53e.50, on
// json.Decoder.Token: the reference the byte scanner is held to.
func checkJSONKeysDecoder(b []byte, foldAt ...string) error {
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

// sameVerdict reports whether the scanner and the decoder agree on b: both
// accept it, or both refuse it, and a duplicate key is refused as one by both.
func sameVerdict(b []byte, foldAt ...string) (bool, error, error) {
	got, want := CheckJSONKeys(b, foldAt...), checkJSONKeysDecoder(b, foldAt...)
	if (got == nil) != (want == nil) {
		return false, got, want
	}
	return errors.Is(got, ErrDuplicateKey) == errors.Is(want, ErrDuplicateKey), got, want
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
	for _, b := range keyCheckCorpus {
		for _, foldAt := range append(keyCheckFoldSets, []string{"a", "a[]", "a[].b.c[]"}) {
			if ok, got, want := sameVerdict([]byte(b), foldAt...); !ok {
				t.Errorf("%q foldAt %q: scanner %v, decoder %v", b, foldAt, got, want)
			}
		}
	}
}

func FuzzCheckJSONKeys(f *testing.F) {
	for _, b := range keyCheckCorpus {
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
