package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// Lookups for someone who holds only Workspace roles (midsommar-clean-6bg5.10).
//
// The admin forms of Workspace objects need a few lists that belong to no
// Workspace: the users and teams an App or data source can be given to, the
// namespace names, the governed-metadata schema. Users, teams, edges and
// metadata are global resources, which a Workspace role never grants, so a
// Workspace-scoped Editor could not fill the forms the API let them submit.
//
// Each such route is opened one by one, never through the shared gate:
//
//   - the route's any-of permission list (lookupPerms) names the global
//     permission first, then the Workspace writes whose form needs the list;
//     registered through InWorkspace, the gate lets a Workspace write pass
//     when it is held in any Workspace;
//   - a wrapper (usersLookup, ...) serves the caller who holds the global
//     permission with the unchanged handler, byte for byte, and everyone
//     else with the reduced answer below.
//
// The reduced answers:
//
//   - users: the caller, and the members of the Workspaces where the caller
//     holds one of the route's writes. A member holds a role binding scoped
//     to the Workspace, directly or through a team. Fields: id, name, email.
//     Search and sort work on those fields only; every other filter of the
//     full route is ignored, so no parameter widens (or probes) the set.
//   - one user: the same set; anyone else is 404, as a missing user.
//   - teams: those holding a role binding scoped to one of those Workspaces.
//     Fields: id, name.
//   - namespaces: names only.
//
// Sub-routes and writes (/users/:id/*, /groups/:id/*) are not opened.

// lookupPerms resolves a lookup route's permission list: global first (it is
// what a denial reports), then the Workspace writes that also open it.
func lookupPerms(global authz.Permission, writes ...authz.Permission) func(*gin.Context) []authz.Permission {
	perms := append([]authz.Permission{global}, writes...)
	return func(*gin.Context) []authz.Permission { return perms }
}

var (
	usersLookupWrites      = []authz.Permission{authz.Write("apps"), authz.Write("datasources")}
	groupsLookupWrites     = []authz.Permission{authz.Write("apps")}
	namespacesLookupWrites = []authz.Permission{authz.Write("llms"), authz.Write("datasources"), authz.Write("apps"),
		authz.Write("tools"), authz.Write("filters"), authz.Write("model-routers"), authz.Write("semantic-routers")}
	metadataLookupWrites = []authz.Permission{authz.Write("llms"), authz.Write("tools"), authz.Write("datasources")}
)

// metadataResolvePerms opens GET /metadata/schemas/resolve to the writer of
// the object type it is asked for (the form that renders the schema), and
// to nobody else below metadata:read.
func metadataResolvePerms(c *gin.Context) []authz.Permission {
	perms := []authz.Permission{authz.Read("metadata")}
	switch strings.TrimSpace(c.Query("object_type")) {
	case models.GovernedObjectTypeLLM:
		perms = append(perms, authz.Write("llms"))
	case models.GovernedObjectTypeTool:
		perms = append(perms, authz.Write("tools"))
	case models.GovernedObjectTypeDatasource:
		perms = append(perms, authz.Write("datasources"))
	}
	return perms
}

// lookupScope says how a lookup answers this caller. full: the caller holds
// the global permission (or the request is not subject to permission checks:
// TestMode, a legacy caller), so the unchanged handler serves it. Otherwise
// the reduced answer applies to the Workspaces where the caller holds one of
// writes: every Workspace (all), or ids.
type lookupScope struct {
	full bool
	all  bool
	ids  []uint
}

func (a *API) lookupScopeOf(c *gin.Context, global authz.Permission, writes []authz.Permission) lookupScope {
	switch a.workspaceCheckMode(c) {
	case workspaceOpen:
		return lookupScope{full: true}
	case workspaceDeny:
		return lookupScope{}
	}
	if authz.Can(c, global) {
		return lookupScope{full: true}
	}
	x, ok := authz.FromContext(c)
	if !ok {
		return lookupScope{}
	}
	seen := map[uint]bool{}
	var scope lookupScope
	for _, p := range writes {
		all, ids := x.WorkspacesWith(p)
		if all {
			scope.all = true
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				scope.ids = append(scope.ids, id)
			}
		}
	}
	sort.Slice(scope.ids, func(i, j int) bool { return scope.ids[i] < scope.ids[j] })
	return scope
}

// workspaceBindingSubjects selects the ids of the subjects of one type that
// hold a role binding scoped to a Workspace of the scope.
func (a *API) workspaceBindingSubjects(scope lookupScope, subjectType string) *gorm.DB {
	q := a.service.DB.Model(&models.RoleBinding{}).Select("subject_id").
		Where("subject_type = ? AND scope_type = ?", subjectType, rbac.ScopeTypeWorkspace)
	if !scope.all {
		ids := make([]string, len(scope.ids))
		for i, id := range scope.ids {
			ids[i] = strconv.FormatUint(uint64(id), 10)
		}
		if len(ids) == 0 {
			return q.Where("1 = 0")
		}
		q = q.Where("scope_id IN ?", ids)
	}
	return q
}

// visibleUsers restricts a users query to the caller and the members of the
// scope's Workspaces.
func (a *API) visibleUsers(c *gin.Context, scope lookupScope) *gorm.DB {
	var callerID uint
	if user, ok := auth.UserFromContext(c); ok {
		callerID = user.ID
	}
	direct := a.workspaceBindingSubjects(scope, models.RoleBindingSubjectUser)
	teams := a.workspaceBindingSubjects(scope, models.RoleBindingSubjectGroup)
	viaTeam := a.service.DB.Table("user_groups").Select("user_id").Where("group_id IN (?)", teams)
	return a.service.DB.Model(&models.User{}).
		Where("(users.id = ? OR users.id IN (?) OR users.id IN (?))", callerID, direct, viaTeam)
}

// lookupUser and lookupGroup are the reduced forms: the agreed fields and
// nothing else.
func lookupUser(u *models.User) gin.H {
	return gin.H{
		"type":       "users",
		"id":         strconv.FormatUint(uint64(u.ID), 10),
		"attributes": gin.H{"name": u.Name, "email": u.Email},
	}
}

func lookupGroup(g *models.Group) gin.H {
	return gin.H{
		"type":       "groups",
		"id":         strconv.FormatUint(uint64(g.ID), 10),
		"attributes": gin.H{"name": g.Name},
	}
}

// lookupOrder is the ORDER BY for a ?sort= value: a listed column, optionally
// with a leading "-", else id. Nothing else reaches the query.
func lookupOrder(sortParam string, columns ...string) string {
	desc := strings.HasPrefix(sortParam, "-")
	name := strings.TrimPrefix(sortParam, "-")
	for _, col := range columns {
		if name == col {
			if desc {
				return col + " DESC, id ASC"
			}
			return col + " ASC, id ASC"
		}
	}
	return "id ASC"
}

// lookupPage applies ?page, ?page_size and ?all to q, sets the total headers
// and reports false after answering an error.
func lookupPage(c *gin.Context, q *gorm.DB) (*gorm.DB, int64, int, bool) {
	pageSize, pageNumber, all := getPaginationParams(c)
	if c.IsAborted() {
		return nil, 0, 0, false
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", "lookup failed")
		return nil, 0, 0, false
	}
	pages := 1
	if !all {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
		q = q.Limit(pageSize).Offset((pageNumber - 1) * pageSize)
	}
	c.Header("X-Total-Count", strconv.FormatInt(total, 10))
	c.Header("X-Total-Pages", strconv.Itoa(pages))
	return q, total, pages, true
}

// lookupSearch narrows q to rows whose listed columns contain ?search.
func lookupSearch(c *gin.Context, q *gorm.DB, columns ...string) *gorm.DB {
	term := strings.ToLower(strings.TrimSpace(c.Query("search")))
	if term == "" {
		return q
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term) + "%"
	conds := make([]string, len(columns))
	args := make([]interface{}, len(columns))
	for i, col := range columns {
		conds[i] = "LOWER(" + col + ") LIKE ? ESCAPE '\\'"
		args[i] = like
	}
	return q.Where("("+strings.Join(conds, " OR ")+")", args...)
}

// usersLookup wraps GET /users.
func (a *API) usersLookup(full gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := a.lookupScopeOf(c, authz.Read("users"), usersLookupWrites)
		if scope.full {
			full(c)
			return
		}
		q := lookupSearch(c, a.visibleUsers(c, scope), "users.name", "users.email")
		q, _, _, ok := lookupPage(c, q)
		if !ok {
			return
		}
		var users []models.User
		if err := q.Select("users.id", "users.name", "users.email").
			Order(lookupOrder(c.Query("sort"), "name", "email")).Find(&users).Error; err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", "lookup failed")
			return
		}
		data := make([]gin.H, len(users))
		for i := range users {
			data[i] = lookupUser(&users[i])
		}
		c.JSON(http.StatusOK, gin.H{"data": data})
	}
}

// userLookup wraps GET /users/:id.
func (a *API) userLookup(full gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := a.lookupScopeOf(c, authz.Read("users"), usersLookupWrites)
		if scope.full {
			full(c)
			return
		}
		notFound := func() {
			// The body getUser gives for a user that does not exist.
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "User not found"}},
			})
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			notFound()
			return
		}
		var user models.User
		if err := a.visibleUsers(c, scope).Select("users.id", "users.name", "users.email").
			Where("users.id = ?", uint(id)).First(&user).Error; err != nil {
			notFound()
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": lookupUser(&user)})
	}
}

// groupsLookup wraps GET /groups.
func (a *API) groupsLookup(full gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := a.lookupScopeOf(c, authz.Read("groups"), groupsLookupWrites)
		if scope.full {
			full(c)
			return
		}
		q := a.service.DB.Model(&models.Group{}).
			Where("groups.id IN (?)", a.workspaceBindingSubjects(scope, models.RoleBindingSubjectGroup))
		q = lookupSearch(c, q, "groups.name")
		q, total, pages, ok := lookupPage(c, q)
		if !ok {
			return
		}
		var groups []models.Group
		if err := q.Select("groups.id", "groups.name").
			Order(lookupOrder(c.Query("sort"), "name")).Find(&groups).Error; err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", "lookup failed")
			return
		}
		data := make([]gin.H, len(groups))
		for i := range groups {
			data[i] = lookupGroup(&groups[i])
		}
		c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"total_count": total, "total_pages": pages}})
	}
}

// namespacesLookup wraps GET /namespaces: the names, without the edge, LLM,
// App and token counts and the license flag of the full list.
func (a *API) namespacesLookup(full gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := a.lookupScopeOf(c, authz.Read("edges"), namespacesLookupWrites)
		if scope.full {
			full(c)
			return
		}
		namespaces, err := a.service.NamespaceService.ListNamespaces()
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", "lookup failed")
			return
		}
		data := make([]gin.H, len(namespaces))
		for i, ns := range namespaces {
			data[i] = gin.H{"type": "namespaces", "id": ns.Name, "attributes": gin.H{"name": ns.Name}}
		}
		c.JSON(http.StatusOK, gin.H{"data": data})
	}
}
