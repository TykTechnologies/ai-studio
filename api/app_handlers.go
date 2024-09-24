package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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

	// Extract datasourceIDs and llmIDs from the input
	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs

	app, err := a.service.CreateApp(
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.UserID,
		datasourceIDs,
		llmIDs,
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

	// Extract datasourceIDs and llmIDs from the input
	datasourceIDs := input.Data.Attributes.DatasourceIDs
	llmIDs := input.Data.Attributes.LLMIDs

	app, err := a.service.UpdateApp(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		datasourceIDs,
		llmIDs,
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
// @Router /users/{userId}/apps [get]
// @Security BearerAuth
func (a *API) getAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
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

// @Summary Get app by name
// @Description Get details of an app by its name
// @Tags apps
// @Accept json
// @Produce json
// @Param name query string true "App name"
// @Success 200 {object} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/by-name [get]
// @Security BearerAuth
func (a *API) getAppByName(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "App name is required"}},
		})
		return
	}

	app, err := a.service.GetAppByName(name)
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

// @Summary Activate app credential
// @Description Activate the credential associated with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/activate-credential [post]
// @Security BearerAuth
func (a *API) activateAppCredential(c *gin.Context) {
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

	err = a.service.ActivateAppCredential(uint(id))
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

// @Summary Deactivate app credential
// @Description Deactivate the credential associated with an app
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/{id}/deactivate-credential [post]
// @Security BearerAuth
func (a *API) deactivateAppCredential(c *gin.Context) {
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

	err = a.service.DeactivateAppCredential(uint(id))
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

func serializeApp(app *models.App) AppResponse {
	return AppResponse{
		Type: "apps",
		ID:   strconv.FormatUint(uint64(app.ID), 10),
		Attributes: struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			UserID        uint   `json:"user_id"`
			CredentialID  uint   `json:"credential_id"`
			DatasourceIDs []uint `json:"datasource_ids"`
			LLMIDs        []uint `json:"llm_ids"`
		}{
			Name:          app.Name,
			Description:   app.Description,
			UserID:        app.UserID,
			CredentialID:  app.CredentialID,
			DatasourceIDs: getDatasourceIDs(app.Datasources),
			LLMIDs:        getLLMIDs(app.LLMs),
		},
	}
}

// Add these new functions to your existing app_handlers.go file

// @Summary List all apps
// @Description Get a list of all apps
// @Tags apps
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {array} AppResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps [get]
// @Security BearerAuth
func (a *API) listApps(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	apps, totalCount, totalPages, err := a.service.ListAppsWithPagination(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Search apps
// @Description Search for apps based on a search term
// @Tags apps
// @Accept json
// @Produce json
// @Param search_term query string true "Search term"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {array} AppResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /apps/search [get]
// @Security BearerAuth
func (a *API) searchApps(c *gin.Context) {
	searchTerm := c.Query("search_term")
	if searchTerm == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Search term is required"}},
		})
		return
	}

	pageSize, pageNumber, all := getPaginationParams(c)

	apps, totalCount, totalPages, err := a.service.SearchApps(searchTerm, pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeApps(apps)})
}

// @Summary Count all apps
// @Description Get the total number of apps
// @Tags apps
// @Accept json
// @Produce json
// @Success 200 {object} map[string]int64
// @Failure 500 {object} ErrorResponse
// @Router /apps/count [get]
// @Security BearerAuth
func (a *API) countApps(c *gin.Context) {
	count, err := a.service.CountApps()
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

// @Summary Count apps by user ID
// @Description Get the total number of apps for a specific user
// @Tags apps
// @Accept json
// @Produce json
// @Param userId path int true "User ID"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{userId}/apps/count [get]
// @Security BearerAuth
func (a *API) countAppsByUserID(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
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

func serializeApps(apps []models.App) []AppResponse {
	result := make([]AppResponse, len(apps))
	for i, app := range apps {
		result[i] = serializeApp(&app)
	}
	return result
}
