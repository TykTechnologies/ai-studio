package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

type NotificationHandlers struct {
	notificationService *services.NotificationService
}

func NewNotificationHandlers(notificationService *services.NotificationService) *NotificationHandlers {
	return &NotificationHandlers{
		notificationService: notificationService,
	}
}

// ListNotifications handles GET /api/v1/notifications
func (h *NotificationHandlers) ListNotifications(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit := 20
	offset := 0

	// Get pagination parameters
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	currentUser := user.(*models.User)
	notifications, err := h.notificationService.GetUserNotifications(currentUser.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, notifications)
}

// UnreadCount handles GET /api/v1/notifications/unread/count
func (h *NotificationHandlers) UnreadCount(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	currentUser := user.(*models.User)
	count, err := h.notificationService.GetUnreadCount(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

// MarkAsRead handles PUT /api/v1/notifications/:id/read
func (h *NotificationHandlers) MarkAsRead(c *gin.Context) {
	notificationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	err = h.notificationService.MarkAsRead(uint(notificationID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// MarkAllAsRead handles PUT /api/v1/notifications/read-all
func (h *NotificationHandlers) MarkAllAsRead(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	currentUser := user.(*models.User)
	err := h.notificationService.MarkAllAsRead(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}
