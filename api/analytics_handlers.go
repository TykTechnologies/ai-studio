package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// getChatRecordsPerDay godoc
// @Summary Get chat records per day
// @Description Get the total number of chat records per day for a given time period
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/chat-records-per-day [get]
func (a *API) getChatRecordsPerDay(c *gin.Context) {
	var startDate, endDate *time.Time

	// Parse start_date if provided
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid start_date format"}},
			})
			return
		}
		startDate = &parsedDate
	}

	// Parse end_date if provided
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid end_date format"}},
			})
			return
		}
		// Set end date to end of day (23:59:59)
		endOfDay := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 23, 59, 59, 0, parsedDate.Location())
		endDate = &endOfDay
	}

	chartData, err := analytics.GetChatRecordsPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get chat records per day"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getToolCallsPerDay godoc
// @Summary Get tool calls per day
// @Description Get the total number of tool calls per day for a given time period
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/tool-calls-per-day [get]
func (a *API) getToolCallsPerDay(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetToolCallsPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get tool calls per day"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getChatRecordsPerUser godoc
// @Summary Get chat records per user
// @Description Get the total number of chat records per user for a given time period
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/chat-records-per-user [get]
func (a *API) getChatRecordsPerUser(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetChatRecordsPerUser(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get chat records per user"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// Helper function to parse date range from query parameters
func getDateRange(c *gin.Context) (time.Time, time.Time, error) {
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	// Set end date to end of day (23:59:59)
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, endDate.Location())

	return startDate, endDate, nil
}

// Helper function to get interaction type from query parameters
func getInteractionType(c *gin.Context) *models.InteractionType {
	typeStr := c.Query("interaction_type")
	if typeStr == "" {
		return nil
	}

	interactionType := models.InteractionType(typeStr)
	if interactionType != models.ChatInteraction && interactionType != models.ProxyInteraction {
		return nil
	}
	return &interactionType
}

// getCostAnalysis godoc
// @Summary Get cost analysis
// @Description Get the total cost per day for a given time period
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/cost-analysis [get]
func (a *API) getCostAnalysis(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartDataMap, err := analytics.GetCostAnalysis(a.service.DB, startDate, endDate, getInteractionType(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get cost analysis"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartDataMap)
}

// getMostUsedLLMModels godoc
// @Summary Get most used LLM models
// @Description Get the usage count for each LLM model
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/most-used-llm-models [get]
func (a *API) getMostUsedLLMModels(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetMostUsedLLMModels(a.service.DB, startDate, endDate, getInteractionType(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get most used LLM models"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getToolUsageStatistics godoc
// @Summary Get tool usage statistics
// @Description Get the usage count for each tool
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/tool-usage-statistics [get]
func (a *API) getToolUsageStatistics(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetToolUsageStatistics(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get tool usage statistics"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getUniqueUsersPerDay godoc
// @Summary Get unique users per day
// @Description Get the number of unique users per day
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/unique-users-per-day [get]
func (a *API) getUniqueUsersPerDay(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetUniqueUsersPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get unique users per day"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getTokenUsagePerUser godoc
// @Summary Get token usage per user
// @Description Get the total token usage for each user
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/token-usage-per-user [get]
func (a *API) getTokenUsagePerUser(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsagePerUser(a.service.DB, startDate, endDate, getInteractionType(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get token usage per user"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getTokenUsagePerApp godoc
// @Summary Get token usage per app
// @Description Get the total token usage for each app
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/token-usage-per-app [get]
func (a *API) getTokenUsagePerApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsagePerApp(a.service.DB, startDate, endDate, getInteractionType(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get token usage per app"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getTokenUsageForApp godoc
// @Summary Get token usage for a specific app
// @Description Get the token usage for a specific app over time
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param app_id query int true "App ID"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/token-usage-for-app [get]
func (a *API) getTokenUsageForApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsageForApp(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get token usage for app"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getChatInteractionsForChat godoc
// @Summary Get chat interactions for a specific chat
// @Description Get the number of interactions for a specific chat over time
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param chat_id query string true "Chat ID"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/chat-interactions-for-chat [get]
func (a *API) getChatInteractionsForChat(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chatID := c.Query("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing chat_id"}},
		})
		return
	}

	chartData, err := analytics.GetChatInteractionsForChat(a.service.DB, startDate, endDate, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get chat interactions for chat"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getModelUsage godoc
// @Summary Get usage statistics for a specific model
// @Description Get the usage statistics for a specific model over time
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param model_name query string true "Model Name"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/model-usage [get]
func (a *API) getModelUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	modelName := c.Query("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing model_name"}},
		})
		return
	}

	chartData, err := analytics.GetModelUsage(a.service.DB, startDate, endDate, modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get model usage"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getVendorUsage godoc
// @Summary Get usage statistics for a specific vendor
// @Description Get the usage statistics for a specific vendor over time
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param vendor query string true "Vendor Name"
// @Param llm_id query int false "LLM ID to filter by"
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/vendor-usage [get]
func (a *API) getVendorUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	vendor := c.Query("vendor")
	if vendor == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing vendor"}},
		})
		return
	}

	var llmID *uint
	if llmIDStr := c.Query("llm_id"); llmIDStr != "" {
		id, err := strconv.ParseUint(llmIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid llm_id"}},
			})
			return
		}
		u := uint(id)
		llmID = &u
	}

	chartData, err := analytics.GetVendorUsage(a.service.DB, startDate, endDate, vendor, llmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get vendor usage"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getUsage godoc
// @Summary Get token usage and cost data with flexible filtering
// @Description Get token usage and cost data filtered by vendor, LLM ID, app ID, and interaction type
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param vendor query string false "Vendor name to filter by"
// @Param llm_id query int false "LLM ID to filter by"
// @Param app_id query int false "App ID to filter by"
// @Param interaction_type query string false "Interaction type to filter by (chat/proxy)"
// @Success 200 {object} models.MultiAxisChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/usage [get]
func (a *API) getUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var llmID *uint
	if llmIDStr := c.Query("llm_id"); llmIDStr != "" {
		id, err := strconv.ParseUint(llmIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid llm_id"}},
			})
			return
		}
		u := uint(id)
		llmID = &u
	}

	var appID *uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		id, err := strconv.ParseUint(appIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid app_id"}},
			})
			return
		}
		u := uint(id)
		appID = &u
	}

	vendor := c.Query("vendor")
	interactionType := getInteractionType(c)

	chartData, err := analytics.GetUsage(a.service.DB, startDate, endDate, vendor, llmID, appID, interactionType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get usage data"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getTokenUsageAndCostForApp godoc
// @Summary Get token usage and cost for a specific app
// @Description Get the token usage and total cost for a specific app over time
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param app_id query int true "App ID"
// @Success 200 {object} analytics.MultiAxisChartData
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/token-usage-and-cost-for-app [get]
func (a *API) getTokenUsageAndCostForApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsageAndCostForApp(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get token usage and cost for app"}},
		})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getTotalCostPerVendorAndModel godoc
// @Summary Get total cost per vendor and model
// @Description Get the total cost per vendor and model for a given time period, including currency
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
//
//	@Success 200 {array} struct {
//		Vendor    string  `json:"vendor"`
//		Model     string  `json:"model"`
//		Currency  string  `json:"currency"`
//		TotalCost float64 `json:"totalCost"`
//	}
//
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/total-cost-per-vendor-and-model [get]
func (a *API) getTotalCostPerVendorAndModel(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var llmID *uint
	if llmIDStr := c.Query("llm_id"); llmIDStr != "" {
		id, err := strconv.ParseUint(llmIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid llm_id"}},
			})
			return
		}
		u := uint(id)
		llmID = &u
	}

	costs, err := analytics.GetTotalCostPerVendorAndModel(a.service.DB, startDate, endDate, getInteractionType(c), llmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get total cost per vendor and model"}},
		})
		return
	}

	c.JSON(http.StatusOK, costs)
}

// getProxyLogsForLLM godoc
// @Summary Get proxy logs for a specific LLM
// @Description Get paginated proxy logs for a specific LLM by joining with chat records
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param llm_id query int true "LLM ID"
// @Param page query int false "Page number (default: 1)"
// @Param page_size query int false "Page size (default: 10)"
// @Success 200 {object} models.PaginatedProxyLogs
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/proxy-logs-for-llm [get]
func (a *API) getProxyLogsForLLM(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	llmID, err := strconv.ParseUint(c.Query("llm_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid llm_id"}},
		})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	logs, totalCount, err := analytics.GetProxyLogsForLLM(a.service.DB, startDate, endDate, uint(llmID), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get proxy logs for LLM"}},
		})
		return
	}

	totalPages := (totalCount + int64(pageSize) - 1) / int64(pageSize)

	response := models.PaginatedProxyLogs{
		Data: make([]models.ProxyLogResponse, len(logs)),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: int(totalPages),
			PageSize:   pageSize,
			PageNumber: page,
		},
	}

	for i, log := range logs {
		response.Data[i] = models.ProxyLogResponse{
			Type: "proxy_log",
			ID:   strconv.FormatUint(uint64(log.ID), 10),
			Attributes: struct {
				AppID        uint      `json:"app_id"`
				UserID       uint      `json:"user_id"`
				TimeStamp    time.Time `json:"time_stamp"`
				Vendor       string    `json:"vendor"`
				RequestBody  string    `json:"request_body"`
				ResponseBody string    `json:"response_body"`
				ResponseCode int       `json:"response_code"`
			}{
				AppID:        log.AppID,
				UserID:       log.UserID,
				TimeStamp:    log.TimeStamp,
				Vendor:       log.Vendor,
				RequestBody:  log.RequestBody,
				ResponseBody: log.ResponseBody,
				ResponseCode: log.ResponseCode,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getProxyLogsForApp godoc
// @Summary Get proxy logs for a specific app
// @Description Get paginated proxy logs for a specific app
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param app_id query int true "App ID"
// @Param page query int false "Page number (default: 1)"
// @Param page_size query int false "Page size (default: 10)"
// @Success 200 {object} models.PaginatedProxyLogs
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/proxy-logs-for-app [get]
func (a *API) getProxyLogsForApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	logs, totalCount, err := analytics.GetProxyLogsForAppID(a.service.DB, startDate, endDate, uint(appID), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get proxy logs for app"}},
		})
		return
	}

	totalPages := (totalCount + int64(pageSize) - 1) / int64(pageSize)

	response := models.PaginatedProxyLogs{
		Data: make([]models.ProxyLogResponse, len(logs)),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: int(totalPages),
			PageSize:   pageSize,
			PageNumber: page,
		},
	}

	for i, log := range logs {
		response.Data[i] = models.ProxyLogResponse{
			Type: "proxy_log",
			ID:   strconv.FormatUint(uint64(log.ID), 10),
			Attributes: struct {
				AppID        uint      `json:"app_id"`
				UserID       uint      `json:"user_id"`
				TimeStamp    time.Time `json:"time_stamp"`
				Vendor       string    `json:"vendor"`
				RequestBody  string    `json:"request_body"`
				ResponseBody string    `json:"response_body"`
				ResponseCode int       `json:"response_code"`
			}{
				AppID:        log.AppID,
				UserID:       log.UserID,
				TimeStamp:    log.TimeStamp,
				Vendor:       log.Vendor,
				RequestBody:  log.RequestBody,
				ResponseBody: log.ResponseBody,
				ResponseCode: log.ResponseCode,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// getBudgetUsage godoc
// @Summary Get current monthly budget usage for apps and LLMs
// @Description Returns usage of monthly budgets for apps and LLMs, with optional date range for total cost
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD) for total cost calculation"
// @Param end_date query string false "End date (YYYY-MM-DD) for total cost calculation"
// @Success 200 {array} struct{Name string `json:"name"`;Type string `json:"type"`;MonthlyBudget *float64 `json:"monthlyBudget"`;CurrentUsage float64 `json:"currentUsage"`;UsagePercent float64 `json:"usagePercent"`;TotalCost float64 `json:"totalCost"`}
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/budget-usage [get]
func (a *API) getBudgetUsage(c *gin.Context) {
	var startDate, endDate time.Time
	var err error

	// Parse start_date if provided
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid start_date format"}},
			})
			return
		}
	}

	// Parse end_date if provided
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid end_date format"}},
			})
			return
		}
	}

	// Parse llm_id if provided
	var llmID *uint
	if llmIDStr := c.Query("llm_id"); llmIDStr != "" {
		id, err := strconv.ParseUint(llmIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Invalid llm_id"}},
			})
			return
		}
		u := uint(id)
		llmID = &u
	}

	usageList, err := analytics.GetBudgetUsage(a.service.DB, &startDate, &endDate, llmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to retrieve budget usage"}},
		})
		return
	}

	response := make([]map[string]interface{}, 0, len(usageList))
	for _, u := range usageList {
		entry := map[string]interface{}{
			"name":            u.Name,
			"entity_type":     u.EntityType,
			"budget":          u.Budget,
			"spent":           u.Spent,
			"usage":           u.Usage,
			"budgetStartDate": u.BudgetStartDate,
			"totalCost":       u.TotalCost,
			"entity_id":       u.EntityID,
			"total_tokens":    u.TotalTokens,
		}
		response = append(response, entry)
	}

	c.JSON(http.StatusOK, response)
}

// getBudgetUsageForApp godoc
// @Summary Get budget usage for a specific app
// @Description Get the current budget usage for a specific app
// @Tags Analytics
// @Accept json
// @Produce json
// @Param app_id query int true "App ID"
//
//	@Success 200 {object} struct {
//		CurrentUsage  float64    `json:"current_usage"`
//		MonthlyBudget *float64   `json:"monthly_budget"`
//		Percentage    *float64   `json:"percentage"`
//		StartDate     time.Time  `json:"start_date"`
//	}
//
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/budget-usage-for-app [get]
func (a *API) getBudgetUsageForApp(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get app"}},
		})
		return
	}

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	if app.BudgetStartDate != nil {
		start = *app.BudgetStartDate
	}

	spent, err := a.service.Budget.GetMonthlySpending(uint(appID), start, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get monthly spending"}},
		})
		return
	}

	var percentage *float64
	if app.MonthlyBudget != nil && *app.MonthlyBudget > 0 {
		p := (spent / *app.MonthlyBudget) * 100
		percentage = &p
	}

	response := struct {
		CurrentUsage  float64   `json:"current_usage"`
		MonthlyBudget *float64  `json:"monthly_budget"`
		Percentage    *float64  `json:"percentage"`
		StartDate     time.Time `json:"start_date"`
	}{
		CurrentUsage:  spent,
		MonthlyBudget: app.MonthlyBudget,
		Percentage:    percentage,
		StartDate:     start,
	}

	c.JSON(http.StatusOK, response)
}
