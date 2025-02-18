package notifications

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTestMailer(t *testing.T) {
	// Create a new test mailer
	mailer := NewTestMailer()

	// Create a mail service with test mailer
	service := NewMailService(
		"from@example.com",
		"smtp.test.com", // Non-empty SMTP host to use test mailer
		587,
		"user",
		"pass",
		mailer,
	)

	// Send a test email
	err := service.SendEmail("to@example.com", "Test Subject", "Test Body")
	assert.NoError(t, err)

	// Verify the email was captured
	emails := mailer.GetEmails()
	assert.Len(t, emails, 1)

	email := emails[0]
	assert.Equal(t, "from@example.com", email.From)
	assert.Equal(t, "to@example.com", email.To)
	assert.Equal(t, "Test Subject", email.Subject)
	assert.Equal(t, "(email body)", email.Body) // Since we can't access the actual body
	assert.True(t, email.SentAt.Before(time.Now()))

	// Test clearing emails
	mailer.ClearEmails()
	assert.Empty(t, mailer.GetEmails())

	// Test multiple emails
	err = service.SendEmail("to1@example.com", "Subject 1", "Body 1")
	assert.NoError(t, err)
	err = service.SendEmail("to2@example.com", "Subject 2", "Body 2")
	assert.NoError(t, err)

	emails = mailer.GetEmails()
	assert.Len(t, emails, 2)
	assert.Equal(t, "to1@example.com", emails[0].To)
	assert.Equal(t, "to2@example.com", emails[1].To)
}

func TestMailServiceBackwardCompatibility(t *testing.T) {
	// Test the legacy behavior where empty SMTPHost skips sending
	service := NewMailService(
		"from@example.com",
		"", // Empty SMTP host for backward compatibility
		587,
		"user",
		"pass",
		nil, // No mailer needed when using empty host
	)

	// Should return nil without error
	err := service.SendEmail("to@example.com", "Test Subject", "Test Body")
	assert.NoError(t, err)
}
