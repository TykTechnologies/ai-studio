package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
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

	llm, err := a.service.CreateLLM(
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

	if llm.Active {
		if a.proxy != nil {
			a.proxy.Reload()
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeLLM(llm)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLM(llm)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLM(llm)})
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

	err = a.service.DeleteLLM(uint(id))
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
	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
}

func serializeLLM(llm *models.LLM) LLMResponse {
	return LLMResponse{
		Type: "llms",
		ID:   strconv.FormatUint(uint64(llm.ID), 10),
		Attributes: struct {
			Name             string           `json:"name"`
			APIKey           string           `json:"api_key"`
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
		}{
			Name:             llm.Name,
			APIKey:           llm.APIKey,
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
		},
	}
}

func serializeLLMs(llms models.LLMs) []LLMResponse {
	result := make([]LLMResponse, len(llms))
	for i, llm := range llms {
		result[i] = serializeLLM(&llm)
	}
	return result
}
