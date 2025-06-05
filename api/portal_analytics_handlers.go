package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// getUserAppTokenUsageAndCost handles GET /common/analytics/token-usage-and-cost-for-app
// Returns token usage and cost data for an app owned by the authenticated user
func (a *API) getUserAppTokenUsageAndCost(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "app_id parameter is required"}},
		})
		return
	}

	appID, err := strconv.ParseUint(appIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id parameter"}},
		})
		return
	}

	// Verify app ownership
	app, err := a.service.GetApp(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	if app.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "You don't have access to this app"}},
		})
		return
	}

	// Get query parameters
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// Call the existing analytics service method
	data, err := a.service.GetTokenUsageAndCostForApp(startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to retrieve token usage and cost data"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getUserAppBudgetUsage handles GET /common/analytics/budget-usage-for-app
// Returns budget usage data for an app owned by the authenticated user
func (a *API) getUserAppBudgetUsage(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "app_id parameter is required"}},
		})
		return
	}

	appID, err := strconv.ParseUint(appIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id parameter"}},
		})
		return
	}

	// Verify app ownership
	app, err := a.service.GetApp(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	if app.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "You don't have access to this app"}},
		})
		return
	}

	// Call the existing analytics service method
	data, err := a.service.GetBudgetUsageForApp(uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to retrieve budget usage data"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getUserAppInteractionsOverTime handles GET /common/analytics/app-interactions-over-time
// Returns app interactions over time data for an app owned by the authenticated user
func (a *API) getUserAppInteractionsOverTime(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "app_id parameter is required"}},
		})
		return
	}

	appID, err := strconv.ParseUint(appIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app_id parameter"}},
		})
		return
	}

	// Verify app ownership
	app, err := a.service.GetApp(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "App not found"}},
		})
		return
	}

	if app.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "You don't have access to this app"}},
		})
		return
	}

	// Get query parameters
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// Call the existing analytics service method
	data, err := a.service.GetAppInteractionsOverTime(uint(appID), startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to retrieve app interactions data"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}
