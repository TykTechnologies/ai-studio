package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gin-gonic/gin"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	// Using orderedmap indirectly through the pb33f API
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"gorm.io/gorm"
)

// ParameterDetail stores information about a single API operation parameter.
type ParameterDetail struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      gin.H  `json:"schema"` // Using gin.H for flexible map structure
}

// RequestBodyDetail stores information about an API operation's request body.
type RequestBodyDetail struct {
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      gin.H  `json:"schema"` // Using gin.H for flexible map structure
	ContentType string `json:"content_type"`
}

// OperationDetail stores detailed information about a single API operation.
type OperationDetail struct {
	OperationID string            `json:"operation_id"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Description string            `json:"description"`
	Parameters  []ParameterDetail `json:"parameters"`
	RequestBody RequestBodyDetail `json:"request_body,omitempty"`
}

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
	pageSize, pageNumber, all := getPaginationParams(c)

	toolCatalogues, totalCount, totalPages, err := a.service.GetAllToolCatalogues(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
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

// Same as toToolResponses but uses toSecureToolResponse to hide sensitive fields
func toSecureToolResponses(tools []models.Tool) []ToolResponse {
	responses := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		responses[i] = toSecureToolResponse(&tool)
	}
	return responses
}

// toSecureToolResponse creates a tool response without sensitive fields for portal users
func toSecureToolResponse(tool *models.Tool) ToolResponse {
	ops := strings.Split(tool.AvailableOperations, ",")
	return ToolResponse{
		Type: "tools",
		ID:   strconv.FormatUint(uint64(tool.ID), 10),
		Attributes: struct {
			Name           string              `json:"name"`
			Description    string              `json:"description"`
			ToolType       string              `json:"tool_type"`
			OASSpec        string              `json:"oas_spec"`
			PrivacyScore   int                 `json:"privacy_score"`
			Operations     []string            `json:"operations"`
			AuthKey        string              `json:"auth_key"`
			AuthSchemaName string              `json:"auth_schema_name"`
			FileStores     []FileStoreResponse `json:"file_stores"`
			Filters        []FilterResponse    `json:"filters"`
			Dependencies   []ToolResponse      `json:"dependencies"`
		}{
			Name:           tool.Name,
			Description:    tool.Description,
			ToolType:       tool.ToolType,
			OASSpec:        "", // Hide OAS spec for security
			PrivacyScore:   tool.PrivacyScore,
			Operations:     ops,
			AuthKey:        "", // Hide auth key for security
			AuthSchemaName: tool.AuthSchemaName,
		},
	}
}

func toToolResponse(tool *models.Tool) ToolResponse {
	ops := strings.Split(tool.AvailableOperations, ",")
	return ToolResponse{
		Type: "tools",
		ID:   strconv.FormatUint(uint64(tool.ID), 10),
		Attributes: struct {
			Name           string              `json:"name"`
			Description    string              `json:"description"`
			ToolType       string              `json:"tool_type"`
			OASSpec        string              `json:"oas_spec"`
			PrivacyScore   int                 `json:"privacy_score"`
			Operations     []string            `json:"operations"`
			AuthKey        string              `json:"auth_key"`
			AuthSchemaName string              `json:"auth_schema_name"`
			FileStores     []FileStoreResponse `json:"file_stores"`
			Filters        []FilterResponse    `json:"filters"`
			Dependencies   []ToolResponse      `json:"dependencies"`
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

// @Summary Add a tool to a tool catalogue
// @Description Add a tool to a specified tool catalogue
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param tool body ToolCatalogueToolInput true "Tool to add"
// @Success 200 {object} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tools [post]
func (a *API) addToolToToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	var input ToolCatalogueToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	toolID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tool ID format"}}})
		return
	}

	err = a.service.AddToolToToolCatalogue(uint(toolID), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	toolCatalogue, err := a.service.GetToolCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponse(toolCatalogue))
}

// @Summary Remove a tool from a tool catalogue
// @Description Remove a tool from a specified tool catalogue
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param toolId path int true "Tool ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tools/{toolId} [delete]
func (a *API) removeToolFromToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	toolID, err := strconv.ParseUint(c.Param("toolId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tool ID format"}}})
		return
	}

	err = a.service.RemoveToolFromToolCatalogue(uint(toolID), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get tools in a tool catalogue
// @Description Get all tools in a specified tool catalogue
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {array} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tools [get]
func (a *API) getToolCatalogueTools(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	tools, err := a.service.GetToolCatalogueTools(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolResponses(tools))
}

// @Summary Get tools in a tool catalogue (secure version for portal users)
// @Description Get all tools in a specified tool catalogue with sensitive fields hidden
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {array} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/tool-catalogues/{id}/tools [get]
func (a *API) getToolCatalogueToolsSecure(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	tools, err := a.service.GetToolCatalogueTools(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	// Use secure response format that hides sensitive fields
	c.JSON(http.StatusOK, toSecureToolResponses(tools))
}

// @Summary Get tool documentation by ID
// @Description Get documentation for a specific tool by its ID
// @Tags tools
// @Produce json
// @Param id path string true "Tool ID"
// @Success 200 {object} gin.H
// @Failure 404 {object} ErrorResponse
// @Router /tools/{id}/documentation [get]
func (a *API) GetToolDocumentation(c *gin.Context) {
	toolID := c.Param("id")

	// Assuming GetToolByID takes string ID and returns a models.Tool object.
	// The previous TODO about ID parsing is still relevant if the service layer expects a different type.
	// Convert string ID to uint as required by service
	idUint, err := strconv.ParseUint(toolID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tool ID: " + err.Error()}}})
		return
	}

	tool, err := a.service.GetToolByID(uint(idUint))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Tool not found: " + toolID}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", "Failed to fetch tool: " + err.Error()}}})
		return
	}

	if tool.OASSpec == "" {
		c.JSON(http.StatusOK, gin.H{"message": "Tool found, but it has no OAS specification."})
		return
	}

	decodedSpec, err := helpers.DecodeToUTF8(tool.OASSpec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", "Failed to decode OAS spec: " + err.Error()}}})
		return
	}

	// Get whitelisted operations from the tool configuration
	whitelistedOperations := tool.GetOperations()
	if len(whitelistedOperations) == 0 {
		c.JSON(http.StatusOK, []OperationDetail{})
		return
	}

	var operationDetailsList []OperationDetail

	// Only process operations that are whitelisted in the tool configuration
	for _, opID := range whitelistedOperations {
		// Use our new helper function that leverages universalclient's AsTool method
		operationDetail, err := getOperationDetailFromSpec([]byte(decodedSpec), opID)
		if err != nil {
			// Skip operations that can't be found in the spec (log warning but don't fail)
			continue
		}

		operationDetailsList = append(operationDetailsList, operationDetail)
	}

	if len(operationDetailsList) == 0 && len(whitelistedOperations) > 0 {
		// This case might happen if whitelisted operations don't match any operation.OperationID in the spec.
		// This indicates a potential mismatch between tool configuration and OpenAPI spec.
		// For now, we proceed, but this is a point of attention.
	}

	c.JSON(http.StatusOK, operationDetailsList)
}

// @Summary Add a tag to a tool catalogue
// @Description Add a tag to a specified tool catalogue
// @Tags tool-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param tag body ToolCatalogueTagInput true "Tag to add"
// @Success 200 {object} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tags [post]
func (a *API) addTagToToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	var input ToolCatalogueTagInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	tagID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tag ID format"}}})
		return
	}

	err = a.service.AddTagToToolCatalogue(uint(tagID), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	toolCatalogue, err := a.service.GetToolCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toToolCatalogueResponse(toolCatalogue))
}

// @Summary Remove a tag from a tool catalogue
// @Description Remove a tag from a specified tool catalogue
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Param tagId path int true "Tag ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tags/{tagId} [delete]
func (a *API) removeTagFromToolCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	tagID, err := strconv.ParseUint(c.Param("tagId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tag ID format"}}})
		return
	}

	err = a.service.RemoveTagFromToolCatalogue(uint(tagID), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get tags in a tool catalogue
// @Description Get all tags in a specified tool catalogue
// @Tags tool-catalogues
// @Produce json
// @Param id path int true "Tool Catalogue ID"
// @Success 200 {array} TagResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tool-catalogues/{id}/tags [get]
func (a *API) getToolCatalogueTags(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	tags, err := a.service.GetToolCatalogueTags(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toTagResponses(tags))
}

// getOperationDetailFromSpec converts an OpenAPI operation to an OperationDetail format
// It leverages the universalclient package for consistency between UI rendering and execution
func getOperationDetailFromSpec(oasSpec []byte, operationID string) (OperationDetail, error) {
	// Create a universal client to leverage its AsTool functionality
	// Pass empty baseURL as we only need the schema functionality
	client, err := universalclient.NewClient(oasSpec, "")
	if err != nil {
		return OperationDetail{}, fmt.Errorf("failed to create universal client: %w", err)
	}

	// First get the tool definition using the existing AsTool method
	tools, err := client.AsTool(operationID)
	if err != nil || len(tools) == 0 {
		return OperationDetail{}, fmt.Errorf("failed to get tool definition: %w", err)
	}

	// Find the operation in the OpenAPI spec
	config := &datamodel.DocumentConfiguration{
		AllowFileReferences:   false,
		AllowRemoteReferences: false,
		BaseURL:               nil,
		RemoteURLHandler:      nil,
	}
	doc, err := libopenapi.NewDocumentWithConfiguration(oasSpec, config)
	if err != nil {
		return OperationDetail{}, fmt.Errorf("failed to parse OAS spec: %w", err)
	}

	// Build a V3 model
	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return OperationDetail{}, fmt.Errorf("failed to build V3 model: %v", errs)
	}

	// Find the operation by ID
	var foundPath string
	var foundOperation *v3.Operation

	// Iterate through all paths and operations to find the one with matching ID
	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		path := pair.Key()
		pathItem := pair.Value()

		operationsMap := pathItem.GetOperations()

		// Use a for loop with known methods since GetOperations doesn't return a standard Go map
		for _, method := range []string{"get", "post", "put", "delete", "options", "head", "patch", "trace"} {
			// Access the operation from the operationsMap using the Get method
			operation, exists := operationsMap.Get(method)
			if !exists {
				continue
			}
			if operation.OperationId == operationID {
				foundPath = path
				foundOperation = operation
				break
			}
		}

		if foundOperation != nil {
			break
		}
	}

	if foundOperation == nil {
		return OperationDetail{}, fmt.Errorf("operation %s not found in spec", operationID)
	}

	// Create the OperationDetail
	opDetail := OperationDetail{
		OperationID: operationID,
		Method:      "POST", // Always show POST since all tool calls go through the proxy as POST requests
		Path:        foundPath,
		Description: foundOperation.Description,
		Parameters:  []ParameterDetail{},
	}

	// Add parameters
	for _, param := range foundOperation.Parameters {
		schemaMap := convertSchemaToGinH(param.Schema.Schema())

		opDetail.Parameters = append(opDetail.Parameters, ParameterDetail{
			Name:        param.Name,
			In:          param.In,
			Description: param.Description,
			Required:    param.Required != nil && *param.Required,
			Schema:      schemaMap,
		})
	}

	// Add request body if present
	if foundOperation.RequestBody != nil && foundOperation.RequestBody.Content != nil {
		if mediaType, ok := foundOperation.RequestBody.Content.Get("application/json"); ok {
			schemaMap := convertSchemaToGinH(mediaType.Schema.Schema())

			opDetail.RequestBody = RequestBodyDetail{
				Description: foundOperation.RequestBody.Description,
				Required:    foundOperation.RequestBody.Required != nil && *foundOperation.RequestBody.Required,
				Schema:      schemaMap,
				ContentType: "application/json",
			}
		}
	}

	return opDetail, nil
}

// convertSchemaToGinH converts a pb33f Schema to gin.H format for consistent JSON serialization
func convertSchemaToGinH(schema *base.Schema) gin.H {
	if schema == nil {
		return gin.H{}
	}

	result := gin.H{}

	if len(schema.Type) > 0 {
		result["type"] = schema.Type[0]
	}

	if schema.Description != "" {
		result["description"] = schema.Description
	}

	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	// Handle Properties
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		properties := gin.H{}
		for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
			properties[pair.Key()] = convertSchemaToGinH(pair.Value().Schema())
		}
		if len(properties) > 0 {
			result["properties"] = properties
		}
	}

	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Add format if present
	if schema.Format != "" {
		result["format"] = schema.Format
	}

	return result
}

// @Summary Get user apps that have access to a tool
// @Description Get list of user's apps that have been granted access to a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path string true "Tool ID"
// @Success 200 {object} gin.H
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/user-apps [get]
func (a *API) getToolUserApps(c *gin.Context) {
	toolID := c.Param("id")

	// Convert string ID to uint
	idUint, err := strconv.ParseUint(toolID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid tool ID: " + err.Error()}}})
		return
	}

	// Get current user from context
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Unauthorized", "User not found in context"}}})
		return
	}

	currentUser, ok := user.(*models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", "Invalid user type in context"}}})
		return
	}

	// Verify tool exists and user has access to it
	tool, err := a.service.GetToolByID(uint(idUint))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Tool not found: " + toolID}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", "Failed to fetch tool: " + err.Error()}}})
		return
	}

	// Get user's apps that have access to this tool
	var userApps []models.App
	err = a.config.DB.Table("apps").
		Joins("JOIN app_tools ON app_tools.app_id = apps.id").
		Where("apps.user_id = ? AND app_tools.tool_id = ?", currentUser.ID, uint(idUint)).
		Preload("Credential").
		Find(&userApps).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", "Failed to fetch user apps: " + err.Error()}}})
		return
	}

	// Serialize the apps for response
	var appResponses []map[string]interface{}
	for _, app := range userApps {
		appResponse := map[string]interface{}{
			"id":          app.ID,
			"name":        app.Name,
			"description": app.Description,
		}

		// Include API secret if credential exists and is loaded
		if app.CredentialID != 0 && app.Credential.Secret != "" {
			appResponse["api_secret"] = app.Credential.Secret
		} else if app.CredentialID != 0 {
			// Fallback: if credential is not loaded properly, fetch it manually
			var credential models.Credential
			if err := a.config.DB.First(&credential, app.CredentialID).Error; err == nil {
				appResponse["api_secret"] = credential.Secret
			}
		}

		appResponses = append(appResponses, appResponse)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      appResponses,
		"tool_name": tool.Name,
	})
}
