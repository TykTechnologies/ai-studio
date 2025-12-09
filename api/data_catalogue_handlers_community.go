//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// Data Catalogue management endpoints - CE: All return 402 Payment Required

// @Summary Create a new data catalogue
// @Description Create a new data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues [post]
// @Security BearerAuth
func (a *API) createDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Get data catalogue by ID
// @Description Get details of a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id} [get]
// @Security BearerAuth
func (a *API) getDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Update data catalogue
// @Description Update an existing data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id} [patch]
// @Security BearerAuth
func (a *API) updateDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Delete data catalogue
// @Description Delete a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id} [delete]
// @Security BearerAuth
func (a *API) deleteDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary List data catalogues
// @Description Get all data catalogues (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues [get]
// @Security BearerAuth
func (a *API) listDataCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Search data catalogues
// @Description Search data catalogues (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/search [get]
// @Security BearerAuth
func (a *API) searchDataCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Add tag to data catalogue
// @Description Add a tag to a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id}/tags [post]
// @Security BearerAuth
func (a *API) addTagToDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Remove tag from data catalogue
// @Description Remove a tag from a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param tagId path int true "Tag ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id}/tags/{tagId} [delete]
// @Security BearerAuth
func (a *API) removeTagFromDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Add datasource to data catalogue
// @Description Add a datasource to a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id}/datasources [post]
// @Security BearerAuth
func (a *API) addDatasourceToDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Remove datasource from data catalogue
// @Description Remove a datasource from a specific data catalogue (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param datasourceId path int true "Datasource ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/{id}/datasources/{datasourceId} [delete]
// @Security BearerAuth
func (a *API) removeDatasourceFromDataCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Get data catalogues by tag
// @Description Get all data catalogues with a specific tag (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param tagName path string true "Tag name"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/by-tag/{tagName} [get]
// @Security BearerAuth
func (a *API) getDataCataloguesByTag(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}

// @Summary Get data catalogues by datasource
// @Description Get all data catalogues containing a specific datasource (Enterprise Edition only)
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param datasourceId path int true "Datasource ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/data-catalogues/by-datasource/{datasourceId} [get]
// @Security BearerAuth
func (a *API) getDataCataloguesByDatasource(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Data catalogue management requires Enterprise Edition"))
}
