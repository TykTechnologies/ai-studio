package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PluginResponse represents a plugin in API responses
type PluginResponse struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Attributes    struct {
		Name         string                 `json:"name"`
		Description  string                 `json:"description"`
		Command      string                 `json:"command"`
		Checksum     string                 `json:"checksum,omitempty"`
		Config       map[string]interface{} `json:"config"`
		HookType     string                 `json:"hook_type"`
		IsActive     bool                   `json:"is_active"`
		Namespace    string                 `json:"namespace"`
		PluginType   string                 `json:"plugin_type"`   // "gateway" or "ai_studio"
		OCIReference string                 `json:"oci_reference"` // OCI artifact reference (for OCI plugins)
		Manifest     map[string]interface{} `json:"manifest"`      // Plugin manifest for UI extensions
		CreatedAt    string                 `json:"created_at"`
		UpdatedAt    string                 `json:"updated_at"`
	} `json:"attributes"`
	Relationships *struct {
		LLMs struct {
			Data []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Attributes struct {
					Name   string `json:"name"`
					Vendor string `json:"vendor"`
					Active bool   `json:"active"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"llms"`
	} `json:"relationships,omitempty"`
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
// @Param is_active query bool false "Filter by active status"
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
	// Check if namespace parameter was actually provided
	_, namespaceProvided := c.GetQuery("namespace")
	// Handle is_active parameter - if not provided, show all plugins
	isActiveParam := c.Query("is_active")
	var isActive bool
	var filterByActive bool
	if isActiveParam != "" {
		isActive = isActiveParam == "true"
		filterByActive = true
	}
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

	// Determine namespace filter: if not provided, pass special value to indicate "no namespace filtering"
	namespaceFilter := namespace
	if !namespaceProvided {
		namespaceFilter = "__ALL_NAMESPACES__" // Special value to indicate no namespace filtering
	}

	// Use filtering based on whether is_active parameter was provided
	if filterByActive {
		plugins, totalCount, err = a.service.PluginService.ListPlugins(page, limit, hookType, isActive, namespaceFilter)
	} else {
		// Get all plugins (both active and inactive)
		plugins, totalCount, err = a.service.PluginService.ListAllPlugins(page, limit, hookType, namespaceFilter)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
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

	// Serialize plugins inline to avoid N+1 queries from function calls
	for i, plugin := range plugins {
		pluginResponse := PluginResponse{
			Type: "plugins",
			ID:   strconv.FormatUint(uint64(plugin.ID), 10),
			Attributes: struct {
				Name         string                 `json:"name"`
				Description  string                 `json:"description"`
				Command      string                 `json:"command"`
				Checksum     string                 `json:"checksum,omitempty"`
				Config       map[string]interface{} `json:"config"`
				HookType     string                 `json:"hook_type"`
				IsActive     bool                   `json:"is_active"`
				Namespace    string                 `json:"namespace"`
				PluginType   string                 `json:"plugin_type"`   // "gateway" or "ai_studio"
				OCIReference string                 `json:"oci_reference"` // OCI artifact reference (for OCI plugins)
				Manifest     map[string]interface{} `json:"manifest"`      // Plugin manifest for UI extensions
				CreatedAt    string                 `json:"created_at"`
				UpdatedAt    string                 `json:"updated_at"`
			}{
				Name:         plugin.Name,
				Description:  plugin.Description,
				Command:      plugin.Command,
				Checksum:     plugin.Checksum,
				Config:       plugin.Config,
				HookType:     plugin.HookType,
				IsActive:     plugin.IsActive,
				Namespace:    plugin.Namespace,
				PluginType:   plugin.GetCapabilityCategory(),
				OCIReference: plugin.OCIReference,
				Manifest:     plugin.Manifest,
				CreatedAt:    plugin.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt:    plugin.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			},
		}

		// Include LLM relationships if they exist (using preloaded data)
		if len(plugin.LLMs) > 0 {
			pluginResponse.Relationships = &struct {
				LLMs struct {
					Data []struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Attributes struct {
							Name   string `json:"name"`
							Vendor string `json:"vendor"`
							Active bool   `json:"active"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"llms"`
			}{}

			pluginResponse.Relationships.LLMs.Data = make([]struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Attributes struct {
					Name   string `json:"name"`
					Vendor string `json:"vendor"`
					Active bool   `json:"active"`
				} `json:"attributes"`
			}, len(plugin.LLMs))

			for j, llm := range plugin.LLMs {
				pluginResponse.Relationships.LLMs.Data[j] = struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Attributes struct {
						Name   string `json:"name"`
						Vendor string `json:"vendor"`
						Active bool   `json:"active"`
					} `json:"attributes"`
				}{
					Type: "llms",
					ID:   strconv.FormatUint(uint64(llm.ID), 10),
					Attributes: struct {
						Name   string `json:"name"`
						Vendor string `json:"vendor"`
						Active bool   `json:"active"`
					}{
						Name:   llm.Name,
						Vendor: string(llm.Vendor),
						Active: llm.Active,
					},
				}
			}
		}

		response.Data[i] = pluginResponse
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

	// Perform API-level security validation on the command field
	if err := validatePluginCommand(req.Command); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Security Validation Failed", Detail: err.Error()}},
		})
		return
	}

	plugin, err := a.service.PluginService.CreatePlugin(&req)
	if err != nil {
		errMsg := err.Error()

		// Check for validation errors that should return 400 instead of 500
		if strings.Contains(errMsg, "cannot be empty") ||
		   strings.Contains(errMsg, "invalid hook type") {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: err.Error()}},
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

	// Auto-load AI Studio plugins if requested or if hook_type is pending
	shouldAutoLoad := req.LoadImmediately || plugin.HookType == "pending"
	if shouldAutoLoad && a.service.AIStudioPluginManager != nil {
		log.Printf("Auto-loading plugin to fetch manifest: %s (ID: %d)", plugin.Name, plugin.ID)

		// Load the plugin
		if _, loadErr := a.service.AIStudioPluginManager.LoadPlugin(plugin.ID); loadErr != nil {
			log.Printf("Warning: Failed to auto-load plugin %s: %v", plugin.Name, loadErr)
		} else {
			// Auto-fetch and parse manifest
			log.Printf("Auto-fetching manifest for plugin: %s", plugin.Name)
			if manifestJSON, manifestErr := a.service.AIStudioPluginManager.GetPluginManifest(plugin.ID); manifestErr != nil {
				log.Printf("Warning: Failed to auto-fetch manifest for plugin %s: %v", plugin.Name, manifestErr)
			} else {
				// Parse manifest
				manifest := &models.PluginManifest{}
				if parseErr := json.Unmarshal([]byte(manifestJSON), manifest); parseErr != nil {
					log.Printf("Warning: Failed to parse auto-fetched manifest for plugin %s: %v", plugin.Name, parseErr)
				} else {
					// Update hook types from manifest if they were pending
					if plugin.HookType == "pending" && manifest.Capabilities.PrimaryHook != "" {
						log.Printf("Updating plugin hook_type from manifest: %s -> %s", plugin.HookType, manifest.Capabilities.PrimaryHook)

						// Update the plugin with manifest-derived hook types
						updateReq := services.UpdatePluginRequest{
							HookType: &manifest.Capabilities.PrimaryHook,
						}

						// If hook_types_customized is false, also update hook_types array from manifest
						if !plugin.HookTypesCustomized && len(manifest.Capabilities.Hooks) > 0 {
							updateReq.HookTypes = manifest.Capabilities.Hooks
						}

						if _, updateErr := a.service.PluginService.UpdatePlugin(plugin.ID, &updateReq); updateErr != nil {
							log.Printf("Warning: Failed to update plugin hook types from manifest: %v", updateErr)
						} else {
							log.Printf("✅ Updated plugin hook types from manifest")
							// Refresh plugin object for response
							if updatedPlugin, getErr := a.service.PluginService.GetPlugin(plugin.ID); getErr == nil {
								plugin = updatedPlugin
							}
						}
					}

					// Register UI components if it's a studio_ui plugin
					if plugin.SupportsHookType(models.HookTypeStudioUI) {
						if registerErr := a.service.PluginManifestService.RegisterPluginUI(plugin, manifest); registerErr != nil {
							log.Printf("Warning: Failed to auto-register UI for plugin %s: %v", plugin.Name, registerErr)
						} else {
							log.Printf("✅ Auto-loaded and registered UI for plugin: %s", plugin.Name)
						}
					}
				}
			}
		}
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

	// Try to parse as plain UpdatePluginRequest (current UI behavior for PATCH)
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

	// Perform API-level security validation on the command field if it's being updated
	if req.Command != nil {
		if err := validatePluginCommand(*req.Command); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Security Validation Failed", Detail: err.Error()}},
			})
			return
		}
	}

	// Get original plugin state to detect activation changes
	originalPlugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Plugin not found"}},
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

	// Handle plugin activation state changes for AI Studio plugins
	if plugin.SupportsHookType(models.HookTypeStudioUI) && a.service.AIStudioPluginManager != nil {
		wasActive := originalPlugin.IsActive
		isNowActive := plugin.IsActive

		// Plugin was deactivated - unload it
		if wasActive && !isNowActive {
			log.Printf("Plugin deactivated, unloading: %s (ID: %d)", plugin.Name, plugin.ID)

			if a.service.AIStudioPluginManager.IsPluginLoaded(plugin.ID) {
				if unloadErr := a.service.AIStudioPluginManager.UnloadPlugin(plugin.ID); unloadErr != nil {
					log.Printf("Warning: Failed to unload deactivated plugin %s: %v", plugin.Name, unloadErr)
				} else {
					log.Printf("✅ Successfully unloaded deactivated plugin: %s", plugin.Name)

					// Clean up UI registry entries for deactivated plugin
					if a.service.PluginManifestService != nil {
						if unloadUIErr := a.service.PluginManifestService.UnloadPluginUI(plugin.ID); unloadUIErr != nil {
							log.Printf("Warning: Failed to clean up UI for deactivated plugin %s: %v", plugin.Name, unloadUIErr)
						} else {
							log.Printf("✅ Cleaned up UI registry for deactivated plugin: %s", plugin.Name)
						}
					}
				}
			}
		}

		// Plugin was activated - load it if load_immediately is set
		if !wasActive && isNowActive && req.LoadImmediately != nil && *req.LoadImmediately {
			log.Printf("Plugin activated with load_immediately, loading: %s (ID: %d)", plugin.Name, plugin.ID)

			if _, loadErr := a.service.AIStudioPluginManager.LoadPlugin(plugin.ID); loadErr != nil {
				log.Printf("Warning: Failed to auto-load activated plugin %s: %v", plugin.Name, loadErr)
			} else {
				log.Printf("✅ Successfully loaded activated plugin: %s", plugin.Name)
			}
		}
	}

	// Auto-load AI Studio plugins if requested on update
	if req.LoadImmediately != nil && *req.LoadImmediately && plugin.SupportsHookType(models.HookTypeStudioUI) && a.service.AIStudioPluginManager != nil {
		log.Printf("Auto-loading AI Studio plugin after update: %s (ID: %d)", plugin.Name, plugin.ID)

		// Unload if currently loaded to ensure fresh reload
		if a.service.AIStudioPluginManager.IsPluginLoaded(plugin.ID) {
			log.Printf("Unloading existing plugin for fresh reload: %s", plugin.Name)
			if unloadErr := a.service.AIStudioPluginManager.UnloadPlugin(plugin.ID); unloadErr != nil {
				log.Printf("Warning: Failed to unload existing plugin %s: %v", plugin.Name, unloadErr)
			}
		}

		// Load the plugin
		if _, loadErr := a.service.AIStudioPluginManager.LoadPlugin(plugin.ID); loadErr != nil {
			log.Printf("Warning: Failed to auto-load plugin %s: %v", plugin.Name, loadErr)
		} else {
			// Auto-fetch and parse manifest
			log.Printf("Auto-fetching manifest for updated plugin: %s", plugin.Name)
			if manifestJSON, manifestErr := a.service.AIStudioPluginManager.GetPluginManifest(plugin.ID); manifestErr != nil {
				log.Printf("Warning: Failed to auto-fetch manifest for plugin %s: %v", plugin.Name, manifestErr)
			} else {
				// Parse and register UI components
				manifest := &models.PluginManifest{}
				if parseErr := json.Unmarshal([]byte(manifestJSON), manifest); parseErr != nil {
					log.Printf("Warning: Failed to parse auto-fetched manifest for plugin %s: %v", plugin.Name, parseErr)
				} else {
					if registerErr := a.service.PluginManifestService.RegisterPluginUI(plugin, manifest); registerErr != nil {
						log.Printf("Warning: Failed to auto-register UI for plugin %s: %v", plugin.Name, registerErr)
					} else {
						log.Printf("✅ Auto-loaded and registered UI for updated plugin: %s", plugin.Name)
					}
				}
			}
		}
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

	// Get plugin details before deletion to check if it needs cleanup
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Plugin not found"}},
		})
		return
	}

	// Stop the plugin process if it's an AI Studio plugin (UI, Agent, or Object Hooks)
	if plugin.SupportsHookType(models.HookTypeStudioUI) ||
	   plugin.SupportsHookType(models.HookTypeAgent) ||
	   plugin.SupportsHookType(models.HookTypeObjectHooks) {
		if a.service.AIStudioPluginManager != nil {
			if a.service.AIStudioPluginManager.IsPluginLoaded(plugin.ID) {
				log.Printf("Stopping plugin process before deletion: %s (ID: %d)", plugin.Name, plugin.ID)

				// Unload the plugin (this stops the process)
				if unloadErr := a.service.AIStudioPluginManager.UnloadPlugin(plugin.ID); unloadErr != nil {
					log.Printf("Warning: Failed to stop plugin process during deletion: %v", unloadErr)
					// Continue with deletion even if unload fails
				} else {
					log.Printf("✅ Successfully stopped plugin process: %s", plugin.Name)
				}

				// Clean up UI registry entries
				if a.service.PluginManifestService != nil {
					if unloadUIErr := a.service.PluginManifestService.UnloadPluginUI(plugin.ID); unloadUIErr != nil {
						log.Printf("Warning: Failed to clean up UI registry during deletion: %v", unloadUIErr)
					} else {
						log.Printf("✅ Cleaned up UI registry for deleted plugin: %s", plugin.Name)
					}
				}
			}
		}
	}

	// Now delete the plugin from the database
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

	log.Printf("✅ Plugin deleted successfully: %s (ID: %d)", plugin.Name, plugin.ID)
	c.Status(http.StatusNoContent)
}

// @Summary Clear plugin data
// @Description Delete all key-value data stored by a plugin
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 204 "No Content - plugin data cleared successfully"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/data [delete]
// @Security BearerAuth
func (a *API) clearPluginData(c *gin.Context) {
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

	// Create plugin KV service
	pluginKVService := services.NewPluginKVService(a.service.GetDB())

	// Clear all plugin data
	if err := pluginKVService.ClearAllPluginData(uint(id)); err != nil {
		if strings.Contains(err.Error(), "plugin not found") {
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

// @Summary Reload AI Studio plugin
// @Description Reload an AI Studio plugin and auto-fetch its manifest
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/reload [post]
// @Security BearerAuth
func (a *API) reloadPlugin(c *gin.Context) {
	if a.service.AIStudioPluginManager == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "AI Studio plugin manager not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	// Get plugin to verify it's an AI Studio plugin
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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

	if !plugin.SupportsHookType(models.HookTypeStudioUI) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Only AI Studio UI plugins can be reloaded"}},
		})
		return
	}

	log.Printf("Reloading AI Studio plugin: %s (ID: %d)", plugin.Name, plugin.ID)

	// Unload plugin if currently loaded to force fresh reload
	if a.service.AIStudioPluginManager.IsPluginLoaded(uint(id)) {
		log.Printf("Unloading existing plugin process for: %s", plugin.Name)
		if unloadErr := a.service.AIStudioPluginManager.UnloadPlugin(uint(id)); unloadErr != nil {
			log.Printf("Warning: Failed to unload existing plugin %s: %v", plugin.Name, unloadErr)
		}
	}

	// Load the plugin fresh (new process)
	if _, loadErr := a.service.AIStudioPluginManager.LoadPlugin(uint(id)); loadErr != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to load plugin: %v", loadErr)}},
		})
		return
	}

	// Auto-fetch and parse manifest
	log.Printf("Auto-fetching manifest for reloaded plugin: %s", plugin.Name)
	manifestJSON, manifestErr := a.service.AIStudioPluginManager.GetPluginManifest(uint(id))
	if manifestErr != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to fetch manifest: %v", manifestErr)}},
		})
		return
	}

	// Parse and register UI components
	manifest := &models.PluginManifest{}
	if parseErr := json.Unmarshal([]byte(manifestJSON), manifest); parseErr != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to parse manifest: %v", parseErr)}},
		})
		return
	}

	if registerErr := a.service.PluginManifestService.RegisterPluginUI(plugin, manifest); registerErr != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to register UI: %v", registerErr)}},
		})
		return
	}

	log.Printf("✅ Successfully reloaded and registered UI for plugin: %s", plugin.Name)

	c.JSON(http.StatusOK, gin.H{
		"message": "Plugin reloaded and manifest registered successfully",
		"plugin":  serializePlugin(plugin),
	})
}

// serializePlugin converts a Plugin model to API response format
func serializePlugin(plugin *models.Plugin) PluginResponse {
	response := PluginResponse{
		Type: "plugins",
		ID:   strconv.FormatUint(uint64(plugin.ID), 10),
		Attributes: struct {
			Name         string                 `json:"name"`
			Description  string                 `json:"description"`
			Command      string                 `json:"command"`
			Checksum     string                 `json:"checksum,omitempty"`
			Config       map[string]interface{} `json:"config"`
			HookType     string                 `json:"hook_type"`
			IsActive     bool                   `json:"is_active"`
			Namespace    string                 `json:"namespace"`
			PluginType   string                 `json:"plugin_type"`   // "gateway" or "ai_studio"
			OCIReference string                 `json:"oci_reference"` // OCI artifact reference (for OCI plugins)
			Manifest     map[string]interface{} `json:"manifest"`      // Plugin manifest for UI extensions
			CreatedAt    string                 `json:"created_at"`
			UpdatedAt    string                 `json:"updated_at"`
		}{
			Name:         plugin.Name,
			Description:  plugin.Description,
			Command:      plugin.Command,
			Checksum:     plugin.Checksum,
			Config:       plugin.Config,
			HookType:     plugin.HookType,
			IsActive:     plugin.IsActive,
			Namespace:    plugin.Namespace,
			PluginType:   plugin.GetCapabilityCategory(),
			OCIReference: plugin.OCIReference,
			Manifest:     plugin.Manifest,
			CreatedAt:    plugin.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    plugin.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}

	// Include LLM relationships if they exist
	if len(plugin.LLMs) > 0 {
		response.Relationships = &struct {
			LLMs struct {
				Data []struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Attributes struct {
						Name   string `json:"name"`
						Vendor string `json:"vendor"`
						Active bool   `json:"active"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"llms"`
		}{}

		response.Relationships.LLMs.Data = make([]struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Attributes struct {
				Name   string `json:"name"`
				Vendor string `json:"vendor"`
				Active bool   `json:"active"`
			} `json:"attributes"`
		}, len(plugin.LLMs))

		for i, llm := range plugin.LLMs {
			response.Relationships.LLMs.Data[i] = struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Attributes struct {
					Name   string `json:"name"`
					Vendor string `json:"vendor"`
					Active bool   `json:"active"`
				} `json:"attributes"`
			}{
				Type: "llms",
				ID:   strconv.FormatUint(uint64(llm.ID), 10),
				Attributes: struct {
					Name   string `json:"name"`
					Vendor string `json:"vendor"`
					Active bool   `json:"active"`
				}{
					Name:   llm.Name,
					Vendor: string(llm.Vendor),
					Active: llm.Active,
				},
			}
		}
	}

	return response
}

// OCI Plugin Endpoints

// @Summary Create plugin from OCI artifact
// @Description Create a new AI Studio plugin from an OCI artifact reference
// @Tags plugins
// @Accept json
// @Produce json
// @Param plugin body services.CreateOCIPluginRequest true "OCI Plugin"
// @Success 201 {object} PluginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/oci [post]
// @Security BearerAuth
func (a *API) createOCIPlugin(c *gin.Context) {
	var req services.CreateOCIPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	plugin, err := a.service.PluginService.CreateOCIPluginFromReference(&req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
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

// @Summary List cached OCI plugins
// @Description Get a list of all cached OCI plugins from the local cache
// @Tags plugins
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/oci/cached [get]
// @Security BearerAuth
func (a *API) listCachedOCIPlugins(c *gin.Context) {
	plugins, err := a.service.PluginService.ListCachedOCIPlugins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plugins})
}

// @Summary Refresh OCI plugin
// @Description Refresh an OCI plugin from the registry to get the latest version
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} PluginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/refresh [post]
// @Security BearerAuth
func (a *API) refreshOCIPlugin(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	plugin, err := a.service.PluginService.RefreshOCIPlugin(uint(id))
	if err != nil {
		if strings.Contains(err.Error(), "not found") || err == gorm.ErrRecordNotFound {
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

// @Summary Get plugins by type
// @Description Get plugins filtered by plugin type (gateway or ai_studio)
// @Tags plugins
// @Accept json
// @Produce json
// @Param type path string true "Plugin type" Enums(gateway, ai_studio)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/type/{type} [get]
// @Security BearerAuth
func (a *API) getPluginsByType(c *gin.Context) {
	typeParam := c.Param("type")

	// Map old type names to hook types for backward compatibility
	var hookTypes []string
	switch typeParam {
	case "gateway":
		hookTypes = []string{models.HookTypeAuth, models.HookTypePreAuth, models.HookTypePostAuth, models.HookTypeOnResponse, models.HookTypeDataCollection}
	case "ai_studio":
		hookTypes = []string{models.HookTypeStudioUI, models.HookTypeAgent, models.HookTypeObjectHooks}
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin type. Must be 'gateway' or 'ai_studio'"}},
		})
		return
	}

	// Get all plugins and filter by hook types
	allPlugins, _, err := a.service.PluginService.ListAllPlugins(1, 10000, "", "__ALL_NAMESPACES__")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Filter plugins that support any of the requested hook types
	var plugins []models.Plugin
	for _, plugin := range allPlugins {
		for _, hookType := range hookTypes {
			if plugin.SupportsHookType(hookType) {
				plugins = append(plugins, plugin)
				break
			}
		}
	}

	// Serialize response
	response := make([]PluginResponse, len(plugins))
	for i, plugin := range plugins {
		response[i] = serializePlugin(&plugin)
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Get AI Studio plugins with manifests
// @Description Get AI Studio plugins that have UI extension manifests
// @Tags plugins
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/ai-studio/manifests [get]
// @Security BearerAuth
func (a *API) getAIStudioPluginsWithManifests(c *gin.Context) {
	// Get all plugins and filter for studio_ui hook type with manifests
	allPlugins, _, err := a.service.PluginService.ListAllPlugins(1, 10000, "", "__ALL_NAMESPACES__")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Filter for AI Studio UI plugins with manifests
	var plugins []models.Plugin
	for _, plugin := range allPlugins {
		if plugin.SupportsHookType(models.HookTypeStudioUI) && len(plugin.Manifest) > 0 {
			plugins = append(plugins, plugin)
		}
	}

	// Serialize response
	response := make([]PluginResponse, len(plugins))
	for i, plugin := range plugins {
		response[i] = serializePlugin(&plugin)
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// Plugin UI Management Endpoints

// @Summary Get UI registry
// @Description Get all registered UI components from plugins
// @Tags plugins
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/ui-registry [get]
// @Security BearerAuth
func (a *API) getUIRegistry(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	entries, err := a.service.PluginManifestService.GetUIRegistry()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// @Summary Get sidebar menu items
// @Description Get sidebar menu items contributed by plugins
// @Tags plugins
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/sidebar-menu [get]
// @Security BearerAuth
func (a *API) getSidebarMenuItems(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	menuItems, err := a.service.PluginManifestService.GetSidebarMenuItems()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": menuItems})
}

// @Summary Load plugin UI
// @Description Mark a plugin's UI as loaded
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/ui/load [post]
// @Security BearerAuth
func (a *API) loadPluginUI(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	err = a.service.PluginManifestService.LoadPluginUI(uint(id))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: err.Error()}},
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

	c.JSON(http.StatusOK, gin.H{"message": "Plugin UI loaded successfully"})
}

// @Summary Unload plugin UI
// @Description Mark a plugin's UI as unloaded
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/ui/unload [post]
// @Security BearerAuth
func (a *API) unloadPluginUI(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	err = a.service.PluginManifestService.UnloadPluginUI(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Plugin UI unloaded successfully"})
}

// @Summary Parse plugin manifest
// @Description Parse and register a plugin's manifest
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/manifest/parse [post]
// @Security BearerAuth
func (a *API) parsePluginManifest(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	// Get plugin
	log.Printf("DEBUG MANIFEST: Getting plugin ID %d", id)
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		log.Printf("DEBUG MANIFEST: Failed to get plugin ID %d: %v", id, err)
		if strings.Contains(err.Error(), "not found") || err == gorm.ErrRecordNotFound {
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

	// For AI Studio plugins, get manifest from running plugin via gRPC
	log.Printf("DEBUG MANIFEST: Plugin retrieved - ID=%d, Name=%s, Hooks=%v", plugin.ID, plugin.Name, plugin.GetAllHookTypes())
	var manifest *models.PluginManifest

	if plugin.SupportsHookType(models.HookTypeStudioUI) && a.service.AIStudioPluginManager != nil {
		log.Printf("DEBUG MANIFEST: Taking AI Studio path for plugin ID %d", plugin.ID)
		// Ensure plugin is loaded
		if !a.service.AIStudioPluginManager.IsPluginLoaded(uint(id)) {
			_, loadErr := a.service.AIStudioPluginManager.LoadPlugin(uint(id))
			if loadErr != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Bad Request", Detail: fmt.Sprintf("Failed to load plugin: %v", loadErr)}},
				})
				return
			}
		}

		// Get manifest from plugin via gRPC
		log.Printf("DEBUG MANIFEST: Getting manifest via gRPC for plugin ID %d", plugin.ID)
		manifestJSON, manifestErr := a.service.AIStudioPluginManager.GetPluginManifest(uint(id))
		if manifestErr != nil {
			log.Printf("DEBUG MANIFEST: Failed to get manifest via gRPC: %v", manifestErr)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: fmt.Sprintf("Failed to get manifest from plugin: %v", manifestErr)}},
			})
			return
		}

		// Parse the manifest JSON
		log.Printf("DEBUG MANIFEST: Parsing manifest JSON (%d bytes)", len(manifestJSON))
		manifest = &models.PluginManifest{}
		if parseErr := json.Unmarshal([]byte(manifestJSON), manifest); parseErr != nil {
			log.Printf("DEBUG MANIFEST: Failed to parse manifest JSON: %v", parseErr)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: fmt.Sprintf("Failed to parse manifest JSON: %v", parseErr)}},
			})
			return
		}

		// Validate manifest
		if validateErr := manifest.ValidateManifest(); validateErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: fmt.Sprintf("Invalid manifest: %v", validateErr)}},
			})
			return
		}

		// Update the plugin's manifest field in the database
		manifestMap := make(map[string]interface{})
		if manifestBytes, err := json.Marshal(manifest); err == nil {
			if err := json.Unmarshal(manifestBytes, &manifestMap); err == nil {
				plugin.Manifest = manifestMap
				// Update plugin via service to ensure proper handling
				updateReq := &services.UpdatePluginRequest{
					Namespace: &plugin.Namespace,
				}
				if _, updateErr := a.service.PluginService.UpdatePlugin(plugin.ID, updateReq); updateErr != nil {
					log.Printf("Warning: Failed to update plugin via service: %v", updateErr)
				}
			}
		}
	} else {
		// For non-AI Studio plugins, use existing manifest parsing
		log.Printf("DEBUG MANIFEST: Taking non-AI Studio path for plugin ID %d", plugin.ID)
		var parseErr error
		manifest, parseErr = a.service.PluginManifestService.ParsePluginManifest(plugin)
		if parseErr != nil {
			log.Printf("DEBUG MANIFEST: Failed to parse non-AI Studio manifest: %v", parseErr)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: parseErr.Error()}},
			})
			return
		}
	}

	// Register UI components
	log.Printf("DEBUG MANIFEST: About to register UI for plugin ID %d, name %s", plugin.ID, plugin.Name)
	log.Printf("DEBUG MANIFEST: Manifest parsed successfully - ID=%s, Version=%s", manifest.ID, manifest.Version)
	err = a.service.PluginManifestService.RegisterPluginUI(plugin, manifest)
	if err != nil {
		log.Printf("DEBUG MANIFEST: RegisterPluginUI failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Plugin manifest parsed and UI registered successfully",
		"manifest": manifest,
	})
}

// @Summary Serve plugin asset
// @Description Serve static assets for plugins (JS, CSS, images, etc.)
// @Tags plugins
// @Accept json
// @Produce application/javascript,text/css,image/*
// @Param id path int true "Plugin ID"
// @Param filepath path string true "Asset file path"
// @Success 200 {file} file
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /plugins/assets/{id}/{filepath} [get]
func (a *API) servePluginAsset(c *gin.Context) {
	if a.service.PluginManifestService == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Plugin manifest service not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	assetPath := c.Param("filepath")
	if assetPath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Asset path is required"}},
		})
		return
	}

	// Get asset from loaded plugin via gRPC
	if a.service.AIStudioPluginManager == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "AI Studio plugin manager not configured"}},
		})
		return
	}

	// Check if plugin is loaded (don't auto-load on asset requests)
	if !a.service.AIStudioPluginManager.IsPluginLoaded(uint(id)) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: fmt.Sprintf("Plugin %d not loaded - please load the plugin first", id)}},
		})
		return
	}

	// Get asset content from plugin via gRPC
	content, mimeType, err := a.service.AIStudioPluginManager.GetPluginAsset(uint(id), assetPath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not loaded") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: err.Error()}},
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

	// Serve the asset content with proper MIME type and CORS headers for dynamic import
	c.Header("Content-Type", mimeType)
	c.Header("Content-Length", fmt.Sprintf("%d", len(content)))
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Data(http.StatusOK, mimeType, content)
}

// @Summary Get plugin status
// @Description Get the runtime status of a plugin (loaded, healthy, etc.)
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/status [get]
// @Security BearerAuth
func (a *API) getPluginStatus(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	// Get plugin from database
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Plugin not found"}},
		})
		return
	}

	status := map[string]interface{}{
		"plugin_id":       plugin.ID,
		"plugin_name":     plugin.Name,
		"plugin_category": plugin.GetCapabilityCategory(),
		"hook_types":      plugin.GetAllHookTypes(),
		"is_active":       plugin.IsActive,
		"command":         plugin.Command,
		"is_oci":          plugin.IsOCIPlugin(),
		"is_local":        plugin.IsLocalPlugin(),
		"is_grpc":         plugin.IsGRPCPlugin(),
		"hook_type":       plugin.HookType,
		"loaded":          false,
		"healthy":         false,
		"load_time":       nil,
		"last_ping":       nil,
		"error":           nil,
	}

	// Check if plugin is loaded (for AI Studio plugins)
	if plugin.SupportsHookType(models.HookTypeStudioUI) && a.service.AIStudioPluginManager != nil {
		if loadedPlugin, exists := a.service.AIStudioPluginManager.GetLoadedPlugin(uint(id)); exists {
			status["loaded"] = true
			status["healthy"] = loadedPlugin.IsHealthy
			status["load_time"] = loadedPlugin.LoadTime
			status["last_ping"] = loadedPlugin.LastPing

			// Try to ping the plugin
			if pingErr := a.service.AIStudioPluginManager.PingPlugin(uint(id)); pingErr != nil {
				status["healthy"] = false
				status["error"] = pingErr.Error()
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": status})
}

// @Summary List loaded plugins
// @Description Get status of all loaded AI Studio plugins
// @Tags plugins
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/plugins/loaded [get]
// @Security BearerAuth
func (a *API) getLoadedPlugins(c *gin.Context) {
	if a.service.AIStudioPluginManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": []map[string]interface{}{},
			"message": "AI Studio plugin manager not configured",
		})
		return
	}

	// Get all AI Studio plugins
	plugins, err := a.service.PluginService.GetPluginsByType("ai_studio")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	var statuses []map[string]interface{}
	for _, plugin := range plugins {
		status := map[string]interface{}{
			"plugin_id":   plugin.ID,
			"plugin_name": plugin.Name,
			"command":     plugin.Command,
			"is_oci":      plugin.IsOCIPlugin(),
			"loaded":      false,
			"healthy":     false,
		}

		if loadedPlugin, exists := a.service.AIStudioPluginManager.GetLoadedPlugin(plugin.ID); exists {
			status["loaded"] = true
			status["healthy"] = loadedPlugin.IsHealthy
			status["load_time"] = loadedPlugin.LoadTime
			status["last_ping"] = loadedPlugin.LastPing
		}

		statuses = append(statuses, status)
	}

	c.JSON(http.StatusOK, gin.H{"data": statuses})
}

// @Summary Call plugin RPC method
// @Description Execute RPC call on loaded AI Studio plugin
// @Tags plugins
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param method path string true "RPC method name"
// @Param payload body map[string]interface{} false "RPC payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/rpc/{method} [post]
// @Security BearerAuth
func (a *API) callPluginRPC(c *gin.Context) {
	if a.service.AIStudioPluginManager == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "AI Studio plugin manager not configured"}},
		})
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID"}},
		})
		return
	}

	method := c.Param("method")
	if method == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "RPC method is required"}},
		})
		return
	}

	// Get request payload
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		// Allow empty payload
		payload = make(map[string]interface{})
	}

	// Validate plugin exists and is active
	plugin, err := a.service.PluginService.GetPlugin(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Plugin not found"}},
		})
		return
	}

	if !plugin.IsActive {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Plugin is not active - RPC calls are disabled"}},
		})
		return
	}

	// Validate plugin is loaded
	if !a.service.AIStudioPluginManager.IsPluginLoaded(uint(id)) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Plugin not loaded"}},
		})
		return
	}

	// TODO: Validate RPC permissions from manifest
	// For MVP, allow all RPC calls to loaded plugins

	// Call plugin RPC method
	response, err := a.service.AIStudioPluginManager.CallPluginRPC(uint(id), method, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// getPluginConfigSchema retrieves the configuration schema for a plugin
func (a *API) getPluginConfigSchema(c *gin.Context) {
	// Parse plugin ID from URL
	pluginIDStr := c.Param("id")
	if pluginIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Plugin ID is required"}},
		})
		return
	}

	id, err := strconv.Atoi(pluginIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID format"}},
		})
		return
	}

	// Get plugin config schema
	ctx := c.Request.Context()
	schemaJSON, err := a.service.PluginService.GetPluginConfigSchema(ctx, uint(id))
	if err != nil {
		log.Printf("Failed to get plugin config schema: %v", err)

		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
		}
		return
	}

	// Parse schema to validate it's valid JSON
	var schemaObj interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &schemaObj); err != nil {
		log.Printf("Plugin returned invalid JSON schema: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Plugin returned invalid JSON schema"}},
		})
		return
	}

	// Return schema with metadata
	response := gin.H{
		"data": gin.H{
			"type": "plugin-config-schema",
			"id":   pluginIDStr,
			"attributes": gin.H{
				"schema": schemaObj,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// refreshPluginConfigSchema forces a refresh of the configuration schema for a plugin
func (a *API) refreshPluginConfigSchema(c *gin.Context) {
	// Parse plugin ID from URL
	pluginIDStr := c.Param("id")
	if pluginIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Plugin ID is required"}},
		})
		return
	}

	id, err := strconv.Atoi(pluginIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid plugin ID format"}},
		})
		return
	}

	// Refresh plugin config schema (bypasses cache)
	ctx := c.Request.Context()
	schemaJSON, err := a.service.PluginService.RefreshPluginConfigSchema(ctx, uint(id))
	if err != nil {
		log.Printf("Failed to refresh plugin config schema: %v", err)

		// Check if it's a not found error
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin not found"}},
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
		}
		return
	}

	// Parse schema to validate it's valid JSON
	var schemaObj interface{}
	if err := json.Unmarshal([]byte(schemaJSON), &schemaObj); err != nil {
		log.Printf("Plugin returned invalid JSON schema on refresh: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Plugin returned invalid JSON schema"}},
		})
		return
	}

	// Return refreshed schema with metadata
	response := gin.H{
		"data": gin.H{
			"type": "plugin-config-schema",
			"id":   pluginIDStr,
			"attributes": gin.H{
				"schema": schemaObj,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}