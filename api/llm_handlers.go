package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new LLM
// @Description Create a new LLM with the provided information
// @Tags llms
// @Accept json
// @Produce json
// @Param llm body LLMInput true "LLM information"
// @Success 201 {object} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms [post]
// @Security BearerAuth
func (a *API) createLLM(c *gin.Context) {
	var input LLMInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	filters := []*models.Filter{}
	for _, f := range input.Data.Attributes.Filters {
		a.service.GetFilterByID(uint(f))
		filters = append(filters, &models.Filter{ID: uint(f)})
	}

	// Use namespace-aware service method if namespace is provided
	var llm *models.LLM
	var err error
	if input.Data.Attributes.Namespace != "" {
		llm, err = a.service.CreateLLMWithNamespace(
			input.Data.Attributes.Name,
			input.Data.Attributes.APIKey,
			input.Data.Attributes.APIEndpoint,
			input.Data.Attributes.PrivacyScore,
			input.Data.Attributes.ShortDescription,
			input.Data.Attributes.LongDescription,
			input.Data.Attributes.LogoURL,
			models.Vendor(input.Data.Attributes.Vendor),
			input.Data.Attributes.Active,
			filters,
			input.Data.Attributes.DefaultModel,
			input.Data.Attributes.AllowedModels,
			input.Data.Attributes.MonthlyBudget,
			parseBudgetStartDate(input.Data.Attributes.BudgetStartDate),
			input.Data.Attributes.Namespace,
		)
	} else {
		llm, err = a.service.CreateLLM(
			input.Data.Attributes.Name,
			input.Data.Attributes.APIKey,
			input.Data.Attributes.APIEndpoint,
			input.Data.Attributes.PrivacyScore,
			input.Data.Attributes.ShortDescription,
			input.Data.Attributes.LongDescription,
			input.Data.Attributes.LogoURL,
			models.Vendor(input.Data.Attributes.Vendor),
			input.Data.Attributes.Active,
			filters,
			input.Data.Attributes.DefaultModel,
			input.Data.Attributes.AllowedModels,
			input.Data.Attributes.MonthlyBudget,
			parseBudgetStartDate(input.Data.Attributes.BudgetStartDate),
		)
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

	if llm.Active {
		if a.proxy != nil {
			a.proxy.Reload()
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": a.serializeLLM(llm)})
}

// @Summary Get an LLM by ID
// @Description Get details of an LLM by its ID
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Success 200 {object} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llms/{id} [get]
// @Security BearerAuth
func (a *API) getLLM(c *gin.Context) {
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

	llm, err := a.service.GetLLMByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "LLM not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLM(llm)})
}

// @Summary Update an LLM
// @Description Update an existing LLM's information
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Param llm body LLMInput true "Updated LLM information"
// @Success 200 {object} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/{id} [patch]
// @Security BearerAuth
func (a *API) updateLLM(c *gin.Context) {
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

	var input LLMInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	thisLLM, err := a.service.GetLLMByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "LLM not found"}},
		})
		return
	}

	filters := []*models.Filter{}
	for _, f := range input.Data.Attributes.Filters {
		a.service.GetFilterByID(uint(f))
		filters = append(filters, &models.Filter{ID: uint(f)})
	}

	llm, err := a.service.UpdateLLM(
		c.Request.Context(),
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.APIKey,
		input.Data.Attributes.APIEndpoint,
		input.Data.Attributes.PrivacyScore,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.LogoURL,
		models.Vendor(input.Data.Attributes.Vendor),
		input.Data.Attributes.Active,
		filters,
		input.Data.Attributes.DefaultModel,
		input.Data.Attributes.AllowedModels,
		input.Data.Attributes.MonthlyBudget,
		parseBudgetStartDate(input.Data.Attributes.BudgetStartDate),
		input.Data.Attributes.Namespace,
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

	// Reload proxy if:
	// 1. Active state changed (either way)
	// 2. LLM is active and any other attributes changed
	if a.proxy != nil {
		activeStateChanged := thisLLM.Active != input.Data.Attributes.Active
		hasChanges := (thisLLM.Name != input.Data.Attributes.Name ||
			thisLLM.APIKey != input.Data.Attributes.APIKey ||
			thisLLM.APIEndpoint != input.Data.Attributes.APIEndpoint ||
			thisLLM.DefaultModel != input.Data.Attributes.DefaultModel ||
			!sliceEqual(thisLLM.AllowedModels, input.Data.Attributes.AllowedModels) ||
			len(thisLLM.Filters) != len(filters))

		if activeStateChanged || (input.Data.Attributes.Active && hasChanges) {
			a.proxy.Reload()
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLM(llm)})
}

func parseBudgetStartDate(dateStr *string) *time.Time {
	if dateStr == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *dateStr)
	if err != nil {
		return nil
	}
	return &t
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// @Summary Delete an LLM
// @Description Delete an LLM by its ID
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/{id} [delete]
// @Security BearerAuth
func (a *API) deleteLLM(c *gin.Context) {
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

	err = a.service.DeleteLLM(c.Request.Context(), uint(id))
	if err != nil {
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

// @Summary List all LLMs
// @Description Get a list of all LLMs
// @Tags llms
// @Accept json
// @Produce json
// @Success 200 {array} LLMResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms [get]
// @Security BearerAuth
func (a *API) listLLMs(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	llms, totalCount, totalPages, err := a.service.GetAllLLMs(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLMs(llms)})
}

// @Summary Search LLMs by name
// @Description Search for LLMs using a name stub
// @Tags llms
// @Accept json
// @Produce json
// @Param name query string true "Name stub to search for"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/search [get]
// @Security BearerAuth
func (a *API) searchLLMs(c *gin.Context) {
	nameStub := c.Query("name")
	if nameStub == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name stub is required"}},
		})
		return
	}

	llms, err := a.service.GetLLMsByNameStub(nameStub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLMs(llms)})
}

// @Summary Get LLMs by maximum privacy score
// @Description Get a list of LLMs with privacy score less than or equal to the specified value
// @Tags llms
// @Accept json
// @Produce json
// @Param max_score query int true "Maximum privacy score"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/max-privacy-score [get]
// @Security BearerAuth
func (a *API) getLLMsByMaxPrivacyScore(c *gin.Context) {
	maxScore, err := strconv.Atoi(c.Query("max_score"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid max_score parameter"}},
		})
		return
	}

	llms, err := a.service.GetLLMsByMaxPrivacyScore(maxScore)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLMs(llms)})
}

// @Summary Get LLMs by minimum privacy score
// @Description Get a list of LLMs with privacy score greater than or equal to the specified value
// @Tags llms
// @Accept json
// @Produce json
// @Param min_score query int true "Minimum privacy score"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/min-privacy-score [get]
// @Security BearerAuth
func (a *API) getLLMsByMinPrivacyScore(c *gin.Context) {
	minScore, err := strconv.Atoi(c.Query("min_score"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid min_score parameter"}},
		})
		return
	}

	llms, err := a.service.GetLLMsByMinPrivacyScore(minScore)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLMs(llms)})
}

// @Summary Get LLMs by privacy score range
// @Description Get a list of LLMs with privacy score within the specified range
// @Tags llms
// @Accept json
// @Produce json
// @Param min_score query int true "Minimum privacy score"
// @Param max_score query int true "Maximum privacy score"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llms/privacy-score-range [get]
// @Security BearerAuth
func (a *API) getLLMsByPrivacyScoreRange(c *gin.Context) {
	minScore, err := strconv.Atoi(c.Query("min_score"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid min_score parameter"}},
		})
		return
	}

	maxScore, err := strconv.Atoi(c.Query("max_score"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid max_score parameter"}},
		})
		return
	}

	if minScore > maxScore {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "min_score cannot be greater than max_score"}},
		})
		return
	}

	llms, err := a.service.GetLLMsByPrivacyScoreRange(minScore, maxScore)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeLLMs(llms)})
}

func (a *API) serializeLLM(llm *models.LLM) LLMResponse {
	// Use preloaded plugins to avoid N+1 queries
	plugins := make([]PluginResponse, len(llm.Plugins))
	for i, plugin := range llm.Plugins {
		plugins[i] = PluginResponse{
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
				PluginType:   plugin.GetCapabilityCategory(),
				OCIReference: plugin.OCIReference,
				Manifest:     plugin.Manifest,
				HookType:    plugin.HookType,
				IsActive:    plugin.IsActive,
				Namespace:   plugin.Namespace,
				CreatedAt:   plugin.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt:   plugin.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			},
		}
	}

	return LLMResponse{
		Type: "llms",
		ID:   strconv.FormatUint(uint64(llm.ID), 10),
		Attributes: struct {
			Name             string           `json:"name"`
			APIKey           string           `json:"api_key"`
			HasAPIKey        bool             `json:"has_api_key"`
			APIEndpoint      string           `json:"api_endpoint"`
			PrivacyScore     int              `json:"privacy_score"`
			ShortDescription string           `json:"short_description"`
			LongDescription  string           `json:"long_description"`
			LogoURL          string           `json:"logo_url"`
			Vendor           string           `json:"vendor"`
			Active           bool             `json:"active"`
			Filters          []FilterResponse `json:"filters"`
			DefaultModel     string           `json:"default_model"`
			AllowedModels    []string         `json:"allowed_models"`
			MonthlyBudget    *float64         `json:"monthly_budget"`
			BudgetStartDate  *time.Time       `json:"budget_start_date"`
			Namespace        string           `json:"namespace"`
			Plugins          []PluginResponse `json:"plugins"`
		}{
			Name:             llm.Name,
			APIKey:           services.REDACTED_VALUE,
			HasAPIKey:        llm.APIKey != "",
			APIEndpoint:      llm.APIEndpoint,
			PrivacyScore:     llm.PrivacyScore,
			ShortDescription: llm.ShortDescription,
			LongDescription:  llm.LongDescription,
			LogoURL:          llm.LogoURL,
			Vendor:           string(llm.Vendor),
			Active:           llm.Active,
			Filters:          serializeFilters(llm.Filters),
			DefaultModel:     llm.DefaultModel,
			AllowedModels:    llm.AllowedModels,
			MonthlyBudget:    llm.MonthlyBudget,
			BudgetStartDate:  llm.BudgetStartDate,
			Namespace:        llm.Namespace,
			Plugins:          plugins,
		},
	}
}

func (a *API) serializeLLMs(llms models.LLMs) []LLMResponse {
	result := make([]LLMResponse, len(llms))
	for i, llm := range llms {
		// Serialize plugins inline to avoid N+1 queries from function calls
		plugins := make([]PluginResponse, len(llm.Plugins))
		for j, plugin := range llm.Plugins {
			plugins[j] = PluginResponse{
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
		}

		result[i] = LLMResponse{
			Type: "llms",
			ID:   strconv.FormatUint(uint64(llm.ID), 10),
			Attributes: struct {
				Name             string           `json:"name"`
				APIKey           string           `json:"api_key"`
				HasAPIKey        bool             `json:"has_api_key"`
				APIEndpoint      string           `json:"api_endpoint"`
				PrivacyScore     int              `json:"privacy_score"`
				ShortDescription string           `json:"short_description"`
				LongDescription  string           `json:"long_description"`
				LogoURL          string           `json:"logo_url"`
				Vendor           string           `json:"vendor"`
				Active           bool             `json:"active"`
				Filters          []FilterResponse `json:"filters"`
				DefaultModel     string           `json:"default_model"`
				AllowedModels    []string         `json:"allowed_models"`
				MonthlyBudget    *float64         `json:"monthly_budget"`
				BudgetStartDate  *time.Time       `json:"budget_start_date"`
				Namespace        string           `json:"namespace"`
				Plugins          []PluginResponse `json:"plugins"`
			}{
				Name:             llm.Name,
				APIKey:           services.REDACTED_VALUE,
				HasAPIKey:        llm.APIKey != "",
				APIEndpoint:      llm.APIEndpoint,
				PrivacyScore:     llm.PrivacyScore,
				ShortDescription: llm.ShortDescription,
				LongDescription:  llm.LongDescription,
				LogoURL:          llm.LogoURL,
				Vendor:           string(llm.Vendor),
				Active:           llm.Active,
				Filters:          serializeFilters(llm.Filters),
				DefaultModel:     llm.DefaultModel,
				AllowedModels:    llm.AllowedModels,
				MonthlyBudget:    llm.MonthlyBudget,
				BudgetStartDate:  llm.BudgetStartDate,
				Namespace:        llm.Namespace,
				Plugins:          plugins,
			},
		}
	}
	return result
}

// @Summary Get LLM plugin configuration
// @Description Get the configuration override for a specific plugin-LLM association
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Param pluginId path int true "Plugin ID"
// @Success 200 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/llms/{id}/plugins/{pluginId}/config [get]
// @Security BearerAuth
func (a *API) getLLMPluginConfig(c *gin.Context) {
	llmID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	pluginID, err := strconv.ParseUint(c.Param("pluginId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid Plugin ID"}},
		})
		return
	}

	config, err := a.service.PluginService.GetLLMPluginConfig(uint(llmID), uint(pluginID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin-LLM association not found"}},
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

	response := gin.H{
		"data": gin.H{
			"type": "llm-plugin-config",
			"id":   fmt.Sprintf("%d-%d", llmID, pluginID),
			"attributes": gin.H{
				"llm_id":          llmID,
				"plugin_id":       pluginID,
				"config_override": config,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Update LLM plugin configuration
// @Description Update the configuration override for a specific plugin-LLM association
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Param pluginId path int true "Plugin ID"
// @Param config body object true "Configuration override object"
// @Success 200 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/llms/{id}/plugins/{pluginId}/config [put]
// @Security BearerAuth
func (a *API) updateLLMPluginConfig(c *gin.Context) {
	llmID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct{
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	pluginID, err := strconv.ParseUint(c.Param("pluginId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct{
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid Plugin ID"}},
		})
		return
	}

	var req struct {
		ConfigOverride map[string]interface{} `json:"config_override"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct{
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if err := a.service.PluginService.UpdateLLMPluginConfig(uint(llmID), uint(pluginID), req.ConfigOverride); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct{
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Plugin-LLM association not found"}},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct{
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	response := gin.H{
		"data": gin.H{
			"type": "llm-plugin-config",
			"id":   fmt.Sprintf("%d-%d", llmID, pluginID),
			"attributes": gin.H{
				"llm_id":          llmID,
				"plugin_id":       pluginID,
				"config_override": req.ConfigOverride,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}
