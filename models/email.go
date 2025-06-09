package models

// EmailSender defines the interface for sending emails
type EmailSender interface {
	SendEmail(to, subject, body string) error
}
