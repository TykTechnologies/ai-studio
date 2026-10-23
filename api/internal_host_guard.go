package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// requireAdminForInternalHost guards the admin-only "Allow internal host"
// switch (allow_internal_host, w4e) of an LLM, embedder, tool, datasource,
// guardrail or plugin. requested is the value in the request (nil: the
// field is absent and the stored value is kept); stored is the asset's
// current value (false for a new asset). A caller who is not an
// administrator (IsAdmin) may not change it: the request is answered 403
// and false is returned.
func (a *API) requireAdminForInternalHost(c *gin.Context, requested *bool, stored bool) bool {
	if requested == nil || *requested == stored {
		return true
	}
	if u := a.currentUser(c); u != nil && u.IsAdmin {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "only an administrator may change allow_internal_host"}},
	})
	return false
}

// internalHostRequested is the requested switch and whether the request set
// it (false: absent, the stored value is kept).
func internalHostRequested(requested *bool) (bool, bool) {
	if requested == nil {
		return false, false
	}
	return *requested, true
}
