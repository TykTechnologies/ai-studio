package nativeapi

import (
	"fmt"
	"strings"
)

// segment is one path segment of a compiled pattern: a literal, or a {param}.
type segment struct {
	literal string
	param   string
}

type pattern struct {
	segs []segment
}

func compilePattern(path string) (pattern, error) {
	if !strings.HasPrefix(path, "/") || path == "/" {
		return pattern{}, fmt.Errorf("path %q must start with / and have a segment", path)
	}
	parts := strings.Split(path[1:], "/")
	p := pattern{segs: make([]segment, 0, len(parts))}
	params := map[string]bool{}
	for _, part := range parts {
		if part == "" {
			return pattern{}, fmt.Errorf("path %q has an empty segment", path)
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := part[1 : len(part)-1]
			if name == "" || strings.ContainsAny(name, "{}") {
				return pattern{}, fmt.Errorf("path %q has an invalid parameter %q", path, part)
			}
			if params[name] {
				return pattern{}, fmt.Errorf("path %q repeats parameter %q", path, name)
			}
			params[name] = true
			p.segs = append(p.segs, segment{param: name})
			continue
		}
		if strings.ContainsAny(part, "{}") {
			return pattern{}, fmt.Errorf("path %q has a partial parameter segment %q", path, part)
		}
		p.segs = append(p.segs, segment{literal: part})
	}
	return p, nil
}

func (p pattern) hasParam(name string) bool {
	for _, s := range p.segs {
		if s.param == name {
			return true
		}
	}
	return false
}

func (p pattern) literals() int {
	n := 0
	for _, s := range p.segs {
		if s.param == "" {
			n++
		}
	}
	return n
}

// shape is the pattern with parameter names erased; two patterns with the
// same shape match the same paths.
func (p pattern) shape() string {
	var b strings.Builder
	for _, s := range p.segs {
		b.WriteByte('/')
		if s.param != "" {
			b.WriteString("{}")
		} else {
			b.WriteString(s.literal)
		}
	}
	return b.String()
}

func (p pattern) match(parts []string) (map[string]string, bool) {
	if len(parts) != len(p.segs) {
		return nil, false
	}
	var params map[string]string
	for i, s := range p.segs {
		if s.param == "" {
			if parts[i] != s.literal {
				return nil, false
			}
			continue
		}
		if parts[i] == "" {
			return nil, false
		}
		if params == nil {
			params = map[string]string{}
		}
		params[s.param] = parts[i]
	}
	return params, true
}

type compiledOp struct {
	op  *Operation
	pat pattern
}

// Matcher resolves a request to an Operation by exact method and path.
type Matcher struct {
	spec Spec
	ops  []compiledOp
}

// Match is a resolved operation and its path parameters.
type Match struct {
	Op     *Operation
	Params map[string]string
}

// ID returns the {id} path parameter, if any.
func (m Match) ID() string { return m.Params[IDParam] }

// NewMatcher validates spec and compiles its operation paths. It rejects two
// operations with the same method whose paths match the same requests.
func NewMatcher(spec Spec) (*Matcher, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	m := &Matcher{spec: spec}
	shapes := map[string]string{}
	for i := range spec.Operations {
		op := &m.spec.Operations[i]
		pat, err := compilePattern(op.Path)
		if err != nil {
			return nil, err
		}
		key := op.Method + " " + pat.shape()
		if other, dup := shapes[key]; dup {
			return nil, fmt.Errorf("nativeapi: operations %q and %q match the same requests", other, op.Name)
		}
		shapes[key] = op.Name
		m.ops = append(m.ops, compiledOp{op: op, pat: pat})
	}
	return m, nil
}

// Spec returns the matcher's spec.
func (m *Matcher) Spec() Spec { return m.spec }

// Match resolves method and a canonical vendor path. Matching is exact: the
// method must be equal, every segment must match, and there is no prefix,
// trailing-slash or empty-segment tolerance. When several patterns match, the
// one with the most literal segments wins ("/batches/count" before
// "/batches/{id}").
func (m *Matcher) Match(method, path string) (Match, bool) {
	if m == nil || !strings.HasPrefix(path, "/") {
		return Match{}, false
	}
	parts := strings.Split(path[1:], "/")
	var (
		best      Match
		bestScore = -1
	)
	for _, c := range m.ops {
		if c.op.Method != method {
			continue
		}
		params, ok := c.pat.match(parts)
		if !ok {
			continue
		}
		if score := c.pat.literals(); score > bestScore {
			best, bestScore = Match{Op: c.op, Params: params}, score
		}
	}
	return best, bestScore >= 0
}

// MatchesPath reports whether any operation (of any method) matches path. The
// proxy uses it to tell "native path, wrong method" (vendor-shaped 405) from a
// path that is not native at all (legacy chain).
func (m *Matcher) MatchesPath(path string) bool {
	if m == nil || !strings.HasPrefix(path, "/") {
		return false
	}
	parts := strings.Split(path[1:], "/")
	for _, c := range m.ops {
		if _, ok := c.pat.match(parts); ok {
			return true
		}
	}
	return false
}

// foldSegments splits path into its non-empty segments.
func foldSegments(path string) []string {
	var out []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// matchFold is match with literal segments compared case-insensitively.
func (p pattern) matchFold(parts []string) bool {
	if len(parts) != len(p.segs) {
		return false
	}
	for i, s := range p.segs {
		if s.param == "" && !strings.EqualFold(parts[i], s.literal) {
			return false
		}
	}
	return true
}

// MatchesFold reports whether path names a path some operation or chain
// hook declares once case and empty segments are ignored ("/v1/Messages/
// Batches/", "/V1/responses"). The proxy refuses such a spelling of a native
// path rather than let it reach the vendor around the native handling: an
// upstream that routes paths case-insensitively would serve it.
func (m *Matcher) MatchesFold(path string) bool {
	if m == nil {
		return false
	}
	parts := foldSegments(path)
	for _, c := range m.ops {
		if c.pat.matchFold(parts) {
			return true
		}
	}
	if m.spec.ChainHooks != nil {
		for _, h := range m.spec.ChainHooks.Hooks {
			if pat, err := compilePattern(h.Path); err == nil && pat.matchFold(parts) {
				return true
			}
		}
	}
	return false
}
