package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// appTracingOption turns the request's tracing overrides into an App option
// (nil when the request has none). Letting the Portal owner read captured
// content is a content-access grant, so changing owner_may_view_content
// from before (the stored value; false on create) also needs traces:write;
// without it the request is refused with the standard 403 and false is
// returned.
func (a *API) appTracingOption(c *gin.Context, t *models.AppTracingSettings, before bool) (services.AppOption, bool) {
	if t == nil {
		return nil, true
	}
	if t.OwnerMayViewContent != before && !a.holds(c, authz.Write("traces")) {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Write("traces")))
		return nil, false
	}
	return services.WithAppTracing(*t), true
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
