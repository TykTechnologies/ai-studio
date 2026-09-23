package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// App grants of Model Routers. The admin App handlers and the portal's
// createUserApp grant routers the same way: validate visibility before the
// App is written (so a refusal leaves nothing behind), then pass the grants to
// the service, which counts routers as providers in the privacy check and
// binds them with the App.

// AppModelRouterOutput is the slim projection of a granted Model Router in
// App responses. Model is what a client sends to the unified ingress for it.
type AppModelRouterOutput struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func appModelRouterOutputs(routers []models.ModelRouter) ([]uint, []AppModelRouterOutput) {
	ids := make([]uint, 0, len(routers))
	out := make([]AppModelRouterOutput, 0, len(routers))
	for _, r := range routers {
		ids = append(ids, r.ID)
		out = append(out, AppModelRouterOutput{ID: r.ID, Name: r.Name, Slug: r.Slug})
	}
	return ids, out
}

// validateAppModelRouterBindings checks that every requested router may be
// granted by this actor. It writes the error response itself and reports
// whether the caller may go on.
func (a *API) validateAppModelRouterBindings(c *gin.Context, actorID uint, actorAdmin bool, routerIDs []uint) bool {
	if _, err := a.service.ValidateModelRouterBindings(actorID, actorAdmin, routerIDs); err != nil {
		if !modelRouterBindingError(c, err) {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		}
		return false
	}
	return true
}

// modelRouterBindingError maps a router grant refusal to its response.
func modelRouterBindingError(c *gin.Context, err error) bool {
	if errors.Is(err, services.ErrModelRouterNotVisible) {
		simpleError(c, http.StatusForbidden, "Forbidden", "User does not have access to one or more specified model routers")
		return true
	}
	return false
}

// appRouterOptions turns an optional list of router ids into service options.
func appRouterOptions(ids *[]uint) []services.AppOption {
	if ids == nil {
		return nil
	}
	return []services.AppOption{services.WithModelRouters(*ids)}
}
