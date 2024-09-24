package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create new LLM settings
// @Description Create new LLM settings with the provided information
// @Tags llm-settings
// @Accept json
// @Produce json
// @Param settings body LLMSettingsInput true "LLM settings information"
// @Success 201 {object} LLMSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llm-settings [post]
// @Security BearerAuth
func (a *API) createLLMSettings(c *gin.Context) {
	var input LLMSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	settings := &models.LLMSettings{
		ModelName:         input.Data.Attributes.ModelName,
		MaxLength:         input.Data.Attributes.MaxLength,
		MaxTokens:         input.Data.Attributes.MaxTokens,
		Metadata:          input.Data.Attributes.Metadata,
		MinLength:         input.Data.Attributes.MinLength,
		RepetitionPenalty: input.Data.Attributes.RepetitionPenalty,
		Seed:              input.Data.Attributes.Seed,
		StopWords:         input.Data.Attributes.StopWords,
		Temperature:       input.Data.Attributes.Temperature,
		TopK:              input.Data.Attributes.TopK,
		TopP:              input.Data.Attributes.TopP,
	}

	createdSettings, err := a.service.CreateLLMSettings(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeLLMSettings(createdSettings)})
}

// @Summary Get LLM settings by ID
// @Description Get details of LLM settings by its ID
// @Tags llm-settings
// @Accept json
// @Produce json
// @Param id path int true "LLM Settings ID"
// @Success 200 {object} LLMSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llm-settings/{id} [get]
// @Security BearerAuth
func (a *API) getLLMSettings(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM Settings ID"}},
		})
		return
	}

	settings, err := a.service.GetLLMSettingsByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "LLM Settings not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMSettings(settings)})
}

// @Summary Update LLM settings
// @Description Update existing LLM settings information
// @Tags llm-settings
// @Accept json
// @Produce json
// @Param id path int true "LLM Settings ID"
// @Param settings body LLMSettingsInput true "Updated LLM settings information"
// @Success 200 {object} LLMSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llm-settings/{id} [patch]
// @Security BearerAuth
func (a *API) updateLLMSettings(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM Settings ID"}},
		})
		return
	}

	var input LLMSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	settings := &models.LLMSettings{
		ID:                uint(id),
		ModelName:         input.Data.Attributes.ModelName,
		MaxLength:         input.Data.Attributes.MaxLength,
		MaxTokens:         input.Data.Attributes.MaxTokens,
		Metadata:          input.Data.Attributes.Metadata,
		MinLength:         input.Data.Attributes.MinLength,
		RepetitionPenalty: input.Data.Attributes.RepetitionPenalty,
		Seed:              input.Data.Attributes.Seed,
		StopWords:         input.Data.Attributes.StopWords,
		Temperature:       input.Data.Attributes.Temperature,
		TopK:              input.Data.Attributes.TopK,
		TopP:              input.Data.Attributes.TopP,
	}

	updatedSettings, err := a.service.UpdateLLMSettings(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMSettings(updatedSettings)})
}

// @Summary Delete LLM settings
// @Description Delete LLM settings by its ID
// @Tags llm-settings
// @Accept json
// @Produce json
// @Param id path int true "LLM Settings ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llm-settings/{id} [delete]
// @Security BearerAuth
func (a *API) deleteLLMSettings(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM Settings ID"}},
		})
		return
	}

	err = a.service.DeleteLLMSettings(uint(id))
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

// @Summary List all LLM settings
// @Description Get a list of all LLM settings
// @Tags llm-settings
// @Accept json
// @Produce json
// @Success 200 {array} LLMSettingsResponse
// @Failure 500 {object} ErrorResponse
// @Router /llm-settings [get]
// @Security BearerAuth
func (a *API) listLLMSettings(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	settings, totalCount, totalPages, err := a.service.GetAllLLMSettings(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeLLMSettingsSlice(settings)})
}

// @Summary Search LLM settings by model name
// @Description Search for LLM settings using a model name stub
// @Tags llm-settings
// @Accept json
// @Produce json
// @Param model_name query string true "Model name stub to search for"
// @Success 200 {array} LLMSettingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /llm-settings/search [get]
// @Security BearerAuth
func (a *API) searchLLMSettings(c *gin.Context) {
	modelNameStub := c.Query("model_name")
	if modelNameStub == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Model name stub is required"}},
		})
		return
	}

	settings, err := a.service.SearchLLMSettingsByModelStub(modelNameStub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMSettingsSlice(settings)})
}

func serializeLLMSettings(settings *models.LLMSettings) LLMSettingsResponse {
	return LLMSettingsResponse{
		Type: "llm-settings",
		ID:   strconv.FormatUint(uint64(settings.ID), 10),
		Attributes: struct {
			ModelName         string                 `json:"model_name"`
			MaxLength         int                    `json:"max_length"`
			MaxTokens         int                    `json:"max_tokens"`
			Metadata          map[string]interface{} `json:"metadata"`
			MinLength         int                    `json:"min_length"`
			RepetitionPenalty float64                `json:"repetition_penalty"`
			Seed              int                    `json:"seed"`
			StopWords         []string               `json:"stop_words"`
			Temperature       float64                `json:"temperature"`
			TopK              int                    `json:"top_k"`
			TopP              float64                `json:"top_p"`
		}{
			ModelName:         settings.ModelName,
			MaxLength:         settings.MaxLength,
			MaxTokens:         settings.MaxTokens,
			Metadata:          settings.Metadata,
			MinLength:         settings.MinLength,
			RepetitionPenalty: settings.RepetitionPenalty,
			Seed:              settings.Seed,
			StopWords:         settings.StopWords,
			Temperature:       settings.Temperature,
			TopK:              settings.TopK,
			TopP:              settings.TopP,
		},
	}
}

func serializeLLMSettingsSlice(settings *models.LLMSettingsSlice) []LLMSettingsResponse {
	result := make([]LLMSettingsResponse, len(*settings))
	for i, s := range *settings {
		result[i] = serializeLLMSettings(&s)
	}
	return result
}
