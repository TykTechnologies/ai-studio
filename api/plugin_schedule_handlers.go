package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// CreatePluginSchedule creates a new schedule for a plugin
// POST /api/v1/plugins/:id/schedules
func (a *API) CreatePluginSchedule(c *gin.Context) {
	pluginIDStr := c.Param("id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	// Parse request body
	var req struct {
		ScheduleID     string                 `json:"schedule_id" binding:"required"`
		Name           string                 `json:"name" binding:"required"`
		Cron           string                 `json:"cron" binding:"required"`
		Timezone       string                 `json:"timezone"`
		Enabled        bool                   `json:"enabled"`
		TimeoutSeconds int                    `json:"timeout_seconds"`
		Config         map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create schedule via service layer
	schedule, err := a.service.CreateSchedule(
		uint(pluginID),
		req.ScheduleID,
		req.Name,
		req.Cron,
		req.Timezone,
		req.TimeoutSeconds,
		req.Config,
		req.Enabled,
	)

	if err != nil {
		if err == services.ErrScheduleAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"error": "Schedule with this ID already exists"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, schedule)
}

// GetPluginSchedules returns all schedules for a plugin
// GET /api/v1/plugins/:id/schedules
func (a *API) GetPluginSchedules(c *gin.Context) {
	pluginIDStr := c.Param("id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	// Get schedules via service layer
	schedules, err := a.service.ListSchedules(uint(pluginID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"plugin_id":   pluginID,
		"schedules":   schedules,
		"total_count": len(schedules),
	})
}

// GetPluginScheduleDetail returns details for a specific schedule
// GET /api/v1/plugins/:id/schedules/:schedule_id
func (a *API) GetPluginScheduleDetail(c *gin.Context) {
	pluginIDStr := c.Param("id")
	scheduleIDStr := c.Param("schedule_id")

	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	scheduleID, err := strconv.ParseUint(scheduleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	// Get schedule via service layer
	schedule, err := a.service.GetSchedule(uint(pluginID), uint(scheduleID))
	if err != nil {
		if err == services.ErrScheduleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, schedule)
}

// GetPluginScheduleExecutions returns execution history for a schedule
// GET /api/v1/plugins/:id/schedules/:schedule_id/executions
func (a *API) GetPluginScheduleExecutions(c *gin.Context) {
	pluginIDStr := c.Param("id")
	scheduleIDStr := c.Param("schedule_id")

	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	scheduleID, err := strconv.ParseUint(scheduleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	// Parse query params
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 200 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Get executions via service layer
	executions, totalCount, successCount, successRate, err := a.service.GetScheduleExecutions(
		uint(pluginID),
		uint(scheduleID),
		limit,
		offset,
	)

	if err != nil {
		if err == services.ErrScheduleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Get schedule name
	schedule, _ := a.service.GetSchedule(uint(pluginID), uint(scheduleID))
	scheduleName := ""
	if schedule != nil {
		scheduleName = schedule.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"schedule_id":   scheduleID,
		"schedule_name": scheduleName,
		"executions":    executions,
		"total_count":   totalCount,
		"success_count": successCount,
		"success_rate":  successRate,
		"limit":         limit,
		"offset":        offset,
	})
}

// UpdatePluginSchedule updates a schedule
// PUT /api/v1/plugins/:id/schedules/:schedule_id
func (a *API) UpdatePluginSchedule(c *gin.Context) {
	pluginIDStr := c.Param("id")
	scheduleIDStr := c.Param("schedule_id")

	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	scheduleID, err := strconv.ParseUint(scheduleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	// Parse request body
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Handle config field specially (convert to JSON string)
	if config, ok := req["config"]; ok {
		if configMap, ok := config.(map[string]interface{}); ok {
			if configJSON, err := json.Marshal(configMap); err == nil {
				req["config"] = string(configJSON)
			}
		}
	}

	// Update schedule via service layer
	schedule, err := a.service.UpdateSchedule(uint(pluginID), uint(scheduleID), req)
	if err != nil {
		if err == services.ErrScheduleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, schedule)
}

// DeletePluginSchedule deletes a schedule
// DELETE /api/v1/plugins/:id/schedules/:schedule_id
func (a *API) DeletePluginSchedule(c *gin.Context) {
	pluginIDStr := c.Param("id")
	scheduleIDStr := c.Param("schedule_id")

	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}

	scheduleID, err := strconv.ParseUint(scheduleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule ID"})
		return
	}

	// Delete schedule via service layer
	if err := a.service.DeleteSchedule(uint(pluginID), uint(scheduleID)); err != nil {
		if err == services.ErrScheduleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Schedule not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Schedule deleted successfully",
		"schedule_id": scheduleID,
	})
}
