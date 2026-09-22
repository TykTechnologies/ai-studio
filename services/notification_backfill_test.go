package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rows written before email framing was stripped at write time still carry
// "Subject: ... Dear Administrator ..." bodies. The backfill rewrites those
// once, leaves clean rows alone, and is a no-op when run again.
func TestBackfillLegacyBodies(t *testing.T) {
	db, ns, admin1, _, user := setupNotificationOptionsTest(t)

	legacy := &models.Notification{
		NotificationID: "legacy_1", Type: "app", Title: "New App", UserID: admin1.ID, SentAt: time.Now(),
		Content: "Subject: New App Created on AI Portal\n\nDear Administrator,\n\nA new application has been created:\n\nName: <b>X</b>\n\nThis is an automated notification. Please do not reply to this email.\n\nBest regards,\nAI Portal System\n",
	}
	require.NoError(t, db.Create(legacy).Error)
	greetingOnly := &models.Notification{
		NotificationID: "legacy_2", Type: "user", Title: "Registered", UserID: user.ID, SentAt: time.Now(),
		Content: "Hi there,\n\nYour export is ready.\n\nRegards,\nExport bot",
	}
	require.NoError(t, db.Create(greetingOnly).Error)
	clean := &models.Notification{
		NotificationID: "clean_1", Type: "tool", Title: "Submission", UserID: admin1.ID, SentAt: time.Now(),
		Content: "A new **tool** submission is waiting for review.\n\n- **Name:** Weather",
	}
	require.NoError(t, db.Create(clean).Error)

	changed, err := ns.BackfillLegacyBodies()
	require.NoError(t, err)
	assert.Equal(t, int64(2), changed)

	// A fresh struct per lookup: GORM folds a populated primary key into
	// the next query's WHERE clause.
	content := func(notificationID string) string {
		var got models.Notification
		require.NoError(t, db.Where("notification_id = ?", notificationID).First(&got).Error)
		return got.Content
	}
	assert.Equal(t, "A new application has been created:\n\nName: X", content("legacy_1"))
	assert.Equal(t, "Your export is ready.", content("legacy_2"))
	assert.Equal(t, clean.Content, content("clean_1"), "a row without email framing is untouched")

	changed, err = ns.BackfillLegacyBodies()
	require.NoError(t, err)
	assert.Equal(t, int64(0), changed, "the backfill is idempotent")
}
