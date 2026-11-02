package nativeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// sameVerdict reports whether the scanner and the decoder agree on b: both
// accept it, or both refuse it with the same error.
func sameVerdict(b []byte, foldAt ...string) (bool, error, error) {
	got, want := CheckJSONKeys(b, foldAt...), checkJSONKeysDecoder(b, foldAt...)
	if (got == nil) != (want == nil) {
		return false, got, want
	}
	if got == nil {
		return true, got, want
	}
	// The same text too: a malformed body's 400 keeps encoding/json's
	// wording (53e.56), a duplicate names the same key.
	return errors.Is(got, ErrDuplicateKey) == errors.Is(want, ErrDuplicateKey) && got.Error() == want.Error(), got, want
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
	corpus := keyCheckCorpus
	// The json.Decoder version had no nesting limit (53e.55): neither has
	// CheckJSONKeys.
	for _, depth := range []int{10000, 10001, 20000} {
		corpus = append(corpus,
			strings.Repeat("[", depth)+"1"+strings.Repeat("]", depth),
			strings.Repeat("[", depth-1)+`{"a":1,"a":2}`+strings.Repeat("]", depth-1))
	}
	// Malformed bodies keep encoding/json's wording (53e.56).
	corpus = append(corpus, `{"a":"\x"}`, `{"a":01}`, `{"a":tru}`, `{`, `{"a":"unterminated`, "\x00", `{"a":1]`, `[1}`)
	for _, b := range corpus {
		for _, foldAt := range append(keyCheckFoldSets, []string{"a", "a[]", "a[].b.c[]"}) {
			if ok, got, want := sameVerdict([]byte(b), foldAt...); !ok {
				t.Errorf("%.80q foldAt %q: scanner %v, decoder %v", b, foldAt, got, want)
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
