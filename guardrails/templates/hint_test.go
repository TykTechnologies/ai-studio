package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHint(t *testing.T) {
	for hint, want := range map[string]struct {
		name    string
		version int
		ok      bool
	}{
		"pr-review@3":            {"pr-review", 3, true},
		"pr-review":              {"pr-review", 0, true},
		" pr-review@12 ":         {"pr-review", 12, true},
		"team.bot_v2":            {"team.bot_v2", 0, true},
		"":                       {},
		"@3":                     {},
		"pr-review@":             {},
		"pr-review@0":            {},
		"pr-review@-1":           {},
		"pr-review@+3":           {},
		"pr-review@x":            {},
		"pr review":              {},
		"pr-review@3@4":          {},
		"pr-rev\u200biew":        {},
		strings.Repeat("a", 200): {},
	} {
		name, version, ok := ParseHint(hint)
		assert.Equal(t, want.ok, ok, "%q", hint)
		assert.Equal(t, want.name, name, "%q", hint)
		assert.Equal(t, want.version, version, "%q", hint)
	}
}

func TestHintContext(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, ctx, WithHint(ctx, ""), "an empty hint leaves the context as it is")
	assert.Equal(t, "", HintFrom(ctx))
	assert.Equal(t, "", HintFrom(nil)) //nolint:staticcheck // a nil context reads as no hint
	assert.Equal(t, "pr-review@3", HintFrom(WithHint(ctx, "pr-review@3")))
}

func hintTemplates() []*Template {
	mk := func(name string, version int) *Template {
		return &Template{Name: name, Version: version, Placement: Placement{Role: RoleUser, Message: MessageAny, Anchor: AnchorBoth},
			Parts: []Part{{Const: cIntro}, {Slot: "diff"}, {Const: cOutro}}}
	}
	return []*Template{mk("a", 1), mk("b", 4), mk("c", 2)}
}

func refs(ts []*Template) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Ref()
	}
	return out
}

func TestPreferHinted(t *testing.T) {
	ts := hintTemplates()
	before := refs(ts)
	for hint, want := range map[string][]string{
		"b@4":       {"b@4", "a@1", "c@2"},
		"b":         {"b@4", "a@1", "c@2"},
		"c@2":       {"c@2", "a@1", "b@4"},
		"a@1":       before,
		"b@3":       before, // a stale version
		"unknown":   before,
		"b@4 extra": before,
		"":          before,
	} {
		assert.Equal(t, want, refs(PreferHinted(ts, hint)), "%q", hint)
	}
	assert.Equal(t, before, refs(ts), "the caller's (cached) list is never reordered")
}

// A hint changes which of two equal matches is reported, and nothing else:
// a template that does not match is not made to, and one that exempts more
// does not beat one that exempts less.
func TestPreferHinted_OnlyBreaksTies(t *testing.T) {
	msg := Message{Role: RoleUser, Text: cIntro + "diff" + cOutro, First: true, Last: true}

	ts := hintTemplates()
	assert.Equal(t, "a@1", MatchMessage(ts, msg).Match.Template.Ref())
	assert.Equal(t, "c@2", MatchMessage(PreferHinted(ts, "c"), msg).Match.Template.Ref(), "a tie goes to the hinted template")

	// The hinted template does not match: the other one still does.
	other := &Template{Name: "other", Version: 1, Placement: Placement{Role: RoleUser, Message: MessageAny, Anchor: AnchorBoth},
		Parts: []Part{{Const: strings.Repeat("o", 40)}, {Slot: "x"}, {Const: cOutro}}}
	ts = []*Template{ts[0], other}
	out := MatchMessage(PreferHinted(ts, "other@1"), msg)
	require.NotNil(t, out.Match)
	assert.Equal(t, "a@1", out.Match.Template.Ref())

	// The hinted template exempts more than another that matches: the one
	// that leaves more text scored still wins.
	wide := &Template{Name: "wide", Version: 1, Placement: Placement{Role: RoleUser, Message: MessageAny, Anchor: AnchorBoth},
		Parts: []Part{{Const: cIntro + "diff"}, {Slot: "nothing"}, {Const: cOutro}}}
	ts = []*Template{hintTemplates()[0], wide}
	out = MatchMessage(PreferHinted(ts, "wide"), msg)
	require.NotNil(t, out.Match)
	assert.Equal(t, "a@1", out.Match.Template.Ref())
	assert.Equal(t, MatchMessage(ts, msg).Match.Exempt, out.Match.Exempt)
}
