package segments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// A tree is a request body parsed just far enough to find string leaves and
// their byte spans, so a rewrite can splice new text into the original bytes
// and leave every other byte (key order, number formatting, whitespace,
// unknown fields) exactly as the client sent it.

type nodeKind uint8

const (
	nodeObject nodeKind = iota
	nodeArray
	nodeString
	nodeScalar // number, true, false, null
)

type node struct {
	kind       nodeKind
	start, end int // byte span of the whole value in the body
	keys       []string
	vals       []*node // object member values (parallel to keys) or array elements
	// str is a string node's decoded value once text has read it, so a Doc
	// read several ways decodes each string once. A tree is read by one
	// goroutine at a time.
	str     string
	decoded bool
}

// get returns the object member named key, or nil.
func (n *node) get(key string) *node {
	if n == nil || n.kind != nodeObject {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

// elems returns the elements of an array node, or nil for anything else.
func (n *node) elems() []*node {
	if n == nil || n.kind != nodeArray {
		return nil
	}
	return n.vals
}

// is reports whether n is a node of kind k; a missing node is no kind.
func (n *node) is(k nodeKind) bool {
	return n != nil && n.kind == k
}

// parseTree parses body into a tree. Bodies that are not valid JSON are
// ErrUnrecognisedBody; an object with a repeated key is ErrDuplicateKey:
// parsers disagree on which copy of a repeated key wins, so a guardrail could
// scan one copy while the vendor reads the other.
func parseTree(body []byte) (*node, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: not valid JSON", ErrUnrecognisedBody)
	}
	p := &treeParser{b: body}
	p.ws()
	return p.value()
}

// treeParser walks a body json.Valid has accepted, so it checks structure
// only where it records something.
type treeParser struct {
	b []byte
	i int
	// at is the path to the value being parsed, for errors.
	at []pathToken
}

// pathToken is an object key (isKey) or an array index.
type pathToken struct {
	key   string
	index int
	isKey bool
}

// pointer renders the current path as a JSON Pointer.
func (p *treeParser) pointer() string {
	s := ""
	for _, t := range p.at {
		if t.isKey {
			s = pointer(s, t.key)
		} else {
			s = pointerIndex(s, t.index)
		}
	}
	return s
}

func (p *treeParser) ws() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

func (p *treeParser) value() (*node, error) {
	switch p.b[p.i] {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		n := &node{kind: nodeString, start: p.i}
		p.skipString()
		n.end = p.i
		return n, nil
	default:
		n := &node{kind: nodeScalar, start: p.i}
		for p.i < len(p.b) {
			c := p.b[p.i]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
				break
			}
			p.i++
		}
		n.end = p.i
		return n, nil
	}
}

// skipString moves past the string starting at p.i.
func (p *treeParser) skipString() {
	p.i++ // opening quote
	for p.b[p.i] != '"' {
		if p.b[p.i] == '\\' {
			p.i++
		}
		p.i++
	}
	p.i++ // closing quote
}

// smallObject is the member count up to which repeated keys are found by a
// linear scan; larger objects use a set.
const smallObject = 16

func (p *treeParser) object() (*node, error) {
	n := &node{kind: nodeObject, start: p.i}
	p.i++
	p.ws()
	var seen map[string]struct{}
	for p.b[p.i] != '}' {
		ks := p.i
		p.skipString()
		key := decodeString(p.b[ks:p.i])
		if n.has(key, &seen) {
			return nil, fmt.Errorf("%w: repeated key%s at %s", ErrDuplicateKey, keyName(key), maskPath(p.pointer()))
		}
		p.ws()
		p.i++ // ':'
		p.ws()
		p.at = append(p.at, pathToken{key: key, isKey: true})
		v, err := p.value()
		p.at = p.at[:len(p.at)-1]
		if err != nil {
			return nil, err
		}
		n.keys = append(n.keys, key)
		n.vals = append(n.vals, v)
		p.ws()
		if p.b[p.i] == ',' {
			p.i++
			p.ws()
		}
	}
	p.i++
	n.end = p.i
	return n, nil
}

// has reports whether key is already a member of the object being built,
// switching to a set once the object outgrows a linear scan.
func (n *node) has(key string, seen *map[string]struct{}) bool {
	if *seen == nil && len(n.keys) < smallObject {
		for _, k := range n.keys {
			if k == key {
				return true
			}
		}
		return false
	}
	if *seen == nil {
		*seen = make(map[string]struct{}, len(n.keys)*2)
		for _, k := range n.keys {
			(*seen)[k] = struct{}{}
		}
	}
	if _, dup := (*seen)[key]; dup {
		return true
	}
	(*seen)[key] = struct{}{}
	return false
}

func (p *treeParser) array() (*node, error) {
	n := &node{kind: nodeArray, start: p.i}
	p.i++
	p.ws()
	for p.b[p.i] != ']' {
		p.at = append(p.at, pathToken{index: len(n.vals)})
		v, err := p.value()
		p.at = p.at[:len(p.at)-1]
		if err != nil {
			return nil, err
		}
		n.vals = append(n.vals, v)
		p.ws()
		if p.b[p.i] == ',' {
			p.i++
			p.ws()
		}
	}
	p.i++
	n.end = p.i
	return n, nil
}

// decodeString decodes a raw JSON string token (quotes included) the way
// encoding/json does, invalid UTF-8 becoming U+FFFD.
func decodeString(raw []byte) string {
	inner := raw[1 : len(raw)-1]
	if bytes.IndexByte(inner, '\\') < 0 && utf8.Valid(inner) {
		return string(inner)
	}
	return unquote(inner)
}

// unquote decodes the inside of a string token json.Valid accepted exactly
// as encoding/json does, without validating it again (that cost a request
// body's whole size once more per read): escapes decoded, a surrogate
// escape that does not pair and each byte of invalid UTF-8 becoming U+FFFD.
func unquote(s []byte) string {
	b := make([]byte, 0, len(s))
	for r := 0; r < len(s); {
		c := s[r]
		switch {
		case c == '\\' && r+1 < len(s):
			switch e := s[r+1]; e {
			case 'b':
				b = append(b, '\b')
			case 'f':
				b = append(b, '\f')
			case 'n':
				b = append(b, '\n')
			case 'r':
				b = append(b, '\r')
			case 't':
				b = append(b, '\t')
			case 'u':
				rr := getu4(s[r:])
				r += 6
				if utf16.IsSurrogate(rr) {
					if dec := utf16.DecodeRune(rr, getu4(s[r:])); dec != unicode.ReplacementChar {
						b = utf8.AppendRune(b, dec)
						r += 6
						continue
					}
					rr = unicode.ReplacementChar
				}
				b = utf8.AppendRune(b, rr)
				continue
			default: // '"', '\\' and '/', the only other escapes json.Valid allows
				b = append(b, e)
			}
			r += 2
		case c < utf8.RuneSelf:
			b = append(b, c)
			r++
		default:
			rr, size := utf8.DecodeRune(s[r:])
			if rr == utf8.RuneError && size == 1 {
				b = utf8.AppendRune(b, utf8.RuneError)
			} else {
				b = append(b, s[r:r+size]...)
			}
			r += size
		}
	}
	return string(b)
}

// getu4 reads the \uXXXX escape at the start of s, or returns -1.
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}

// text returns the decoded value of a string node.
func (n *node) text(body []byte) (string, bool) {
	if n == nil || n.kind != nodeString {
		return "", false
	}
	if !n.decoded {
		n.str, n.decoded = decodeString(body[n.start:n.end]), true
	}
	return n.str, true
}

// encodeString is the JSON string token for s. HTML characters are kept as
// they are so a rewritten leaf reads like the rest of the body.
func encodeString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// pointer appends one reference token to a JSON Pointer (RFC 6901).
func pointer(parent string, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return parent + "/" + token
}

func pointerIndex(parent string, i int) string {
	return parent + "/" + strconv.Itoa(i)
}

// resolve finds the node a JSON Pointer names, or nil.
func (n *node) resolve(ptr string) *node {
	if ptr == "" {
		return n
	}
	if ptr[0] != '/' {
		return nil
	}
	cur := n
	for _, tok := range strings.Split(ptr[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch cur.kind {
		case nodeObject:
			cur = cur.get(tok)
		case nodeArray:
			i, ok := arrayIndex(tok)
			if !ok || i >= len(cur.vals) {
				return nil
			}
			cur = cur.vals[i]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

// arrayIndex parses an RFC 6901 array index: decimal digits, no sign and no
// leading zero.
func arrayIndex(tok string) (int, bool) {
	if tok == "" || (len(tok) > 1 && tok[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return 0, false
		}
	}
	i, err := strconv.Atoi(tok)
	return i, err == nil
}

// Apply returns body with each edit made: the string leaf at the edit's
// JSON Pointer replaced by New, or the node there by Value. Every other byte
// of the body is kept, so tool calls, ids, images, cache_control and unknown
// fields reach the vendor unchanged. An edit whose pointer names no string
// leaf, whose leaf no longer reads Old, or that overlaps another is an
// error, and nothing is rewritten, so a rewrite never lands on text other
// than the text that was classified. With no edits the body is returned as
// it is.
func Apply(body []byte, edits []Edit) ([]byte, error) {
	if len(edits) == 0 {
		return body, nil
	}
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	out, _, err := applyTree(body, root, edits)
	return out, err
}

// applyTree is Apply on a body already parsed into root. leavesOnly reports
// that every edit replaced a string leaf with a string, so the result is
// valid JSON with the body's keys and structure.
func applyTree(body []byte, root *node, edits []Edit) (out []byte, leavesOnly bool, err error) {
	leavesOnly = true
	type splice struct {
		start, end int
		with       []byte
	}
	splices := make([]splice, 0, len(edits))
	for _, e := range edits {
		target := root.resolve(e.Path)
		if e.Value != nil {
			if target == nil || !json.Valid(e.Value) {
				return nil, false, fmt.Errorf("segments: cannot replace %q with the given value", e.Path)
			}
			leavesOnly = false
			splices = append(splices, splice{start: target.start, end: target.end, with: e.Value})
			continue
		}
		if target == nil || target.kind != nodeString {
			return nil, false, fmt.Errorf("segments: %q is not a string in the body", e.Path)
		}
		if old, _ := target.text(body); old != e.Old {
			return nil, false, fmt.Errorf("segments: %q no longer holds the text that was classified", e.Path)
		}
		splices = append(splices, splice{start: target.start, end: target.end, with: encodeString(e.New)})
	}
	sort.Slice(splices, func(i, j int) bool { return splices[i].start < splices[j].start })
	for i := 1; i < len(splices); i++ {
		if splices[i].start < splices[i-1].end {
			return nil, false, fmt.Errorf("segments: two edits touch the same part of the body")
		}
	}

	grow := 0
	for _, s := range splices {
		grow += len(s.with) - (s.end - s.start)
	}
	out = make([]byte, 0, len(body)+max(grow, 0))
	at := 0
	for _, s := range splices {
		out = append(out, body[at:s.start]...)
		out = append(out, s.with...)
		at = s.end
	}
	return append(out, body[at:]...), leavesOnly, nil
}
