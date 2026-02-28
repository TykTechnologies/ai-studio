package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// actorID extracts the authenticated user's ID from the gin context.
// Returns 0 if the user is not present (should not happen on admin-only routes).
func actorID(c *gin.Context) uint {
	u, ok := c.Get("user")
	if !ok {
		return 0
	}
	user, ok := u.(*models.User)
	if !ok {
		return 0
	}
	return user.ID
}

// WebhookHandlers provides HTTP handlers for outbound webhook subscription management.
type WebhookHandlers struct {
	svc *services.WebhookService
}

// NewWebhookHandlers constructs a WebhookHandlers.
func NewWebhookHandlers(svc *services.WebhookService) *WebhookHandlers {
	return &WebhookHandlers{svc: svc}
}

func (h *WebhookHandlers) webhookError(c *gin.Context, status int, title, detail string) {
	c.JSON(status, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: title, Detail: detail}},
	})
}

// webhookRequest is the JSON body for create/update.
type webhookRequest struct {
	Name            string                         `json:"name"`
	URL             string                         `json:"url"`
	Secret          string                         `json:"secret"`
	Enabled         bool                           `json:"enabled"`
	Description     string                         `json:"description"`
	Topics          []string                       `json:"topics"`
	RetryPolicy     models.WebhookRetryPolicy      `json:"retry_policy"`
	TransportConfig models.WebhookTransportConfig  `json:"transport_config"`
}

// webhookResponse is the JSON representation returned to callers.
type webhookResponse struct {
	ID              uint                           `json:"id"`
	Name            string                         `json:"name"`
	URL             string                         `json:"url"`
	Secret          string                         `json:"secret"`
	Enabled         bool                           `json:"enabled"`
	Description     string                         `json:"description"`
	Topics          []string                       `json:"topics"`
	RetryPolicy     models.WebhookRetryPolicy      `json:"retry_policy"`
	TransportConfig models.WebhookTransportConfig  `json:"transport_config"`
}

func topicsToStrings(topics []models.WebhookTopic) []string {
	out := make([]string, len(topics))
	for i, t := range topics {
		out[i] = t.Topic
	}
	return out
}

func stringsToTopics(subID uint, topics []string) []models.WebhookTopic {
	out := make([]models.WebhookTopic, len(topics))
	for i, t := range topics {
		out[i] = models.WebhookTopic{SubscriptionID: subID, Topic: t}
	}
	return out
}

func toResponse(sub *models.WebhookSubscription) webhookResponse {
	return webhookResponse{
		ID:              sub.ID,
		Name:            sub.Name,
		URL:             sub.URL,
		Secret:          sub.Secret,
		Enabled:         sub.Enabled,
		Description:     sub.Description,
		Topics:          topicsToStrings(sub.Topics),
		RetryPolicy:     sub.RetryPolicy,
		TransportConfig: sub.TransportConfig,
	}
}

func applyRequest(sub *models.WebhookSubscription, req webhookRequest) {
	sub.Name = req.Name
	sub.URL = req.URL
	sub.Secret = req.Secret
	sub.Enabled = req.Enabled
	sub.Description = req.Description
	sub.RetryPolicy = req.RetryPolicy
	sub.TransportConfig = req.TransportConfig
	sub.Topics = stringsToTopics(sub.ID, req.Topics)
}

// Create handles POST /api/v1/webhooks
func (h *WebhookHandlers) Create(c *gin.Context) {
	var req webhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	if err := h.svc.ValidateURL(req.URL); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	if err := services.ValidateTopics(req.Topics); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	var sub models.WebhookSubscription
	applyRequest(&sub, req)

	if err := h.svc.CreateWebhook(&sub); err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to create webhook")
		return
	}

	c.JSON(http.StatusCreated, toResponse(&sub))
}

// List handles GET /api/v1/webhooks
func (h *WebhookHandlers) List(c *gin.Context) {
	subs, err := h.svc.ListWebhooks()
	if err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to list webhooks")
		return
	}

	resp := make([]webhookResponse, len(subs))
	for i := range subs {
		resp[i] = toResponse(&subs[i])
	}
	c.JSON(http.StatusOK, resp)
}

// Get handles GET /api/v1/webhooks/:id
func (h *WebhookHandlers) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	sub, err := h.svc.GetWebhook(uint(id))
	if err != nil {
		h.webhookError(c, http.StatusNotFound, "Not Found", "webhook not found")
		return
	}

	c.JSON(http.StatusOK, toResponse(sub))
}

// Update handles PUT /api/v1/webhooks/:id
func (h *WebhookHandlers) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	existing, err := h.svc.GetWebhook(uint(id))
	if err != nil {
		h.webhookError(c, http.StatusNotFound, "Not Found", "webhook not found")
		return
	}

	var req webhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	if err := h.svc.ValidateURL(req.URL); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	if err := services.ValidateTopics(req.Topics); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	applyRequest(existing, req)

	if err := h.svc.UpdateWebhook(existing); err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to update webhook")
		return
	}

	c.JSON(http.StatusOK, toResponse(existing))
}

// Delete handles DELETE /api/v1/webhooks/:id
func (h *WebhookHandlers) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	if err := h.svc.DeleteWebhook(uint(id)); err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to delete webhook")
		return
	}

	c.Status(http.StatusNoContent)
}

// ListDeliveries handles GET /api/v1/webhooks/:id/deliveries
func (h *WebhookHandlers) ListDeliveries(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	pageSize, pageNumber, _ := getPaginationParams(c)

	logs, totalCount, totalPages, err := h.svc.ListDeliveryLogs(uint(id), pageSize, pageNumber)
	if err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to list deliveries")
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": logs})
}

// Test handles POST /api/v1/webhooks/:id/test
func (h *WebhookHandlers) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	sub, err := h.svc.GetWebhook(uint(id))
	if err != nil {
		h.webhookError(c, http.StatusNotFound, "Not Found", "webhook not found")
		return
	}

	if err := h.svc.TestWebhook(sub); err != nil {
		h.webhookError(c, http.StatusBadGateway, "Bad Gateway", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "test delivery sent successfully"})
}

// RetryDelivery handles POST /api/v1/webhooks/:id/deliveries/:log_id/retry
func (h *WebhookHandlers) RetryDelivery(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid webhook id")
		return
	}

	logID, err := strconv.ParseUint(c.Param("log_id"), 10, 64)
	if err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", "invalid log id")
		return
	}

	if err := h.svc.RetryDelivery(uint(id), uint(logID), actorID(c)); err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to enqueue retry")
		return
	}

	c.Status(http.StatusAccepted)
}

// ListTopics handles GET /api/v1/webhooks/topics
func (h *WebhookHandlers) ListTopics(c *gin.Context) {
	c.JSON(http.StatusOK, services.KnownWebhookTopics)
}

// GetConfig handles GET /api/v1/webhooks/config
func (h *WebhookHandlers) GetConfig(c *gin.Context) {
	cfg, err := h.svc.GetWebhookConfig()
	if err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to get webhook config")
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// UpdateConfig handles PUT /api/v1/webhooks/config
func (h *WebhookHandlers) UpdateConfig(c *gin.Context) {
	var cfg models.WebhookConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		h.webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	if err := h.svc.UpdateWebhookConfig(&cfg); err != nil {
		h.webhookError(c, http.StatusInternalServerError, "Internal Server Error", "failed to update webhook config")
		return
	}

	c.JSON(http.StatusOK, cfg)
}
