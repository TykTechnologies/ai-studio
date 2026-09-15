package models

// UserMessage is one user turn handed to a chat session.
type UserMessage struct {
	FileRef []string
	Payload string
	// RunID identifies the turn for v2 (event-mode) sessions so a run handler
	// can follow exactly its own output. Empty means "assign one".
	RunID string
}
