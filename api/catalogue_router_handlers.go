package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Routers of an LLM catalogue, edited from the catalogue. The same
// memberships can be edited from each router (PUT /model-routers/{id}/catalogues,
// PUT /semantic-routers/{id}/catalogues).

// CatalogueRouterRef is one router published in a catalogue.
type CatalogueRouterRef struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Active bool   `json:"active"`
}

// CatalogueRoutersInput replaces a catalogue's routers. A list left out
// leaves that kind unchanged; an empty list removes them all.
type CatalogueRoutersInput struct {
	ModelRouterIDs    *[]uint `json:"model_router_ids"`
	SemanticRouterIDs *[]uint `json:"semantic_router_ids"`
}

func catalogueRoutersResponse(r *services.CatalogueRouters) gin.H {
	mr := make([]CatalogueRouterRef, 0, len(r.ModelRouters))
	for _, x := range r.ModelRouters {
		mr = append(mr, CatalogueRouterRef{ID: x.ID, Name: x.Name, Slug: x.Slug, Active: x.Active})
	}
	sr := make([]CatalogueRouterRef, 0, len(r.SemanticRouters))
	for _, x := range r.SemanticRouters {
		sr = append(sr, CatalogueRouterRef{ID: x.ID, Name: x.Name, Slug: x.Slug, Active: x.Active})
	}
	return gin.H{"data": gin.H{"model_routers": mr, "semantic_routers": sr}}
}

// routersAvailable gates the endpoints on an edition with routers (a
// variable so tests can exercise both editions).
var routersAvailable = func() bool {
	return model_router.IsEnterpriseAvailable() || semantic_router.IsEnterpriseAvailable()
}

// @Summary List the routers published in an LLM catalogue
// @Description Model Routers and Semantic Routers in the catalogue (Enterprise)
// @Tags catalogues
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /catalogues/{id}/routers [get]
// @Security BearerAuth
func (a *API) getCatalogueRouters(c *gin.Context) {
	if !routersAvailable() {
		simpleError(c, http.StatusPaymentRequired, "Payment Required", "routers are an Enterprise Edition feature")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid catalogue ID")
		return
	}
	r, err := a.service.GetCatalogueRouters(uint(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		simpleError(c, http.StatusNotFound, "Not Found", "Catalogue not found")
		return
	}
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	c.JSON(http.StatusOK, catalogueRoutersResponse(r))
}

// @Summary Set the routers published in an LLM catalogue
// @Description Replaces the catalogue's Model Routers and/or Semantic Routers (Enterprise). Teams holding the catalogue see them in the portal and may add them to their Apps.
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Param body body CatalogueRoutersInput true "Router ids"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /catalogues/{id}/routers [put]
// @Security BearerAuth
func (a *API) setCatalogueRouters(c *gin.Context) {
	if !routersAvailable() {
		simpleError(c, http.StatusPaymentRequired, "Payment Required", "routers are an Enterprise Edition feature")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid catalogue ID")
		return
	}
	var in CatalogueRoutersInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	r, err := a.service.SetCatalogueRouters(uint(id), in.ModelRouterIDs, in.SemanticRouterIDs)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", "Catalogue not found")
		return
	case errors.Is(err, services.ErrInvalidRouterReference):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	case err != nil:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	c.JSON(http.StatusOK, catalogueRoutersResponse(r))
}
