package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PluginResponse represents a plugin in API responses
type PluginResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name        string                 `json:"name"`
		Slug        string                 `json:"slug"`
		Description string                 `json:"description"`
		Command     string                 `json:"command"`
		Checksum    string                 `json:"checksum,omitempty"`
		Config      map[string]interface{} `json:"config"`
		HookType    string                 `json:"hook_type"`
		IsActive    bool                   `json:"is_active"`
		Namespace   string                 `json:"namespace"`
		CreatedAt   string                 `json:"created_at"`
		UpdatedAt   string                 `json:"updated_at"`
	} `json:"attributes"`
}

// PluginListResponse represents a list of plugins
type PluginListResponse struct {
	Data []PluginResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// @Summary List plugins
// @Description Get a list of plugins with optional filtering
// @Tags plugins
// @Accept json
// @Produce json
// @Param hook_type query string false "Filter by hook type (pre_auth, auth, post_auth, on_response, data_collection)"
// @Param active query bool false "Filter by active status" default(true)
// @Param namespace query string false "Filter by namespace"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} PluginListResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins [get]
// @Security BearerAuth
func (a *API) listPlugins(c *gin.Context) {
	// Parse query parameters
	hookType := c.Query("hook_type")
	namespace := c.Query("namespace")
	isActive := c.DefaultQuery("active", "true") == "true"
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	// Validate parameters
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var plugins []models.Plugin
	var totalCount int64
	var err error

	if namespace != "" {
		// Use namespace-aware method
		plugins, err = a.service.PluginService.GetActivePluginsInNamespace(namespace)
		totalCount = int64(len(plugins))
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
	} else {
		// Use standard pagination method
		plugins, totalCount, err = a.service.PluginService.ListPlugins(page, limit, hookType, isActive)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
	}

	// Calculate pagination
	totalPages := int(totalCount) / limit
	if int(totalCount)%limit != 0 {
		totalPages++
	}

	// Serialize response
	response := PluginListResponse{
		Data: make([]PluginResponse, len(plugins)),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: totalPages,
			PageSize:   limit,
			PageNumber: page,
		},
	}

	for i, plugin := range plugins {
		response.Data[i] = serializePlugin(&plugin)
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Create plugin
// @Description Create a new plugin configuration
// @Tags plugins
// @Accept json
// @Produce json
// @Param plugin body services.CreatePluginRequest true "Plugin configuration"
// @Success 201 {object} PluginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins [post]
// @Security BearerAuth
func (a *API) createPlugin(c *gin.Context) {
	var req services.CreatePluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	plugin, err := a.service.PluginService.CreatePlugin(&req)
	if err != nil {
		if err.Error() == "plugin slug '"+req.Slug+"' already exists" {
			c.JSON(http.StatusConflict, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Conflict", Detail: err.Error()}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializePlugin(plugin)})
}

// @Summary Get plugin
// @Description Get a specific plugin by ID
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id} [get]
// @Security BearerAuth
func (a *API) getPlugin(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		if err.Error() == "plugin not found: "+strconv.FormatUint(id, 10) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializePlugin(plugin)})
}

// @Summary Update plugin
// @Description Update an existing plugin configuration
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param plugin body services.UpdatePluginRequest true "Updated plugin configuration"
// @Success 200 {object} PluginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id} [patch]
// @Security BearerAuth
func (a *API) updatePlugin(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	var req services.UpdatePluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	plugin, err := a.service.PluginService.UpdatePlugin(uint(id), &req)
	if err != nil {
		if err.Error() == "plugin not found: "+strconv.FormatUint(id, 10) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializePlugin(plugin)})
}

// @Summary Delete plugin
// @Description Delete a plugin configuration
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id} [delete]
// @Security BearerAuth
func (a *API) deletePlugin(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	if err := a.service.PluginService.DeletePlugin(uint(id)); err != nil {
		if err.Error() == "plugin not found: "+strconv.FormatUint(id, 10) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Test plugin
// @Description Test a plugin configuration
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param test_data body map[string]interface{} false "Test data for plugin"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/test [post]
// @Security BearerAuth
func (a *API) testPlugin(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	var testData map[string]interface{}
	if err := c.ShouldBindJSON(&testData); err != nil {
		// Empty body is okay for testing
		testData = make(map[string]interface{})
	}

	result, err := a.service.PluginService.TestPlugin(uint(id), testData)
	if err != nil {
		if err.Error() == "plugin not found: "+strconv.FormatUint(id, 10) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
			return
		}
		
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

// @Summary Get LLM plugins
// @Description Get plugins associated with an LLM
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Success 200 {array} PluginResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/llms/{id}/plugins [get]
// @Security BearerAuth
func (a *API) getLLMPlugins(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	plugins, err := a.service.PluginService.GetPluginsForLLM(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	response := make([]PluginResponse, len(plugins))
	for i, plugin := range plugins {
		response[i] = serializePlugin(&plugin)
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Update LLM plugins
// @Description Update plugin associations for an LLM
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Param plugin_ids body struct{PluginIDs []uint `json:"plugin_ids"`} true "Plugin IDs in execution order"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/llms/{id}/plugins [put]
// @Security BearerAuth
func (a *API) updateLLMPlugins(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	var req struct {
		PluginIDs []uint `json:"plugin_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if err := a.service.PluginService.UpdateLLMPlugins(uint(id), req.PluginIDs); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "LLM or plugin not found"}},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "LLM plugins updated successfully"})
}

// serializePlugin converts a Plugin model to API response format
func serializePlugin(plugin *models.Plugin) PluginResponse {
	return PluginResponse{
		Type: "plugins",
		ID:   strconv.FormatUint(uint64(plugin.ID), 10),
		Attributes: struct {
			Name        string                 `json:"name"`
			Slug        string                 `json:"slug"`
			Description string                 `json:"description"`
			Command     string                 `json:"command"`
			Checksum    string                 `json:"checksum,omitempty"`
			Config      map[string]interface{} `json:"config"`
			HookType    string                 `json:"hook_type"`
			IsActive    bool                   `json:"is_active"`
			Namespace   string                 `json:"namespace"`
			CreatedAt   string                 `json:"created_at"`
			UpdatedAt   string                 `json:"updated_at"`
		}{
			Name:        plugin.Name,
			Slug:        plugin.Slug,
			Description: plugin.Description,
			Command:     plugin.Command,
			Checksum:    plugin.Checksum,
			Config:      plugin.Config,
			HookType:    plugin.HookType,
			IsActive:    plugin.IsActive,
			Namespace:   plugin.Namespace,
			CreatedAt:   plugin.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   plugin.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
}