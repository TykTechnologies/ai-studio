package api

import (
	"net/http"
	"reflect"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// appTracingOption turns the request's tracing overrides into an App option
// (nil when the request has none). Every tracing setting decides what is
// captured, for how long, or who may read it (owner_may_view_content is a
// content-access grant), so changing any of them from before (the stored
// settings; empty on create) needs traces:write, not just apps:write
// (jyx.99). Without it the request is refused with the standard 403 and
// false is returned.
func (a *API) appTracingOption(c *gin.Context, t *models.AppTracingSettings, before models.AppTracingSettings) (services.AppOption, bool) {
	if t == nil {
		return nil, true
	}
	if !a.requireTracingWrite(c, !reflect.DeepEqual(*t, before)) {
		return nil, false
	}
	return services.WithAppTracing(*t), true
}

// requireTracingWrite writes the standard 403 and returns false when a
// request changes a tracing setting without traces:write.
func (a *API) requireTracingWrite(c *gin.Context, changed bool) bool {
	if !changed || a.holds(c, authz.Write("traces")) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Write("traces")))
	return false
}

// tracingCapChanged reports whether a request's tracing_content_mode_max
// (nil when absent) differs from the stored cap ("" on create).
func tracingCapChanged(requested *string, stored string) bool {
	return requested != nil && *requested != stored
}

// validateTracingContentModeMax checks a tool or datasource content mode cap
// (nil when the request leaves it alone), writing a 400 and returning false
// when it is not a known mode.
func validateTracingContentModeMax(c *gin.Context, mode *string) bool {
	if mode == nil {
		return true
	}
	if err := models.ValidateTracingContentMode("tracing_content_mode_max", *mode); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return false
	}
	return true
}
