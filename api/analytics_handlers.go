package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/chat-records-per-day [get]
func (a *API) getChatRecordsPerDay(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetChatRecordsPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/tool-calls-per-day [get]
func (a *API) getToolCallsPerDay(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetToolCallsPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/chat-records-per-user [get]
func (a *API) getChatRecordsPerUser(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetChatRecordsPerUser(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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

	return startDate, endDate, nil
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/cost-analysis [get]
func (a *API) getCostAnalysis(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartDataMap, err := analytics.GetCostAnalysis(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/most-used-llm-models [get]
func (a *API) getMostUsedLLMModels(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetMostUsedLLMModels(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/tool-usage-statistics [get]
func (a *API) getToolUsageStatistics(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetToolUsageStatistics(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/unique-users-per-day [get]
func (a *API) getUniqueUsersPerDay(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetUniqueUsersPerDay(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/token-usage-per-user [get]
func (a *API) getTokenUsagePerUser(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsagePerUser(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/token-usage-per-app [get]
func (a *API) getTokenUsagePerApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsagePerApp(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/token-usage-for-app [get]
func (a *API) getTokenUsageForApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsageForApp(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/chat-interactions-for-chat [get]
func (a *API) getChatInteractionsForChat(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	chatID := c.Query("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing chat_id"}},
		})
		return
	}

	chartData, err := analytics.GetChatInteractionsForChat(a.service.DB, startDate, endDate, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/model-usage [get]
func (a *API) getModelUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	modelName := c.Query("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing model_name"}},
		})
		return
	}

	chartData, err := analytics.GetModelUsage(a.service.DB, startDate, endDate, modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Success 200 {object} analytics.ChartData
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/vendor-usage [get]
func (a *API) getVendorUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	vendor := c.Query("vendor")
	if vendor == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Missing vendor"}},
		})
		return
	}

	chartData, err := analytics.GetVendorUsage(a.service.DB, startDate, endDate, vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get vendor usage"}},
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
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/token-usage-and-cost-for-app [get]
func (a *API) getTokenUsageAndCostForApp(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id"}},
		})
		return
	}

	chartData, err := analytics.GetTokenUsageAndCostForApp(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
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
// @Success 200 {array} analytics.VendorModelCost
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analytics/total-cost-per-vendor-and-model [get]
func (a *API) getTotalCostPerVendorAndModel(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	costs, err := analytics.GetTotalCostPerVendorAndModel(a.service.DB, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get total cost per vendor and model"}},
		})
		return
	}

	c.JSON(http.StatusOK, costs)
}
