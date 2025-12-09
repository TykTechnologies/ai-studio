//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// Tool Catalogue management endpoints - CE: All return 402 Payment Required

// @Summary Create a new tool catalogue
// @Description Create a new tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues [post]
// @Security BearerAuth
func (a *API) createToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Get tool catalogue by ID
// @Description Get details of a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id} [get]
// @Security BearerAuth
func (a *API) getToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Update tool catalogue
// @Description Update an existing tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id} [patch]
// @Security BearerAuth
func (a *API) updateToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Delete tool catalogue
// @Description Delete a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id} [delete]
// @Security BearerAuth
func (a *API) deleteToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary List tool catalogues
// @Description Get all tool catalogues (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues [get]
// @Security BearerAuth
func (a *API) listToolCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Search tool catalogues
// @Description Search tool catalogues (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/search [get]
// @Security BearerAuth
func (a *API) searchToolCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Add tool to tool catalogue
// @Description Add a tool to a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tools [post]
// @Security BearerAuth
func (a *API) addToolToToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Remove tool from tool catalogue
// @Description Remove a tool from a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param toolId path int true "Tool ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tools/{toolId} [delete]
// @Security BearerAuth
func (a *API) removeToolFromToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary List tool catalogue tools
// @Description Get all tools in a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tools [get]
// @Security BearerAuth
func (a *API) getToolCatalogueTools(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary List tool catalogue tools (secure)
// @Description Get all tools in a specific tool catalogue with redacted sensitive data (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tools/secure [get]
// @Security BearerAuth
func (a *API) getToolCatalogueToolsSecure(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Get tool documentation
// @Description Get documentation for a specific tool (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param catalogueId path int true "Tool Catalogue ID"
// @Param toolId path int true "Tool ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{catalogueId}/tools/{toolId}/documentation [get]
// @Security BearerAuth
func (a *API) GetToolDocumentation(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Add tag to tool catalogue
// @Description Add a tag to a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tags [post]
// @Security BearerAuth
func (a *API) addTagToToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Remove tag from tool catalogue
// @Description Remove a tag from a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param tagId path int true "Tag ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tags/{tagId} [delete]
// @Security BearerAuth
func (a *API) removeTagFromToolCatalogue(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Get tool catalogue tags
// @Description Get all tags for a specific tool catalogue (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{id}/tags [get]
// @Security BearerAuth
func (a *API) getToolCatalogueTags(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}

// @Summary Get user apps for a tool
// @Description Get all user apps that use a specific tool (Enterprise Edition only)
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param catalogueId path int true "Tool Catalogue ID"
// @Param toolId path int true "Tool ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/tool-catalogues/{catalogueId}/tools/{toolId}/apps [get]
// @Security BearerAuth
func (a *API) getToolUserApps(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Tool catalogue management requires Enterprise Edition"))
}
