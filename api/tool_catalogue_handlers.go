package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Create a new tool catalogue
// @Description Create a new tool catalogue with the provided information
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param toolCatalogue body ToolCatalogueInput true "Tool Catalogue information"
// @Success 201 {object} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues [post]
func (a *API) createToolCatalogue(c *gin.Context) {
	var input ToolCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	if strings.TrimSpace(input.Data.Attributes.Name) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Name is required"}}})
		return
	}

	toolCatalogue, err := a.service.CreateToolCatalogue(
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusCreated, toToolCatalogueResponse(toolCatalogue))
}

// @Summary Get a tool catalogue by ID
// @Description Get details of a tool catalogue by its ID
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {object} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tool-catalogues/{id} [get]
func (a *API) getToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	toolCatalogue, err := a.service.GetToolCatalogueByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Tool Catalogue not found"}}})
			return
		}
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Not Found", "Tool Catalogue not found"}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponse(toolCatalogue))
}

// @Summary Update a tool catalogue
// @Description Update an existing tool catalogue's information
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param toolCatalogue body ToolCatalogueInput true "Updated Tool Catalogue information"
// @Success 200 {object} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id} [patch]
func (a *API) updateToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	var input ToolCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	toolCatalogue, err := a.service.UpdateToolCatalogue(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Tool Catalogue not found"}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponse(toolCatalogue))
}

// @Summary Delete a tool catalogue
// @Description Delete a tool catalogue by its ID
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id} [delete]
func (a *API) deleteToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	err = a.service.DeleteToolCatalogue(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Tool Catalogue not found"}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all tool catalogues
// @Description Get a list of all tool catalogues
// @Tags tool-catalogues
// @Produce json
// @Success 200 {array} ToolCatalogueResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues [get]
func (a *API) listToolCatalogues(c *gin.Context) {
	toolCatalogues, err := a.service.GetAllToolCatalogues()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponses(toolCatalogues))
}

// @Summary Search tool catalogues
// @Description Search for tool catalogues using a query string
// @Tags tool-catalogues
// @Produce json
// @Param query query string true "Search query"
// @Success 200 {array} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/search [get]
func (a *API) searchToolCatalogues(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Search query is required"}}})
		return
	}

	toolCatalogues, err := a.service.SearchToolCatalogues(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponses(toolCatalogues))
}

// Helper functions to convert models to responses
func toToolCatalogueResponse(tc *models.ToolCatalogue) ToolCatalogueResponse {
	return ToolCatalogueResponse{
		Type: "tool-catalogues",
		ID:   strconv.FormatUint(uint64(tc.ID), 10),
		Attributes: struct {
			Name             string         `json:"name"`
			ShortDescription string         `json:"short_description"`
			LongDescription  string         `json:"long_description"`
			Icon             string         `json:"icon"`
			Tools            []ToolResponse `json:"tools"`
			Tags             []TagResponse  `json:"tags"`
		}{
			Name:             tc.Name,
			ShortDescription: tc.ShortDescription,
			LongDescription:  tc.LongDescription,
			Icon:             tc.Icon,
			Tools:            toToolResponses(tc.Tools),
			Tags:             toTagResponses(tc.Tags),
		},
	}
}

func toToolCatalogueResponses(tcs []models.ToolCatalogue) []ToolCatalogueResponse {
	responses := make([]ToolCatalogueResponse, len(tcs))
	for i, tc := range tcs {
		responses[i] = toToolCatalogueResponse(&tc)
	}
	return responses
}

func toToolResponses(tools []models.Tool) []ToolResponse {
	responses := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		responses[i] = toToolResponse(&tool)
	}
	return responses
}

func toToolResponse(tool *models.Tool) ToolResponse {
	ops := strings.Split(tool.AvailableOperations, ",")
	return ToolResponse{
		Type: "tools",
		ID:   strconv.FormatUint(uint64(tool.ID), 10),
		Attributes: struct {
			Name           string   `json:"name"`
			Description    string   `json:"description"`
			ToolType       string   `json:"tool_type"`
			OASSpec        []byte   `json:"oas_spec"`
			PrivacyScore   int      `json:"privacy_score"`
			Operations     []string `json:"operations"`
			AuthKey        string   `json:"auth_key"`
			AuthSchemaName string   `json:"auth_schema_name"`
		}{
			Name:           tool.Name,
			Description:    tool.Description,
			ToolType:       tool.ToolType,
			OASSpec:        tool.OASSpec,
			PrivacyScore:   tool.PrivacyScore,
			Operations:     ops,
			AuthKey:        tool.AuthKey,
			AuthSchemaName: tool.AuthSchemaName,
		},
	}
}

func toTagResponses(tags []models.Tag) []TagResponse {
	responses := make([]TagResponse, len(tags))
	for i, tag := range tags {
		responses[i] = toTagResponse(&tag)
	}
	return responses
}

func toTagResponse(tag *models.Tag) TagResponse {
	return TagResponse{
		Type: "tags",
		ID:   strconv.FormatUint(uint64(tag.ID), 10),
		Attributes: struct {
			Name string `json:"name"`
		}{
			Name: tag.Name,
		},
	}
}
