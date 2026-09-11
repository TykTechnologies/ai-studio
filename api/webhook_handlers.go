package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/webhooks"
	"github.com/gin-gonic/gin"
)

// webhooksService returns the service attached to services.Service, or the
// per-API community stub set up in NewAPI (tests that never call
// InitWebhooks): every call on the stub is refused with ErrEnterpriseFeature,
// exactly like a CE build.
func (a *API) webhooksService() webhooks.Service {
	if a.service != nil && a.service.Webhooks != nil {
		return a.service.Webhooks
	}
	return a.webhooksFallback
}

// bindOptionalJSON decodes a JSON body into dst when one is present. An empty
// body is fine (the caller's defaults apply); anything else must parse. It
// does not trust Content-Length, so a body sent with a wrong length is still
// validated rather than skipped.
func bindOptionalJSON(c *gin.Context, dst interface{}) error {
	if c.Request.Body == nil {
		return nil
	}
	err := c.ShouldBindJSON(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// exportWriter sets the download headers on the first byte written, so a
// failure before any output can still produce a JSON error response.
type exportWriter struct {
	c        *gin.Context
	format   string
	filename string
	started  bool
}

func (w *exportWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.started = true
		w.c.Header("Content-Type", webhooks.ExportContentType(w.format))
		w.c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", w.filename))
		w.c.Status(http.StatusOK)
	}
	return w.c.Writer.Write(p)
}

// webhookActor reads the authenticated administrator from the gin context.
func webhookActor(c *gin.Context) (webhooks.Actor, bool) {
	user, exists := c.Get("user")
	if !exists {
		return webhooks.Actor{}, false
	}
	u, ok := user.(*models.User)
	if !ok || u == nil {
		return webhooks.Actor{}, false
	}
	return webhooks.Actor{UserID: u.ID, Email: u.Email, Name: u.Name}, true
}

func webhookError(c *gin.Context, status int, title, detail string) {
	c.JSON(status, models.ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: title, Detail: detail}},
	})
}

func webhookBadRequest(c *gin.Context, detail string) {
	webhookError(c, http.StatusBadRequest, "Bad Request", detail)
}

// webhookErrorResponse maps service errors onto the API error envelope.
func webhookErrorResponse(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, webhooks.ErrEnterpriseFeature):
		webhookError(c, http.StatusForbidden, "Enterprise Feature", err.Error())
	case errors.Is(err, webhooks.ErrDisabled):
		webhookError(c, http.StatusConflict, "Webhooks Disabled", err.Error())
	case errors.Is(err, webhooks.ErrNotFound):
		webhookError(c, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, webhooks.ErrConflict):
		webhookError(c, http.StatusConflict, "Conflict", err.Error())
	case errors.Is(err, webhooks.ErrInvalidState), errors.Is(err, webhooks.ErrNotApproved):
		webhookError(c, http.StatusConflict, "Invalid State", err.Error())
	case errors.Is(err, webhooks.ErrSameApprover):
		webhookError(c, http.StatusForbidden, "Forbidden", err.Error())
	case errors.Is(err, webhooks.ErrURLPolicy), errors.Is(err, webhooks.ErrTemplate):
		webhookError(c, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
	case errors.Is(err, webhooks.ErrValidation):
		webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
	default:
		webhookError(c, http.StatusInternalServerError, "Internal Server Error", fallback)
	}
}

// requireWebhookActor writes a 401 and returns false when no user is on the context.
func requireWebhookActor(c *gin.Context) (webhooks.Actor, bool) {
	actor, ok := webhookActor(c)
	if !ok {
		webhookError(c, http.StatusUnauthorized, "Unauthorized", "User not found in context")
	}
	return actor, ok
}

// parseWebhookTime accepts YYYY-MM-DD or RFC3339. Date-only end values are
// pushed to the end of that day.
func parseWebhookTime(value string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339, got %q", value)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, nil
}

// parseDeliveryQuery reads the shared delivery-log filter parameters.
func parseDeliveryQuery(c *gin.Context) (webhooks.DeliveryQuery, error) {
	var q webhooks.DeliveryQuery

	if v := c.Query("start_date"); v != "" {
		t, err := parseWebhookTime(v, false)
		if err != nil {
			return q, fmt.Errorf("invalid start_date: %w", err)
		}
		q.Start = t
	}
	if v := c.Query("end_date"); v != "" {
		t, err := parseWebhookTime(v, true)
		if err != nil {
			return q, fmt.Errorf("invalid end_date: %w", err)
		}
		q.End = t
	}
	if !q.Start.IsZero() && !q.End.IsZero() && q.End.Before(q.Start) {
		return q, errors.New("end_date must not be before start_date")
	}

	q.TargetID = strings.TrimSpace(c.Query("target_id"))
	q.Topic = strings.TrimSpace(c.Query("topic"))
	q.EventID = strings.TrimSpace(c.Query("event_id"))
	q.Kind = strings.TrimSpace(c.Query("kind"))
	q.Search = strings.TrimSpace(c.Query("search"))
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		for _, s := range strings.Split(v, ",") {
			s = strings.ToLower(strings.TrimSpace(s))
			if s == "" {
				continue
			}
			switch s {
			case models.WebhookDeliveryQueued, models.WebhookDeliveryInFlight, models.WebhookDeliverySucceeded,
				models.WebhookDeliveryRetrying, models.WebhookDeliveryDeadLettered, models.WebhookDeliveryCancelled:
				q.Statuses = append(q.Statuses, s)
			default:
				return q, fmt.Errorf("invalid status %q", s)
			}
		}
	}

	for _, field := range []struct {
		name string
		val  *string
		max  int
	}{
		{"target_id", &q.TargetID, 36}, {"topic", &q.Topic, 200}, {"event_id", &q.EventID, 128},
		{"kind", &q.Kind, 16}, {"search", &q.Search, 255},
	} {
		if len(*field.val) > field.max {
			return q, fmt.Errorf("%s is too long", field.name)
		}
	}

	if v := c.Query("page"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
			return q, errors.New("invalid page")
		}
		q.Page = n
	}
	if v := c.Query("page_size"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 || n > webhooks.MaxPageSize {
			return q, fmt.Errorf("invalid page_size, expected 1 to %d", webhooks.MaxPageSize)
		}
		q.PageSize = n
	}
	if v := strings.ToLower(c.Query("sort")); v == "asc" {
		q.SortAsc = true
	}
	return q, nil
}

// --- Status, topics, templates ---

// getWebhooksStatus godoc
// @Summary Webhooks availability and health
// @Description Reports whether webhooks are available (Enterprise), enabled, connected to the event bus, and queue depth
// @Tags Webhooks
// @Produce json
// @Success 200 {object} webhooks.Status
// @Router /webhooks/status [get]
func (a *API) getWebhooksStatus(c *gin.Context) {
	c.JSON(http.StatusOK, a.webhooksService().Status())
}

// listWebhookTopics godoc
// @Summary Topics a target can subscribe to
// @Description Known system topics plus topics observed on the event bus recently
// @Tags Webhooks
// @Produce json
// @Success 200 {object} webhooks.TopicList
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/topics [get]
func (a *API) listWebhookTopics(c *gin.Context) {
	topics, err := a.webhooksService().ListTopics(c.Request.Context())
	if err != nil {
		webhookErrorResponse(c, err, "Failed to list topics")
		return
	}
	c.JSON(http.StatusOK, topics)
}

// listWebhookPresets godoc
// @Summary Built-in payload templates
// @Tags Webhooks
// @Produce json
// @Success 200 {array} webhooks.Preset
// @Router /webhooks/templates/presets [get]
func (a *API) listWebhookPresets(c *gin.Context) {
	presets := a.webhooksService().ListPresets()
	if presets == nil {
		presets = []webhooks.Preset{}
	}
	c.JSON(http.StatusOK, presets)
}

// previewWebhookTemplate godoc
// @Summary Render a payload template against a sample event
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param body body webhooks.PreviewInput true "Template to render"
// @Success 200 {object} webhooks.PreviewResult
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/templates/preview [post]
func (a *API) previewWebhookTemplate(c *gin.Context) {
	var in webhooks.PreviewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	res, err := a.webhooksService().PreviewTemplate(c.Request.Context(), in)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to render template")
		return
	}
	c.JSON(http.StatusOK, res)
}

// --- Targets ---

// listWebhookTargets godoc
// @Summary List webhook targets
// @Tags Webhooks
// @Produce json
// @Param status query string false "pending, approved, rejected or revoked"
// @Param search query string false "Substring over name, URL and description"
// @Success 200 {object} webhooks.TargetList
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/targets [get]
func (a *API) listWebhookTargets(c *gin.Context) {
	f := webhooks.TargetFilter{
		Status: strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Search: strings.TrimSpace(c.Query("search")),
	}
	if len(f.Search) > 255 {
		webhookBadRequest(c, "search is too long")
		return
	}
	list, err := a.webhooksService().ListTargets(c.Request.Context(), f)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to list webhook targets")
		return
	}
	c.JSON(http.StatusOK, list)
}

// createWebhookTargetResponse is returned once, with the signing secret.
type createWebhookTargetResponse struct {
	Target        models.WebhookTargetResponse `json:"target"`
	SigningSecret string                       `json:"signing_secret"`
}

// createWebhookTarget godoc
// @Summary Create a webhook target (pending approval)
// @Description The target is created pending and receives nothing until an administrator approves it. The signing secret is returned once.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param body body webhooks.TargetInput true "Target"
// @Success 201 {object} createWebhookTargetResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 422 {object} models.ErrorResponse
// @Router /webhooks/targets [post]
func (a *API) createWebhookTarget(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var in webhooks.TargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, secret, err := a.webhooksService().CreateTarget(c.Request.Context(), actor, in)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to create webhook target")
		return
	}
	c.JSON(http.StatusCreated, createWebhookTargetResponse{Target: *view, SigningSecret: secret})
}

// getWebhookTarget godoc
// @Summary Get a webhook target with delivery statistics
// @Tags Webhooks
// @Produce json
// @Param id path string true "Target ID"
// @Success 200 {object} webhooks.TargetDetail
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/targets/{id} [get]
func (a *API) getWebhookTarget(c *gin.Context) {
	detail, err := a.webhooksService().GetTarget(c.Request.Context(), c.Param("id"))
	if err != nil {
		webhookErrorResponse(c, err, "Failed to get webhook target")
		return
	}
	c.JSON(http.StatusOK, detail)
}

// updateWebhookTargetInput carries the optimistic lock version with the patch.
type updateWebhookTargetInput struct {
	LockVersion int `json:"lock_version"`
	webhooks.TargetPatch
}

// updateWebhookTargetResponse reports whether the change re-pended the target.
type updateWebhookTargetResponse struct {
	Target   models.WebhookTargetResponse `json:"target"`
	Repended bool                         `json:"repended"`
}

// updateWebhookTarget godoc
// @Summary Update a webhook target
// @Description Changing the URL or custom headers of an approved target sends it back to pending. Send lock_version from the last read.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Target ID"
// @Param body body updateWebhookTargetInput true "Patch"
// @Success 200 {object} updateWebhookTargetResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Failure 422 {object} models.ErrorResponse
// @Router /webhooks/targets/{id} [patch]
func (a *API) updateWebhookTarget(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var in updateWebhookTargetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	view, repended, err := a.webhooksService().UpdateTarget(c.Request.Context(), actor, c.Param("id"), in.LockVersion, in.TargetPatch)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to update webhook target")
		return
	}
	c.JSON(http.StatusOK, updateWebhookTargetResponse{Target: *view, Repended: repended})
}

// deleteWebhookTarget godoc
// @Summary Delete a webhook target
// @Description Cancels queued deliveries; the delivery log is kept.
// @Tags Webhooks
// @Param id path string true "Target ID"
// @Success 204
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/targets/{id} [delete]
func (a *API) deleteWebhookTarget(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	if err := a.webhooksService().DeleteTarget(c.Request.Context(), actor, c.Param("id")); err != nil {
		webhookErrorResponse(c, err, "Failed to delete webhook target")
		return
	}
	c.Status(http.StatusNoContent)
}

type webhookReviewInput struct {
	Note   string `json:"note"`
	Reason string `json:"reason"`
}

// targetAction runs one state transition and writes the resulting target.
func (a *API) targetAction(c *gin.Context, fallback string, fn func(actor webhooks.Actor, id string, in webhookReviewInput) (*models.WebhookTargetResponse, error)) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var in webhookReviewInput
	if err := bindOptionalJSON(c, &in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	if len(in.Note) > 1024 || len(in.Reason) > 1024 {
		webhookBadRequest(c, "note or reason is too long")
		return
	}
	view, err := fn(actor, c.Param("id"), in)
	if err != nil {
		webhookErrorResponse(c, err, fallback)
		return
	}
	c.JSON(http.StatusOK, view)
}

// approveWebhookTarget godoc
// @Summary Approve a webhook target
// @Description Allows deliveries to the target's URL. Recorded in the audit trail.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Target ID"
// @Param body body webhookReviewInput false "Optional note"
// @Success 200 {object} models.WebhookTargetResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/approve [post]
func (a *API) approveWebhookTarget(c *gin.Context) {
	a.targetAction(c, "Failed to approve webhook target", func(actor webhooks.Actor, id string, in webhookReviewInput) (*models.WebhookTargetResponse, error) {
		return a.webhooksService().ApproveTarget(c.Request.Context(), actor, id, in.Note)
	})
}

// rejectWebhookTarget godoc
// @Summary Reject a pending webhook target
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Target ID"
// @Param body body webhookReviewInput false "Reason"
// @Success 200 {object} models.WebhookTargetResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/reject [post]
func (a *API) rejectWebhookTarget(c *gin.Context) {
	a.targetAction(c, "Failed to reject webhook target", func(actor webhooks.Actor, id string, in webhookReviewInput) (*models.WebhookTargetResponse, error) {
		return a.webhooksService().RejectTarget(c.Request.Context(), actor, id, in.Reason)
	})
}

// revokeWebhookTarget godoc
// @Summary Revoke an approved webhook target
// @Description Stops all deliveries and cancels queued ones.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Target ID"
// @Param body body webhookReviewInput false "Reason"
// @Success 200 {object} models.WebhookTargetResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/revoke [post]
func (a *API) revokeWebhookTarget(c *gin.Context) {
	a.targetAction(c, "Failed to revoke webhook target", func(actor webhooks.Actor, id string, in webhookReviewInput) (*models.WebhookTargetResponse, error) {
		return a.webhooksService().RevokeTarget(c.Request.Context(), actor, id, in.Reason)
	})
}

// pauseWebhookTarget godoc
// @Summary Pause deliveries to a target
// @Description Deliveries keep queueing but are not sent until resumed.
// @Tags Webhooks
// @Produce json
// @Param id path string true "Target ID"
// @Success 200 {object} models.WebhookTargetResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/pause [post]
func (a *API) pauseWebhookTarget(c *gin.Context) {
	a.targetAction(c, "Failed to pause webhook target", func(actor webhooks.Actor, id string, _ webhookReviewInput) (*models.WebhookTargetResponse, error) {
		return a.webhooksService().PauseTarget(c.Request.Context(), actor, id)
	})
}

// resumeWebhookTarget godoc
// @Summary Resume deliveries to a paused target
// @Tags Webhooks
// @Produce json
// @Param id path string true "Target ID"
// @Success 200 {object} models.WebhookTargetResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/resume [post]
func (a *API) resumeWebhookTarget(c *gin.Context) {
	a.targetAction(c, "Failed to resume webhook target", func(actor webhooks.Actor, id string, _ webhookReviewInput) (*models.WebhookTargetResponse, error) {
		return a.webhooksService().ResumeTarget(c.Request.Context(), actor, id)
	})
}

// rotateWebhookSecret godoc
// @Summary Rotate a target's signing secret
// @Description Returns the new secret once. The previous secret keeps producing a second signature for the configured grace period.
// @Tags Webhooks
// @Produce json
// @Param id path string true "Target ID"
// @Success 200 {object} webhooks.RotatedSecret
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/rotate-secret [post]
func (a *API) rotateWebhookSecret(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	res, err := a.webhooksService().RotateSecret(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		webhookErrorResponse(c, err, "Failed to rotate signing secret")
		return
	}
	c.JSON(http.StatusOK, res)
}

type webhookTestInput struct {
	Topic string `json:"topic"`
}

// testWebhookTarget godoc
// @Summary Send a test event to an approved target
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Target ID"
// @Param body body webhookTestInput false "Optional topic for the sample event"
// @Success 202 {object} map[string]string "delivery_id"
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/targets/{id}/test [post]
func (a *API) testWebhookTarget(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var in webhookTestInput
	if err := bindOptionalJSON(c, &in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	id, err := a.webhooksService().SendTest(c.Request.Context(), actor, c.Param("id"), strings.TrimSpace(in.Topic))
	if err != nil {
		webhookErrorResponse(c, err, "Failed to send test event")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"delivery_id": id})
}

// --- Deliveries ---

// listWebhookDeliveries godoc
// @Summary Search the delivery log
// @Description Page through deliveries with filters. Newest first unless sort=asc.
// @Tags Webhooks
// @Produce json
// @Param target_id query string false "Target ID"
// @Param topic query string false "Exact topic"
// @Param status query string false "Comma-separated statuses: queued, in_flight, retrying, succeeded, dead_lettered, cancelled"
// @Param event_id query string false "Event ID"
// @Param kind query string false "event, test or replay"
// @Param start_date query string false "Start (YYYY-MM-DD or RFC3339)"
// @Param end_date query string false "End (YYYY-MM-DD or RFC3339)"
// @Param search query string false "Delivery or event ID (exact), or substring of target URL, topic or last error; bounded to the last 30 days unless start_date is given"
// @Param page query int false "Page (1-based)"
// @Param page_size query int false "Page size (max 500)"
// @Param sort query string false "asc or desc"
// @Success 200 {object} webhooks.DeliveryPage
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/deliveries [get]
func (a *API) listWebhookDeliveries(c *gin.Context) {
	q, err := parseDeliveryQuery(c)
	if err != nil {
		webhookBadRequest(c, err.Error())
		return
	}
	page, err := a.webhooksService().ListDeliveries(c.Request.Context(), q)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to list deliveries")
		return
	}
	c.JSON(http.StatusOK, page)
}

// getWebhookDelivery godoc
// @Summary Get one delivery with its attempts, event and target
// @Tags Webhooks
// @Produce json
// @Param id path string true "Delivery ID"
// @Success 200 {object} webhooks.DeliveryDetail
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /webhooks/deliveries/{id} [get]
func (a *API) getWebhookDelivery(c *gin.Context) {
	detail, err := a.webhooksService().GetDelivery(c.Request.Context(), c.Param("id"))
	if err != nil {
		webhookErrorResponse(c, err, "Failed to get delivery")
		return
	}
	c.JSON(http.StatusOK, detail)
}

type webhookReplayInput struct {
	ReRender bool `json:"re_render"`
}

// replayWebhookDelivery godoc
// @Summary Replay a delivery
// @Description Enqueues a new delivery of the same event to the same target. Only terminal deliveries can be replayed.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param id path string true "Delivery ID"
// @Param body body webhookReplayInput false "Set re_render to render the payload again from the stored event"
// @Success 202 {object} map[string]string "delivery_id"
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/deliveries/{id}/replay [post]
func (a *API) replayWebhookDelivery(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var in webhookReplayInput
	if err := bindOptionalJSON(c, &in); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	id, err := a.webhooksService().ReplayDelivery(c.Request.Context(), actor, c.Param("id"), in.ReRender)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to replay delivery")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"delivery_id": id})
}

// cancelWebhookDelivery godoc
// @Summary Cancel a queued or retrying delivery
// @Tags Webhooks
// @Param id path string true "Delivery ID"
// @Success 204
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse
// @Router /webhooks/deliveries/{id}/cancel [post]
func (a *API) cancelWebhookDelivery(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	if err := a.webhooksService().CancelDelivery(c.Request.Context(), actor, c.Param("id")); err != nil {
		webhookErrorResponse(c, err, "Failed to cancel delivery")
		return
	}
	c.Status(http.StatusNoContent)
}

// replayWebhookDeadLetters godoc
// @Summary Replay dead letters in bulk
// @Description Replays the given delivery IDs, or every dead letter for a target (all targets when omitted), up to max (default 100, cap 500).
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param body body webhooks.ReplayRequest false "Selection"
// @Success 200 {object} webhooks.ReplayResult
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/deliveries/replay [post]
func (a *API) replayWebhookDeadLetters(c *gin.Context) {
	actor, ok := requireWebhookActor(c)
	if !ok {
		return
	}
	var req webhooks.ReplayRequest
	if err := bindOptionalJSON(c, &req); err != nil {
		webhookBadRequest(c, "invalid request body")
		return
	}
	if req.Max < 0 || req.Max > webhooks.MaxBulkReplay {
		webhookBadRequest(c, fmt.Sprintf("max must be between 1 and %d", webhooks.MaxBulkReplay))
		return
	}
	if len(req.IDs) > webhooks.MaxBulkReplay {
		webhookBadRequest(c, fmt.Sprintf("at most %d ids per request", webhooks.MaxBulkReplay))
		return
	}
	res, err := a.webhooksService().ReplayDeadLetters(c.Request.Context(), actor, req)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to replay dead letters")
		return
	}
	c.JSON(http.StatusOK, res)
}

// exportWebhookDeliveries godoc
// @Summary Export deliveries
// @Description Downloads matching deliveries (max 50,000) as CSV or JSON
// @Tags Webhooks
// @Produce text/csv,application/json
// @Param format query string false "csv (default) or json"
// @Success 200 {string} string "file"
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/deliveries/export [get]
func (a *API) exportWebhookDeliveries(c *gin.Context) {
	q, err := parseDeliveryQuery(c)
	if err != nil {
		webhookBadRequest(c, err.Error())
		return
	}
	format := strings.ToLower(c.DefaultQuery("format", webhooks.FormatCSV))
	if format != webhooks.FormatCSV && format != webhooks.FormatJSON {
		webhookBadRequest(c, "format must be csv or json")
		return
	}
	// Streamed batch by batch straight to the response; memory stays flat
	// however large the log is.
	w := &exportWriter{
		c:        c,
		format:   format,
		filename: fmt.Sprintf("webhook_deliveries_%s.%s", time.Now().UTC().Format("20060102_150405"), format),
	}
	if err := a.webhooksService().Export(c.Request.Context(), q, format, w); err != nil {
		if !w.started {
			webhookErrorResponse(c, err, "Failed to export deliveries")
			return
		}
		// Headers are gone; the client gets a truncated file and the error
		// is left on the context for the logger.
		_ = c.Error(err)
	}
}

// getWebhookStats godoc
// @Summary Delivery statistics
// @Tags Webhooks
// @Produce json
// @Param window query string false "1h, 24h (default) or 7d"
// @Success 200 {object} webhooks.Stats
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /webhooks/stats [get]
func (a *API) getWebhookStats(c *gin.Context) {
	window := strings.ToLower(c.DefaultQuery("window", "24h"))
	if _, ok := webhooks.StatsWindows[window]; !ok {
		webhookBadRequest(c, "window must be one of 1h, 24h, 7d")
		return
	}
	stats, err := a.webhooksService().Stats(c.Request.Context(), window)
	if err != nil {
		webhookErrorResponse(c, err, "Failed to compute statistics")
		return
	}
	c.JSON(http.StatusOK, stats)
}
