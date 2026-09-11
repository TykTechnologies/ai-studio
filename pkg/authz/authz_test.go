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

func TestPublish_OnlyOnPublishableResources(t *testing.T) {
	want := []string{"agents", "apps", "datasources", "llms", "metadata", "model-routers", "plugins", "tools"}
	assert.ElementsMatch(t, want, Publishable())
	for _, r := range Catalogue() {
		offers := false
		for _, a := range r.Actions {
			offers = offers || a == ActionPublish
		}
		assert.Equal(t, offers, Publish(r.Key).Valid(), r.Key)
	}
	_, err := Parse("analytics:publish")
	assert.Error(t, err, "publish is not offered by resources without a live switch")
}

func TestSet_PublishImpliesReadOnly(t *testing.T) {
	s := NewSet(Publish("llms"))
	assert.True(t, s.Has(Publish("llms")))
	assert.True(t, s.Has(Read("llms")), "publish implies read")
	assert.False(t, s.Has(Write("llms")), "publish does not imply write")
	assert.False(t, s.Has(Delete("llms")))
	assert.False(t, NewSet(Write("llms")).Has(Publish("llms")), "write does not imply publish")
}

func TestPluginResources_ReplaceUnregisterAndUmbrella(t *testing.T) {
	const key = "plugin:com.example.assets"
	t.Cleanup(func() { UnregisterPlugin(key) })

	before := Version()
	require.NoError(t, Replace(Resource{Key: key, Label: "Assets", Group: "Plugins", Plugin: key, PluginLabel: "Assets",
		Actions: []Action{ActionRead, ActionWrite, ActionExecute}}))
	require.NoError(t, Replace(Resource{Key: key + ":types", Label: "Asset types", Group: "Plugins", Plugin: key, PluginLabel: "Assets",
		Actions: []Action{ActionRead, ActionWrite, ActionDelete, ActionPublish}}))
	assert.Greater(t, Version(), before)

	// Parsing and lookup work like built-ins once registered.
	p, err := Parse(key + ":types:publish")
	require.NoError(t, err)
	assert.Equal(t, key+":types", p.Resource())
	assert.Equal(t, ActionPublish, p.Action())
	_, err = Parse(key + ":types:execute")
	assert.Error(t, err, "action not offered")

	// Plugin resources sort after built-ins within the Plugins group, base first.
	var plugins []Resource
	for _, r := range Catalogue() {
		if r.Group == "Plugins" {
			plugins = append(plugins, r)
		}
	}
	require.GreaterOrEqual(t, len(plugins), 4)
	assert.False(t, plugins[0].Dynamic)
	assert.Equal(t, key, plugins[len(plugins)-2].Key)
	assert.Equal(t, key+":types", plugins[len(plugins)-1].Key)
	assert.Len(t, PluginResources(key), 2)

	// Umbrella: plugins:execute grants every plugin permission; plugins:read does not.
	assert.True(t, NewSet(Execute("plugins")).Has(Write(key)))
	assert.True(t, NewSet(Execute("plugins")).Has(Publish(key+":types")))
	assert.False(t, NewSet(Read("plugins")).Has(Read(key)))
	assert.False(t, NewSet(Write("plugins")).Has(Read(key)))
	assert.True(t, NewSet(Write(key)).Has(Read(key)), "implied read on a plugin resource")
	assert.False(t, NewSet(Write(key)).Has(Read(key+":types")))
	assert.True(t, NewSet(Read(key)).HasPluginGrant())
	assert.True(t, NewSet(Execute("plugins")).HasPluginGrant())
	assert.False(t, NewSet(Read("plugins")).HasPluginGrant())

	// Replace is an upsert; Register on a plugin key panics; built-ins cannot be replaced.
	require.NoError(t, Replace(Resource{Key: key, Label: "Assets v2", Group: "Plugins", Plugin: key, PluginLabel: "Assets v2", Actions: []Action{ActionRead}}))
	r, ok := ResourceByKey(key)
	require.True(t, ok)
	assert.Equal(t, "Assets v2", r.Label)
	assert.Error(t, Replace(Resource{Key: "llms", Label: "x", Group: "LLM management", Actions: readOnly}))
	assert.Error(t, Replace(Resource{Key: "not-a-plugin", Label: "x", Group: "Plugins", Actions: readOnly}), "dynamic needs the plugin prefix")
	assert.Error(t, Replace(Resource{Key: key + ":sub", Label: "x", Group: "Plugins", Plugin: "plugin:other", Actions: readOnly}), "plugin must be the key prefix")

	// Stored plugin permissions survive unregistration for role storage only.
	assert.Equal(t, 2, UnregisterPlugin(key))
	assert.False(t, Unregister("llms"), "built-ins never unregister")
	_, err = Parse(key + ":read")
	assert.Error(t, err)
	stored, err := ParseStored(key + ":read")
	require.NoError(t, err)
	assert.Equal(t, Permission(key+":read"), stored)
	_, err = ParseStored("plugin::read")
	assert.Error(t, err)
	_, err = ParseStored(key + ":fly")
	assert.Error(t, err)
	_, err = ParseStored("spaceships:read")
	assert.Error(t, err)
	assert.True(t, NewSetFromStrings([]string{key + ":read"}).IsEmpty(), "unregistered plugin grants are not evaluated")
}

func TestPermission_Constructors(t *testing.T) {
	assert.Equal(t, Permission("llms:read"), Read("llms"))
	assert.Equal(t, Permission("llms:write"), Write("llms"))
	assert.Equal(t, Permission("llms:delete"), Delete("llms"))
	assert.Equal(t, Permission("tools:execute"), Execute("tools"))
	assert.Equal(t, Permission("llms:publish"), Publish("llms"))
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
