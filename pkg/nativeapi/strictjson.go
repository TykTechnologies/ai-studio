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
