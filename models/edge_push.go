package models

import (
	"encoding/json"
	"time"
)

// A configuration push (a "reload" in the API) is a PushOperation with one
// EdgePushCommand per target edge. Both live in the database so that any
// replica can deliver a command (whichever holds the edge's stream) and any
// replica can report the outcome. See features/ClusterControlPlane.md.

// Push operation statuses. in_progress until every command is terminal.
const (
	PushOperationInProgress       = "in_progress"
	PushOperationSucceeded        = "succeeded"
	PushOperationSucceededWarning = "succeeded_with_warnings"
	PushOperationPartiallyFailed  = "partially_failed"
	PushOperationFailed           = "failed"
	PushOperationExpired          = "expired"
)

// Push command statuses. pending → claimed → sent → one terminal status;
// claimed and sent go back to pending when a transport failure allows
// another attempt.
const (
	PushCommandPending          = "pending"
	PushCommandClaimed          = "claimed"
	PushCommandSent             = "sent"
	PushCommandSucceeded        = "succeeded"
	PushCommandSucceededWarning = "succeeded_with_warning"
	PushCommandFailed           = "failed"
	PushCommandExpired          = "expired"
)

// PushCommandTerminal lists the statuses a command never leaves.
var PushCommandTerminal = []string{PushCommandSucceeded, PushCommandSucceededWarning, PushCommandFailed, PushCommandExpired}

// PushCommandInFlight lists the statuses of a command delivered, or being
// delivered, on a stream.
var PushCommandInFlight = []string{PushCommandClaimed, PushCommandSent}

// PushOperation is one push: to one edge, a namespace, or every namespace.
type PushOperation struct {
	OperationID string     `json:"operation_id" gorm:"primaryKey;size:64"`
	Scope       string     `json:"scope" gorm:"size:16;not null"` // edge | namespace | all
	Namespace   string     `json:"namespace" gorm:"size:255"`
	InitiatedBy string     `json:"initiated_by" gorm:"size:255"`
	Status      string     `json:"status" gorm:"size:32;not null;index"`
	Total       int        `json:"total"`
	CreatedAt   time.Time  `json:"created_at" gorm:"index"`
	DeadlineAt  time.Time  `json:"deadline_at"`
	CompletedAt *time.Time `json:"completed_at" gorm:"index"` // retention
}

// PushAttempt records one delivery attempt of a command, so a failure can
// be explained attempt by attempt.
type PushAttempt struct {
	Attempt int       `json:"attempt"`
	Node    string    `json:"node"`
	At      time.Time `json:"at"`
	Outcome string    `json:"outcome"`
}

// EdgePushCommand is the push of one operation to one edge.
type EdgePushCommand struct {
	ID          int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	OperationID string `json:"operation_id" gorm:"size:64;not null;uniqueIndex:idx_edge_push_commands_op_edge"`
	EdgeID      string `json:"edge_id" gorm:"size:255;not null;uniqueIndex:idx_edge_push_commands_op_edge;index;index:idx_edge_push_commands_status_edge,priority:2"`
	Namespace   string `json:"namespace" gorm:"size:255"`
	// The janitor's and dispatcher's queries lead with status; the
	// composite indexes serve them (see Coordinator.janitor, dispatch).
	Status string `json:"status" gorm:"size:32;not null;index:idx_edge_push_commands_status_deadline,priority:1;index:idx_edge_push_commands_status_claim,priority:1;index:idx_edge_push_commands_status_edge,priority:1;index:idx_edge_push_commands_status_created,priority:1"`

	Attempts    int `json:"attempts"`
	MaxAttempts int `json:"max_attempts"`
	// ClaimedBy is the replica delivering it, on StreamSessionID; the claim
	// lapses at ClaimExpiresAt if the command is never sent.
	ClaimedBy       string     `json:"claimed_by" gorm:"size:128"`
	ClaimExpiresAt  *time.Time `json:"claim_expires_at" gorm:"index:idx_edge_push_commands_status_claim,priority:2"`
	StreamSessionID string     `json:"stream_session_id" gorm:"size:64"`
	SentAt          *time.Time `json:"sent_at"`

	// Phase and Message are the edge's latest report (or, once terminal,
	// the reason for the outcome).
	Phase   string     `json:"phase" gorm:"size:32"`
	PhaseAt *time.Time `json:"phase_at"`
	Message string     `json:"message" gorm:"type:text"`
	// Warning explains a succeeded_with_warning outcome.
	Warning string `json:"warning" gorm:"type:text"`

	// ExpectedChecksum is the namespace's checksum when the edge answered
	// READY, LoadedChecksum the one the edge had loaded then.
	ExpectedChecksum string `json:"expected_checksum" gorm:"size:64"`
	LoadedChecksum   string `json:"loaded_checksum" gorm:"size:64"`

	History []PushAttempt `json:"history" gorm:"serializer:json;type:text"`

	// Version increases with every change. Replicas change a command by
	// reading it, checking the change still applies and writing it back
	// only if Version is unchanged, so no change (or history entry) is
	// lost to a concurrent one.
	Version int64 `json:"-" gorm:"not null;default:0"`

	DeadlineAt  time.Time  `json:"deadline_at" gorm:"index:idx_edge_push_commands_status_deadline,priority:2"`
	CreatedAt   time.Time  `json:"created_at" gorm:"index:idx_edge_push_commands_status_created,priority:2"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// PushAttemptsJSON encodes an attempt history for a map-based update, which
// bypasses the History field's serializer.
func PushAttemptsJSON(h []PushAttempt) string {
	if h == nil {
		h = []PushAttempt{}
	}
	b, err := json.Marshal(h)
	if err != nil {
		return "[]"
	}
	return string(b)
}
