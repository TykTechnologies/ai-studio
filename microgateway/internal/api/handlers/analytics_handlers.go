// internal/api/handlers/analytics_handlers.go
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// GetAnalyticsEvents returns analytics events for an app
func GetAnalyticsEvents(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get app ID from query or auth context
		var appID uint
		if appIDParam := c.Query("app_id"); appIDParam != "" {
			id, err := strconv.ParseUint(appIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid app ID",
					"message": "app_id must be a valid number",
				})
				return
			}
			appID = uint(id)
		} else {
			// Use authenticated app's ID
			appID = auth.GetAppID(c)
			if appID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing app ID",
					"message": "app_id query parameter is required",
				})
				return
			}
		}

		// Parse pagination parameters
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

		// Validate pagination
		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 1000 {
			limit = 50
		}

		// Get events
		events, total, err := serviceContainer.AnalyticsService.GetEvents(appID, page, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get analytics events",
				"message": err.Error(),
			})
			return
		}

		// Calculate pagination info
		totalPages := (total + int64(limit) - 1) / int64(limit)

		c.JSON(http.StatusOK, gin.H{
			"data": events,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			},
		})
	}
}

// GetAnalyticsSummary returns analytics summary for an app
func GetAnalyticsSummary(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get app ID from query or auth context
		var appID uint
		if appIDParam := c.Query("app_id"); appIDParam != "" {
			id, err := strconv.ParseUint(appIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid app ID",
					"message": "app_id must be a valid number",
				})
				return
			}
			appID = uint(id)
		} else {
			appID = auth.GetAppID(c)
			if appID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing app ID",
					"message": "app_id query parameter is required",
				})
				return
			}
		}

		// Parse time range
		startTimeStr := c.DefaultQuery("start_time", time.Now().AddDate(0, 0, -7).Format(time.RFC3339))
		endTimeStr := c.DefaultQuery("end_time", time.Now().Format(time.RFC3339))

		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid start_time",
				"message": "start_time must be in RFC3339 format",
			})
			return
		}

		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid end_time",
				"message": "end_time must be in RFC3339 format",
			})
			return
		}

		// Get analytics summary
		summary, err := serviceContainer.AnalyticsService.GetSummary(appID, startTime, endTime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get analytics summary",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": summary,
			"time_range": gin.H{
				"start_time": startTime,
				"end_time":   endTime,
			},
		})
	}
}

// GetCostAnalysis returns cost analysis for an app
func GetCostAnalysis(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get app ID from query or auth context
		var appID uint
		if appIDParam := c.Query("app_id"); appIDParam != "" {
			id, err := strconv.ParseUint(appIDParam, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid app ID",
					"message": "app_id must be a valid number",
				})
				return
			}
			appID = uint(id)
		} else {
			appID = auth.GetAppID(c)
			if appID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing app ID",
					"message": "app_id query parameter is required",
				})
				return
			}
		}

		// Parse time range
		startTimeStr := c.DefaultQuery("start_time", time.Now().AddDate(0, -1, 0).Format(time.RFC3339))
		endTimeStr := c.DefaultQuery("end_time", time.Now().Format(time.RFC3339))

		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid start_time",
				"message": "start_time must be in RFC3339 format",
			})
			return
		}

		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid end_time",
				"message": "end_time must be in RFC3339 format",
			})
			return
		}

		// Get cost analysis
		analysis, err := serviceContainer.AnalyticsService.GetCostAnalysis(appID, startTime, endTime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to get cost analysis",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": analysis,
			"time_range": gin.H{
				"start_time": startTime,
				"end_time":   endTime,
			},
		})
	}
}

// FlushAnalytics manually triggers analytics buffer flush (admin endpoint)
func FlushAnalytics(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := serviceContainer.AnalyticsService.Flush()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to flush analytics",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Analytics buffer flushed successfully",
		})
	}
}

// GetAnalyticsEventRequest returns the request payload for a specific analytics event
func GetAnalyticsEventRequest(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid event ID",
				"message": "Event ID must be a valid number",
			})
			return
		}

		// Get analytics event with request body
		var event database.AnalyticsEvent
		err = serviceContainer.DB.First(&event, eventID).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Event not found",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"event_id":     event.ID,
				"request_body": event.RequestBody,
				"endpoint":     event.Endpoint,
				"method":       event.Method,
				"timestamp":    event.CreatedAt,
			},
		})
	}
}

// GetAnalyticsEventResponse returns the response payload for a specific analytics event
func GetAnalyticsEventResponse(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid event ID",
				"message": "Event ID must be a valid number",
			})
			return
		}

		// Get analytics event with response body
		var event database.AnalyticsEvent
		err = serviceContainer.DB.First(&event, eventID).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Event not found",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"event_id":      event.ID,
				"response_body": event.ResponseBody,
				"endpoint":      event.Endpoint,
				"status_code":   event.StatusCode,
				"timestamp":     event.CreatedAt,
			},
		})
	}
}

// GetAnalyticsStats returns analytics service statistics (admin endpoint)
func GetAnalyticsStats(serviceContainer *services.ServiceContainer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if analyticsService, ok := serviceContainer.AnalyticsService.(*services.DatabaseAnalyticsService); ok {
			stats := analyticsService.GetStats()

			c.JSON(http.StatusOK, gin.H{
				"data": stats,
			})
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "Analytics stats not available",
			})
		}
	}
}