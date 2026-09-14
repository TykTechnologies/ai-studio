package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationOptionsTest(t *testing.T) (*gorm.DB, *NotificationService, *models.User, *models.User, *models.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	ns := NewTestNotificationService(db)

	admin1 := &models.User{Email: "admin1@test.com", Name: "Admin One", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, admin1.Create(db))
	admin2 := &models.User{Email: "admin2@test.com", Name: "Admin Two", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, admin2.Create(db))
	user := &models.User{Email: "user@test.com", Name: "User", IsAdmin: false, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, user.Create(db))

	return db, ns, admin1, admin2, user
}

func storedNotifications(t *testing.T, db *gorm.DB, prefix string) map[uint]models.Notification {
	t.Helper()
	var stored []models.Notification
	require.NoError(t, db.Where("notification_id LIKE ?", prefix+"%").Find(&stored).Error)
	byUser := map[uint]models.Notification{}
	for _, n := range stored {
		byUser[n.UserID] = n
	}
	return byUser
}

// The user who performed the action is never a recipient, whether they are
// reached through the admin fan-out or named as the specific user.
func TestNotifyWithOptions_SkipsActor(t *testing.T) {
	db, ns, admin1, admin2, user := setupNotificationOptionsTest(t)

	t.Run("admin acting is excluded from the admin fan-out", func(t *testing.T) {
		err := ns.NotifyWithOptions("act_1", "New App", "body", models.NotifyAdmins|user.ID, NotifyOptions{ActorID: admin1.ID})
		require.NoError(t, err)

		got := storedNotifications(t, db, "act_1")
		assert.NotContains(t, got, admin1.ID, "the admin who created the app is not told about it")
		assert.Contains(t, got, admin2.ID)
		assert.Contains(t, got, user.ID)
	})

	t.Run("user acting is excluded as the specific recipient", func(t *testing.T) {
		err := ns.NotifyWithOptions("act_2", "Registered", "body", user.ID, NotifyOptions{ActorID: user.ID})
		require.NoError(t, err)

		got := storedNotifications(t, db, "act_2")
		assert.Empty(t, got, "a user is not told that they registered")
	})

	t.Run("no actor means everyone is notified", func(t *testing.T) {
		err := ns.NotifyWithOptions("act_3", "Plugin", "body", models.NotifyAdmins, NotifyOptions{})
		require.NoError(t, err)

		got := storedNotifications(t, db, "act_3")
		assert.Contains(t, got, admin1.ID)
		assert.Contains(t, got, admin2.ID)
	})
}

// The in-app record carries the type, link and plain summary; the email
// keeps the full body.
func TestNotifyWithOptions_StoresLinkAndSummary(t *testing.T) {
	db, ns, admin1, _, _ := setupNotificationOptionsTest(t)

	email := "Subject: New App\n\nDear Administrator,\n\nA new app was created.\n\nBest regards,\nAI Portal"
	err := ns.NotifyWithOptions("link_1", "New App", email, models.NotifyAdmins, NotifyOptions{
		Type:    "app",
		Link:    "/admin/apps/7",
		Summary: "Dev One created the app \"Billing Copilot\".",
	})
	require.NoError(t, err)

	got := storedNotifications(t, db, "link_1")[admin1.ID]
	assert.Equal(t, "app", got.Type)
	assert.Equal(t, "/admin/apps/7", got.Link)
	assert.Equal(t, "Dev One created the app \"Billing Copilot\".", got.Content)

	mailer := ns.GetMailer().(*notifications.TestMailer)
	emails := mailer.GetEmails()
	require.NotEmpty(t, emails, "the email still goes out")
	assert.Equal(t, "New App", emails[len(emails)-1].Subject)
}

func TestNotifyWithOptions_SkipEmail(t *testing.T) {
	db, ns, _, _, user := setupNotificationOptionsTest(t)
	mailer := ns.GetMailer().(*notifications.TestMailer)
	mailer.ClearEmails()

	err := ns.NotifyWithOptions("skip_1", "Approved", "Your app is approved.", user.ID, NotifyOptions{
		Type: "app", Link: "/portal/apps/3", SkipEmail: true,
	})
	require.NoError(t, err)

	got := storedNotifications(t, db, "skip_1")
	require.Contains(t, got, user.ID)
	assert.Equal(t, "/portal/apps/3", got[user.ID].Link)
	assert.Empty(t, mailer.GetEmails(), "SkipEmail records the notification without mailing it")
}

// Template-driven notifications with no explicit summary get the rendered
// body with its email framing removed.
func TestNotifyTemplate_StripsEmailFramingForInApp(t *testing.T) {
	db, ns, admin1, _, _ := setupNotificationOptionsTest(t)

	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "framed.tmpl")
	require.NoError(t, os.WriteFile(templatePath, []byte(
		"Subject: New User Registration on AI Portal\n\nDear Admin,\n\nA new user has registered on the AI Portal. Here are the details:\n\nName: {{.Name}}\nEmail: {{.Email}}\n\nPlease review this registration.\n\nBest regards,\nAI Portal Team\n"), 0644))

	err := ns.NotifyTemplate("tmpl_1", "New User", templatePath,
		map[string]interface{}{"Name": "Dev One", "Email": "dev1@test.com"}, models.NotifyAdmins, NotifyOptions{Type: "user"})
	require.NoError(t, err)

	got := storedNotifications(t, db, "tmpl_1")[admin1.ID]
	assert.NotContains(t, got.Content, "Subject:")
	assert.NotContains(t, got.Content, "Dear Admin")
	assert.NotContains(t, got.Content, "Best regards")
	assert.NotContains(t, got.Content, "AI Portal Team")
	assert.Contains(t, got.Content, "Name: Dev One")
	assert.Contains(t, got.Content, "Email: dev1@test.com")
	assert.True(t, len(got.Content) > 0 && got.Content[0] == 'A', "content starts at the first real line")
}

func TestStripEmailFraming(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"full email": {
			in:   "Subject: New App Created on AI Portal\n\nDear Administrator,\n\nA new application has been created:\n\nName: X\n\nThis is an automated notification. Please do not reply to this email.\n\nBest regards,\nAI Portal System\n",
			want: "A new application has been created:\n\nName: X",
		},
		"markdown composed in code is untouched": {
			in:   "A new **tool** submission is waiting for review.\n\n- **Name:** Weather\n\n[Open the submission queue](/admin/submissions/3)",
			want: "A new **tool** submission is waiting for review.\n\n- **Name:** Weather\n\n[Open the submission queue](/admin/submissions/3)",
		},
		"greeting only": {
			in:   "Hi there,\nYour export is ready.",
			want: "Your export is ready.",
		},
		"sign-off only": {
			in:   "Your budget is at 90%.\n\nRegards,\nBudget bot",
			want: "Your budget is at 90%.",
		},
		"empty": {in: "", want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, stripEmailFraming(tc.in))
		})
	}
}

// Object names are user-supplied and interpolated into in-app text, so any
// markup in them is dropped before the record is stored.
func TestNotifyWithOptions_StripsMarkupFromInAppText(t *testing.T) {
	db, ns, admin1, _, _ := setupNotificationOptionsTest(t)

	err := ns.NotifyWithOptions("xss_1", "New app <img src=x onerror=alert(1)>", "body", models.NotifyAdmins, NotifyOptions{
		Type:    "app",
		Summary: "Dev created the app \"<script>alert(1)</script>Billing\".",
	})
	require.NoError(t, err)

	got := storedNotifications(t, db, "xss_1")[admin1.ID]
	assert.Equal(t, "New app", got.Title)
	assert.Equal(t, "Dev created the app \"alert(1)Billing\".", got.Content)
	assert.NotContains(t, got.Content, "<")
}
