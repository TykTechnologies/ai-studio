package notifications

import (
	"sync"
	"time"

	"github.com/go-mail/mail"
)

// TestEmail represents an email captured during testing
type TestEmail struct {
	From    string
	To      string
	Subject string
	Body    string
	SentAt  time.Time
}

// TestMailer implements the Mailer interface for testing purposes
type TestMailer struct {
	emails []TestEmail
	mu     sync.RWMutex
}

// NewTestMailer creates a new TestMailer instance
func NewTestMailer() *TestMailer {
	return &TestMailer{
		emails: make([]TestEmail, 0),
	}
}

// DialAndSend implements the Mailer interface by storing the email for later verification
func (m *TestMailer) DialAndSend(msgs ...*mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, msg := range msgs {
		email := TestEmail{
			From:    msg.GetHeader("From")[0],
			To:      msg.GetHeader("To")[0],
			Subject: msg.GetHeader("Subject")[0],
			Body:    "(email body)", // Since we can't access the internal body, we just acknowledge it was set
			SentAt:  time.Now(),
		}
		m.emails = append(m.emails, email)
	}
	return nil
}

// GetEmails returns all emails that have been "sent" through this mailer
func (m *TestMailer) GetEmails() []TestEmail {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent modification of internal state
	result := make([]TestEmail, len(m.emails))
	copy(result, m.emails)
	return result
}

// ClearEmails removes all stored emails
func (m *TestMailer) ClearEmails() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.emails = make([]TestEmail, 0)
}
