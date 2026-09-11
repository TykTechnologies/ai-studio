package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
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
	return a.holds(c, authz.Publish(resource))
}

// holds reports whether the caller holds p, treating TestMode and requests
// without an authorization context (legacy callers, handler tests) as
// allowed, exactly like the route middleware does.
func (a *API) holds(c *gin.Context, p authz.Permission) bool {
	if a.config != nil && a.config.TestMode {
		return true
	}
	if _, ok := authz.FromContext(c); !ok {
		return true
	}
	return authz.Can(c, p)
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

// publishGateOpen checks the governed-metadata publish gate (Enterprise):
// fields marked required_on_publish must be filled before an object goes
// live. objectID is "" for an object being created; pending holds the
// governed_metadata attribute of the request, if any, merged over the
// stored values. On failure it answers 422 with one entry per field, code
// required_on_publish, in the same envelope as other metadata validation
// errors, and returns false. Permission checks come first: this gate only
// runs for callers who may publish.
func (a *API) publishGateOpen(c *gin.Context, objectType, objectID string, pending *map[string]interface{}) bool {
	if !governed_metadata.IsEnterpriseAvailable() {
		return true
	}
	var values map[string]interface{}
	if pending != nil {
		values = *pending
	}
	result, err := a.governedMetadata().ValidateForPublish(c.Request.Context(), objectType, objectID, values)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return false
	}
	if result == nil || result.Valid {
		return true
	}
	writeMetadataValidationResponse(c, result)
	return false
}
