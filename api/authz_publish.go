package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
)

// Publish is the workflow verb of the permission model: a route annotated
// with write lets a role create and edit an object, but flipping the object's
// live switch (active, enabled) additionally needs "<resource>:publish".
//
// Update handlers call requirePublishIfChanged with the stored and requested
// values of the switch; create handlers call requirePublishToCreateLive. Both
// answer 403 with the standard authz envelope (code permission_denied and the
// missing permission) so the UI's typed error path and the Enterprise audit
// denial recording behave exactly as for a route-level denial.
//
// TestMode and requests without an authorization context (legacy callers,
// handler tests) are allowed through, mirroring requireRoutePermission and
// revealAPIKeys.

// canPublish reports whether the caller may flip the live switch of resource.
func (a *API) canPublish(c *gin.Context, resource string) bool {
	if a.config != nil && a.config.TestMode {
		return true
	}
	if _, ok := authz.FromContext(c); !ok {
		return true
	}
	return authz.Can(c, authz.Publish(resource))
}

// requirePublishIfChanged returns false, having written a 403, when the
// request changes the live switch and the caller lacks publish. An unchanged
// switch never needs publish, so a writer can keep editing a live object.
func (a *API) requirePublishIfChanged(c *gin.Context, resource string, before, after bool) bool {
	if before == after {
		return true
	}
	return a.requirePublish(c, resource)
}

// requirePublishToCreateLive returns false, having written a 403, when a new
// object is being created already live and the caller lacks publish.
func (a *API) requirePublishToCreateLive(c *gin.Context, resource string, live bool) bool {
	if !live {
		return true
	}
	return a.requirePublish(c, resource)
}

// requirePublish is the unconditional form used by the dedicated
// activate/deactivate routes' handlers when they are shared with a write path.
func (a *API) requirePublish(c *gin.Context, resource string) bool {
	if a.canPublish(c, resource) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Publish(resource)))
	return false
}
