package authz

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedScopes is the scope kind of every resource tied to a Workspace:
// the 17 stamped tables (plugins and webhooks carry the stamp but stay
// global because they act on, or receive events from, every Workspace) and
// the App credentials, which follow their App. Every other catalogue
// resource must be ScopeGlobal. Registering a resource, or
// changing a kind, means updating this table: that is the point.
var expectedScopes = map[string]ScopeKind{
	"llms":             ScopeWorkspaceWrite,
	"tools":            ScopeWorkspaceWrite,
	"datasources":      ScopeWorkspaceWrite,
	"filters":          ScopeWorkspaceWrite,
	"embedders":        ScopeWorkspaceWrite,
	"secrets":          ScopeWorkspaceWrite,
	"filestores":       ScopeWorkspaceWrite,
	"mcp-servers":      ScopeWorkspaceWrite,
	"apps":             ScopeWorkspaceFull,
	"agents":           ScopeWorkspaceFull,
	"model-routers":    ScopeWorkspaceFull,
	"semantic-routers": ScopeWorkspaceFull,
	"catalogues":       ScopeWorkspaceFull,
	"data-catalogues":  ScopeWorkspaceFull,
	"tool-catalogues":  ScopeWorkspaceFull,
	"credentials":      ScopeWorkspaceParent,
	"mcp-credentials":  ScopeWorkspaceParent,
	"plugins":          ScopeGlobal,
	"webhooks":         ScopeGlobal,
}

func TestCatalogue_ScopeKinds(t *testing.T) {
	for _, r := range Catalogue() {
		want, listed := expectedScopes[r.Key]
		if !listed {
			want = ScopeGlobal
		}
		assert.Equal(t, want, r.Scope, "scope kind of %q", r.Key)
	}
	for key := range expectedScopes {
		_, ok := ResourceByKey(key)
		assert.True(t, ok, "resource %q is not registered", key)
	}
	var wantScoped []string
	for key, kind := range expectedScopes {
		if kind.InWorkspace() {
			wantScoped = append(wantScoped, key)
		}
	}
	assert.ElementsMatch(t, wantScoped, WorkspaceResources())

	// Parent-scoped resources name a fully scoped parent; no other kind
	// names a parent.
	for _, r := range Catalogue() {
		if r.Scope == ScopeWorkspaceParent {
			assert.Equal(t, "apps", r.ScopeParent, r.Key)
			assert.Equal(t, ScopeWorkspaceFull, ResourceScope(r.ScopeParent), r.Key)
		} else {
			assert.Empty(t, r.ScopeParent, r.Key)
		}
	}
	assert.True(t, ScopeWorkspaceParent.ReadScoped())
	assert.True(t, ScopeWorkspaceFull.ReadScoped())
	assert.False(t, ScopeWorkspaceWrite.ReadScoped())
	assert.False(t, ScopeWorkspaceParent.Stamped())
	assert.True(t, ScopeWorkspaceParent.InWorkspace())
	var wantStamped []string
	for key, kind := range expectedScopes {
		if kind.Stamped() {
			wantStamped = append(wantStamped, key)
		}
	}
	assert.ElementsMatch(t, wantStamped, StampedResources())
	assert.Equal(t, ScopeGlobal, ScopeOf(FullAdmin))
	assert.Equal(t, ScopeGlobal, ScopeOf(AnyAdmin))
	assert.Equal(t, ScopeGlobal, ScopeOf("spaceships:read"))
	assert.Equal(t, ScopeWorkspaceFull, ScopeOf(Write("apps")))

	ws, ok := ResourceByKey("workspaces")
	require.True(t, ok)
	assert.Equal(t, "Access", ws.Group)
	assert.True(t, ws.Privileged)
	assert.Equal(t, crud, ws.Actions)
	assert.Equal(t, ScopeGlobal, ws.Scope)
}

func TestRegister_RejectsBadScope(t *testing.T) {
	assert.Panics(t, func() {
		Register(Resource{Key: "zz-bad-scope", Label: "x", Group: "Settings", Actions: readOnly, Scope: "galaxy"})
	})
	assert.Panics(t, func() {
		Register(Resource{Key: "zz-no-parent", Label: "x", Group: "Settings", Actions: readOnly, Scope: ScopeWorkspaceParent})
	}, "a parent scope needs a parent")
	assert.Panics(t, func() {
		Register(Resource{Key: "zz-stray-parent", Label: "x", Group: "Settings", Actions: readOnly, Scope: ScopeWorkspaceFull, ScopeParent: "apps"})
	}, "only a parent scope names a parent")
	assert.Panics(t, func() {
		Register(Resource{Key: "zz-self", Label: "x", Group: "Settings", Actions: readOnly, Scope: ScopeWorkspaceParent, ScopeParent: "zz-self"})
	})
	const key = "plugin:com.example.scoped"
	t.Cleanup(func() { UnregisterPlugin(key) })
	assert.Error(t, Replace(Resource{Key: key, Label: "x", Group: "Plugins", Plugin: key, Actions: readOnly, Scope: ScopeWorkspaceWrite}),
		"plugin resources are always global")
}

func TestGrants_CanIn(t *testing.T) {
	const w1, w2 = 1, 2

	global := GlobalGrants(NewSet(Write("llms")))
	assert.True(t, global.CanIn(w1, Write("llms")))
	assert.True(t, global.CanIn(w2, Write("llms")))
	assert.True(t, global.CanIn(w1, Read("llms")), "implied read")
	assert.False(t, global.CanIn(w1, Delete("llms")))

	scoped := Grants{Global: NewSet(), Workspaces: map[uint]Set{w1: NewSet(Write("llms"))}}
	assert.True(t, scoped.CanIn(w1, Write("llms")))
	assert.True(t, scoped.CanIn(w1, Read("llms")), "implied read holds inside a Workspace set")
	assert.False(t, scoped.CanIn(w2, Write("llms")))
	assert.False(t, scoped.CanIn(0, Write("llms")), "Workspace 0 is never looked up")
	assert.False(t, scoped.Can(Write("llms")), "Can is global only")

	both := Grants{Global: NewSet(Read("apps")), Workspaces: map[uint]Set{w2: NewSet(Write("apps"))}}
	assert.True(t, both.CanIn(w1, Read("apps")))
	assert.False(t, both.CanIn(w1, Write("apps")))
	assert.True(t, both.CanIn(w2, Write("apps")))
}

func TestGrants_GlobalResourcesIgnoreWorkspaceSets(t *testing.T) {
	// Even a hand-built Workspace set holding global permissions (or the
	// wildcard) never satisfies a global permission (decision D2).
	g := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: NewSet(Write("users"), Execute("edges"), Execute("plugins"))}}
	assert.False(t, g.CanIn(1, Write("users")))
	assert.False(t, g.CanAnywhere(Execute("edges")))
	all, ids := g.WorkspacesWith(Write("users"))
	assert.False(t, all)
	assert.Empty(t, ids)

	wild := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: Wildcard()}}
	assert.False(t, wild.CanIn(1, FullAdmin))
	assert.False(t, wild.CanIn(1, Write("roles")))
	assert.True(t, wild.CanIn(1, Write("llms")))
}

func TestGrants_PluginUmbrellaIsGlobalOnly(t *testing.T) {
	const key = "plugin:com.example.umbrella"
	t.Cleanup(func() { UnregisterPlugin(key) })
	require.NoError(t, Replace(Resource{Key: key, Label: "U", Group: "Plugins", Plugin: key, Actions: crud}))

	scoped := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: NewSet(Execute("plugins"))}}
	assert.False(t, scoped.CanAnywhere(Write(key)))
	assert.False(t, scoped.CanIn(1, Write(key)))
	assert.False(t, scoped.Can(Write("plugins")))
	assert.False(t, scoped.CanAnywhere(Execute("plugins")))

	global := GlobalGrants(NewSet(Execute("plugins")))
	assert.True(t, global.CanAnywhere(Write(key)), "the umbrella still holds on global grants")
	assert.True(t, global.CanIn(1, Write(key)))
}

func TestGrants_CanAnywhereAndWorkspacesWith(t *testing.T) {
	g := Grants{
		Global:     NewSet(Read("llms")),
		Workspaces: map[uint]Set{3: NewSet(Write("tools")), 1: NewSet(Write("tools"), Read("apps")), 2: NewSet()},
	}
	assert.True(t, g.CanAnywhere(Write("tools")))
	assert.True(t, g.CanAnywhere(Read("tools")))
	assert.True(t, g.CanAnywhere(Read("llms")))
	assert.False(t, g.CanAnywhere(Write("llms")))
	assert.True(t, g.CanAnywhere(AnyAdmin))

	all, ids := g.WorkspacesWith(Write("tools"))
	assert.False(t, all)
	assert.Equal(t, []uint{1, 3}, ids)

	all, ids = g.WorkspacesWith(Read("llms"))
	assert.True(t, all, "a global grant covers every Workspace")
	assert.Nil(t, ids)

	all, ids = g.WorkspacesWith(Read("apps"))
	assert.False(t, all)
	assert.Equal(t, []uint{1}, ids)

	scopedOnly := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: NewSet(Read("apps"))}}
	assert.True(t, scopedOnly.CanAnywhere(AnyAdmin), "a scoped-only user is on the admin surface")
	assert.False(t, scopedOnly.Global.Has(AnyAdmin))
	nobody := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: NewSet()}}
	assert.False(t, nobody.CanAnywhere(AnyAdmin))
}

func TestGrants_ParentScopedCredentials(t *testing.T) {
	// D2 keeps credentials in a scoped binding; the check takes the parent
	// App's Workspace.
	s := NewScopedSetFromStrings([]string{"credentials:read", "credentials:write", "mcp-credentials:execute"})
	assert.Equal(t, []string{"credentials:read", "credentials:write", "mcp-credentials:execute"}, s.List())
	g := Grants{Global: NewSet(), Workspaces: map[uint]Set{1: s}}
	assert.True(t, g.CanIn(1, Write("credentials")))
	assert.False(t, g.CanIn(2, Read("credentials")), "another Workspace's App credentials")
	assert.True(t, g.CanAnywhere(Execute("mcp-credentials")))
	all, ids := g.WorkspacesWith(Read("credentials"))
	assert.False(t, all)
	assert.Equal(t, []uint{1}, ids)
}

func TestNewScopedSetFromStrings(t *testing.T) {
	s := NewScopedSetFromStrings([]string{"llms:write", "users:write", "edges:execute", "plugins:execute", "webhooks:execute", "nope:read"})
	assert.Equal(t, []string{"llms:write"}, s.List(), "D2: global permissions are dropped")

	wild := NewScopedSetFromStrings([]string{"*"})
	assert.False(t, wild.IsFullAdmin(), "a scoped wildcard is never full admin")
	for _, key := range WorkspaceResources() {
		r, _ := ResourceByKey(key)
		for _, a := range r.Actions {
			assert.True(t, wild.Has(P(key, a)), "%s:%s", key, a)
		}
	}
	assert.False(t, wild.Has(Write("users")))
	assert.False(t, wild.Has(Read("plugins")))
	assert.False(t, wild.Has(Read("webhooks")))
	assert.False(t, wild.Has(Read("workspaces")))
	assert.True(t, wild.Has(Read("credentials")), "credentials follow their App")
}

func TestContext_GrantsAndPackageHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		return c
	}

	calls := 0
	c := newCtx()
	WithContext(c, NewGrantsContext(7, false, func() (Grants, error) {
		calls++
		return Grants{Global: NewSet(Read("llms")), Workspaces: map[uint]Set{1: NewSet(Write("llms"))}}, nil
	}))
	assert.True(t, CanIn(c, 1, Write("llms")))
	assert.False(t, CanIn(c, 2, Write("llms")))
	assert.False(t, Can(c, Write("llms")), "Can stays global")
	assert.True(t, CanAnywhere(c, Write("llms")))
	all, ids := WorkspacesWith(c, Write("llms"))
	assert.False(t, all)
	assert.Equal(t, []uint{1}, ids)
	set, err := Permissions(c)
	require.NoError(t, err)
	assert.Equal(t, []string{"llms:read"}, set.List(), "Permissions returns the global set only")
	assert.Equal(t, 1, calls, "resolved once per request")

	// A Set resolver (NewContext) is the global set with no Workspaces.
	c = newCtx()
	WithContext(c, NewContext(7, false, func() (Set, error) { return NewSet(Write("apps")), nil }))
	assert.True(t, CanIn(c, 9, Write("apps")))
	all, _ = WorkspacesWith(c, Write("apps"))
	assert.True(t, all)

	// Resolution errors fail closed.
	c = newCtx()
	WithContext(c, NewGrantsContext(7, true, func() (Grants, error) { return Grants{}, errors.New("db down") }))
	assert.False(t, CanIn(c, 1, Read("llms")))
	assert.False(t, CanAnywhere(c, Read("llms")))
	all, ids = WorkspacesWith(c, Read("llms"))
	assert.False(t, all)
	assert.Nil(t, ids)

	// No Context: the admin-or-not rule.
	c = newCtx()
	c.Set("user", adminFlag(true))
	assert.True(t, CanIn(c, 1, Write("llms")))
	assert.True(t, CanAnywhere(c, Write("llms")))
	all, _ = WorkspacesWith(c, Write("llms"))
	assert.True(t, all)
	c = newCtx()
	c.Set("user", adminFlag(false))
	assert.False(t, CanIn(c, 1, Read("llms")))
	assert.False(t, CanAnywhere(c, Read("llms")))
	all, _ = WorkspacesWith(c, Read("llms"))
	assert.False(t, all)
}

type adminFlag bool

func (a adminFlag) AdminFlag() bool { return bool(a) }
