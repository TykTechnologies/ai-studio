package notifications

import (
	"github.com/go-mail/mail"
)

type EmailSender interface {
	SendEmail(to, subject, body string) error
}

type MailService struct {
	FromEmail string
	SMTPHost  string
	SMTPPort  int
	Username  string
	Password  string
	Mailer    Mailer
}

type Mailer interface {
	DialAndSend(m ...*mail.Message) error
}

func NewMailService(fromEmail, smtpHost string, smtpPort int, username, password string, mailer Mailer) *MailService {
	return &MailService{
		FromEmail: fromEmail,
		SMTPHost:  smtpHost,
		SMTPPort:  smtpPort,
		Username:  username,
		Password:  password,
		Mailer:    mailer,
	}
}

func (m *MailService) SendEmail(to, subject, body string) error {
	msg := mail.NewMessage()
	msg.SetHeader("From", m.FromEmail)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", body)

	if m.SMTPHost == "" {
		return nil // Skip sending in test/dev mode
	}
	return m.Mailer.DialAndSend(msg)
}
