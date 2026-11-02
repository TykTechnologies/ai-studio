package nativeapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// ErrDuplicateKey is a JSON object with two keys the gateway and a vendor may
// read differently.
var ErrDuplicateKey = errors.New("duplicate key")

// CheckJSONKeys reports whether b is one JSON value without duplicate keys
// the gateway and a vendor might read differently:
//
//   - Exact duplicates are refused in every object, at any depth. JSON
//     parsers disagree on which one wins (Go's encoding/json, behind the
//     gateway's model checks and filter extractors, takes the last; a vendor
//     may take the first), so {"model":"ok","model":"evil"}, or a message
//     with two "content" keys, could show the gateway one value and the
//     vendor another.
//   - Keys equal under Unicode simple case folding are refused only in the
//     objects at foldAt: the ones the gateway decodes into Go structs, whose
//     fields encoding/json matches case-insensitively ({"messages":[x],
//     "Messages":[y]} shows a struct y and the vendor x). Elsewhere, such as
//     a tool's JSON Schema properties or a tool call's arguments, "id" and
//     "ID" are different keys to everyone.
//
// A foldAt path is a dotted key path from the root ("" is the root object);
// a "[]" suffix names the elements of an array: "messages[]" is every
// message, "messages[].content[]" every content block.
func CheckJSONKeys(b []byte, foldAt ...string) error {
	// A byte scanner rather than json.Decoder.Token: this runs on every
	// native Messages request, before the model checks decode the same body,
	// and v2.2.2 had no such pass (53e.50). It accepts exactly the JSON
	// encoding/json accepts and compares keys as encoding/json decodes them;
	// TestCheckJSONKeys_MatchesDecoder and FuzzCheckJSONKeys hold it to the
	// json.Decoder version it replaced.
	s := keyScanner{b: b}
	for _, p := range foldAt {
		if s.fold == nil {
			s.fold = make(map[string]bool, len(foldAt))
		}
		s.fold[p] = true
	}
	s.skipSpace()
	if s.i == len(b) {
		return errors.New("empty JSON body")
	}
	err := s.value()
	if err == nil {
		s.skipSpace()
		if s.i != len(b) {
			err = errors.New("trailing data after JSON value")
		}
	}
	if err != nil && !errors.Is(err, ErrDuplicateKey) {
		// Malformed: report it in encoding/json's words, as before (53e.56).
		// Only a body the client must fix pays for the second read; should
		// the decoder ever accept it, the scanner's refusal stands.
		if derr := checkJSONKeysDecoder(b, foldAt...); derr != nil {
			return derr
		}
	}
	return err
}

// DuplicateKey is the first exact duplicate key FindDuplicateKey found: Key
// as encoding/json decodes it, in the object at Path.
type DuplicateKey struct {
	Key  string
	Path []PathElem
}

// PathElem is one step from the root to a value: an object key, or (when
// IsIndex) an array index.
type PathElem struct {
	Key     string
	Index   int
	IsIndex bool
}

// FindDuplicateKey reads all of b, as CheckJSONKeys does with no foldAt.
// It returns a syntax error when b is not one JSON value json.Valid would
// accept (nesting deeper than encoding/json's 10000 included), even after a
// duplicate (a caller that ignores bodies it cannot read keeps ignoring
// them); otherwise the first exact duplicate key in document order, or nil.
func FindDuplicateKey(b []byte) (*DuplicateKey, error) {
	s := keyScanner{b: b, all: true, maxDepth: maxNestingDepth}
	s.skipSpace()
	if s.i == len(b) {
		return nil, errors.New("empty JSON body")
	}
	if err := s.value(); err != nil {
		return nil, err
	}
	s.skipSpace()
	if s.i != len(b) {
		return nil, errors.New("trailing data after JSON value")
	}
	return s.dup, nil
}

// path is the path to the innermost open object.
func (s *keyScanner) path() []PathElem {
	var p []PathElem
	for _, f := range s.stack[:len(s.stack)-1] {
		if f.isArray {
			p = append(p, PathElem{Index: f.index, IsIndex: true})
		} else {
			p = append(p, PathElem{Key: f.key})
		}
	}
	return p
}

// keyFrame is one open object or array.
type keyFrame struct {
	path    string
	isArray bool
	exact   map[string]bool
	folded  map[string]bool // nil: case variants are not checked here
	key     string          // the key whose value comes next
	index   int             // the element being read, in an array
}

type keyScanner struct {
	b     []byte
	i     int
	fold  map[string]bool
	stack []*keyFrame
	// all keeps scanning past the first duplicate, which dup records, so
	// a body that is not valid JSON is reported as such (FindDuplicateKey).
	all bool
	dup *DuplicateKey
	// maxDepth, when not 0, is the deepest nesting accepted: json.Valid's
	// limit for FindDuplicateKey, whose callers decided validity with it
	// (53e.55). CheckJSONKeys, like the json.Decoder version, has none.
	maxDepth int
}

// maxNestingDepth is encoding/json's: json.Valid refuses deeper input.
const maxNestingDepth = 10000

// push opens an object or array.
func (s *keyScanner) push(f *keyFrame) error {
	s.stack = append(s.stack, f)
	if s.maxDepth > 0 && len(s.stack) > s.maxDepth {
		return s.syntaxError("exceeded max depth")
	}
	return nil
}

func (s *keyScanner) syntaxError(what string) error {
	return fmt.Errorf("invalid JSON at offset %d: %s", s.i, what)
}

func (s *keyScanner) skipSpace() {
	for s.i < len(s.b) {
		switch s.b[s.i] {
		case ' ', '\t', '\n', '\r':
			s.i++
		default:
			return
		}
	}
}

// childPath is the foldAt path of the value about to be read.
func (s *keyScanner) childPath() string {
	if len(s.stack) == 0 {
		return ""
	}
	f := s.stack[len(s.stack)-1]
	if f.isArray {
		return f.path + "[]"
	}
	if f.path == "" {
		return f.key
	}
	return f.path + "." + f.key
}

// value reads one JSON value and everything nested in it. Nesting is kept on
// s.stack, not the Go stack, so deep input cannot overflow it.
func (s *keyScanner) value() error {
	for {
		s.skipSpace()
		if s.i == len(s.b) {
			return s.syntaxError("unexpected end of input")
		}
		switch c := s.b[s.i]; {
		case c == '{':
			s.i++
			p := s.childPath()
			f := &keyFrame{path: p, exact: map[string]bool{}}
			if s.fold[p] {
				f.folded = map[string]bool{}
			}
			if err := s.push(f); err != nil {
				return err
			}
			s.skipSpace()
			if s.i < len(s.b) && s.b[s.i] == '}' {
				s.i++
				s.stack = s.stack[:len(s.stack)-1]
				break
			}
			if err := s.member(f); err != nil {
				return err
			}
			continue // the member's value
		case c == '[':
			s.i++
			if err := s.push(&keyFrame{path: s.childPath(), isArray: true}); err != nil {
				return err
			}
			s.skipSpace()
			if s.i < len(s.b) && s.b[s.i] == ']' {
				s.i++
				s.stack = s.stack[:len(s.stack)-1]
				break
			}
			continue // the first element
		case c == '"':
			if _, err := s.str(false); err != nil {
				return err
			}
		case c == '-' || (c >= '0' && c <= '9'):
			if err := s.number(); err != nil {
				return err
			}
		case c == 't':
			if err := s.literal("true"); err != nil {
				return err
			}
		case c == 'f':
			if err := s.literal("false"); err != nil {
				return err
			}
		case c == 'n':
			if err := s.literal("null"); err != nil {
				return err
			}
		default:
			return s.syntaxError(fmt.Sprintf("invalid character %q looking for a value", c))
		}
		// A value ended: close what it ends, then expect the next one.
		for {
			if len(s.stack) == 0 {
				return nil
			}
			f := s.stack[len(s.stack)-1]
			s.skipSpace()
			if s.i == len(s.b) {
				return s.syntaxError("unexpected end of input")
			}
			c := s.b[s.i]
			if f.isArray {
				if c == ']' {
					s.i++
					s.stack = s.stack[:len(s.stack)-1]
					continue
				}
				if c != ',' {
					return s.syntaxError("expected ',' or ']' after an array element")
				}
				s.i++
				f.index++
				break // the next element
			}
			if c == '}' {
				s.i++
				s.stack = s.stack[:len(s.stack)-1]
				continue
			}
			if c != ',' {
				return s.syntaxError("expected ',' or '}' after an object value")
			}
			s.i++
			s.skipSpace()
			if err := s.member(f); err != nil {
				return err
			}
			break // the member's value
		}
	}
}

// member reads an object key, checks it against the object's other keys,
// and reads the ':' after it.
func (s *keyScanner) member(f *keyFrame) error {
	if s.i == len(s.b) || s.b[s.i] != '"' {
		return s.syntaxError("expected an object key")
	}
	key, err := s.str(true)
	if err != nil {
		return err
	}
	if f.exact[key] {
		if !s.all {
			return fmt.Errorf("%w %q", ErrDuplicateKey, key)
		}
		if s.dup == nil {
			s.dup = &DuplicateKey{Key: key, Path: s.path()}
		}
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
	s.skipSpace()
	if s.i == len(s.b) || s.b[s.i] != ':' {
		return s.syntaxError("expected ':' after an object key")
	}
	s.i++
	return nil
}

// str reads a string at s.i and, for a key, returns it as encoding/json
// decodes it: for plain ASCII without escapes the bytes themselves, otherwise
// encoding/json's own decoding (invalid UTF-8 and lone surrogates become
// U+FFFD, as for every key it decodes). A value's text is not needed.
func (s *keyScanner) str(key bool) (string, error) {
	start := s.i
	s.i++ // opening quote
	plain := true
	for {
		// Skip the bulk of a long string fast: only '"', '\\' and control
		// characters need a look.
		j := s.i
		for j < len(s.b) && !strStop[s.b[j]] {
			j++
		}
		if key && plain {
			for _, c := range s.b[s.i:j] {
				if c >= 0x80 {
					plain = false
					break
				}
			}
		}
		s.i = j
		if s.i == len(s.b) {
			return "", s.syntaxError("unterminated string")
		}
		switch c := s.b[s.i]; {
		case c == '"':
			s.i++
			raw := s.b[start:s.i]
			if !key {
				return "", nil
			}
			if plain {
				return string(raw[1 : len(raw)-1]), nil
			}
			var out string
			if err := json.Unmarshal(raw, &out); err != nil {
				return "", err
			}
			return out, nil
		case c == '\\':
			plain = false
			if s.i+1 == len(s.b) {
				return "", s.syntaxError("unterminated string")
			}
			switch s.b[s.i+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				s.i += 2
			case 'u':
				if s.i+6 > len(s.b) {
					return "", s.syntaxError("unterminated string")
				}
				for _, h := range s.b[s.i+2 : s.i+6] {
					if !isHex(h) {
						return "", s.syntaxError("invalid \\u escape")
					}
				}
				s.i += 6
			default:
				return "", s.syntaxError("invalid escape in string")
			}
		default:
			return "", s.syntaxError("control character in string")
		}
	}
}

// strStop marks the bytes a string scan must look at: '"', '\\' and the
// control characters JSON does not allow in a string.
var strStop = func() (t [256]bool) {
	for c := 0; c < 0x20; c++ {
		t[c] = true
	}
	t['"'], t['\\'] = true, true
	return t
}()

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// number reads -?(0|[1-9][0-9]*)(.[0-9]+)?([eE][+-]?[0-9]+)?.
func (s *keyScanner) number() error {
	digits := func() int {
		n := 0
		for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
			s.i++
			n++
		}
		return n
	}
	if s.b[s.i] == '-' {
		s.i++
	}
	switch {
	case s.i < len(s.b) && s.b[s.i] == '0':
		s.i++
	case digits() == 0:
		return s.syntaxError("invalid number")
	}
	if s.i < len(s.b) && s.b[s.i] == '.' {
		s.i++
		if digits() == 0 {
			return s.syntaxError("invalid number")
		}
	}
	if s.i < len(s.b) && (s.b[s.i] == 'e' || s.b[s.i] == 'E') {
		s.i++
		if s.i < len(s.b) && (s.b[s.i] == '+' || s.b[s.i] == '-') {
			s.i++
		}
		if digits() == 0 {
			return s.syntaxError("invalid number")
		}
	}
	return nil
}

func (s *keyScanner) literal(lit string) error {
	if !bytes.HasPrefix(s.b[s.i:], []byte(lit)) {
		return s.syntaxError("invalid literal")
	}
	s.i += len(lit)
	return nil
}

// checkJSONKeysDecoder is CheckJSONKeys as it was before 53e.50, on
// json.Decoder.Token. CheckJSONKeys runs it on a body the scanner found
// malformed, so the client's 400 keeps encoding/json's wording (53e.56);
// the tests hold the scanner's verdicts to it.
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

// FoldKey maps key to a canonical form under Unicode simple case folding, so
// FoldKey(a) == FoldKey(b) exactly when strings.EqualFold(a, b).
func FoldKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		min := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < min {
				min = f
			}
		}
		b.WriteRune(min)
	}
	return b.String()
}
