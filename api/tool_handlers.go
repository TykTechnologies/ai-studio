package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new tool
// @Description Create a new tool with the provided information
// @Tags tools
// @Accept json
// @Produce json
// @Param tool body ToolInput true "Tool information"
// @Success 201 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools [post]
// @Security BearerAuth
func (a *API) createTool(c *gin.Context) {
	var input ToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if input.Data.Attributes.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name is required"}},
		})
		return
	}

	if input.Data.Attributes.ToolType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Tool type is required"}},
		})
		return
	}

	if input.Data.Attributes.ToolType == models.ToolTypeREST {
		if len(input.Data.Attributes.OASSpec) == 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "OAS spec is required for REST tools"}},
			})
			return
		}
	}

	tool, err := a.service.CreateTool(
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.ToolType,
		input.Data.Attributes.OASSpec,
		input.Data.Attributes.PrivacyScore,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeTool(tool)})
}

// @Summary Get a tool by ID
// @Description Get details of a tool by its ID
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tools/{id} [get]
// @Security BearerAuth
func (a *API) getTool(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool)})
}

// @Summary Update a tool
// @Description Update an existing tool's information
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param tool body ToolInput true "Updated tool information"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id} [patch]
// @Security BearerAuth
func (a *API) updateTool(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	var input ToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.UpdateTool(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.ToolType,
		input.Data.Attributes.OASSpec,
		input.Data.Attributes.PrivacyScore,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool)})
}

// @Summary Delete a tool
// @Description Delete a tool by its ID
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id} [delete]
// @Security BearerAuth
func (a *API) deleteTool(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	err = a.service.DeleteTool(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get all tools
// @Description Get a list of all tools
// @Tags tools
// @Accept json
// @Produce json
// @Success 200 {array} ToolResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools [get]
// @Security BearerAuth
func (a *API) getAllTools(c *gin.Context) {
	tools, err := a.service.GetAllTools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools)})
}

// @Summary Get tools by type
// @Description Get a list of tools of a specific type
// @Tags tools
// @Accept json
// @Produce json
// @Param type query string true "Tool Type"
// @Success 200 {array} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/by-type [get]
// @Security BearerAuth
func (a *API) getToolsByType(c *gin.Context) {
	toolType := c.Query("type")
	if toolType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Tool type is required"}},
		})
		return
	}

	tools, err := a.service.GetToolsByType(toolType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools)})
}

// @Summary Search tools
// @Description Search for tools by name or description
// @Tags tools
// @Accept json
// @Produce json
// @Param query query string true "Search Query"
// @Success 200 {array} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/search [get]
// @Security BearerAuth
func (a *API) searchTools(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Search query is required"}},
		})
		return
	}

	tools, err := a.service.SearchTools(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools)})
}

func serializeTool(tool *models.Tool) ToolResponse {
	return ToolResponse{
		Type: "tools",
		ID:   strconv.FormatUint(uint64(tool.ID), 10),
		Attributes: struct {
			Name           string   `json:"name"`
			Description    string   `json:"description"`
			ToolType       string   `json:"tool_type"`
			OASSpec        []byte   `json:"oas_spec"`
			PrivacyScore   float64  `json:"privacy_score"`
			Operations     []string `json:"operations"`
			AuthKey        string   `json:"auth_key"`
			AuthSchemaName string   `json:"auth_schema_name"`
		}{
			Name:           tool.Name,
			Description:    tool.Description,
			ToolType:       tool.ToolType,
			OASSpec:        tool.OASSpec,
			PrivacyScore:   tool.PrivacyScore,
			Operations:     tool.GetOperations(),
			AuthKey:        tool.AuthKey,
			AuthSchemaName: tool.AuthSchemaName,
		},
	}
}

func serializeTools(tools []models.Tool) []ToolResponse {
	result := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		result[i] = serializeTool(&tool)
	}
	return result
}
