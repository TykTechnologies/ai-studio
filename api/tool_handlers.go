package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
		input.Data.Attributes.AuthSchemaName,
		input.Data.Attributes.AuthKey,
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeTool(tool, a.config.DB)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
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
		input.Data.Attributes.AuthSchemaName,
		input.Data.Attributes.AuthKey,
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
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
	pageSize, pageNumber, all := getPaginationParams(c)

	tools, totalCount, totalPages, err := a.service.GetAllTools(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools, a.config.DB)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools, a.config.DB)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTools(tools, a.config.DB)})
}

// @Summary Add operation to tool
// @Description Add an operation to a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param operation body OperationInput true "Operation to add"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/operations [post]
// @Security BearerAuth
func (a *API) addOperationToTool(c *gin.Context) {
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

	var input OperationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.AddOperationToTool(uint(id), input.Data.Attributes.Operation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Remove operation from tool
// @Description Remove an operation from a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param operation body OperationInput true "Operation to remove"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/operations [delete]
// @Security BearerAuth
func (a *API) removeOperationFromTool(c *gin.Context) {
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

	var input OperationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.RemoveOperationFromTool(uint(id), input.Data.Attributes.Operation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
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

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Get tool operations
// @Description Get all operations associated with a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} OperationsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/operations [get]
// @Security BearerAuth
func (a *API) getToolOperations(c *gin.Context) {
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

	operations, err := a.service.GetToolOperations(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": OperationsResponse{Operations: operations}})
}

// api/tool_handlers.go

// Add these new handlers to the existing file:

// @Summary Add FileStore to Tool
// @Description Add a FileStore to a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filestore_id path int true "FileStore ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filestores/{filestore_id} [post]
// @Security BearerAuth
func (a *API) addFileStoreToTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.AddFileStoreToTool(uint(toolID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Remove FileStore from Tool
// @Description Remove a FileStore from a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filestore_id path int true "FileStore ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filestores/{filestore_id} [delete]
// @Security BearerAuth
func (a *API) removeFileStoreFromTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.RemoveFileStoreFromTool(uint(toolID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Get Tool FileStores
// @Description Get all FileStores associated with a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {array} FileStoreResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filestores [get]
// @Security BearerAuth
func (a *API) getToolFileStores(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	fileStores, err := a.service.GetToolFileStores(uint(toolID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeFileStores(fileStores)})
}

// @Summary Set Tool FileStores
// @Description Replace all FileStore associations for a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filestore_ids body []int true "Array of FileStore IDs"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filestores [put]
// @Security BearerAuth
func (a *API) setToolFileStores(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	var fileStoreIDs []uint
	if err := c.ShouldBindJSON(&fileStoreIDs); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.SetToolFileStores(uint(toolID), fileStoreIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Add Filter to Tool
// @Description Add a Filter to a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filter_id path int true "Filter ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filters/{filter_id} [post]
// @Security BearerAuth
func (a *API) addFilterToTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	filterID, err := strconv.ParseUint(c.Param("filter_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filter ID"}},
		})
		return
	}

	err = a.service.AddFilterToTool(uint(toolID), uint(filterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Remove Filter from Tool
// @Description Remove a Filter from a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filter_id path int true "Filter ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filters/{filter_id} [delete]
// @Security BearerAuth
func (a *API) removeFilterFromTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	filterID, err := strconv.ParseUint(c.Param("filter_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filter ID"}},
		})
		return
	}

	err = a.service.RemoveFilterFromTool(uint(toolID), uint(filterID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Get Tool Filters
// @Description Get all Filters associated with a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {array} FilterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filters [get]
// @Security BearerAuth
func (a *API) getToolFilters(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	filters, err := a.service.GetToolFilters(uint(toolID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeFiltersForTool(filters)})
}

// @Summary Set Tool Filters
// @Description Replace all Filter associations for a specific Tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param filter_ids body []int true "Array of Filter IDs"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/filters [put]
// @Security BearerAuth
func (a *API) setToolFilters(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	var filterIDs []uint
	if err := c.ShouldBindJSON(&filterIDs); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.SetToolFilters(uint(toolID), filterIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

func serializeFiltersForTool(filters []models.Filter) []FilterResponse {
	result := make([]FilterResponse, len(filters))
	for i, filter := range filters {
		result[i] = FilterResponse{
			Type: "filters",
			ID:   strconv.FormatUint(uint64(filter.ID), 10),
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Script      []byte `json:"script"`
			}{
				Name:        filter.Name,
				Description: filter.Description,
				Script:      filter.Script,
			},
		}
	}
	return result
}

func serializeTool(tool *models.Tool, db *gorm.DB) ToolResponse {
	fileStores, _ := tool.GetFileStores(db)
	filters, _ := tool.GetFilters(db)
	dependencies, _ := tool.GetDependencies(db)

	response := ToolResponse{
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
			Operations:     tool.GetOperations(),
			AuthKey:        tool.AuthKey,
			AuthSchemaName: tool.AuthSchemaName,
			FileStores:     serializeFileStores(fileStores),
			Filters:        serializeFiltersForTool(filters),
			Dependencies:   serializeToolsPointers(dependencies, db),
		},
	}

	return response
}

func serializeTools(tools []models.Tool, db *gorm.DB) []ToolResponse {
	result := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		result[i] = serializeTool(&tool, db)
	}
	return result
}

func serializeToolsPointers(tools []*models.Tool, db *gorm.DB) []ToolResponse {
	result := make([]ToolResponse, len(tools))
	for i, tool := range tools {
		result[i] = serializeTool(tool, db)
	}
	return result
}

// @Summary Add dependency to tool
// @Description Add a dependency to a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param dependency_id path int true "Dependency ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/dependencies/{dependency_id} [post]
// @Security BearerAuth
func (a *API) addDependencyToTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	dependencyID, err := strconv.ParseUint(c.Param("dependency_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid dependency ID"}},
		})
		return
	}

	err = a.service.AddDependencyToTool(uint(toolID), uint(dependencyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Remove dependency from tool
// @Description Remove a dependency from a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param dependency_id path int true "Dependency ID"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/dependencies/{dependency_id} [delete]
// @Security BearerAuth
func (a *API) removeDependencyFromTool(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	dependencyID, err := strconv.ParseUint(c.Param("dependency_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid dependency ID"}},
		})
		return
	}

	err = a.service.RemoveDependencyFromTool(uint(toolID), uint(dependencyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary Get tool dependencies
// @Description Get all dependencies associated with a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} DependencyListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/dependencies [get]
// @Security BearerAuth
func (a *API) getToolDependencies(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	dependencies, err := a.service.GetToolDependencies(uint(toolID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeToolsPointers(dependencies, a.config.DB)})
}

// @Summary Set tool dependencies
// @Description Replace all dependencies for a specific tool
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param dependency_ids body []int true "Array of dependency IDs"
// @Success 200 {object} ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/dependencies [put]
// @Security BearerAuth
func (a *API) setToolDependencies(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	var dependencyIDs []uint
	if err := c.ShouldBindJSON(&dependencyIDs); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err = a.service.SetToolDependencies(uint(toolID), dependencyIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tool not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTool(tool, a.config.DB)})
}

// @Summary List Tool Operations from OpenAPI Spec
// @Description List all operations available in the tool's OpenAPI specification
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} OperationsListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/spec-operations [get]
// @Security BearerAuth
func (a *API) listToolSpecOperations(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	operations, err := a.service.ListToolOperationsFromSpec(uint(toolID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": struct {
			Type       string   `json:"type"`
			Operations []string `json:"operations"`
		}{
			Type:       "spec-operations",
			Operations: operations,
		},
	})
}

// CallOperationInput represents the input for calling a tool operation
type CallOperationInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			OperationID string                 `json:"operation_id"`
			Parameters  map[string][]string    `json:"parameters"`
			Payload     map[string]interface{} `json:"payload"`
			Headers     map[string][]string    `json:"headers"`
		} `json:"attributes"`
	} `json:"data"`
}

// @Summary Call Tool Operation
// @Description Call an operation from the tool's OpenAPI specification
// @Tags tools
// @Accept json
// @Produce json
// @Param id path int true "Tool ID"
// @Param operation body CallOperationInput true "Operation details"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tools/{id}/call-operation [post]
// @Security BearerAuth
func (a *API) callToolOperation(c *gin.Context) {
	toolID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool ID"}},
		})
		return
	}

	var input CallOperationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	result, err := a.service.CallToolOperation(
		uint(toolID),
		input.Data.Attributes.OperationID,
		input.Data.Attributes.Parameters,
		input.Data.Attributes.Payload,
		input.Data.Attributes.Headers,
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

	c.JSON(http.StatusOK, gin.H{"data": result})
}
