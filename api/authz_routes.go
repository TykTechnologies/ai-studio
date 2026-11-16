package api

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// permEntry is what the route registry stores per METHOD+path: a fixed
// permission, a fixed any-of list, or a per-request resolver (used where the
// resource is derived from a path parameter, e.g. governed metadata on an
// object, or the per-plugin permission of a plugin route). A request passes
// when the caller holds any one of the resolved permissions.
type permEntry struct {
	perm       authz.Permission
	anyOf      []authz.Permission
	resolve    func(*gin.Context) authz.Permission
	resolveAny func(*gin.Context) []authz.Permission
	// inWorkspace marks a Workspace-checked route: its handler checks the
	// object's Workspace (authz_workspace.go), so the route gate lets a
	// permission on a Workspace resource through when it is held in any
	// Workspace. Every other route needs the permission globally (.37).
	inWorkspace bool
}

// permission returns the first (primary) permission the route needs; it is
// what a denial reports.
func (e permEntry) permission(c *gin.Context) authz.Permission {
	ps := e.permissions(c)
	if len(ps) == 0 {
		return authz.FullAdmin
	}
	return ps[0]
}

// permissions returns every permission that satisfies the route.
func (e permEntry) permissions(c *gin.Context) []authz.Permission {
	switch {
	case e.resolveAny != nil:
		return e.resolveAny(c)
	case e.resolve != nil:
		return []authz.Permission{e.resolve(c)}
	case len(e.anyOf) > 0:
		return e.anyOf
	default:
		return []authz.Permission{e.perm}
	}
}

// dynamic reports whether the entry's permissions are computed per request
// (and therefore cannot be validated at registration time).
func (e permEntry) dynamic() bool { return e.resolve != nil || e.resolveAny != nil }

// routeKey builds the registry key gin's router will produce for a route:
// "METHOD /absolute/path". It follows gin's joinPaths so the completeness
// test can cross-check the registry against engine.Routes().
func routeKey(method, absolutePath string) string {
	return method + " " + absolutePath
}

func joinRoutePath(base, rel string) string {
	if rel == "" {
		return base
	}
	final := path.Join(base, rel)
	if strings.HasSuffix(rel, "/") && !strings.HasSuffix(final, "/") {
		final += "/"
	}
	return final
}

// permRouter wraps a gin RouterGroup so every route registered through it
// carries a permission annotation. Annotations are recorded on the API so the
// enforcing middleware can look them up by the matched route, and so a test
// can prove no route under /api/v1 is left unannotated.
type permRouter struct {
	g   *gin.RouterGroup
	api *API
	// inWorkspace: routes registered through this router are
	// Workspace-checked; see InWorkspace.
	inWorkspace bool
}

func (a *API) permGroup(g *gin.RouterGroup) *permRouter {
	if a.routePerms == nil {
		a.routePerms = map[string]permEntry{}
	}
	return &permRouter{g: g, api: a}
}

// Raw exposes the underlying group for Use() and for the rare route that is
// registered outside the annotation scheme.
func (r *permRouter) Raw() *gin.RouterGroup { return r.g }

// Group creates an annotated sub-group sharing the same registry.
func (r *permRouter) Group(rel string, mw ...gin.HandlerFunc) *permRouter {
	return &permRouter{g: r.g.Group(rel, mw...), api: r.api, inWorkspace: r.inWorkspace}
}

// InWorkspace returns a router whose routes are registered as
// Workspace-checked: v1.InWorkspace().PATCH("/llms/:id", authz.Write("llms"), h).
// Mark a route only once its handler applies the object checks of
// authz_workspace.go (requireVisible, requireInWorkspace, workspaceListScope,
// resolveCreateWorkspace, requireMove) to every object it touches; the gate
// then admits callers who hold the permission in any Workspace. A fixed
// permission on a marked route must be on a Workspace resource.
func (r *permRouter) InWorkspace() *permRouter {
	return &permRouter{g: r.g, api: r.api, inWorkspace: true}
}

// Handle registers a route with a fixed permission. It panics if the
// permission is not in the catalogue so a typo fails at boot and in tests.
func (r *permRouter) Handle(method, rel string, p authz.Permission, h ...gin.HandlerFunc) {
	if !p.Valid() {
		panic(fmt.Sprintf("authz: route %s %s annotated with unknown permission %q", method, joinRoutePath(r.g.BasePath(), rel), p))
	}
	r.record(method, rel, permEntry{perm: p})
	r.g.Handle(method, rel, h...)
}

// HandleFn registers a route whose permission depends on the request.
func (r *permRouter) HandleFn(method, rel string, resolve func(*gin.Context) authz.Permission, h ...gin.HandlerFunc) {
	r.record(method, rel, permEntry{resolve: resolve})
	r.g.Handle(method, rel, h...)
}

// HandleAny registers a route open to the holder of any one of the listed
// permissions. The first is the primary one a denial reports.
func (r *permRouter) HandleAny(method, rel string, perms []authz.Permission, h ...gin.HandlerFunc) {
	if len(perms) == 0 {
		panic(fmt.Sprintf("authz: route %s %s has an empty any-of list", method, joinRoutePath(r.g.BasePath(), rel)))
	}
	for _, p := range perms {
		if !p.Valid() {
			panic(fmt.Sprintf("authz: route %s %s annotated with unknown permission %q", method, joinRoutePath(r.g.BasePath(), rel), p))
		}
	}
	r.record(method, rel, permEntry{anyOf: perms})
	r.g.Handle(method, rel, h...)
}

// HandleAnyFn registers a route whose any-of permission list depends on
// the request (e.g. the platform-level plugins permission or the per-plugin
// one of the plugin named in the path).
func (r *permRouter) HandleAnyFn(method, rel string, resolve func(*gin.Context) []authz.Permission, h ...gin.HandlerFunc) {
	r.record(method, rel, permEntry{resolveAny: resolve})
	r.g.Handle(method, rel, h...)
}

func (r *permRouter) record(method, rel string, e permEntry) {
	key := routeKey(method, joinRoutePath(r.g.BasePath(), rel))
	if r.inWorkspace {
		e.inWorkspace = true
		if !e.dynamic() {
			for _, p := range e.permissions(nil) {
				if !authz.ScopeOf(p).InWorkspace() {
					panic(fmt.Sprintf("authz: route %s is Workspace-checked but %q is not a Workspace permission", key, p))
				}
			}
		}
	}
	if _, dup := r.api.routePerms[key]; dup {
		panic("authz: route registered twice: " + key)
	}
	r.api.routePerms[key] = e
}

func (r *permRouter) GET(rel string, p authz.Permission, h ...gin.HandlerFunc) {
	r.Handle("GET", rel, p, h...)
}
func (r *permRouter) POST(rel string, p authz.Permission, h ...gin.HandlerFunc) {
	r.Handle("POST", rel, p, h...)
}
func (r *permRouter) PUT(rel string, p authz.Permission, h ...gin.HandlerFunc) {
	r.Handle("PUT", rel, p, h...)
}
func (r *permRouter) PATCH(rel string, p authz.Permission, h ...gin.HandlerFunc) {
	r.Handle("PATCH", rel, p, h...)
}
func (r *permRouter) DELETE(rel string, p authz.Permission, h ...gin.HandlerFunc) {
	r.Handle("DELETE", rel, p, h...)
}

// routePermission returns the annotation for the matched route, if any: the
// full any-of list (the first is the primary permission) and whether the
// route is Workspace-checked.
func (a *API) routePermission(c *gin.Context) ([]authz.Permission, bool, bool) {
	e, ok := a.routePerms[routeKey(c.Request.Method, c.FullPath())]
	if !ok {
		return nil, false, false
	}
	return e.permissions(c), e.inWorkspace, true
}

// pluginForAuthz resolves the plugin named by the :id path parameter for the
// permission resolvers, from the in-memory cache the plugin permission sync
// maintains, falling back to the database for a plugin that has no admin
// surface (and therefore no cache entry). nil for an unknown plugin, so the
// caller falls back to the platform-level permission and the handler
// answers 404.
func (a *API) pluginForAuthz(c *gin.Context) *models.Plugin {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || a.service == nil || a.service.PluginService == nil {
		return nil
	}
	if info, ok := services.LookupPluginPermissionInfo(uint(id)); ok && info.Plugin != nil {
		return info.Plugin
	}
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil || plugin == nil {
		return nil
	}
	return plugin
}

// pluginPermissionKey resolves the plugin named by the :id path parameter to
// its permission key, or "" for an unknown plugin.
func (a *API) pluginPermissionKey(c *gin.Context) string {
	if plugin := a.pluginForAuthz(c); plugin != nil {
		return plugin.PermissionKey()
	}
	return ""
}

// pluginOrPlatformPermission resolves to [plugins:<action>, plugin:<key>:<action>]:
// the platform-level grant or the per-plugin one both open the route.
func (a *API) pluginOrPlatformPermission(action authz.Action) func(*gin.Context) []authz.Permission {
	return func(c *gin.Context) []authz.Permission {
		perms := []authz.Permission{authz.P("plugins", action)}
		if key := a.pluginPermissionKey(c); key != "" {
			perms = append(perms, authz.P(key, action))
		}
		return perms
	}
}

// pluginRPCPermission resolves an admin RPC call to the per-plugin
// permission of the method: what the manifest's rbac.rpc_methods declares
// for it, else the plugin's base write. plugins:execute holders pass through
// the umbrella rule in authz.Set.Has. A declared permission whose resource is
// not registered (manifest drift) falls back to the base write so a typo
// never opens a method.
func (a *API) pluginRPCPermission(c *gin.Context) authz.Permission {
	plugin := a.pluginForAuthz(c)
	if plugin == nil {
		return authz.Execute("plugins")
	}
	fallback := authz.P(plugin.PermissionKey(), authz.ActionWrite)
	declared := plugin.RPCMethodPermission(c.Param("method"))
	if declared == "" {
		return fallback
	}
	p := authz.Permission(services.ResolvePluginPermission(plugin, declared))
	if !p.Valid() {
		return fallback
	}
	return p
}

// metadataObjectPermission resolves governed-metadata-on-object routes to
// the permission of the object being annotated, so an editor who may change
// an LLM may also fill in its metadata without holding schema governance
// rights. Unknown object types fall back to the metadata resource itself.
func metadataObjectPermission(action authz.Action) func(*gin.Context) authz.Permission {
	objectResources := map[string]string{
		"llm":             "llms",
		"tool":            "tools",
		"datasource":      "datasources",
		"plugin_resource": "plugins",
	}
	return func(c *gin.Context) authz.Permission {
		if res, ok := objectResources[c.Param("object_type")]; ok {
			return authz.P(res, action)
		}
		return authz.P("metadata", action)
	}
}

// metadataObjectModels are the stamped objects whose governed metadata the
// object routes edit; onMetadataObject checks their Workspace.
var metadataObjectModels = map[string]struct {
	resource string
	model    func() interface{}
}{
	models.GovernedObjectTypeLLM:        {"llms", func() interface{} { return &models.LLM{} }},
	models.GovernedObjectTypeTool:       {"tools", func() interface{} { return &models.Tool{} }},
	models.GovernedObjectTypeDatasource: {"datasources", func() interface{} { return &models.Datasource{} }},
}

// onMetadataObject is route middleware for the governed-metadata object
// routes. A write on a stamped object (LLM, tool, datasource) needs the
// object's permission in its Workspace (requireOnObject: 403
// permission_denied). Reads stay read-global for these resources (D4), as
// on the objects' own routes. Other object types resolve to a global
// permission at the route gate (plugins, metadata) and pass here.
func (a *API) onMetadataObject(action authz.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, ok := metadataObjectModels[c.Param("object_type")]
		if !ok || action == authz.ActionRead {
			c.Next()
			return
		}
		id, err := strconv.ParseUint(c.Param("object_id"), 10, 32)
		if err == nil && !a.requireOnObject(c, target.model(), uint(id), authz.P(target.resource, action)) {
			return
		}
		c.Next()
	}
}
