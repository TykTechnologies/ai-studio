package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogue_IsWellFormed(t *testing.T) {
	cat := Catalogue()
	require.NotEmpty(t, cat)
	seen := map[string]bool{}
	for _, r := range cat {
		assert.False(t, seen[r.Key], "duplicate key %s", r.Key)
		seen[r.Key] = true
		assert.NotEmpty(t, r.Label, r.Key)
		assert.Contains(t, Groups, r.Group, r.Key)
		assert.NotEmpty(t, r.Actions, r.Key)
		assert.Equal(t, ActionRead, r.Actions[0], "%s must offer read first", r.Key)
	}
	// Every group in display order is populated by at least one resource.
	for _, g := range Groups {
		found := false
		for _, r := range cat {
			if r.Group == g {
				found = true
				break
			}
		}
		assert.True(t, found, "group %q has no resources", g)
	}
}

func TestCatalogue_SortedByGroupThenLabel(t *testing.T) {
	cat := Catalogue()
	for i := 1; i < len(cat); i++ {
		gi, gj := groupRank[cat[i-1].Group], groupRank[cat[i].Group]
		if gi == gj {
			assert.LessOrEqual(t, cat[i-1].Label, cat[i].Label)
		} else {
			assert.Less(t, gi, gj)
		}
	}
}

func TestPermission_ParseAndLookup(t *testing.T) {
	for _, p := range All() {
		got, err := Parse(string(p))
		require.NoError(t, err, p)
		assert.Equal(t, p, got)
		r, ok := Lookup(p)
		assert.True(t, ok, p)
		assert.Equal(t, p.Resource(), r.Key)
	}

	_, err := Parse("llms:fly")
	assert.Error(t, err)
	_, err = Parse("spaceships:read")
	assert.Error(t, err)
	_, err = Parse("analytics:write") // resource exists but not the action
	assert.Error(t, err)
	_, err = Parse(string(AnyAdmin))
	assert.Error(t, err, "AnyAdmin is a route sentinel, not a grant")

	full, err := Parse("*")
	require.NoError(t, err)
	assert.Equal(t, FullAdmin, full)
	assert.True(t, FullAdmin.Valid())
	assert.True(t, AnyAdmin.Valid())
	assert.Equal(t, "", FullAdmin.Resource())
}

func TestPermission_Constructors(t *testing.T) {
	assert.Equal(t, Permission("llms:read"), Read("llms"))
	assert.Equal(t, Permission("llms:write"), Write("llms"))
	assert.Equal(t, Permission("llms:delete"), Delete("llms"))
	assert.Equal(t, Permission("tools:execute"), Execute("tools"))
	assert.Equal(t, "data-catalogues", Read("data-catalogues").Resource())
	assert.Equal(t, ActionRead, Read("data-catalogues").Action())
}

func TestSet_Semantics(t *testing.T) {
	s := NewSet(Write("llms"), Read("users"))

	assert.True(t, s.Has(Write("llms")))
	assert.True(t, s.Has(Read("llms")), "write implies read")
	assert.False(t, s.Has(Delete("llms")))
	assert.True(t, s.Has(Read("users")))
	assert.False(t, s.Has(Write("users")))
	assert.False(t, s.Has(FullAdmin))
	assert.True(t, s.Has(AnyAdmin), "any non-empty set is on the admin surface")
	assert.True(t, s.HasAll(Read("llms"), Read("users")))
	assert.False(t, s.HasAll(Read("llms"), Read("tools")))
	assert.True(t, s.HasAny(Read("tools"), Read("users")))
	assert.Equal(t, []string{"llms:write", "users:read"}, s.List())

	empty := NewSet()
	assert.True(t, empty.IsEmpty())
	assert.False(t, empty.Has(AnyAdmin))
	assert.False(t, empty.Has(Read("llms")))

	w := Wildcard()
	assert.True(t, w.IsFullAdmin())
	assert.True(t, w.Has(FullAdmin))
	assert.True(t, w.Has(AnyAdmin))
	assert.True(t, w.Has(Delete("sso-profiles")))
	assert.Equal(t, []string{"*"}, w.List())

	assert.True(t, NewSet(FullAdmin).IsFullAdmin(), "FullAdmin in a set makes it the wildcard")
	assert.True(t, s.Union(w).IsFullAdmin())
	u := s.Union(NewSet(Read("tools")))
	assert.True(t, u.Has(Read("tools")))
	assert.True(t, u.Has(Write("llms")))
	assert.False(t, s.Has(Read("tools")), "union does not mutate the receiver")
}

func TestSet_FromStringsSkipsUnknown(t *testing.T) {
	s := NewSetFromStrings([]string{"llms:read", "bogus:read", "_any", "", "tools:execute"})
	assert.Equal(t, []string{"llms:read", "tools:execute"}, s.List())
	assert.True(t, NewSetFromStrings([]string{"*"}).IsFullAdmin())
}

func TestFilterHelpers(t *testing.T) {
	reads := AllWithAction(ActionRead)
	assert.Equal(t, len(Catalogue()), len(reads), "every resource offers read")
	nonSensitiveReads := Filter(func(r Resource, a Action) bool { return a == ActionRead && !r.Sensitive })
	assert.Less(t, len(nonSensitiveReads), len(reads))
	for _, p := range nonSensitiveReads {
		r, _ := Lookup(p)
		assert.False(t, r.Sensitive)
	}
}

func TestRegister_Panics(t *testing.T) {
	assert.Panics(t, func() { Register(Resource{Key: "llms", Label: "x", Group: "Chat", Actions: readOnly}) }, "duplicate")
	assert.Panics(t, func() { Register(Resource{Key: "zzz", Label: "x", Group: "Nope", Actions: readOnly}) }, "unknown group")
	assert.Panics(t, func() { Register(Resource{Key: "zzz", Label: "x", Group: "Chat"}) }, "no actions")
	assert.Panics(t, func() { Register(Resource{Key: "zzz", Label: "x", Group: "Chat", Actions: []Action{"fly"}}) }, "bad action")
}

func TestDenied_Body(t *testing.T) {
	b := Denied(Write("llms"))
	require.Len(t, b.Errors, 1)
	assert.Equal(t, "Forbidden", b.Errors[0].Title)
	assert.Equal(t, CodePermissionDenied, b.Errors[0].Code)
	assert.Equal(t, "llms:write", b.Errors[0].Permission)

	b = Denied(FullAdmin)
	assert.Equal(t, "", b.Errors[0].Permission)
	assert.Contains(t, b.Errors[0].Detail, "administrator")
}
