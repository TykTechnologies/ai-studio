package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AppInput defines the expected input structure for creating or updating an app.
type AppInput struct {
	Data struct {
		Type       string `json:"type" binding:"required,eq=app"`
		Attributes struct {
			Name            string     `json:"name"`
			Description     string     `json:"description"`
			UserID          uint       `json:"user_id"`
			DatasourceIDs   []uint     `json:"datasource_ids"`
			LLMIDs          []uint     `json:"llm_ids"`
			ToolIDs         []string   `json:"tool_ids"`       // Added for tools
			MonthlyBudget   *float64   `json:"monthly_budget"`
			BudgetStartDate *time.Time `json:"budget_start_date"`
		} `json:"attributes" binding:"required"`
	} `json:"data" binding:"required"`
}

// AppResponse defines the structure for app-related API responses.
type AppResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name            string     `json:"name"`
		Description     string     `json:"description"`
		UserID          uint       `json:"user_id"`
		CredentialID    uint       `json:"credential_id"`
		DatasourceIDs   []uint     `json:"datasource_ids"`
		LLMIDs          []uint     `json:"llm_ids"`
		ToolIDs         []uint     `json:"tool_ids"`
		MonthlyBudget   *float64   `json:"monthly_budget"`
		BudgetStartDate *time.Time `json:"budget_start_date"`
	} `json:"attributes"`
}

// @Summary Create a new app
// @Description Create a new app with the provided information
// @Tags apps
// @Accept json
// @Produce json
// @Param app body AppInput true "App information"
// @Success 201 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps [post]
// @Security BearerAuth
func (a *API) createApp(c *gin.Context) {
	var input AppInput
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

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs

	app, err := a.service.CreateApp(
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.UserID,
		datasourceIDs,
		llmIDs,
		toolIDs, // Pass toolIDs to service method
		input.Data.Attributes.MonthlyBudget,
		input.Data.Attributes.BudgetStartDate,
	)
	if err != nil {
		if err == services.ERRPrivacyScoreMismatch {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: "Datasources have higher privacy requirements than the selected LLMs. Please select LLMs with equal or higher privacy scores."}},
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeApp(app)})
}

// @Summary Get an app by ID
// @Description Get details of an app by its ID
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/{id} [get]
// @Security BearerAuth
func (a *API) getApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	app, err := a.service.GetAppByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeApp(app)})
}

// @Summary Update an app
// @Description Update an existing app's information
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param app body AppInput true "Updated app information"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id} [patch]
// @Security BearerAuth
func (a *API) updateApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	var input AppInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs
	toolIDs := input.Data.Attributes.ToolIDs // Added toolIDs

	app, err := a.service.UpdateApp(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.UserID,
		datasourceIDs,
		llmIDs,
		toolIDs, // Pass toolIDs to service method
		input.Data.Attributes.MonthlyBudget,
		input.Data.Attributes.BudgetStartDate,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		if err == services.ERRPrivacyScoreMismatch {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Privacy Score Mismatch", Detail: "Datasources have higher privacy requirements than the selected LLMs. Please select LLMs with equal or higher privacy scores."}},
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

	c.JSON(http.StatusOK, gin.H{"data": serializeApp(app)})
}

// @Summary Delete an app
// @Description Delete an app by its ID
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id} [delete]
// @Security BearerAuth
func (a *API) deleteApp(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	err = a.service.DeleteApp(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
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

// @Summary Get apps by user ID
// @Description Get a list of apps for a specific user
// @Tags apps
// @Accept json
// @Produce json
// @Param userId path int true "User ID"
// @Success 200 {array} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{user_id}/apps [get]
// @Security BearerAuth
func (a *API) getAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	apps, err := a.service.GetAppsByUserID(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Count apps by user ID
// @Description Get the total number of apps for a specific user
// @Tags apps
// @Accept json
// @Produce json
// @Param user_id path int true "User ID"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{user_id}/apps/count [get]
// @Security BearerAuth
func (a *API) countAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	count, err := a.service.CountAppsByUserID(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}



func serializeApp(app *models.App) AppResponse {
	return AppResponse{
		Type: "app",
		ID:   strconv.FormatUint(uint64(app.ID), 10),
		Attributes: struct {
			Name            string     `json:"name"`
			Description     string     `json:"description"`
			UserID          uint       `json:"user_id"`
			CredentialID    uint       `json:"credential_id"`
			DatasourceIDs   []uint     `json:"datasource_ids"`
			LLMIDs          []uint     `json:"llm_ids"`
			ToolIDs         []uint     `json:"tool_ids"`
			MonthlyBudget   *float64   `json:"monthly_budget"`
			BudgetStartDate *time.Time `json:"budget_start_date"`
		}{
			Name:            app.Name,
			Description:     app.Description,
			UserID:          app.UserID,
			CredentialID:    app.CredentialID,
			DatasourceIDs:   getDatasourceIDs(app.Datasources),
			LLMIDs:          getLLMIDs(app.LLMs),
			ToolIDs:         getToolIDs(app.Tools),
			MonthlyBudget:   app.MonthlyBudget,
			BudgetStartDate: app.BudgetStartDate,
		},
	}
}

func getDatasourceIDs(datasources []models.Datasource) []uint {
	ids := make([]uint, len(datasources))
	for i, ds := range datasources {
		ids[i] = ds.ID
	}
	return ids
}

func getLLMIDs(llms []models.LLM) []uint {
	ids := make([]uint, len(llms))
	for i, llm := range llms {
		ids[i] = llm.ID
	}
	return ids
}

func getToolIDs(tools []models.Tool) []uint {
	ids := make([]uint, len(tools))
	for i, tool := range tools {
		ids[i] = tool.ID
	}
	return ids
}
// @Summary Add a tool to an app
// @Description Associate a tool with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param app_id path int true "App ID"
// @Param tool_id path int true "Tool ID"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{app_id}/tools/{tool_id} [post]
// @Security BearerAuth
func (a *API) addToolToApp(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Bad Request", "Invalid App ID"}}})
		return
	}
	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Bad Request", "Invalid Tool ID"}}})
		return
	}

	app, err := a.service.AddToolToApp(uint(appID), uint(toolID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Not Found", "App or Tool not found"}}})
			return
		}
		// Consider other specific errors, e.g., if the tool is already added
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Internal Server Error", err.Error()}}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeApp(app)})
}

// @Summary Remove a tool from an app
// @Description Disassociate a tool from an app
// @Tags apps
// @Produce json
// @Param app_id path int true "App ID"
// @Param tool_id path int true "Tool ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{app_id}/tools/{tool_id} [delete]
// @Security BearerAuth
func (a *API) removeToolFromApp(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Bad Request", "Invalid App ID"}}})
		return
	}
	toolID, err := strconv.ParseUint(c.Param("tool_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Bad Request", "Invalid Tool ID"}}})
		return
	}

	err = a.service.RemoveToolFromApp(uint(appID), uint(toolID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Not Found", "App or Tool not found, or tool not associated with app"}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Internal Server Error", err.Error()}}})
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Get tools for an app
// @Description Retrieve all tools associated with a specific app
// @Tags apps
// @Produce json
// @Param app_id path int true "App ID"
// @Success 200 {object} AppResponse // Should be []ToolResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{app_id}/tools [get]
// @Security BearerAuth
func (a *API) getAppTools(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Bad Request", "Invalid App ID"}}})
		return
	}

	tools, err := a.service.GetAppTools(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Not Found", "App not found"}}})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct{ Title, Detail string }{{"Internal Server Error", err.Error()}}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": getToolIDs(tools)})
}


