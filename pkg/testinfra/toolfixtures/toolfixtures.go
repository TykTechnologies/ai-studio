// Package toolfixtures holds vendor request bodies that carry tool use, with
// the segments a lossless segmenter is expected to find in them, for tests of
// tool-result guardrails (epic midsommar-clean-4uc). Every fixture plants an
// injection canary ("[canary:...]") only in tool output, and some plant the
// PII marker in a tool result, so a test can assert that a guardrail sees
// tool output, acts on the right segment, and leaves everything else in the
// body intact.
//
// The package is stdlib only so any package can import it from tests without
// an import cycle. See README.md for the manifest fields.
package toolfixtures

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

//go:embed fixtures/*.json fixtures/*.jsonl
var files embed.FS

// Markers planted in the fixtures.
const (
	// CanaryPrefix starts every injection canary. Each canary sentence also
	// matches the built-in injection.indirect_marker and
	// injection.instruction_override patterns.
	CanaryPrefix = "[canary:"
	// PIIEmail is the e-mail address planted in PII-flagged segments.
	PIIEmail = "jane.doe@example.com"
)

// Fixture statuses.
const (
	// StatusInScope fixtures are in the epic's format list.
	StatusInScope = "in_scope"
	// StatusOpenQuestion fixtures hold tool output the epic does not decide
	// about yet (server-side tool results inside an assistant turn).
	StatusOpenQuestion = "open_question"
	// StatusControl fixtures have no segmenter; the guardrail must still
	// classify the raw body.
	StatusControl = "control"
)

// Segment is one expected segment. Path is an RFC 6901 JSON pointer to the
// string leaf. ToolCallID is empty where the format has no call id (legacy
// function calling, Ollama, Gemini without ids).
type Segment struct {
	Role       string `json:"role"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Contains   string `json:"contains"`
	Injection  bool   `json:"injection,omitempty"`
	PII        bool   `json:"pii,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	// LastUser marks segments after the last assistant message: what
	// scope last_user selects once tool results count as user-side input.
	LastUser bool `json:"last_user,omitempty"`
	// Proposed marks an expectation the epic has not settled (see the
	// fixture's notes).
	Proposed bool   `json:"proposed,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// IsTool reports whether the segment is tool output.
func (s Segment) IsTool() bool { return s.Role == "tool" }

// Fixture is one request body and what a segmenter should find in it.
type Fixture struct {
	Name string `json:"name"`
	// File is the body's file name under fixtures/; default Name + ".json".
	// A .jsonl body (a batch input file) decodes as an array of its lines,
	// so pointers start with the line index.
	File        string    `json:"file,omitempty"`
	Format      string    `json:"format"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	Segments    []Segment `json:"segments"`
	// OptionalSegments are string leaves a segmenter may or may not report
	// (titles, URLs, empty outputs). They are never flagged.
	OptionalSegments []string `json:"optional_segments,omitempty"`
	// NotSegments are string leaves that must never be reported: binary
	// data, signatures, encrypted blobs, model-generated arguments.
	NotSegments []string `json:"not_segments,omitempty"`
	// Preserve are pointers whose values must be unchanged after a rewrite
	// that touches only flagged segments: ids, flags, images, unknown fields.
	Preserve []string `json:"preserve"`
	Notes    string   `json:"notes,omitempty"`

	// Body is the raw request body as stored, byte for byte.
	Body []byte `json:"-"`
}

// Select returns the expected segments that satisfy keep.
func (f Fixture) Select(keep func(Segment) bool) []Segment {
	var out []Segment
	for _, s := range f.Segments {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

// ToolSegments returns the expected tool-output segments.
func (f Fixture) ToolSegments() []Segment { return f.Select(Segment.IsTool) }

// InjectionSegments returns the segments carrying the canary.
func (f Fixture) InjectionSegments() []Segment {
	return f.Select(func(s Segment) bool { return s.Injection })
}

// PIISegments returns the segments carrying PIIEmail.
func (f Fixture) PIISegments() []Segment {
	return f.Select(func(s Segment) bool { return s.PII })
}

// LastUserSegments returns what scope last_user should select.
func (f Fixture) LastUserSegments() []Segment {
	return f.Select(func(s Segment) bool { return s.LastUser })
}

// AllUserSegments returns what scope all_user should select: every user
// text segment and every tool segment.
func (f Fixture) AllUserSegments() []Segment {
	return f.Select(func(s Segment) bool { return s.Role == "user" || s.Role == "tool" })
}

// Paths returns the paths of segs.
func Paths(segs []Segment) []string {
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		out = append(out, s.Path)
	}
	return out
}

var (
	loadOnce sync.Once
	loaded   []Fixture
	loadErr  error
)

func load() ([]Fixture, error) {
	loadOnce.Do(func() {
		raw, err := files.ReadFile("fixtures/manifest.json")
		if err != nil {
			loadErr = err
			return
		}
		var m struct {
			Fixtures []Fixture `json:"fixtures"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			loadErr = fmt.Errorf("manifest: %w", err)
			return
		}
		for i := range m.Fixtures {
			file := m.Fixtures[i].File
			if file == "" {
				file = m.Fixtures[i].Name + ".json"
			}
			body, err := files.ReadFile("fixtures/" + file)
			if err != nil {
				loadErr = fmt.Errorf("fixture %s: %w", m.Fixtures[i].Name, err)
				return
			}
			m.Fixtures[i].Body = body
		}
		loaded = m.Fixtures
	})
	return loaded, loadErr
}

// All returns every fixture. Each call returns fresh copies of the bodies,
// so a test may modify them.
func All() []Fixture {
	fs, err := load()
	if err != nil {
		panic(err)
	}
	out := make([]Fixture, len(fs))
	for i, f := range fs {
		f.Body = bytes.Clone(f.Body)
		out[i] = f
	}
	return out
}

// Get returns the named fixture; it panics on an unknown name.
func Get(name string) Fixture {
	for _, f := range All() {
		if f.Name == name {
			return f
		}
	}
	panic("toolfixtures: unknown fixture " + name)
}

// ByFormat returns the fixtures in a format ("anthropic", "openai",
// "google_ai", "vertex", "ollama", "openai_responses", "anthropic_batch",
// "bedrock").
func ByFormat(formats ...string) []Fixture {
	var out []Fixture
	for _, f := range All() {
		for _, want := range formats {
			if f.Format == want {
				out = append(out, f)
			}
		}
	}
	return out
}

// Decode parses a JSON body keeping numbers exact. A body holding several
// JSON values (JSONL) decodes as an array of them.
func Decode(body []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var vals []any
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	switch len(vals) {
	case 0:
		return nil, io.ErrUnexpectedEOF
	case 1:
		return vals[0], nil
	}
	return vals, nil
}

// Lookup resolves an RFC 6901 pointer in a decoded document.
func Lookup(doc any, pointer string) (any, bool) {
	if pointer == "" {
		return doc, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	cur := doc
	for _, tok := range strings.Split(pointer[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[tok]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// LookupString resolves pointer in body and returns the string there.
func LookupString(body []byte, pointer string) (string, bool) {
	doc, err := Decode(body)
	if err != nil {
		return "", false
	}
	v, ok := Lookup(doc, pointer)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// DiffPaths returns the pointers at which two JSON documents differ, sorted.
// Objects are compared key by key (a key present on one side only is a
// difference at that key), arrays index by index (a length change is a
// difference at the array), numbers by exact value (so 1.50 equals 1.5 but
// a large integer that lost precision through float64 does not), everything
// else by value. Key order and whitespace are not differences.
func DiffPaths(before, after []byte) ([]string, error) {
	a, err := Decode(before)
	if err != nil {
		return nil, fmt.Errorf("before: %w", err)
	}
	b, err := Decode(after)
	if err != nil {
		return nil, fmt.Errorf("after: %w", err)
	}
	var out []string
	diff("", a, b, &out)
	sort.Strings(out)
	return out, nil
}

func escapeToken(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func diff(ptr string, a, b any, out *[]string) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, ptr)
			return
		}
		for k, x := range av {
			y, ok := bv[k]
			if !ok {
				*out = append(*out, ptr+"/"+escapeToken(k))
				continue
			}
			diff(ptr+"/"+escapeToken(k), x, y, out)
		}
		for k := range bv {
			if _, ok := av[k]; !ok {
				*out = append(*out, ptr+"/"+escapeToken(k))
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			*out = append(*out, ptr)
			return
		}
		for i := range av {
			diff(ptr+"/"+strconv.Itoa(i), av[i], bv[i], out)
		}
	case json.Number:
		bv, ok := b.(json.Number)
		if !ok || !numbersEqual(av, bv) {
			*out = append(*out, ptr)
		}
	default:
		if a != b {
			*out = append(*out, ptr)
		}
	}
}

func numbersEqual(a, b json.Number) bool {
	if a == b {
		return true
	}
	x, ok1 := new(big.Rat).SetString(string(a))
	y, ok2 := new(big.Rat).SetString(string(b))
	return ok1 && ok2 && x.Cmp(y) == 0
}

// AssertOnlyChanged fails t unless after differs from before at most at the
// allowed pointers, and is still valid JSON.
func AssertOnlyChanged(t testing.TB, before, after []byte, allowed ...string) {
	t.Helper()
	diffs, err := DiffPaths(before, after)
	if err != nil {
		t.Fatalf("rewritten body is not valid JSON: %v\n%s", err, after)
	}
	ok := map[string]bool{}
	for _, p := range allowed {
		ok[p] = true
	}
	for _, d := range diffs {
		if !ok[d] {
			t.Errorf("body changed at %s, which is not a rewritten segment (allowed: %v)", d, allowed)
		}
	}
}

// AssertPreserved fails t unless every Preserve pointer of f resolves to the
// same value in after as in f.Body.
func AssertPreserved(t testing.TB, f Fixture, after []byte) {
	t.Helper()
	a, err := Decode(f.Body)
	if err != nil {
		t.Fatalf("%s: fixture is not JSON: %v", f.Name, err)
	}
	b, err := Decode(after)
	if err != nil {
		t.Fatalf("%s: rewritten body is not valid JSON: %v\n%s", f.Name, err, after)
	}
	for _, p := range f.Preserve {
		x, ok := Lookup(a, p)
		if !ok {
			t.Fatalf("%s: preserve pointer %s does not resolve in the fixture", f.Name, p)
		}
		y, ok := Lookup(b, p)
		if !ok {
			t.Errorf("%s: %s is gone after the rewrite", f.Name, p)
			continue
		}
		var d []string
		diff(p, x, y, &d)
		if len(d) > 0 {
			t.Errorf("%s: %s changed after the rewrite (at %v)", f.Name, p, d)
		}
	}
}
