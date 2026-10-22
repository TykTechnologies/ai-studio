package segments

import (
	"fmt"
	"strings"
)

// Doc is a request body parsed once. ExtractUnits, NativeContextManagement,
// CacheBreakpoints, CheckToolPairing and Apply each validate and parse the
// body they are given; a caller that reads one body several ways and then
// rewrites it (the Prompt Compressor reads, rewrites and reads the result
// back) parses it once with Parse and uses the Doc's methods, which answer
// exactly as those functions do. A Doc is for one goroutine at a time: it
// remembers each string it has decoded.
type Doc struct {
	format string
	body   []byte
	root   *node
}

// Parse parses body, in format, once. A body that is not JSON is
// ErrUnrecognisedBody and one with a repeated key ErrDuplicateKey, as for
// ExtractUnits; a format ExtractUnits has no segmenter for is
// ErrUnsupportedFormat.
func Parse(format string, body []byte) (*Doc, error) {
	if !Supports(format) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	return &Doc{format: format, body: body, root: root}, nil
}

// Body returns the bytes the Doc was parsed from.
func (d *Doc) Body() []byte { return d.body }

// As returns the Doc read in another format, sharing the parsed body: a
// caller that reads part of a body before it knows the format (the tool
// definitions, say) parses it once. A format ExtractUnits has no segmenter
// for is ErrUnsupportedFormat.
func (d *Doc) As(format string) (*Doc, error) {
	if !Supports(format) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	return &Doc{format: format, body: d.body, root: d.root}, nil
}

// Raw returns the bytes of the value at the JSON Pointer ptr, as the body
// holds them, or nil when there is none.
func (d *Doc) Raw(ptr string) []byte {
	n := d.root.resolve(ptr)
	if n == nil {
		return nil
	}
	return d.body[n.start:n.end]
}

// Units is ExtractUnitsWith on the Doc's body.
func (d *Doc) Units(opts Options) ([]Unit, error) {
	x, err := extractTree(d.format, d.body, d.root, opts)
	if err != nil {
		return nil, err
	}
	return x.result(), nil
}

// NativeContextManagement is NativeContextManagement on the Doc's body.
func (d *Doc) NativeContextManagement() bool {
	return nativeContext(d.format, d.body, d.root)
}

// CacheBreakpoints is CacheBreakpoints on the Doc's body.
func (d *Doc) CacheBreakpoints() ([]Breakpoint, error) {
	switch f := strings.ToLower(d.format); f {
	case FormatAnthropic, FormatBedrock:
		return cacheBreakpoints(f, d.root)
	default:
		// Every other format Parse accepts carries none (CacheBreakpoints);
		// the body's keys were checked by Parse.
		return nil, nil
	}
}

// CheckToolPairing is CheckToolPairing on the Doc's body.
func (d *Doc) CheckToolPairing() error {
	return toolPairing(d.format, d.body, d.root)
}

// Apply is Apply on the Doc's body, and returns the rewritten body parsed.
// When every edit replaces a string leaf (no Edit.Value), the result is valid
// JSON with the body's keys by construction (each new string is encoded and
// spliced over exactly one string token), so it is parsed without being
// validated again; otherwise it is validated as Parse does. With no edits it
// returns d itself.
func (d *Doc) Apply(edits []Edit) (*Doc, error) {
	if len(edits) == 0 {
		return d, nil
	}
	out, leavesOnly, err := applyTree(d.body, d.root, edits)
	if err != nil {
		return nil, err
	}
	if !leavesOnly {
		return Parse(d.format, out)
	}
	p := &treeParser{b: out}
	p.ws()
	root, err := p.value()
	if err != nil {
		return nil, err
	}
	return &Doc{format: d.format, body: out, root: root}, nil
}
