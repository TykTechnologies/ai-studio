package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Publishing a Model Router: it is placed in LLM catalogues, next to the LLMs
// it routes to, and teams granted a catalogue see it in the portal and may
// add it to their Apps.

// RouterCatalogueRef names a catalogue a router is published in.
type RouterCatalogueRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func routerCatalogueRefs(cats []models.Catalogue) []RouterCatalogueRef {
	out := make([]RouterCatalogueRef, 0, len(cats))
	for _, c := range cats {
		out = append(out, RouterCatalogueRef{ID: c.ID, Name: c.Name})
	}
	return out
}

type modelRouterCataloguesInput struct {
	CatalogueIDs []uint `json:"catalogue_ids"`
}

// setModelRouterCatalogues godoc
// @Summary Publish a model router in LLM catalogues
// @Description Replaces the LLM catalogues the router is published in (Enterprise). Teams granted one of them see the router in the portal and may add it to their Apps.
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model Router ID"
// @Param body body modelRouterCataloguesInput true "Catalogue ids"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /model-routers/{id}/catalogues [put]
// @Security BearerAuth
func (a *API) setModelRouterCatalogues(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid model router ID")
		return
	}
	var in modelRouterCataloguesInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	cats, err := a.service.SetModelRouterCatalogues(uint(id), in.CatalogueIDs)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", "Model router not found")
		return
	case errors.Is(err, services.ErrInvalidCatalogueReference):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	case err != nil:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"catalogues": routerCatalogueRefs(cats)}})
}
