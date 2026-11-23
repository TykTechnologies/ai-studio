package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
)

// A user whose only role is scoped to a Workspace could not fill the admin
// forms: the user, team, namespace, vendor and metadata lookups behind them
// needed their permission globally (midsommar-clean-6bg5.10). Each lookup now
// also answers a Workspace writer, with a reduced and filtered result.

// lookupFixture has two Workspaces with members and a caller; the lookup
// routes are served under the real /api/v1 paths, so the gate reads the
// permission annotations api.go registered for them.
type lookupFixture struct {
	*workspaceFixture
	t      *testing.T
	engine *gin.Engine
	grants authz.Grants
	caller *models.User
	users  map[string]*models.User
	teams  map[string]*models.Group
}

func newLookupFixture(t *testing.T) *lookupFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &lookupFixture{workspaceFixture: newWorkspaceFixture(t), t: t, users: map[string]*models.User{}, teams: map[string]*models.Group{}}
	a := f.api

	mkUser := func(name string) *models.User {
		u := models.NewUser()
		u.Name, u.Email, u.Password, u.EmailVerified = name, name+"@example.com", "hash", true
		u.APIKey = "key-" + name
		require.NoError(t, u.Create(f.db))
		f.users[name] = u
		return u
	}
	mkTeam := func(name string, members ...*models.User) *models.Group {
		g := &models.Group{Name: name}
		require.NoError(t, f.db.Create(g).Error)
		for _, m := range members {
			require.NoError(t, f.db.Exec("INSERT INTO user_groups (user_id, group_id) VALUES (?, ?)", m.ID, g.ID).Error)
		}
		f.teams[name] = g
		return g
	}
	role := &models.Role{Slug: "lookup-editor", Name: "Lookup editor"}
	require.NoError(t, f.db.Create(role).Error)
	bind := func(subjectType string, subjectID uint, ws uint) {
		b := &models.RoleBinding{SubjectType: subjectType, SubjectID: subjectID, RoleID: role.ID}
		if ws != 0 {
			b.ScopeType, b.ScopeID = rbac.ScopeTypeWorkspace, strconv.FormatUint(uint64(ws), 10)
		}
		require.NoError(t, f.db.Create(b).Error)
	}

	f.caller = mkUser("caller")
	bind(models.RoleBindingSubjectUser, f.caller.ID, f.w1)
	bind(models.RoleBindingSubjectUser, mkUser("alice-direct-w1").ID, f.w1)
	bob := mkUser("bob-team-w1")
	bind(models.RoleBindingSubjectGroup, mkTeam("team-w1", bob).ID, f.w1)
	bind(models.RoleBindingSubjectUser, mkUser("carol-direct-w2").ID, f.w2)
	dan := mkUser("dan-team-w2")
	bind(models.RoleBindingSubjectGroup, mkTeam("team-w2", dan).ID, f.w2)
	bind(models.RoleBindingSubjectUser, mkUser("erin-global").ID, 0)
	frank := mkUser("frank-none")
	mkTeam("team-unbound", frank)
	bind(models.RoleBindingSubjectGroup, mkTeam("team-global", frank).ID, 0)

	f.engine = gin.New()
	g := f.engine.Group("/api/v1", func(c *gin.Context) {
		c.Set("user", f.caller)
		grants := f.grants
		authz.WithContext(c, authz.NewGrantsContext(f.caller.ID, false, func() (authz.Grants, error) { return grants, nil }))
	}, a.requireRoutePermission())
	g.GET("/users", a.usersLookup(a.listUsers))
	g.GET("/users/:id", a.userLookup(a.getUser))
	g.GET("/users/:id/groups", a.getUserGroups)
	g.GET("/groups", a.groupsLookup(a.listGroups))
	g.GET("/groups/:id", a.getGroup)
	g.GET("/namespaces", a.namespacesLookup(a.listNamespaces))
	g.GET("/vendors/llm-drivers", a.getAvailableLLMDrivers)
	g.GET("/vendors/embedders", a.getAvailableEmbedders)
	g.GET("/vendors/vector-stores", a.getAvailableVectorStores)
	g.GET("/metadata/schemas/resolve", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	g.GET("/metadata/vocabularies", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	g.GET("/metadata/schemas", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return f
}

func scoped(ws uint, perms ...authz.Permission) authz.Grants {
	return authz.Grants{Global: authz.NewSet(), Workspaces: map[uint]authz.Set{ws: authz.NewSet(perms...)}}
}

func (f *lookupFixture) get(g authz.Grants, path string) *httptest.ResponseRecorder {
	f.grants = g
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

type lookupRow struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Attributes map[string]interface{} `json:"attributes"`
}

func (f *lookupFixture) rows(w *httptest.ResponseRecorder) []lookupRow {
	f.t.Helper()
	require.Equal(f.t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data []lookupRow `json:"data"`
	}
	require.NoError(f.t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Data
}

func names(rows []lookupRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i], _ = r.Attributes["name"].(string)
	}
	return out
}

func TestWorkspaceLookups_Users(t *testing.T) {
	f := newLookupFixture(t)
	editorW1 := scoped(f.w1, authz.Write("apps"), authz.Read("apps"), authz.Read("users"), authz.Write("users"))

	// Filtered: the caller and the members of W1, directly or through a team.
	rows := f.rows(f.get(editorW1, "/api/v1/users?all=true&sort=name"))
	assert.Equal(t, []string{"alice-direct-w1", "bob-team-w1", "caller"}, names(rows))
	// Reduced: id, name and email, nothing else.
	for _, r := range rows {
		assert.Equal(t, "users", r.Type)
		assert.ElementsMatch(t, []string{"name", "email"}, keysOf(r.Attributes))
	}
	body := f.get(editorW1, "/api/v1/users?all=true").Body.String()
	for _, leak := range []string{"key-", "is_admin", "roles", "groups", "carol", "dan-", "erin", "frank"} {
		assert.NotContains(t, body, leak)
	}

	// datasources:write opens it too; a Workspace role without either does not.
	assert.Len(t, f.rows(f.get(scoped(f.w1, authz.Write("datasources")), "/api/v1/users?all=true")), 3)
	for name, g := range map[string]authz.Grants{
		"viewer in a Workspace":          scoped(f.w1, authz.Read("apps"), authz.Read("datasources"), authz.Read("users")),
		"another Workspace write":        scoped(f.w1, authz.Write("llms")),
		"no role":                        authz.GlobalGrants(authz.NewSet()),
		"users:read only in a Workspace": scoped(f.w1, authz.Read("users")),
	} {
		assert.Equal(t, http.StatusForbidden, f.get(g, "/api/v1/users").Code, name)
		assert.Equal(t, http.StatusForbidden, f.get(g, fmt.Sprintf("/api/v1/users/%d", f.caller.ID)).Code, name)
	}

	// Another Workspace's writer sees that Workspace's members, not W1's.
	assert.Equal(t, []string{"caller", "carol-direct-w2", "dan-team-w2"},
		names(f.rows(f.get(scoped(f.w2, authz.Write("apps")), "/api/v1/users?all=true&sort=name"))))

	// Search, sort and paging work on the reduced set only.
	assert.Equal(t, []string{"bob-team-w1"}, names(f.rows(f.get(editorW1, "/api/v1/users?search=BOB"))))
	assert.Empty(t, f.rows(f.get(editorW1, "/api/v1/users?search=carol")))
	assert.Equal(t, []string{"caller", "bob-team-w1", "alice-direct-w1"}, names(f.rows(f.get(editorW1, "/api/v1/users?all=true&sort=-email"))))
	w := f.get(editorW1, "/api/v1/users?page=2&page_size=2&sort=name")
	assert.Equal(t, []string{"caller"}, names(f.rows(w)))
	assert.Equal(t, "3", w.Header().Get("X-Total-Count"))
	assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

	// No parameter widens or probes the set: the full route's filters and an
	// unknown sort column are ignored.
	for _, q := range []string{"has_api_key=true", "disabled=true", "account_type=service", "auth_source=x",
		"exclude_group_id=1", "workspace_id=" + strconv.Itoa(int(f.w2)), "sort=api_key", "sort=password", "scope=global"} {
		assert.Len(t, f.rows(f.get(editorW1, "/api/v1/users?all=true&"+q)), 3, q)
	}

	// By id: a visible user reduced, anyone else 404 with the body of a
	// missing user.
	missing := f.get(editorW1, "/api/v1/users/999999")
	require.Equal(t, http.StatusNotFound, missing.Code)
	for _, name := range []string{"caller", "alice-direct-w1", "bob-team-w1"} {
		w := f.get(editorW1, fmt.Sprintf("/api/v1/users/%d", f.users[name].ID))
		require.Equal(t, http.StatusOK, w.Code, name)
		assert.JSONEq(t, fmt.Sprintf(`{"data":{"type":"users","id":"%d","attributes":{"name":%q,"email":%q}}}`,
			f.users[name].ID, name, name+"@example.com"), w.Body.String())
	}
	for _, name := range []string{"carol-direct-w2", "dan-team-w2", "erin-global", "frank-none"} {
		w := f.get(editorW1, fmt.Sprintf("/api/v1/users/%d", f.users[name].ID))
		assert.Equal(t, http.StatusNotFound, w.Code, name)
		assert.Equal(t, missing.Body.String(), w.Body.String(), "hidden reads exactly as missing")
	}

	// Sub-routes stay global-only.
	assert.Equal(t, http.StatusForbidden, f.get(editorW1, fmt.Sprintf("/api/v1/users/%d/groups", f.caller.ID)).Code)
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The holder of the global permission is served by the unchanged handler.
func TestWorkspaceLookups_GlobalHolderGetsTheFullAnswer(t *testing.T) {
	f := newLookupFixture(t)
	direct := func(h gin.HandlerFunc, path string) string {
		e := gin.New()
		e.GET("/api/v1/users", func(c *gin.Context) {
			c.Set("user", f.caller)
			g := authz.GlobalGrants(authz.NewSet(authz.Read("users")))
			authz.WithContext(c, authz.NewGrantsContext(f.caller.ID, false, func() (authz.Grants, error) { return g, nil }))
		}, h)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Body.String()
	}
	reader := authz.GlobalGrants(authz.NewSet(authz.Read("users")))
	w := f.get(reader, "/api/v1/users?all=true&sort=name")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, direct(f.api.listUsers, "/api/v1/users?all=true&sort=name"), w.Body.String(), "byte for byte the old answer")
	assert.Len(t, f.rows(w), 9, "every user, the seeded member and owner included")
	assert.Contains(t, w.Body.String(), "erin-global")

	// A global reader who also holds a Workspace write still gets the full answer.
	both := authz.Grants{Global: authz.NewSet(authz.Read("users")), Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Write("apps"))}}
	assert.Equal(t, w.Body.String(), f.get(both, "/api/v1/users?all=true&sort=name").Body.String())

	// A global apps:write without users:read: the reduced answer over every Workspace's members.
	writer := authz.GlobalGrants(authz.NewSet(authz.Write("apps")))
	assert.Equal(t, []string{"alice-direct-w1", "bob-team-w1", "caller", "carol-direct-w2", "dan-team-w2"},
		names(f.rows(f.get(writer, "/api/v1/users?all=true&sort=name"))))
}

func TestWorkspaceLookups_Groups(t *testing.T) {
	f := newLookupFixture(t)
	editorW1 := scoped(f.w1, authz.Write("apps"), authz.Read("groups"))

	w := f.get(editorW1, "/api/v1/groups?all=true")
	rows := f.rows(w)
	assert.Equal(t, []string{"team-w1"}, names(rows))
	assert.JSONEq(t, fmt.Sprintf(`[{"type":"groups","id":"%d","attributes":{"name":"team-w1"}}]`, f.teams["team-w1"].ID), lookupJSON(t, rows))
	for _, leak := range []string{"team-w2", "team-unbound", "team-global", "bob", "user_count", "catalogue"} {
		assert.NotContains(t, w.Body.String(), leak)
	}
	assert.Empty(t, f.rows(f.get(editorW1, "/api/v1/groups?search=team-w2")))
	assert.Equal(t, []string{"team-w2"}, names(f.rows(f.get(scoped(f.w2, authz.Write("apps")), "/api/v1/groups"))))

	// datasources:write does not need teams; a viewer gets nothing; sub-routes stay closed.
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Write("datasources")), "/api/v1/groups").Code)
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Read("apps"), authz.Read("groups")), "/api/v1/groups").Code)
	assert.Equal(t, http.StatusForbidden, f.get(editorW1, fmt.Sprintf("/api/v1/groups/%d", f.teams["team-w1"].ID)).Code)

	// The global holder gets the full list (its shape differs per edition).
	full := f.get(authz.GlobalGrants(authz.NewSet(authz.Read("groups"))), "/api/v1/groups?all=true")
	require.Equal(t, http.StatusOK, full.Code, full.Body.String())
	assert.Contains(t, full.Body.String(), "user_count")
}

func lookupJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestWorkspaceLookups_NamespacesVendorsAndMetadata(t *testing.T) {
	f := newLookupFixture(t)
	require.NoError(t, f.db.Create(&models.LLM{Name: "ns-llm", Vendor: models.OPENAI, Namespace: "payments"}).Error)

	// Namespaces: names only for a Workspace writer.
	w := f.get(scoped(f.w1, authz.Write("apps")), "/api/v1/namespaces")
	rows := f.rows(w)
	assert.ElementsMatch(t, []string{"global", "payments"}, names(rows))
	for _, r := range rows {
		assert.Equal(t, []string{"name"}, keysOf(r.Attributes))
	}
	for _, leak := range []string{"edge_count", "llm_count", "app_count", "token_count", "licensed"} {
		assert.NotContains(t, w.Body.String(), leak)
	}
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Read("apps"), authz.Read("edges")), "/api/v1/namespaces").Code)
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Write("secrets")), "/api/v1/namespaces").Code)

	// Vendor catalogues: the same permission, held in any Workspace.
	assert.Equal(t, http.StatusOK, f.get(scoped(f.w1, authz.Read("datasources")), "/api/v1/vendors/embedders").Code)
	assert.Equal(t, http.StatusOK, f.get(scoped(f.w1, authz.Read("datasources")), "/api/v1/vendors/vector-stores").Code)
	assert.Equal(t, http.StatusOK, f.get(scoped(f.w1, authz.Read("llms")), "/api/v1/vendors/llm-drivers").Code)
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Read("llms")), "/api/v1/vendors/embedders").Code)
	assert.Equal(t, http.StatusForbidden, f.get(authz.GlobalGrants(authz.NewSet()), "/api/v1/vendors/llm-drivers").Code)

	// The resolved metadata schema: only for the writer of the object type asked for.
	llmWriter := scoped(f.w1, authz.Write("llms"))
	assert.Equal(t, http.StatusNoContent, f.get(llmWriter, "/api/v1/metadata/schemas/resolve?object_type=llm").Code)
	assert.Equal(t, http.StatusForbidden, f.get(llmWriter, "/api/v1/metadata/schemas/resolve?object_type=tool").Code)
	assert.Equal(t, http.StatusForbidden, f.get(llmWriter, "/api/v1/metadata/schemas/resolve?object_type=plugin_resource").Code)
	assert.Equal(t, http.StatusForbidden, f.get(llmWriter, "/api/v1/metadata/schemas/resolve").Code)
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Read("llms"), authz.Read("metadata")), "/api/v1/metadata/schemas/resolve?object_type=llm").Code)
	assert.Equal(t, http.StatusNoContent, f.get(scoped(f.w1, authz.Write("datasources")), "/api/v1/metadata/schemas/resolve?object_type=datasource").Code)
	assert.Equal(t, http.StatusNoContent, f.get(authz.GlobalGrants(authz.NewSet(authz.Read("metadata"))), "/api/v1/metadata/schemas/resolve?object_type=plugin_resource").Code)
	assert.Equal(t, http.StatusNoContent, f.get(llmWriter, "/api/v1/metadata/vocabularies").Code)
	assert.Equal(t, http.StatusForbidden, f.get(scoped(f.w1, authz.Write("apps")), "/api/v1/metadata/vocabularies").Code)
	// The rest of the metadata API stays global-only.
	assert.Equal(t, http.StatusForbidden, f.get(llmWriter, "/api/v1/metadata/schemas").Code)
}
