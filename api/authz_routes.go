package api

import (
	"fmt"
	"path"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
)

// permEntry is what the route registry stores per METHOD+path. Either a
// fixed permission or a per-request resolver (used where the resource is
// derived from a path parameter, e.g. governed metadata on an object).
type permEntry struct {
	perm    authz.Permission
	resolve func(*gin.Context) authz.Permission
}

func (e permEntry) permission(c *gin.Context) authz.Permission {
	if e.resolve != nil {
		return e.resolve(c)
	}
	return e.perm
}

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
	return &permRouter{g: r.g.Group(rel, mw...), api: r.api}
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

func (r *permRouter) record(method, rel string, e permEntry) {
	key := routeKey(method, joinRoutePath(r.g.BasePath(), rel))
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

// routePermission returns the annotation for the matched route, if any.
func (a *API) routePermission(c *gin.Context) (authz.Permission, bool) {
	e, ok := a.routePerms[routeKey(c.Request.Method, c.FullPath())]
	if !ok {
		return "", false
	}
	return e.permission(c), true
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
