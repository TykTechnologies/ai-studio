package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workspaceFixture is an API with TestMode off and three Workspaces:
// Default, W1 and W2.
type workspaceFixture struct {
	api         *API
	db          *gorm.DB
	def, w1, w2 uint
}

func newWorkspaceFixture(t *testing.T) *workspaceFixture {
	t.Helper()
	api, _, _ := setupAuthzAPI(t)
	db := api.service.DB
	def, err := api.service.DefaultWorkspaceID()
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	w2 := &models.Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)
	return &workspaceFixture{api: api, db: db, def: def, w1: w1.ID, w2: w2.ID}
}

// grantSets names the four standard callers.
func (f *workspaceFixture) callers(perms ...authz.Permission) map[string]authz.Grants {
	return map[string]authz.Grants{
		"global":   authz.GlobalGrants(authz.NewSet(perms...)),
		"scopedW1": {Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(perms...)}},
		"scopedW2": {Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w2: authz.NewSet(perms...)}},
		"none":     authz.GlobalGrants(authz.NewSet()),
	}
}

// ctx builds a request context under /api/v1 carrying the grants.
func (f *workspaceFixture) ctx(g *authz.Grants, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/zz?"+rawQuery, nil)
	if g != nil {
		grants := *g
		authz.WithContext(c, authz.NewGrantsContext(7, false, func() (authz.Grants, error) { return grants, nil }))
	}
	return c, w
}

func TestWorkspaceHelpers_RequireInWorkspace(t *testing.T) {
	f := newWorkspaceFixture(t)
	p := authz.Write("llms")
	want := map[string]map[uint]bool{
		"global":   {f.w1: true, f.w2: true, 0: true},
		"scopedW1": {f.w1: true, f.w2: false, 0: false},
		"scopedW2": {f.w1: false, f.w2: true, 0: false},
		"none":     {f.w1: false, f.w2: false, 0: false},
	}
	for name, g := range f.callers(p) {
		for ws, ok := range want[name] {
			g := g
			c, w := f.ctx(&g, "")
			assert.Equal(t, ok, f.api.requireInWorkspace(c, p, ws), "%s in %d", name, ws)
			if !ok {
				assert.Equal(t, http.StatusForbidden, w.Code)
				assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()))
			}
		}
	}

	// workspace_id 0 is the Default workspace (.41).
	g := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.def: authz.NewSet(p)}}
	c, _ := f.ctx(&g, "")
	assert.True(t, f.api.requireInWorkspace(c, p, 0))
	c, _ = f.ctx(&g, "")
	assert.True(t, f.api.requireInWorkspace(c, p, f.def))
}

func TestWorkspaceHelpers_RequireVisible(t *testing.T) {
	f := newWorkspaceFixture(t)
	notFound := func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"errors": []gin.H{{"title": "Not Found", "detail": "App not found"}}})
	}
	for name, g := range f.callers(authz.Read("apps")) {
		g := g
		c, w := f.ctx(&g, "")
		ok := f.api.requireVisible(c, "apps", f.w2, notFound)
		visible := name == "global" || name == "scopedW2"
		assert.Equal(t, visible, ok, name)
		if !visible {
			assert.Equal(t, http.StatusNotFound, w.Code, "hidden, not forbidden")
			assert.JSONEq(t, `{"errors":[{"title":"Not Found","detail":"App not found"}]}`, w.Body.String(),
				"the handler's own not-found body")
		}
	}

	// Write implies read inside a Workspace.
	g := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Write("apps"))}}
	c, _ := f.ctx(&g, "")
	assert.True(t, f.api.requireVisible(c, "apps", f.w1, nil))

	// Read-global resources are always visible (D4); credentials follow the
	// parent App (pass the App's Workspace).
	none := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Read("llms"))}}
	c, _ = f.ctx(&none, "")
	assert.True(t, f.api.requireVisible(c, "llms", f.w2, nil))
	creds := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Read("credentials"))}}
	c, w := f.ctx(&creds, "")
	assert.False(t, f.api.requireVisible(c, "credentials", f.w2, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWorkspaceHelpers_ListScope(t *testing.T) {
	f := newWorkspaceFixture(t)
	// The 0 row stands for one an older build wrote.
	for i, ws := range []uint{0, f.def, f.w1, f.w2} {
		require.NoError(t, f.db.Create(&models.App{Name: fmt.Sprintf("app-%d", i), WorkspaceID: ws}).Error)
	}
	names := func(g authz.Grants, query string) []uint {
		c, _ := f.ctx(&g, query)
		var apps []models.App
		q := f.db.Model(&models.App{}).Scopes(f.api.workspaceListScope(c, "apps", ""))
		filter, ok := f.api.workspaceFilterParam(c, "")
		require.True(t, ok)
		require.NoError(t, q.Scopes(filter).Order("id").Find(&apps).Error)
		var out []uint
		for _, a := range apps {
			out = append(out, a.WorkspaceID)
		}
		return out
	}
	callers := f.callers(authz.Read("apps"))
	assert.Equal(t, []uint{0, f.def, f.w1, f.w2}, names(callers["global"], ""))
	assert.Equal(t, []uint{f.w1}, names(callers["scopedW1"], ""))
	assert.Nil(t, names(callers["none"], ""))
	onDefault := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.def: authz.NewSet(authz.Read("apps"))}}
	assert.Equal(t, []uint{0, f.def}, names(onDefault, ""), "a 0 row is a Default row (.41)")

	// ?workspace_id= narrows, never widens.
	assert.Equal(t, []uint{f.w2}, names(callers["global"], "workspace_id="+uintStr(f.w2)))
	assert.Nil(t, names(callers["scopedW1"], "workspace_id="+uintStr(f.w2)))
	assert.Equal(t, []uint{0, f.def}, names(callers["global"], "workspace_id="+uintStr(f.def)))

	// Read-global resources are not scoped.
	c, _ := f.ctx(&authz.Grants{Global: authz.NewSet()}, "")
	var n int64
	require.NoError(t, f.db.Model(&models.App{}).Scopes(f.api.workspaceListScope(c, "llms", "")).Count(&n).Error)
	assert.EqualValues(t, 4, n)

	// A malformed filter is a 400.
	c, w := f.ctx(&authz.Grants{Global: authz.Wildcard()}, "workspace_id=abc")
	_, ok := f.api.workspaceFilterParam(c, "")
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func uintStr(v uint) string { return strconv.FormatUint(uint64(v), 10) }

func TestWorkspaceHelpers_ResolveCreateWorkspace(t *testing.T) {
	f := newWorkspaceFixture(t)
	p := authz.Write("llms")
	ptr := func(v uint) *uint { return &v }

	type res struct {
		ws     uint
		ok     bool
		status int
		code   string
	}
	run := func(g authz.Grants, requested *uint) res {
		c, w := f.ctx(&g, "")
		ws, ok := f.api.resolveCreateWorkspace(c, "llms", requested)
		r := res{ws: ws, ok: ok, status: w.Code}
		if !ok {
			r.code = decodeErrorCode(t, w.Body.Bytes())
		}
		return r
	}
	callers := f.callers(p)

	// Landing rule (D5).
	assert.Equal(t, res{ws: f.def, ok: true, status: 200}, run(callers["global"], nil))
	assert.Equal(t, res{ws: f.w1, ok: true, status: 200}, run(callers["scopedW1"], nil))
	both := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(p), f.w2: authz.NewSet(p)}}
	r := run(both, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	assert.Equal(t, authz.CodeWorkspaceRequired, r.code)
	assert.Equal(t, http.StatusForbidden, run(callers["none"], nil).status)

	// Requested Workspace.
	assert.Equal(t, res{ws: f.w2, ok: true, status: 200}, run(callers["global"], ptr(f.w2)))
	assert.Equal(t, res{ws: f.w2, ok: true, status: 200}, run(both, ptr(f.w2)))
	uncovered := run(callers["scopedW1"], ptr(f.w2))
	missing := run(callers["global"], ptr(f.w2+1000))
	assert.Equal(t, http.StatusUnprocessableEntity, uncovered.status)
	assert.Equal(t, authz.CodeInvalidWorkspace, uncovered.code)

	// Identical body for a missing and an uncovered Workspace (.37).
	bodyOf := func(g authz.Grants, requested uint) string {
		c, w := f.ctx(&g, "")
		f.api.resolveCreateWorkspace(c, "llms", &requested)
		return w.Body.String()
	}
	assert.Equal(t, missing.status, uncovered.status)
	assert.Equal(t, bodyOf(callers["global"], f.w2+1000), bodyOf(callers["scopedW1"], f.w2))
	assert.Equal(t, http.StatusUnprocessableEntity, run(callers["global"], ptr(0)).status)
}

func TestWorkspaceHelpers_RequireMove(t *testing.T) {
	f := newWorkspaceFixture(t)
	p := authz.Write("apps")
	callers := f.callers(p)
	both := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(p), f.w2: authz.NewSet(p)}}
	move := func(g authz.Grants, from, to uint) (bool, int) {
		c, w := f.ctx(&g, "")
		ok := f.api.requireMove(c, "apps", from, to)
		return ok, w.Code
	}
	ok, _ := move(callers["global"], f.w1, f.w2)
	assert.True(t, ok)
	ok, _ = move(both, f.w1, f.w2)
	assert.True(t, ok, "write covers both")
	ok, code := move(callers["scopedW1"], f.w1, f.w2)
	assert.False(t, ok, "write covers the source only")
	assert.Equal(t, http.StatusUnprocessableEntity, code)
	ok, code = move(callers["scopedW2"], f.w1, f.w2)
	assert.False(t, ok, "write covers the destination only")
	assert.Equal(t, http.StatusForbidden, code)
	ok, _ = move(callers["scopedW1"], f.w1, f.w1)
	assert.True(t, ok, "no move")
	onDefault := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.def: authz.NewSet(p)}}
	ok, _ = move(onDefault, 0, f.def)
	assert.True(t, ok, "0 is the Default: not a move")
	ok, code = move(callers["global"], f.w1, f.w2+1000)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnprocessableEntity, code)
}

func TestWorkspaceHelpers_PublishIn(t *testing.T) {
	f := newWorkspaceFixture(t)
	p := authz.Publish("llms")
	callers := f.callers(p)
	g := callers["scopedW1"]
	c, _ := f.ctx(&g, "")
	assert.True(t, f.api.requirePublishIfChangedIn(c, "llms", f.w1, false, true))
	c, w := f.ctx(&g, "")
	assert.False(t, f.api.requirePublishIfChangedIn(c, "llms", f.w2, false, true), "publish in W1 never releases a W2 object")
	assert.Equal(t, http.StatusForbidden, w.Code)
	c, _ = f.ctx(&g, "")
	assert.True(t, f.api.requirePublishIfChangedIn(c, "llms", f.w2, true, true), "unchanged switch")
	c, _ = f.ctx(&g, "")
	assert.True(t, f.api.requirePublishToCreateLiveIn(c, "llms", f.w2, false))
	c, _ = f.ctx(&g, "")
	assert.False(t, f.api.requirePublishToCreateLiveIn(c, "llms", f.w2, true))
}

func TestWorkspaceHelpers_ModesWithoutContext(t *testing.T) {
	f := newWorkspaceFixture(t)

	// Under /api/v1 with no context: fail closed.
	c, w := f.ctx(nil, "")
	assert.False(t, f.api.requireInWorkspace(c, authz.Write("llms"), f.w1))
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Marked by the route gate, any path: fail closed.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/common/zz", nil)
	c.Set(authzGatedKey, true)
	assert.False(t, f.api.visibleIn(c, "apps", f.w1))

	// Legacy caller outside /api/v1 and the gate: allowed, like holds.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/common/zz", nil)
	assert.True(t, f.api.requireInWorkspace(c, authz.Write("llms"), f.w1))
	assert.True(t, f.api.visibleIn(c, "apps", f.w2))

	// TestMode: every helper is a no-op.
	f.api.config.TestMode = true
	t.Cleanup(func() { f.api.config.TestMode = false })
	c, _ = f.ctx(nil, "")
	assert.True(t, f.api.requireInWorkspace(c, authz.Write("llms"), f.w1))
	ws, ok := f.api.resolveCreateWorkspace(c, "llms", nil)
	assert.True(t, ok)
	assert.Equal(t, f.def, ws)
}

// Community Edition: grants are global only, so the helpers reduce to the
// admin-or-not rule.
func TestWorkspaceHelpers_GlobalOnlyGrantsAreAdminOrNot(t *testing.T) {
	f := newWorkspaceFixture(t)
	admin := authz.GlobalGrants(authz.Wildcard())
	nobody := authz.GlobalGrants(authz.NewSet())
	for _, ws := range []uint{0, f.def, f.w1} {
		c, _ := f.ctx(&admin, "")
		assert.True(t, f.api.requireInWorkspace(c, authz.Write("llms"), ws))
		c, _ = f.ctx(&nobody, "")
		assert.False(t, f.api.requireInWorkspace(c, authz.Write("llms"), ws))
	}
	c, _ := f.ctx(&admin, "")
	ws, ok := f.api.resolveCreateWorkspace(c, "apps", nil)
	assert.True(t, ok)
	assert.Equal(t, f.def, ws)
}

func TestLockWorkspaceForInsert(t *testing.T) {
	f := newWorkspaceFixture(t)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return services.LockWorkspaceForInsert(tx, f.w1)
	}))
	err := f.db.Transaction(func(tx *gorm.DB) error {
		return services.LockWorkspaceForInsert(tx, f.w2+1000)
	})
	assert.True(t, isWorkspaceNotFound(err))
	assert.True(t, isWorkspaceNotFound(services.LockWorkspaceForInsert(f.db, 0)))
}

// The route gate (.37): a Workspace-checked route admits a permission held
// in any Workspace; every other route needs it globally; AnyAdmin admits a
// caller who holds anything anywhere.
func TestRouteGate_WorkspaceChecked(t *testing.T) {
	f := newWorkspaceFixture(t)
	gin.SetMode(gin.TestMode)

	var grants authz.Grants
	engine := gin.New()
	user := &models.User{ID: 7, Email: "probe@tyk.io"}
	group := engine.Group("/api/v1/zz", func(c *gin.Context) {
		c.Set("user", user)
		g := grants
		authz.WithContext(c, authz.NewGrantsContext(user.ID, false, func() (authz.Grants, error) { return g, nil }))
	}, f.api.requireRoutePermission())
	pr := f.api.permGroup(group)
	reached := func(c *gin.Context) {
		_, gated := c.Get(authzGatedKey)
		assert.True(t, gated, "the gate marks requests it passes")
		c.Status(http.StatusNoContent)
	}
	pr.InWorkspace().POST("/llms", authz.Write("llms"), reached)
	pr.POST("/llms-unmarked", authz.Write("llms"), reached)
	pr.POST("/users", authz.Write("users"), reached)
	pr.GET("/any", authz.AnyAdmin, reached)

	call := func(g authz.Grants, method, path string) int {
		grants = g
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w.Code
	}
	scopedLLMWrite := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Write("llms"))}}
	noLLM := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Read("apps"))}}

	assert.Equal(t, http.StatusNoContent, call(scopedLLMWrite, "POST", "/api/v1/zz/llms"), "scoped write reaches a Workspace-checked route")
	assert.Equal(t, http.StatusForbidden, call(noLLM, "POST", "/api/v1/zz/llms"), "no llms permission anywhere")
	assert.Equal(t, http.StatusForbidden, call(scopedLLMWrite, "POST", "/api/v1/zz/llms-unmarked"), "unmarked routes fail closed for scoped callers")
	assert.Equal(t, http.StatusNoContent, call(authz.GlobalGrants(authz.NewSet(authz.Write("llms"))), "POST", "/api/v1/zz/llms-unmarked"))
	assert.Equal(t, http.StatusForbidden, call(scopedLLMWrite, "POST", "/api/v1/zz/users"))
	hostile := authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Write("users"))}}
	assert.Equal(t, http.StatusForbidden, call(hostile, "POST", "/api/v1/zz/users"), "a global permission in a Workspace set grants nothing")
	assert.Equal(t, http.StatusNoContent, call(noLLM, "GET", "/api/v1/zz/any"), "a scoped-only user is on the admin surface")
	assert.Equal(t, http.StatusForbidden, call(authz.GlobalGrants(authz.NewSet()), "GET", "/api/v1/zz/any"))
}
