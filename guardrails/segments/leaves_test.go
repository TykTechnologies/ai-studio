package segments

// Leaf-level helpers for the tests: production code reads units
// (ExtractUnits) and writes with Apply.

// Extract returns the string leaves of body, in body order, with Index set
// to the position in the result.
func Extract(format string, body []byte) ([]Segment, error) {
	x, err := extract(format, body, Options{})
	if err != nil {
		return nil, err
	}
	return x.leaves, nil
}

// Rewrite is Apply for edits that do not name the text they replace: it
// replaces the string leaf at each JSON Pointer with the new text, whatever
// the leaf holds.
func Rewrite(body []byte, edits map[string]string) ([]byte, error) {
	list := make([]Edit, 0, len(edits))
	root, err := parseTree(body)
	for ptr, text := range edits {
		e := Edit{Path: ptr, New: text}
		if err == nil {
			if n := root.resolve(ptr); n != nil && n.kind == nodeString {
				e.Old, _ = n.text(body)
			}
		}
		list = append(list, e)
	}
	return Apply(body, list)
}
