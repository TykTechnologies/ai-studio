package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// An App's owner reports a guardrail block of their App as a false positive
// from the portal (features/GuardrailExceptions.md). The report is a
// proposed guardrail exception scoped from the event (filter, detector,
// channel, tool), softening to the softest action the App's capability
// allows there. Owners can never approve it: the exception is proposed
// whoever reports it, and approving needs a full administrator on the admin
// API.

// GuardrailBlockReportInput is the owner's reason.
type GuardrailBlockReportInput struct {
	Justification string `json:"justification"`
}

// notReportableEventTypes are guardrail events that record no detector's
// verdict, so there is nothing an exception could soften.
var notReportableEventTypes = map[string]bool{
	"guardrail.error": true, "guardrail.unscanned": true, "guardrail.unreadable_request": true,
	"guardrail.unredactable": true, "guardrail.template_mimicry": true, "guardrail.judge": true,
	"guardrail.exception_ignored": true,
}

// responseScopes are the filter scopes of response filters, which exceptions
// do not soften.
var responseScopes = map[string]bool{"proxy_response": true, "chat_response": true}

// reportableBlock reads, from an App's compliance event and its metadata,
// the filter and channel a false-positive report would scope an exception
// to; ok is false when the event cannot be reported.
func reportableBlock(e *models.ComplianceEvent, m map[string]any) (filterID uint, ch guardrails.Channel, ok bool) {
	if e.AppID == 0 || !strings.HasPrefix(e.EventType, "guardrail.") || notReportableEventTypes[e.EventType] || responseScopes[e.FilterScope] {
		return 0, "", false
	}
	if det, _ := m["detector"].(string); det == "" {
		return 0, "", false
	}
	id, isNum := m["filter_id"].(float64)
	if !isNum || id <= 0 || id != float64(uint32(id)) {
		return 0, "", false
	}
	ch, ok = eventChannel(m)
	return uint(id), ch, ok
}

// eventChannel is the channel of the event's first flagged segment (else its
// kinds and roles), as the runtime matches exceptions (scripting channelOf).
func eventChannel(m map[string]any) (guardrails.Channel, bool) {
	kind, role := "", ""
	if segs, _ := m["segments"].([]any); len(segs) > 0 {
		if seg, _ := segs[0].(map[string]any); seg != nil {
			kind, _ = seg["kind"].(string)
			role, _ = seg["role"].(string)
		}
	}
	if kind == "" && role == "" {
		if ks, _ := m["kinds"].([]any); len(ks) == 1 {
			kind, _ = ks[0].(string)
		}
		if rs, _ := m["roles"].([]any); len(rs) == 1 {
			role, _ = rs[0].(string)
		}
	}
	switch {
	case kind == guardrails.KindToolResult || role == guardrails.RoleTool:
		return guardrails.ChannelToolResult, true
	case kind == guardrails.KindToolCall:
		return guardrails.ChannelToolCall, true
	case kind == guardrails.KindToolDefinition:
		return guardrails.ChannelToolDefinition, true
	case role == guardrails.RoleSystem:
		return guardrails.ChannelTextSystem, true
	case role == "user" || role == "assistant":
		return guardrails.ChannelTextUser, true
	}
	return "", false
}

// eventToolName is the tool of a tool channel event when it names one.
func eventToolName(m map[string]any) string {
	if segs, _ := m["segments"].([]any); len(segs) > 0 {
		if seg, _ := segs[0].(map[string]any); seg != nil {
			if t, _ := seg["tool"].(string); t != "" {
				return t
			}
		}
	}
	if ts, _ := m["tool_names"].([]any); len(ts) == 1 {
		t, _ := ts[0].(string)
		return t
	}
	return ""
}

// reportGuardrailBlock godoc
// @Summary Report a guardrail block of one of the user's Apps as a false positive
// @Description Proposes a guardrail exception for the App from one of its blocks (GET .../guardrail-blocks, reportable): the event's filter, detector, channel and tool, softened to the softest action the App's capability allows there. Proposed for an administrator to approve; App owners cannot approve it.
// @Tags portal
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param event_id path int true "Compliance event ID"
// @Param report body GuardrailBlockReportInput true "Why it is a false positive"
// @Success 201 {object} GuardrailExceptionResponse
// @Router /common/apps/{id}/guardrail-blocks/{event_id}/report [post]
// @Security BearerAuth
func (a *API) reportGuardrailBlock(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, errorResponse("Unauthorized", "User not found in context"))
		return
	}
	currentUser := user.(*models.User)
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid app ID"))
		return
	}
	eventID, err := strconv.ParseUint(c.Param("event_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid event ID"))
		return
	}
	var in GuardrailBlockReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, errorResponse("Not Found", "App not found"))
		return
	}
	if app.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, errorResponse("Forbidden", "You don't have permission to report this app's guardrail blocks"))
		return
	}
	// Only the blocks the owner can see: shared with App owners.
	var ev models.ComplianceEvent
	if err := a.service.DB.Where("id = ? AND app_id = ? AND owner_visible = ?", eventID, app.ID, true).First(&ev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("Not Found", "Guardrail block not found"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("Internal Server Error", "Could not read the guardrail block"))
		return
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(ev.Metadata), &m)
	filterID, ch, ok := reportableBlock(&ev, m)
	if !ok {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "This block cannot be reported as a false positive"))
		return
	}
	floor := guardrails.Floor(guardrails.Capability(app.GuardrailCapability), ch)
	if floor == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "This app's guardrail capability allows no softening here; ask an administrator"))
		return
	}
	action := ""
	for _, a := range guardrails.ExceptionActions(ch) {
		if guardrails.ActionRank(a) >= guardrails.ActionRank(floor) {
			action = a
			break
		}
	}
	detector, _ := m["detector"].(string)
	channel := string(ch)
	toolName := ""
	if ch.IsTool() {
		toolName = eventToolName(m)
	}
	justification := strings.TrimSpace(in.Justification)
	if justification == "" {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "justification is required"))
		return
	}
	justification = "Reported by the app owner: " + justification
	source := uint(ev.ID)
	// Proposed whoever reports it: the portal never approves.
	e, err := a.service.CreateGuardrailException(app.ID, services.GuardrailExceptionActor{UserID: currentUser.ID, FullAdmin: false}, services.GuardrailExceptionSpec{
		FilterID: &filterID, Detector: &detector, Channel: &channel, ToolName: &toolName,
		Action: &action, Justification: &justification, SourceEvent: &source,
	})
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeGuardrailException(e)})
}
