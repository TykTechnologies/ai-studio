package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/gin-gonic/gin"
)

// guardrailBlocksLimit is the most entries one portal request lists.
const guardrailBlocksLimit = 100

// GuardrailBlock is one request of an App that a guardrail acted on, as the
// App's owner sees it: no text, no scores, no event metadata.
type GuardrailBlock struct {
	Timestamp  time.Time `json:"timestamp"`
	Filter     string    `json:"filter"`
	Action     string    `json:"action"`
	ReasonCode string    `json:"reason_code,omitempty"`
	Reason     string    `json:"reason"`
	RequestID  string    `json:"request_id,omitempty"`
}

// getUserAppGuardrailBlocks godoc
// @Summary List the guardrail blocks of one of the user's Apps
// @Description The requests of an App that guardrail filters shared with App owners (evidence.share_with_app_owners) blocked, quarantined or changed in the last days (default 7, at most 90): when, the filter, the action, the reason code and sentence the caller got, and the request id. Never the text. Newest first, at most 100.
// @Tags Portal
// @Produce json
// @Param id path int true "App ID"
// @Param days query int false "How many days back (1-90, default 7)"
// @Success 200 {array} GuardrailBlock
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/apps/{id}/guardrail-blocks [get]
func (a *API) getUserAppGuardrailBlocks(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, errorResponse("Unauthorized", "User not found in context"))
		return
	}
	currentUser := user.(*models.User)
	appID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid app ID"))
		return
	}
	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, errorResponse("Not Found", "App not found"))
		return
	}
	if app.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, errorResponse("Forbidden", "You don't have permission to see this app's guardrail blocks"))
		return
	}
	days := 7
	if v := c.Query("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "days must be between 1 and 90"))
			return
		}
		days = n
	}

	var events []models.ComplianceEvent
	if err := a.service.DB.
		Where("app_id = ? AND owner_visible = ? AND time_stamp >= ?", app.ID, true, time.Now().AddDate(0, 0, -days)).
		Order("time_stamp desc").Limit(guardrailBlocksLimit).
		Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("Internal Server Error", "Failed to list guardrail blocks"))
		return
	}
	out := make([]GuardrailBlock, 0, len(events))
	for _, e := range events {
		var m map[string]any
		_ = json.Unmarshal([]byte(e.Metadata), &m)
		if m == nil {
			m = map[string]any{}
		}
		action, code, reason := scripting.OwnerSummary(e.EventType, m)
		rid, _ := m["request_id"].(string)
		out = append(out, GuardrailBlock{Timestamp: e.TimeStamp, Filter: e.FilterName, Action: action, ReasonCode: code, Reason: reason, RequestID: rid})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func errorResponse(title, detail string) ErrorResponse {
	return ErrorResponse{Errors: []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}{{Title: title, Detail: detail}}}
}
