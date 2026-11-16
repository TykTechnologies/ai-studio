package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// App grants of routers (Model and Semantic). The admin App handlers and the
// portal's createUserApp grant routers the same way: validate visibility
// before the App is written (so a refusal leaves nothing behind), then pass
// the grants to the service, which counts routers as providers in the privacy
// check and binds them with the App.

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

// semanticRouterBindingError maps a Semantic Router grant refusal to its
// response.
func semanticRouterBindingError(c *gin.Context, err error) bool {
	if errors.Is(err, services.ErrSemanticRouterNotVisible) {
		simpleError(c, http.StatusForbidden, "Forbidden", "User does not have access to one or more specified semantic routers")
		return true
	}
	return false
}

// appRouterOptions turns optional lists of Model and Semantic Router ids
// into service options (nil: leave that kind alone).
func appRouterOptions(modelIDs, semanticIDs *[]uint) []services.AppOption {
	var opts []services.AppOption
	if modelIDs != nil {
		opts = append(opts, services.WithModelRouters(*modelIDs))
	}
	if semanticIDs != nil {
		opts = append(opts, services.WithSemanticRouters(*semanticIDs))
	}
	return opts
}

// validateAppRouterBindings checks both kinds of router grant for this
// actor. It writes the error response itself and reports whether the caller
// may go on.
func (a *API) validateAppRouterBindings(c *gin.Context, actorID uint, actorAdmin bool, modelIDs, semanticIDs *[]uint) bool {
	if modelIDs != nil && !a.validateAppModelRouterBindings(c, actorID, actorAdmin, *modelIDs) {
		return false
	}
	if semanticIDs != nil {
		if _, err := a.service.ValidateSemanticRouterBindings(actorID, actorAdmin, *semanticIDs); err != nil {
			if !semanticRouterBindingError(c, err) {
				simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			}
			return false
		}
	}
	return true
}

// adminAppRouterOptions validates the router grants an admin App create or
// update asks for (nil: the request leaves that kind alone) and returns them
// as service options. It writes the error response itself and reports whether
// the caller may go on. appID is the App being updated (0 on create).
//
// With Workspace checks on (.49), routers are read-scoped: every router the
// request names must be visible to the caller (404 as a missing one
// otherwise), and on update the stored grants the caller cannot see are kept
// (mergeVisibleLinks). Only the visible, requested routers then go through
// the grant rule, and the "may grant any active router" path is for
// administrators and global App writers only, never for a writer scoped to
// the App's Workspace.
func (a *API) adminAppRouterOptions(c *gin.Context, appID uint, modelIDs, semanticIDs *[]uint) ([]services.AppOption, bool) {
	if modelIDs == nil && semanticIDs == nil {
		return nil, true
	}
	merge := func(resource string, model interface{}, table, col string, submitted *[]uint, notFound string) (*[]uint, bool) {
		if submitted == nil || !a.workspaceChecksOn(c) {
			return submitted, true
		}
		var existing []uint
		if appID != 0 {
			var err error
			if existing, err = a.linkedIDs(table, "app_id", col, appID); err != nil {
				simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
				c.Abort()
				return nil, false
			}
		}
		ids, ok := a.mergeVisibleLinks(c, resource, model, *submitted, existing, notFound)
		return &ids, ok
	}
	// Visibility first (the merge rejects a hidden or missing router with a
	// 404) before the grant rule could say anything about it; the grant
	// rule then sees only what the request named.
	mergedModel, ok := merge("model-routers", &models.ModelRouter{}, "app_model_routers", "model_router_id", modelIDs, "Model router not found")
	if !ok {
		return nil, false
	}
	mergedSemantic, ok := merge("semantic-routers", &models.SemanticRouter{}, "app_semantic_routers", "semantic_router_id", semanticIDs, "Semantic router not found")
	if !ok {
		return nil, false
	}
	actorID, actorAdmin := adminAppActor(c) // global, never the App's Workspace
	if !a.validateAppRouterBindings(c, actorID, actorAdmin, modelIDs, semanticIDs) {
		return nil, false
	}
	modelIDs, semanticIDs = mergedModel, mergedSemantic
	return appRouterOptions(modelIDs, semanticIDs), true
}

// AppSemanticRouterOutput is the slim projection of a granted Semantic Router
// in App responses. Models are what a client sends to the unified ingress.
type AppSemanticRouterOutput struct {
	ID     uint     `json:"id"`
	Name   string   `json:"name"`
	Slug   string   `json:"slug"`
	Models []string `json:"models"`
}

func appSemanticRouterOutputs(routers []models.SemanticRouter) ([]uint, []AppSemanticRouterOutput) {
	ids := make([]uint, 0, len(routers))
	out := make([]AppSemanticRouterOutput, 0, len(routers))
	for _, r := range routers {
		ids = append(ids, r.ID)
		out = append(out, AppSemanticRouterOutput{ID: r.ID, Name: r.Name, Slug: r.Slug,
			Models: sr.ModelsFor(r.Slug, r.Config())})
	}
	return ids, out
}
