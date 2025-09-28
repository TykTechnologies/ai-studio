package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Plugin workflow API responses

type ValidateAndLoadResponse struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Attributes struct {
		Command      string                 `json:"command"`
		PluginType   string                 `json:"plugin_type"`
		ConfigSchema map[string]interface{} `json:"config_schema,omitempty"`
		Manifest     map[string]interface{} `json:"manifest,omitempty"`
		Scopes       []string               `json:"scopes"`
		Status       string                 `json:"status"`
		LoadedAt     string                 `json:"loaded_at"`
	} `json:"attributes"`
}

type ApprovalRequest struct {
	Approved bool `json:"approved" binding:"required"`
}

type WorkflowStatusResponse struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Attributes struct {
		Status                     string   `json:"status"`
		ServiceScopes              []string `json:"service_scopes"`
		ServiceAccessAuthorized    bool     `json:"service_access_authorized"`
		RequiresApproval          bool     `json:"requires_approval"`
		LastManifestLoadedAt      string   `json:"last_manifest_loaded_at,omitempty"`
	} `json:"attributes"`
}

// @Summary Validate plugin command and load metadata
// @Description Validates a plugin command and loads both config schema and manifest in a single operation
// @Tags plugins,workflow
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param body body object false "Request body with command (for command changes)"
// @Success 200 {object} ValidateAndLoadResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/validate-and-load [post]
// @Security BearerAuth
func (a *API) validateAndLoadPlugin(c *gin.Context) {
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

	// Check if there's a command override in the request body (for command changes)
	var requestBody struct {
		Command string `json:"command"`
	}
	command := plugin.Command
	if err := c.ShouldBindJSON(&requestBody); err == nil && requestBody.Command != "" {
		command = requestBody.Command
		log.Info().
			Uint("plugin_id", uint(id)).
			Str("original_command", plugin.Command).
			Str("new_command", command).
			Msg("Using new command for metadata loading")
	}

	// Create metadata loader if not exists
	if a.service.PluginMetadataLoader == nil {
		a.service.PluginMetadataLoader = services.NewPluginMetadataLoader(a.service.DB, a.service.AIStudioPluginManager)
	}

	// Load plugin metadata (config schema + manifest)
	ctx := c.Request.Context()
	metadata, err := a.service.PluginMetadataLoader.LoadPluginMetadata(ctx, command)
	if err != nil {
		log.Error().
			Err(err).
			Uint("plugin_id", uint(id)).
			Str("command", command).
			Msg("Failed to load plugin metadata")

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to load plugin metadata: %v", err)}},
		})
		return
	}

	// Extract scopes from metadata
	scopes := a.service.PluginMetadataLoader.ExtractScopesFromMetadata(metadata)

	// For AI Studio plugins, store the scopes (but don't authorize yet)
	if plugin.IsAIStudioPlugin() && len(scopes) > 0 {
		// Update plugin with extracted scopes (but keep ServiceAccessAuthorized as false)
		plugin.ServiceScopes = scopes
		plugin.ServiceAccessAuthorized = false

		if err := plugin.Update(a.service.DB); err != nil {
			log.Warn().
				Err(err).
				Uint("plugin_id", uint(id)).
				Msg("Failed to store extracted scopes in plugin")
		}
	}

	// Parse config schema for response
	var configSchema map[string]interface{}
	if metadata.ConfigSchema != "" {
		if err := json.Unmarshal([]byte(metadata.ConfigSchema), &configSchema); err != nil {
			log.Warn().
				Err(err).
				Uint("plugin_id", uint(id)).
				Msg("Failed to parse config schema JSON")
		}
	}

	// Parse manifest for response
	var manifestMap map[string]interface{}
	if metadata.Manifest != nil {
		manifestBytes, err := json.Marshal(metadata.Manifest)
		if err == nil {
			json.Unmarshal(manifestBytes, &manifestMap)
		}
	}

	// Determine status
	status := "ready"
	if plugin.IsAIStudioPlugin() && len(scopes) > 0 {
		status = "scopes_pending" // AI Studio plugins with scopes need approval
	}

	response := ValidateAndLoadResponse{
		Type: "plugin-metadata",
		ID:   idParam,
		Attributes: struct {
			Command      string                 `json:"command"`
			PluginType   string                 `json:"plugin_type"`
			ConfigSchema map[string]interface{} `json:"config_schema,omitempty"`
			Manifest     map[string]interface{} `json:"manifest,omitempty"`
			Scopes       []string               `json:"scopes"`
			Status       string                 `json:"status"`
			LoadedAt     string                 `json:"loaded_at"`
		}{
			Command:      command,
			PluginType:   plugin.PluginType,
			ConfigSchema: configSchema,
			Manifest:     manifestMap,
			Scopes:       scopes,
			Status:       status,
			LoadedAt:     metadata.LoadTime.Format("2006-01-02T15:04:05Z07:00"),
		},
	}

	log.Info().
		Uint("plugin_id", uint(id)).
		Str("command", command).
		Str("plugin_type", plugin.PluginType).
		Int("scopes_count", len(scopes)).
		Bool("has_config_schema", configSchema != nil).
		Bool("has_manifest", manifestMap != nil).
		Str("status", status).
		Msg("Successfully loaded plugin metadata")

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Approve or deny plugin service scopes
// @Description Admin endpoint to approve or deny service access scopes for AI Studio plugins
// @Tags plugins,workflow
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Param body body ApprovalRequest true "Approval decision"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/approve-scopes [post]
// @Security BearerAuth
func (a *API) approvePluginScopes(c *gin.Context) {
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

	var request ApprovalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid request body"}},
		})
		return
	}

	// Use existing service method to handle approval/denial
	err = a.service.PluginService.AuthorizePluginServiceAccess(uint(id), request.Approved)
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

	status := "approved"
	message := "Plugin service scopes approved successfully"
	if !request.Approved {
		status = "denied"
		message = "Plugin service scopes denied"
	}

	log.Info().
		Uint("plugin_id", uint(id)).
		Bool("approved", request.Approved).
		Msg("Plugin service scopes approval decision processed")

	c.JSON(http.StatusOK, gin.H{
		"message": message,
		"status":  status,
	})
}

// @Summary Get plugin workflow status
// @Description Get the current workflow status and approval state for a plugin
// @Tags plugins,workflow
// @Accept json
// @Produce json
// @Param id path int true "Plugin ID"
// @Success 200 {object} WorkflowStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/plugins/{id}/workflow-status [get]
// @Security BearerAuth
func (a *API) getPluginWorkflowStatus(c *gin.Context) {
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

	plugin, err := a.service.PluginService.GetPlugin(uint(id))
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

	// Determine workflow status
	status := "ready"
	requiresApproval := false

	if plugin.IsAIStudioPlugin() && len(plugin.ServiceScopes) > 0 {
		if !plugin.ServiceAccessAuthorized {
			status = "scopes_pending"
			requiresApproval = true
		} else {
			status = "ready"
		}
	}

	response := WorkflowStatusResponse{
		Type: "plugin-workflow-status",
		ID:   idParam,
		Attributes: struct {
			Status                     string   `json:"status"`
			ServiceScopes              []string `json:"service_scopes"`
			ServiceAccessAuthorized    bool     `json:"service_access_authorized"`
			RequiresApproval          bool     `json:"requires_approval"`
			LastManifestLoadedAt      string   `json:"last_manifest_loaded_at,omitempty"`
		}{
			Status:                  status,
			ServiceScopes:           plugin.ServiceScopes,
			ServiceAccessAuthorized: plugin.ServiceAccessAuthorized,
			RequiresApproval:        requiresApproval,
		},
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}