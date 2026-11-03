package segments

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// checkKeysTree is CheckKeys as it was before 53e.52, on parseTree: the
// reference the scanner-based CheckKeys is held to.
func checkKeysTree(body []byte) error {
	_, err := parseTree(body)
	return err
}

var checkKeysCorpus = []string{
	``, ` `, `{}`, `[]`, `"x"`, `1`, `null`, `nope`, `{`, `{"a":1,}`, `{"a"}`, `{"a":1} {}`,
	`{"model":"m","messages":[{"role":"user","content":"hi"}]}`,
	// Repeated keys, at the root and nested, named (schema fields) or masked.
	`{"model":"ok","model":"evil"}`,
	`{"model":"ok","model":"evil"}`,
	`{"messages":[{"role":"user","content":"a"},{"role":"user","content":"b","content":"c"}]}`,
	`{"messages":[{"role":"user","content":[{"type":"text","text":"a","text":"b"}]}]}`,
	`{"tools":[{"input_schema":{"properties":{"secret_key":{},"secret_key":{}}}}]}`,
	`{"a/b":{"c~d":[{},{"x":1,"x":2}]}}`,
	`[[{"a":1}],[{"b":1,"b":2}]]`,
	`{"":1,"":2}`,
	// Case variants are not CheckKeys' business.
	`{"model":"ok","Model":"evil"}`,
	// A repeated key followed by a syntax error is not JSON, as before.
	`{"model":"ok","model":"evil"`,
	`{"model":"ok","model":"evil",}`,
	`{"model":"ok","model":"evil"} x`,
	`{"messages":[{"content":"a","content":"b"}],"x":tru}`,
	// Two repeated keys: the first in document order is reported.
	`{"a":{"stream":1,"stream":2},"model":1,"model":2}`,
	`{"model":1,"a":{"stream":1,"stream":2},"model":2}`,
	// Key decoding: invalid UTF-8 and lone surrogates as U+FFFD.
	"{\"a\xff\":1,\"a\\ufffd\":2}",
	`{"a\ud800":1,"a�":2}`,
	"{\"a\xff\":1,\"a\xfe\":2}",
	`{"a":"\u00"}`, "{\"a\":\"\x01\"}",
}

// TestCheckKeys_MatchesParseTree: the scanner-based CheckKeys returns what
// the parseTree version did, error text included (53e.52).
func TestCheckKeys_MatchesParseTree(t *testing.T) {
	long := strings.Repeat("lorem ipsum \\u00e9 ", 20000)
	corpus := append([]string{
		fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"%s"}],"model":"evil"}`, long),
		fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"%s"}],"model":"evil"`, long),
	}, checkKeysCorpus...)
	// encoding/json's nesting limit is 10000: deeper is not JSON to
	// parseTree, a repeated key in it included (53e.55).
	for _, depth := range []int{9999, 10000, 10001} {
		corpus = append(corpus,
			strings.Repeat("[", depth)+"1"+strings.Repeat("]", depth),
			strings.Repeat("[", depth-1)+`{"a":1,"a":2}`+strings.Repeat("]", depth-1),
			strings.Repeat(`{"a":`, depth)+"1"+strings.Repeat("}", depth),
		)
	}
	for _, b := range corpus {
		got, want := CheckKeys([]byte(b)), checkKeysTree([]byte(b))
		if !sameCheckKeysVerdict(got, want) {
			t.Errorf("%.80q: CheckKeys %v, parseTree %v", b, got, want)
		}
	}
	err := CheckKeys([]byte(`{"model":"ok","model":"evil"`))
	if !errors.Is(err, ErrUnrecognisedBody) || errors.Is(err, ErrDuplicateKey) {
		t.Errorf("a repeated key then a syntax error: %v, want ErrUnrecognisedBody only", err)
	}
}

func sameCheckKeysVerdict(got, want error) bool {
	if (got == nil) != (want == nil) {
		return false
	}
	if got == nil {
		return true
	}
	if errors.Is(got, ErrDuplicateKey) != errors.Is(want, ErrDuplicateKey) ||
		errors.Is(got, ErrUnrecognisedBody) != errors.Is(want, ErrUnrecognisedBody) {
		return false
	}
	return got.Error() == want.Error()
}

func FuzzCheckKeys(f *testing.F) {
	for _, b := range checkKeysCorpus {
		f.Add([]byte(b))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if got, want := CheckKeys(b), checkKeysTree(b); !sameCheckKeysVerdict(got, want) {
			t.Fatalf("%q: CheckKeys %v, parseTree %v", b, got, want)
		}
	})
}

func BenchmarkCheckKeys(b *testing.B) {
	body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4-5","max_tokens":10,"messages":[{"role":"user","content":"%s"}]}`,
		strings.Repeat("lorem ipsum dolor sit amet ", 256*1024/27)))
	for _, impl := range []struct {
		name string
		fn   func([]byte) error
	}{{"scanner", CheckKeys}, {"parseTree", checkKeysTree}} {
		b.Run(impl.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				if err := impl.fn(body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
