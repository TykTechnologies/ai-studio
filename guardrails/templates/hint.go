package templates

import (
	"context"
	"strconv"
	"strings"
)

// HintHeader is the request header a caller may send to name the template
// its prompt was built from: "<name>@<version>", or "<name>" for the
// approved version of that name. It only orders the search: the hinted
// template is tried first, so it wins a tie between templates that exempt
// the same text. It never makes a template match, never makes one that is
// not approved eligible, and never exempts more than matching without it
// would. A hint that names nothing approved is ignored. The gateway strips
// it before a request leaves for the vendor.
const HintHeader = "X-Tyk-Prompt-Template"

// maxHintLen bounds a hint worth parsing: a name (128) plus "@" and a
// version.
const maxHintLen = 160

type hintKey struct{}

// WithHint returns ctx carrying the caller's template hint, the raw header
// value. An empty hint returns ctx unchanged.
func WithHint(ctx context.Context, hint string) context.Context {
	if hint == "" {
		return ctx
	}
	return context.WithValue(ctx, hintKey{}, hint)
}

// HintFrom is the hint WithHint stored in ctx, "" for none.
func HintFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(hintKey{}).(string)
	return s
}

// ParseHint reads a hint: a template name (letters, digits, '.', '_' and
// '-') with an optional "@<version>". version is 0 when the hint names no
// version. ok is false for anything else, which the caller ignores.
func ParseHint(hint string) (name string, version int, ok bool) {
	hint = strings.TrimSpace(hint)
	if hint == "" || len(hint) > maxHintLen {
		return "", 0, false
	}
	name = hint
	if at := strings.LastIndexByte(hint, '@'); at >= 0 {
		name = hint[:at]
		v, err := strconv.Atoi(hint[at+1:])
		if err != nil || v < 1 || strings.HasPrefix(hint[at+1:], "+") {
			return "", 0, false
		}
		version = v
	}
	if name == "" {
		return "", 0, false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return "", 0, false
		}
	}
	return name, version, true
}

// PreferHinted returns ts with the template the hint names moved to the
// front, so MatchMessage tries it first. ts is the app's approved templates;
// a hint that names none of them (unknown, not approved, retired, an older
// version) or does not parse returns ts as it is. ts itself is never
// modified: it may be a cached list shared between requests.
//
// Ordering is all a hint does. MatchMessage still tries every template and
// still picks the match that exempts the fewest characters; the order only
// breaks a tie between equal matches.
func PreferHinted(ts []*Template, hint string) []*Template {
	name, version, ok := ParseHint(hint)
	if !ok {
		return ts
	}
	at := -1
	for i, t := range ts {
		if t != nil && t.Name == name && (version == 0 || t.Version == version) {
			at = i
			break
		}
	}
	if at <= 0 {
		return ts
	}
	out := make([]*Template, 0, len(ts))
	out = append(out, ts[at])
	out = append(out, ts[:at]...)
	return append(out, ts[at+1:]...)
}
