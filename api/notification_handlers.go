package api

import (
	"errors"
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
//
// Query: limit (default 20), offset (default 0), unread=true to list only
// unread ones. Response: {"data":[...], "meta":{"total","unread","limit","offset"}}
// where total counts the rows matching the filter and unread is the user's
// unread count regardless of the filter.
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

	unreadOnly := c.Query("unread") == "true"

	currentUser := user.(*models.User)
	notifications, counts, err := h.notificationService.ListUserNotifications(currentUser.ID, limit, offset, unreadOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": notifications,
		"meta": gin.H{
			"total":  counts.Total,
			"unread": counts.Unread,
			"limit":  limit,
			"offset": offset,
		},
	})
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

// MarkAsRead handles PUT /api/v1/notifications/:id/read. Only the
// recipient can mark a notification read; anyone else's id is a 404.
func (h *NotificationHandlers) MarkAsRead(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	notificationID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	currentUser := user.(*models.User)
	err = h.notificationService.MarkAsRead(currentUser.ID, uint(notificationID))
	if err != nil {
		if errors.Is(err, services.ErrNotificationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
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
