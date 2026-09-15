package models

// UserMessage is one user turn handed to a chat session.
type UserMessage struct {
	FileRef []string
	Payload string
	// RunID identifies the turn for v2 (event-mode) sessions so a run handler
	// can follow exactly its own output. Empty means "assign one".
	RunID string
	// Regenerate asks the session to re-run the model on the stored history
	// without adding a new user message (the caller has already removed the
	// previous reply). Payload and FileRef are ignored.
	Regenerate bool
}
