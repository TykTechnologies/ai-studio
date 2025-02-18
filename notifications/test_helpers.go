package notifications

// NewTestMailService creates a new MailService configured for testing
func NewTestMailService() *MailService {
	return NewMailService(
		"test@example.com",
		"smtp.test.com",
		587,
		"user",
		"pass",
		NewTestMailer(),
	)
}
