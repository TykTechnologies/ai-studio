package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// Guardrail exceptions of an app (features/GuardrailExceptions.md). Anyone
// who may write the app proposes and edits; a full administrator approves
// (the route's permission), and a full administrator's own exception or
// change is approved as it is made. These routes are the only way to manage
// exceptions: no plugin SDK service, gRPC management call or agent scope
// reaches them.

// GuardrailExceptionInput creates or changes an exception. On PATCH omitted
// fields are left as they are; filter_id and source_event are set on
// create only.
type GuardrailExceptionInput struct {
	FilterID         *uint   `json:"filter_id"`
	Detector         *string `json:"detector"`
	Channel          *string `json:"channel"`
	ToolName         *string `json:"tool_name"`
	Action           *string `json:"action"`
	Justification    *string `json:"justification"`
	ExpiresAfterDays *int    `json:"expires_after_days"`
	SourceEvent      *uint   `json:"source_event"`
}

// GuardrailExceptionResponse is one exception.
type GuardrailExceptionResponse struct {
	ID       uint   `json:"id"`
	AppID    uint   `json:"app_id"`
	FilterID uint   `json:"filter_id"`
	Detector string `json:"detector"`
	Channel  string `json:"channel"`
	ToolName string `json:"tool_name"`
	Action   string `json:"action"`
	Status   string `json:"status"`

	Justification    string     `json:"justification"`
	ProposedBy       uint       `json:"proposed_by"`
	ApprovedBy       *uint      `json:"approved_by"`
	ApprovedAt       *time.Time `json:"approved_at"`
	ExpiresAfterDays int        `json:"expires_after_days"`
	LastHitAt        *time.Time `json:"last_hit_at"`
	HitCount         int64      `json:"hit_count"`
	SourceEvent      *uint      `json:"source_event"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// ContentHash identifies the reviewed content; approving takes it back.
	ContentHash string `json:"content_hash"`
}

// GuardrailExceptionApproveInput binds an approval to what was reviewed.
type GuardrailExceptionApproveInput struct {
	// ContentHash is the content_hash of the exception the administrator
	// reviewed, as the API returned it. Required.
	ContentHash string `json:"content_hash"`
}

// GuardrailExceptionMeta describes the rules the API applies, so the UI
// offers only what it would accept.
type GuardrailExceptionMeta struct {
	MaxApproved int `json:"max_approved"`
	// Actions lists, per channel, the actions an exception may soften to.
	Actions map[string][]string `json:"actions"`
	// Floors lists, per capability and channel, the softest action an
	// exception may reach ("" = none).
	Floors map[string]map[string]string `json:"floors"`
}

func guardrailExceptionMeta() GuardrailExceptionMeta {
	m := GuardrailExceptionMeta{
		MaxApproved: models.MaxApprovedGuardrailExceptionsPerApp,
		Actions:     map[string][]string{},
		Floors:      map[string]map[string]string{},
	}
	for _, ch := range guardrails.Channels {
		m.Actions[string(ch)] = guardrails.ExceptionActions(ch)
	}
	for _, c := range []guardrails.Capability{guardrails.CapabilityReadOnly, guardrails.CapabilityReadTools, guardrails.CapabilityActing} {
		row := map[string]string{}
		for _, ch := range guardrails.Channels {
			row[string(ch)] = guardrails.Floor(c, ch)
		}
		m.Floors[string(c)] = row
	}
	return m
}

func serializeGuardrailException(e *models.GuardrailException) GuardrailExceptionResponse {
	return GuardrailExceptionResponse{
		ID: e.ID, AppID: e.AppID, FilterID: e.FilterID, Detector: e.Detector, Channel: e.Channel,
		ToolName: e.ToolName, Action: e.Action, Status: e.Status, Justification: e.Justification,
		ProposedBy: e.ProposedBy, ApprovedBy: e.ApprovedBy, ApprovedAt: e.ApprovedAt,
		ExpiresAfterDays: e.ExpiresAfterDays, LastHitAt: e.LastHitAt, HitCount: e.HitCount,
		SourceEvent: e.SourceEvent, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		ContentHash: e.ContentHash(),
	}
}

func serializeGuardrailExceptions(list []models.GuardrailException) []GuardrailExceptionResponse {
	out := make([]GuardrailExceptionResponse, len(list))
	for i := range list {
		out[i] = serializeGuardrailException(&list[i])
	}
	return out
}

func (in GuardrailExceptionInput) spec() services.GuardrailExceptionSpec {
	return services.GuardrailExceptionSpec{
		FilterID: in.FilterID, Detector: in.Detector, Channel: in.Channel, ToolName: in.ToolName,
		Action: in.Action, Justification: in.Justification, ExpiresAfterDays: in.ExpiresAfterDays,
		SourceEvent: in.SourceEvent,
	}
}

// guardrailExceptionActor is the caller; a full administrator's changes are
// approved as they are made.
func guardrailExceptionActor(c *gin.Context) services.GuardrailExceptionActor {
	return services.GuardrailExceptionActor{UserID: currentUserID(c), FullAdmin: authz.Can(c, authz.FullAdmin)}
}

// guardrailExceptionIDs reads the app and exception ids of the route.
func guardrailExceptionIDs(c *gin.Context, withException bool) (appID, id uint, ok bool) {
	a, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid app ID"))
		return 0, 0, false
	}
	if !withException {
		return uint(a), 0, true
	}
	e, err := strconv.ParseUint(c.Param("eid"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid exception ID"))
		return 0, 0, false
	}
	return uint(a), uint(e), true
}

// guardrailExceptionChangedTitle is the error title of an approval refused
// because the exception changed since it was reviewed.
const guardrailExceptionChangedTitle = "Guardrail Exception Changed"

func guardrailExceptionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, errorResponse("Not Found", "App, filter or guardrail exception not found"))
	case errors.Is(err, services.ErrGuardrailExceptionChanged):
		c.JSON(http.StatusConflict, errorResponse(guardrailExceptionChangedTitle, err.Error()))
	case errors.Is(err, services.ErrInvalidGuardrailException):
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
	case errors.Is(err, services.ErrGuardrailExceptionConflict):
		c.JSON(http.StatusConflict, errorResponse("Conflict", err.Error()))
	default:
		c.JSON(http.StatusInternalServerError, errorResponse("Internal Server Error", "Could not process the guardrail exception"))
	}
}

// @Summary List an app's guardrail exceptions
// @Description Lists the guardrail exceptions of an app, optionally by status (proposed, approved, retired). The response's meta holds the rules the API applies (GuardrailExceptionMeta).
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param status query string false "Status filter"
// @Success 200 {array} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions [get]
// @Security BearerAuth
func (a *API) listGuardrailExceptions(c *gin.Context) {
	appID, _, ok := guardrailExceptionIDs(c, false)
	if !ok {
		return
	}
	list, err := a.service.ListGuardrailExceptions(appID, c.Query("status"))
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailExceptions(list), "meta": guardrailExceptionMeta()})
}

// @Summary Get an app's guardrail exception
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param eid path int true "Exception ID"
// @Success 200 {object} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions/{eid} [get]
// @Security BearerAuth
func (a *API) getGuardrailException(c *gin.Context) {
	appID, id, ok := guardrailExceptionIDs(c, true)
	if !ok {
		return
	}
	e, err := a.service.GetGuardrailException(appID, id)
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailException(e)})
}

// @Summary Create a guardrail exception
// @Description Softens a guardrail filter's action on a verdict for this app, one filter, detector ("" = any) and channel (text_user, text_system, tool_result, tool_call, tool_definition; tool_name narrows a tool channel). The action must be within the floor for the app's guardrail capability. A full administrator's exception is approved at once; anyone else's is proposed and the administrators are notified.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param exception body GuardrailExceptionInput true "Exception"
// @Success 201 {object} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions [post]
// @Security BearerAuth
func (a *API) createGuardrailException(c *gin.Context) {
	appID, _, ok := guardrailExceptionIDs(c, false)
	if !ok {
		return
	}
	var in GuardrailExceptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	e, err := a.service.CreateGuardrailException(appID, guardrailExceptionActor(c), in.spec())
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeGuardrailException(e)})
}

// @Summary Change a guardrail exception
// @Description Changes an exception. A change to an approved exception returns it to proposed, unless a full administrator makes it. A retired exception cannot change.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param eid path int true "Exception ID"
// @Param exception body GuardrailExceptionInput true "Changes"
// @Success 200 {object} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions/{eid} [patch]
// @Security BearerAuth
func (a *API) updateGuardrailException(c *gin.Context) {
	appID, id, ok := guardrailExceptionIDs(c, true)
	if !ok {
		return
	}
	var in GuardrailExceptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	e, err := a.service.UpdateGuardrailException(appID, id, guardrailExceptionActor(c), in.spec())
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailException(e)})
}

// @Summary Delete a guardrail exception
// @Tags apps
// @Param id path int true "App ID"
// @Param eid path int true "Exception ID"
// @Success 204
// @Router /apps/{id}/guardrail-exceptions/{eid} [delete]
// @Security BearerAuth
func (a *API) deleteGuardrailException(c *gin.Context) {
	appID, id, ok := guardrailExceptionIDs(c, true)
	if !ok {
		return
	}
	if err := a.service.DeleteGuardrailException(appID, id); err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Approve a guardrail exception
// @Description Approves a proposed exception (full administrators only). content_hash must be the content_hash of the exception as reviewed; an exception that changed since is refused with 409.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param eid path int true "Exception ID"
// @Param approval body GuardrailExceptionApproveInput true "Reviewed content hash"
// @Success 200 {object} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions/{eid}/approve [post]
// @Security BearerAuth
func (a *API) approveGuardrailException(c *gin.Context) {
	appID, id, ok := guardrailExceptionIDs(c, true)
	if !ok {
		return
	}
	var in GuardrailExceptionApproveInput
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
			return
		}
	}
	e, err := a.service.ApproveGuardrailException(appID, id, currentUserID(c), in.ContentHash)
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailException(e)})
}

// @Summary Retire a guardrail exception
// @Description Retires an exception: it is kept for the record and never applies again.
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param eid path int true "Exception ID"
// @Success 200 {object} GuardrailExceptionResponse
// @Router /apps/{id}/guardrail-exceptions/{eid}/retire [post]
// @Security BearerAuth
func (a *API) retireGuardrailException(c *gin.Context) {
	appID, id, ok := guardrailExceptionIDs(c, true)
	if !ok {
		return
	}
	e, err := a.service.RetireGuardrailException(appID, id)
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailException(e)})
}

// @Summary List a filter's guardrail exceptions
// @Description Lists the exceptions that soften this filter's verdicts, across apps, optionally by status.
// @Tags filters
// @Produce json
// @Param id path int true "Filter ID"
// @Param status query string false "Status filter"
// @Success 200 {array} GuardrailExceptionResponse
// @Router /filters/{id}/exceptions [get]
// @Security BearerAuth
func (a *API) listFilterGuardrailExceptions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid filter ID"))
		return
	}
	list, err := a.service.ListFilterGuardrailExceptions(uint(id), c.Query("status"))
	if err != nil {
		guardrailExceptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeGuardrailExceptions(list)})
}
