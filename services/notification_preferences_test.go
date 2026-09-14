package services_test

import (
	"fmt"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Email delivery preference and recipient rules on the delivery core.

func newUser(t *testing.T, db *gorm.DB, email string, admin bool) *models.User {
	t.Helper()
	u := &models.User{Email: email, Name: email, IsAdmin: admin, NotificationsEnabled: admin, EmailVerified: true}
	require.NoError(t, u.Create(db))
	return u
}

func emailsTo(mailer *notifications.TestMailer, to string) int {
	n := 0
	for _, e := range mailer.GetEmails() {
		if e.To == to {
			n++
		}
	}
	return n
}

func TestEmailNotificationsEnabled_DefaultTrapAndSwitchOff(t *testing.T) {
	db := apitest.SetupTestDB(t)

	// gorm default:true: an explicit false on insert comes back true, which
	// is why the preference is only ever switched off by column update.
	u := &models.User{Email: "pref@test.com", Name: "Pref", EmailVerified: true, EmailNotificationsEnabled: false}
	require.NoError(t, u.Create(db))
	var stored models.User
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.True(t, stored.EmailNotificationsEnabled, "insert-time false is swallowed by the column default")

	require.NoError(t, models.SetEmailNotificationsEnabled(db, u.ID, false))
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.False(t, stored.EmailNotificationsEnabled, "a user can turn email off")

	require.NoError(t, models.SetEmailNotificationsEnabled(db, u.ID, true))
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.True(t, stored.EmailNotificationsEnabled)
}

func TestDeliver_HonoursEmailPreference(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewTestNotificationService(db)
	mailer := svc.GetMailer().(*notifications.TestMailer)

	wantsMail := newUser(t, db, "mail@test.com", false)
	noMail := newUser(t, db, "nomail@test.com", false)
	require.NoError(t, models.SetEmailNotificationsEnabled(db, noMail.ID, false))

	for _, u := range []*models.User{wantsMail, noMail} {
		require.NoError(t, svc.NotifyDirect(fmt.Sprintf("pref_%d", u.ID), "app", "Approved", "Your app is live", u.ID))
	}

	assert.Equal(t, 1, emailsTo(mailer, wantsMail.Email))
	assert.Equal(t, 0, emailsTo(mailer, noMail.Email), "no email when the recipient switched it off")

	var stored []models.Notification
	require.NoError(t, db.Where("user_id IN ?", []uint{wantsMail.ID, noMail.ID}).Find(&stored).Error)
	assert.Len(t, stored, 2, "the in-app record is kept either way")
}

func TestNotifyWithOptions_SingleUserBranchSkipsDisabled(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewTestNotificationService(db)

	active := newUser(t, db, "active@test.com", false)
	disabled := newUser(t, db, "disabled@test.com", false)
	require.NoError(t, models.SetDisabled(db, disabled.ID, true))

	// A non-admin has notifications_enabled=false by rule (it is the admin
	// fan-out consent), yet still receives feedback on their own objects.
	require.NoError(t, svc.NotifyDirect("single_active", "app", "Approved", "body", active.ID))
	require.NoError(t, svc.NotifyDirect("single_disabled", "app", "Approved", "body", disabled.ID))

	var count int64
	require.NoError(t, db.Model(&models.Notification{}).Where("user_id = ?", active.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, db.Model(&models.Notification{}).Where("user_id = ?", disabled.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "a disabled account receives nothing")
}

func TestListUserNotifications_MetaAndMarkAsReadOwnership(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := services.NewTestNotificationService(db)

	a := newUser(t, db, "a@test.com", false)
	b := newUser(t, db, "b@test.com", false)
	now := time.Now()
	for i, n := range []models.Notification{
		{Title: "a1", UserID: a.ID, Read: true, SentAt: now.Add(-3 * time.Hour)},
		{Title: "a2", UserID: a.ID, SentAt: now.Add(-2 * time.Hour)},
		{Title: "a3", UserID: a.ID, SentAt: now.Add(-1 * time.Hour)},
		{Title: "b1", UserID: b.ID, SentAt: now},
	} {
		n.NotificationID = fmt.Sprintf("n%d", i)
		require.NoError(t, db.Create(&n).Error)
	}

	list, counts, err := svc.ListUserNotifications(a.ID, 2, 0, false)
	require.NoError(t, err)
	assert.Equal(t, services.NotificationCounts{Total: 3, Unread: 2}, counts)
	require.Len(t, list, 2)
	assert.Equal(t, "a3", list[0].Title)

	list, counts, err = svc.ListUserNotifications(a.ID, 10, 0, true)
	require.NoError(t, err)
	assert.Equal(t, services.NotificationCounts{Total: 2, Unread: 2}, counts)
	assert.Len(t, list, 2)

	_, counts, err = svc.ListUserNotifications(424242, 10, 0, false)
	require.NoError(t, err)
	assert.Equal(t, services.NotificationCounts{}, counts, "an empty inbox counts as zero, not NULL")

	var b1 models.Notification
	require.NoError(t, db.Where("title = ?", "b1").First(&b1).Error)
	assert.ErrorIs(t, svc.MarkAsRead(a.ID, b1.ID), services.ErrNotificationNotFound)
	assert.NoError(t, svc.MarkAsRead(b.ID, b1.ID))
	assert.ErrorIs(t, svc.MarkAsRead(b.ID, 424242), services.ErrNotificationNotFound)
}
