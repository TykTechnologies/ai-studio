package services

import (
	"bytes"
	"io"
	"mime/quotedprintable"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/go-mail/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostileText = "![x](https://evil/p.png) [y](javascript:alert(1)) <b>"

func TestEscapeMarkdown(t *testing.T) {
	assert.Equal(t, `\!\[x\]\(https://evil/p.png\) \[y\]\(javascript:alert\(1\)\) \<b\>`, EscapeMarkdown(hostileText))
	assert.Equal(t, `a\\b \*c\* \_d\_ \`+"`"+`e\`+"`"+` \#f`, EscapeMarkdown("a\\b *c* _d_ `e` #f"))
	assert.Equal(t, "plain text", EscapeMarkdown("plain text"))
}

// The in-app record is plain text: escaped values read as written, with no
// backslashes, while real markup around them is still stripped.
func TestStripMarkup_RemovesEscapes(t *testing.T) {
	in := "Justification: " + EscapeMarkdown(hostileText) + " in **bold** and [a link](/x)"
	got := stripMarkup(in)
	assert.Equal(t, "Justification: "+hostileText+" in bold and a link", got)
	assert.NotContains(t, got, `\`)
	assert.Equal(t, "a_b *c*", stripMarkup(EscapeMarkdown("a_b *c*")))
}

// captureMailer keeps the bodies of the emails sent.
type captureMailer struct {
	mu     sync.Mutex
	bodies []string
}

func (m *captureMailer) DialAndSend(msgs ...*mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range msgs {
		var buf bytes.Buffer
		if _, err := msg.WriteTo(&buf); err != nil {
			return err
		}
		raw := buf.String()
		if i := strings.Index(raw, "\r\n\r\n"); i >= 0 {
			raw = raw[i+4:]
		}
		b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(raw)))
		if err != nil {
			b = []byte(raw)
		}
		m.bodies = append(m.bodies, string(b))
	}
	return nil
}

// A proposal's justification (user input) is escaped in the markdown body
// sent by email and reads cleanly in the in-app record.
func TestNotifyGuardrailExceptionProposed_EscapesUserText(t *testing.T) {
	f := newExceptionFixture(t, "read_only")
	mailer := &captureMailer{}
	f.svc.NotificationService = NewNotificationService(f.db, "test@example.com", "localhost", 25, "u", "p", mailer)
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.admin.ID).Update("email_notifications_enabled", true).Error)
	require.NoError(t, f.db.Model(&models.App{}).Where("id = ?", f.app.ID).Update("name", "Bot *[evil](javascript:x)*").Error)

	s := f.spec("text_user", guardrails.ActionLog)
	s.Justification = ptr(hostileText)
	s.Detector = ptr("prompt_[injection]")
	_, err := f.svc.CreateGuardrailException(f.app.ID, f.asWriter, s)
	require.NoError(t, err)

	var n models.Notification
	require.NoError(t, f.db.Where("notification_id LIKE ?", "guardrail_exception_proposed_%").First(&n).Error)
	assert.Contains(t, n.Content, "Justification: "+hostileText, "the bell reads the text as written")
	assert.Contains(t, n.Content, "Bot *[evil](javascript:x)*")
	assert.Contains(t, n.Content, "prompt_[injection]")
	assert.NotContains(t, n.Content, `\`, "no escaping backslashes in the in-app record")

	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	require.NotEmpty(t, mailer.bodies)
	body := mailer.bodies[0]
	assert.Contains(t, body, EscapeMarkdown(hostileText), "the markdown body is escaped")
	assert.Contains(t, body, EscapeMarkdown("Bot *[evil](javascript:x)*"))
	assert.Contains(t, body, EscapeMarkdown("prompt_[injection]"))
	assert.NotContains(t, body, "!"+"[x](https://evil")
	assert.Contains(t, body, "[Review it on the app page](/admin/apps/", "our own link is left alone")
}

// The capability-reset notification escapes the app's name too.
func TestNotifyGuardrailCapabilityReset_EscapesAppName(t *testing.T) {
	f := newDriftFixture(t, true)
	mailer := &captureMailer{}
	f.svc.NotificationService = NewNotificationService(f.db, "test@example.com", "localhost", 25, "u", "p", mailer)
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.admin.ID).Update("email_notifications_enabled", true).Error)
	require.NoError(t, f.db.Model(&models.App{}).Where("id = ?", f.app.ID).Update("name", hostileText).Error)

	_, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID, WithCapabilityActor(f.writer.ID, false))
	require.NoError(t, err)

	n := f.resetNotifications(t)
	require.Len(t, n, 1)
	assert.Contains(t, n[0].Content, hostileText)
	assert.NotContains(t, n[0].Content, `\`)
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	require.NotEmpty(t, mailer.bodies)
	assert.Contains(t, mailer.bodies[0], EscapeMarkdown(hostileText))
}
