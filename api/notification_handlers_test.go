package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The notification list carries paging meta and lowercase keys, honours
// unread=true, and mark-read only works on the caller's own notifications.

func seedNotification(t *testing.T, f *lifecycleFixture, userID uint, title string, read bool, sentAt time.Time) models.Notification {
	t.Helper()
	n := models.Notification{
		NotificationID: fmt.Sprintf("%s_%d_%d", title, userID, sentAt.UnixNano()),
		Type:           "app",
		Title:          title,
		Content:        "body",
		UserID:         userID,
		Read:           read,
		SentAt:         sentAt,
		Link:           "/admin/apps/1",
	}
	require.NoError(t, f.auth.Config.DB.Create(&n).Error)
	return n
}

type notificationListResponse struct {
	Data []map[string]interface{} `json:"data"`
	Meta struct {
		Total  int `json:"total"`
		Unread int `json:"unread"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	} `json:"meta"`
}

func TestNotifications_ListMetaAndUnreadFilter(t *testing.T) {
	f := setupLifecycleAPI(t)
	now := time.Now()
	seedNotification(t, f, f.member.ID, "oldest read", true, now.Add(-3*time.Hour))
	seedNotification(t, f, f.member.ID, "middle unread", false, now.Add(-2*time.Hour))
	newest := seedNotification(t, f, f.member.ID, "newest unread", false, now.Add(-time.Hour))
	seedNotification(t, f, f.admin.ID, "someone else's", false, now)

	w := apitest.PerformAuthRequest(f.router, "GET", "/common/api/v1/notifications?limit=2", nil, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp notificationListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.Meta.Total, "total counts the member's rows only")
	assert.Equal(t, 2, resp.Meta.Unread)
	assert.Equal(t, 2, resp.Meta.Limit)
	assert.Equal(t, 0, resp.Meta.Offset)
	require.Len(t, resp.Data, 2, "limit applies")
	assert.Equal(t, "newest unread", resp.Data[0]["title"], "sent_at desc")
	for _, key := range []string{"id", "title", "content", "read", "type", "sent_at", "link", "created_at"} {
		_, ok := resp.Data[0][key]
		assert.True(t, ok, "lowercase key %q present", key)
	}
	_, upper := resp.Data[0]["Title"]
	assert.False(t, upper, "Go field names are gone")
	assert.Equal(t, float64(newest.ID), resp.Data[0]["id"])

	w = apitest.PerformAuthRequest(f.router, "GET", "/common/api/v1/notifications?unread=true&limit=20&offset=1", nil, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total, "total follows the unread filter")
	assert.Equal(t, 2, resp.Meta.Unread)
	assert.Equal(t, 1, resp.Meta.Offset)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "middle unread", resp.Data[0]["title"])

	// An empty page is [] not null.
	w = apitest.PerformAuthRequest(f.router, "GET", "/common/api/v1/notifications?offset=10", nil, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)

	w = apitest.PerformRequest(f.router, "GET", "/common/api/v1/notifications", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNotifications_MarkReadOwnership(t *testing.T) {
	f := setupLifecycleAPI(t)
	mine := seedNotification(t, f, f.member.ID, "mine", false, time.Now())
	theirs := seedNotification(t, f, f.admin.ID, "theirs", false, time.Now())

	w := apitest.PerformAuthRequest(f.router, "PUT", fmt.Sprintf("/common/api/v1/notifications/%d/read", theirs.ID), nil, f.member.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code, "another user's notification reads as missing")
	var stored models.Notification
	require.NoError(t, f.auth.Config.DB.First(&stored, theirs.ID).Error)
	assert.False(t, stored.Read)

	w = apitest.PerformAuthRequest(f.router, "PUT", fmt.Sprintf("/common/api/v1/notifications/%d/read", mine.ID), nil, f.member.APIKey)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var own models.Notification
	require.NoError(t, f.auth.Config.DB.First(&own, mine.ID).Error)
	assert.True(t, own.Read)

	w = apitest.PerformAuthRequest(f.router, "PUT", "/common/api/v1/notifications/999999/read", nil, f.member.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = apitest.PerformAuthRequest(f.router, "PUT", "/common/api/v1/notifications/abc/read", nil, f.member.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
