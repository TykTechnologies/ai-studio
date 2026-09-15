package models

// ToolResult is a client-side (human-in-the-loop) tool outcome supplied by
// the browser for a tool call the session parked.
type ToolResult struct {
	ToolCallID string
	Result     string
	IsError    bool
}

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
	// ToolResults resumes a turn that is waiting on client-side tools. When
	// set, Payload is ignored.
	ToolResults []ToolResult
}
