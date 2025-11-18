//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// LLM Catalogue management endpoints - CE: All return 402 Payment Required

// @Summary Create a new catalogue
// @Description Create a new LLM catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues [post]
// @Security BearerAuth
func (a *API) createCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Get catalogue by ID
// @Description Get details of a specific LLM catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id} [get]
// @Security BearerAuth
func (a *API) getCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Update catalogue
// @Description Update an existing LLM catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id} [patch]
// @Security BearerAuth
func (a *API) updateCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Delete catalogue
// @Description Delete a specific LLM catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id} [delete]
// @Security BearerAuth
func (a *API) deleteCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary List catalogues
// @Description Get all LLM catalogues (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues [get]
// @Security BearerAuth
func (a *API) listCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Search catalogues
// @Description Search LLM catalogues (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/search [get]
// @Security BearerAuth
func (a *API) searchCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Add LLM to catalogue
// @Description Add an LLM to a specific catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id}/llms [post]
// @Security BearerAuth
func (a *API) addLLMToCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Remove LLM from catalogue
// @Description Remove an LLM from a specific catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Param llmId path int true "LLM ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id}/llms/{llmId} [delete]
// @Security BearerAuth
func (a *API) removeLLMFromCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary List catalogue LLMs
// @Description Get all LLMs in a specific catalogue (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/{id}/llms [get]
// @Security BearerAuth
func (a *API) listCatalogueLLMs(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}

// @Summary Search catalogues by name
// @Description Search LLM catalogues by name prefix (Enterprise Edition only)
// @Tags catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/catalogues/search-by-name [get]
// @Security BearerAuth
func (a *API) searchCataloguesByNameStub(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("LLM catalogue management requires Enterprise Edition"))
}
