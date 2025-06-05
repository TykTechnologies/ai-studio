package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// getUserAppTokenUsageAndCost handles GET /analytics/token-usage-and-cost-for-app
// Returns token usage and cost data for an app owned by the authenticated user
func (a *API) getUserAppTokenUsageAndCost(c *gin.Context) {
	userID, err := a.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: err.Error()})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "app_id parameter is required"})
		return
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid app_id parameter"})
		return
	}

	// Verify app ownership
	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, models.APIError{Error: "App not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Check if the user owns the app
	if app.UserID != uint(userID) && !a.isAdmin(c) {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: "You don't have permission to access this app"})
		return
	}

	// Get query parameters and parse dates
	startDate, endDate, err := a.getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Call the analytics package directly
	chartData, err := analytics.GetTokenUsageAndCostForApp(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, chartData)
}

// getUserAppBudgetUsage handles GET /analytics/budget-usage-for-app
// Returns budget usage data for an app owned by the authenticated user
func (a *API) getUserAppBudgetUsage(c *gin.Context) {
	userID, err := a.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: err.Error()})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "app_id parameter is required"})
		return
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid app_id parameter"})
		return
	}

	// Verify app ownership
	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, models.APIError{Error: "App not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Check if the user owns the app
	if app.UserID != uint(userID) && !a.isAdmin(c) {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: "You don't have permission to access this app"})
		return
	}

	if app.MonthlyBudget == nil || *app.MonthlyBudget == 0 {
		c.JSON(http.StatusOK, analytics.BudgetData{
			Labels: []string{"No Budget Set"},
			Data:   []float64{0},
		})
		return
	}

	// Get the start of the current month
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Get the spending for the current month
	spent, err := a.service.Budget.GetMonthlySpending(uint(appID), start, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Calculate the remaining budget
	remaining := *app.MonthlyBudget - spent
	if remaining < 0 {
		remaining = 0
	}

	// Return the budget data
	c.JSON(http.StatusOK, analytics.BudgetData{
		Labels: []string{"Spent", "Remaining"},
		Data:   []float64{spent, remaining},
	})
}

// getUserAppInteractionsOverTime handles GET /analytics/app-interactions-over-time
// Returns app interactions over time data for an app owned by the authenticated user
func (a *API) getUserAppInteractionsOverTime(c *gin.Context) {
	userID, err := a.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: err.Error()})
		return
	}

	appIDStr := c.Query("app_id")
	if appIDStr == "" {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "app_id parameter is required"})
		return
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid app_id parameter"})
		return
	}

	// Verify app ownership
	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, models.APIError{Error: "App not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Check if the user owns the app
	if app.UserID != uint(userID) && !a.isAdmin(c) {
		c.JSON(http.StatusUnauthorized, models.APIError{Error: "You don't have permission to access this app"})
		return
	}

	// Get query parameters and parse dates
	startDate, endDate, err := a.getDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Call the analytics package directly
	chartData, err := analytics.GetAppInteractionsOverTime(a.service.DB, startDate, endDate, uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, chartData)
}
