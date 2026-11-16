package api

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
)

// rbacContext attaches a lazy grants resolver for the authenticated user
// so handlers (and the enforcing middleware) can ask authz.Can without any
// database work until something actually checks a permission. It is
// installed on every authenticated group; it never rejects a request.
func (a *API) rbacContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, ok := auth.UserFromContext(c); ok {
			svc := a.service.Authz()
			ctx := c.Request.Context()
			authz.WithContext(c, authz.NewGrantsContext(user.ID, user.IsAdmin, func() (authz.Grants, error) {
				return svc.ResolveGrants(ctx, user)
			}))
		}
		c.Next()
	}
}

// ssoConfigGuard protects identity provider configuration. With roles active
// the sso-profiles permission on each route is the whole rule; otherwise the
// legacy per-user AccessToSSOConfig flag applies as it always has.
func (a *API) ssoConfigGuard() gin.HandlerFunc {
	legacy := a.auth.SSOOnly()
	return func(c *gin.Context) {
		if a.service.Authz().Enabled() {
			c.Next()
			return
		}
		legacy(c)
	}
}

// requireRoutePermission enforces the permission a route was annotated with
// (see authz_routes.go). It replaces AdminOnly on the management API:
//
//   - Community Edition resolves admins to the wildcard and everyone else to
//     nothing, so the behaviour is exactly the old admin-or-not check.
//   - Enterprise Edition resolves the user's roles.
//
// A permission passes when it is held globally. On a Workspace-checked route
// (permRouter.InWorkspace) a permission on a Workspace resource also passes
// when it is held in any Workspace: that is only the capability gate, and
// the handler checks the object's Workspace. Every other route fails closed
// for a caller whose grant is scoped (.37). AnyAdmin passes for a caller who
// holds anything, globally or in a Workspace.
//
// Unannotated routes fail closed to full-administrator access and log once.
// TestMode short-circuits like the other auth middlewares so handler tests
// keep running without credentials. A request that passes is marked
// (authzGatedKey) so the Workspace helpers fail closed if it has no
// authorization context.
func (a *API) requireRoutePermission() gin.HandlerFunc {
	var warned sync.Map
	return func(c *gin.Context) {
		if a.config.TestMode {
			c.Next()
			return
		}
		user, ok := auth.UserFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, authz.Unauthenticated())
			return
		}
		key := routeKey(c.Request.Method, c.FullPath())
		perms, inWorkspace, annotated := a.routePermission(c)
		if !annotated || len(perms) == 0 {
			perms = []authz.Permission{authz.FullAdmin}
			if _, seen := warned.LoadOrStore(key, true); !seen {
				slog.Warn("authz: route has no permission annotation; requiring full administrator access", "route", key)
			}
		}
		perm := perms[0] // the primary permission, reported on denial
		grants, err := requestGrants(c)
		if err != nil {
			slog.Error("authz: could not resolve permissions; denying", "user", user.Email, "route", key, "error", err)
			c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
			return
		}
		if !routeGatePasses(grants, perms, inWorkspace) {
			slog.Warn("authz: permission denied", "user", user.Email, "route", key, "permission", string(perm))
			c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(perm))
			return
		}
		c.Set(authzGatedKey, true)
		c.Next()
	}
}

// requestGrants returns the request's resolved grants; empty with no
// authorization context.
func requestGrants(c *gin.Context) (authz.Grants, error) {
	x, ok := authz.FromContext(c)
	if !ok {
		return authz.GlobalGrants(authz.NewSet()), nil
	}
	return x.Grants()
}

// routeGatePasses is the route gate's rule; see requireRoutePermission.
func routeGatePasses(g authz.Grants, perms []authz.Permission, inWorkspace bool) bool {
	for _, p := range perms {
		switch {
		case g.Can(p):
			return true
		case p == authz.AnyAdmin && g.CanAnywhere(p):
			return true
		case inWorkspace && authz.ScopeOf(p).InWorkspace() && g.CanAnywhere(p):
			return true
		}
	}
	return false
}
