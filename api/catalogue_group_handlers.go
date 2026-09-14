package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// CatalogueGroupsResponse lists the teams a catalogue is shared with.
// @Description Teams using a catalogue
type CatalogueGroupsResponse struct {
	Data []services.CatalogueGroup `json:"data"`
}

// respondCatalogueGroups is the shared body of the three "teams using this
// catalogue" routes: 404 for an unknown catalogue, otherwise the teams
// ordered by name (an empty array when none).
func (a *API) respondCatalogueGroups(c *gin.Context, kind string, model interface{}) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid ID format"}}})
		return
	}

	found, err := a.rowExists(model, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Not Found", Detail: "Catalogue not found"}}})
		return
	}

	groups, err := a.service.GetCatalogueGroups(kind, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}
	c.JSON(http.StatusOK, CatalogueGroupsResponse{Data: groups})
}

// @Summary Teams using an LLM catalogue
// @Description List the teams that have been granted the catalogue, with their member counts, ordered by name
// @Tags catalogues
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} CatalogueGroupsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /catalogues/{id}/groups [get]
// @Security BearerAuth
func (a *API) getCatalogueGroups(c *gin.Context) {
	a.respondCatalogueGroups(c, "catalogues", &models.Catalogue{})
}

// @Summary Teams using a data catalogue
// @Description List the teams that have been granted the data catalogue, with their member counts, ordered by name
// @Tags data-catalogues
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} CatalogueGroupsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /data-catalogues/{id}/groups [get]
// @Security BearerAuth
func (a *API) getDataCatalogueGroups(c *gin.Context) {
	a.respondCatalogueGroups(c, "data-catalogues", &models.DataCatalogue{})
}

// @Summary Teams using a tool catalogue
// @Description List the teams that have been granted the tool catalogue, with their member counts, ordered by name
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} CatalogueGroupsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tool-catalogues/{id}/groups [get]
// @Security BearerAuth
func (a *API) getToolCatalogueGroups(c *gin.Context) {
	a.respondCatalogueGroups(c, "tool-catalogues", &models.ToolCatalogue{})
}
