package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// Guardrail exception statuses.
const (
	GuardrailExceptionProposed = "proposed"
	GuardrailExceptionApproved = "approved"
	GuardrailExceptionRetired  = "retired"
)

// DefaultGuardrailExceptionExpiryDays is how long an approved exception
// stays approved without a hit before it drops back to proposed.
const DefaultGuardrailExceptionExpiryDays = 30

// MaxApprovedGuardrailExceptionsPerApp caps an app's approved exceptions.
const MaxApprovedGuardrailExceptionsPerApp = 20

// GuardrailException softens the action a guardrail filter takes on a
// verdict for one app (features/GuardrailExceptions.md): for findings of
// one filter, detector ("" = any) and channel (and, on tool channels,
// optionally one tool), the action becomes Action, bounded by the floor for
// the app's guardrail capability (guardrails.CheckException). It is scoped
// by who sends the traffic and where the text sits, never by content, path
// or phrase. Only approved exceptions apply.
type GuardrailException struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AppID    uint   `json:"app_id" gorm:"not null;index:idx_guardrail_exception_app_status"`
	FilterID uint   `json:"filter_id" gorm:"not null;index"`
	Detector string `json:"detector" gorm:"size:128;not null;default:''"`
	Channel  string `json:"channel" gorm:"size:32;not null"`
	// ToolName narrows a tool channel to one tool; "" is any tool.
	ToolName string `json:"tool_name" gorm:"size:256;not null;default:''"`
	// Action is what the exception softens the filter's action to (log,
	// quarantine, remove).
	Action string `json:"action" gorm:"size:16;not null"`

	Status        string `json:"status" gorm:"size:16;not null;default:proposed;index:idx_guardrail_exception_app_status"`
	Justification string `json:"justification" gorm:"size:500;not null"`
	// ProposedBy proposed it; ApprovedBy and ApprovedAt record its approval
	// (a full administrator's own exception is approved as it is created,
	// ApprovedBy = ProposedBy). Cleared when it lapses or changes.
	ProposedBy uint       `json:"proposed_by"`
	ApprovedBy *uint      `json:"approved_by"`
	ApprovedAt *time.Time `json:"approved_at"`
	// ExpiresAfterDays is how long it stays approved without a hit
	// (counted from its approval when it has never applied).
	ExpiresAfterDays int        `json:"expires_after_days" gorm:"not null;default:30"`
	LastHitAt        *time.Time `json:"last_hit_at"`
	HitCount         int64      `json:"hit_count" gorm:"not null;default:0"`
	// SourceEvent is the compliance event the exception was created from.
	SourceEvent *uint `json:"source_event"`
}

// TableName is the table's name.
func (GuardrailException) TableName() string { return "guardrail_exceptions" }

// Lapsed reports an approved exception that has gone ExpiresAfterDays
// without a hit (counted from its approval when it has never hit).
func (e *GuardrailException) Lapsed(now time.Time) bool {
	if e.Status != GuardrailExceptionApproved || e.ExpiresAfterDays <= 0 {
		return false
	}
	since := e.LastHitAt
	if since == nil || (e.ApprovedAt != nil && e.ApprovedAt.After(*since)) {
		since = e.ApprovedAt
	}
	if since == nil {
		return false
	}
	return now.Sub(*since) > time.Duration(e.ExpiresAfterDays)*24*time.Hour
}

// ContentHash binds an approval to what a reviewer saw: the hex SHA-256 of
// every field a writer can change on a proposed exception. Approving takes
// it back and refuses an exception whose hash has changed.
func (e *GuardrailException) ContentHash() string {
	b, err := json.Marshal(struct {
		FilterID         uint   `json:"filter_id"`
		Detector         string `json:"detector"`
		Channel          string `json:"channel"`
		ToolName         string `json:"tool_name"`
		Action           string `json:"action"`
		Justification    string `json:"justification"`
		ExpiresAfterDays int    `json:"expires_after_days"`
	}{e.FilterID, e.Detector, e.Channel, e.ToolName, e.Action, e.Justification, e.ExpiresAfterDays})
	if err != nil {
		// Strings and ints always marshal.
		panic(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SameScope reports whether two exceptions cover the same findings.
func (e *GuardrailException) SameScope(o *GuardrailException) bool {
	return e.AppID == o.AppID && e.FilterID == o.FilterID && e.Detector == o.Detector &&
		e.Channel == o.Channel && e.ToolName == o.ToolName
}

// GetGuardrailException loads one exception of an app.
func GetGuardrailException(db *gorm.DB, appID, id uint) (*GuardrailException, error) {
	var e GuardrailException
	if err := db.Where("app_id = ?", appID).First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// ListGuardrailExceptions lists an app's exceptions by id; status filters
// when not empty.
func ListGuardrailExceptions(db *gorm.DB, appID uint, status string) ([]GuardrailException, error) {
	var out []GuardrailException
	q := db.Where("app_id = ?", appID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("id").Find(&out).Error
	return out, err
}

// ListGuardrailExceptionsForFilter lists the exceptions of a filter across
// apps, by app and id; status filters when not empty.
func ListGuardrailExceptionsForFilter(db *gorm.DB, filterID uint, status string) ([]GuardrailException, error) {
	var out []GuardrailException
	q := db.Where("filter_id = ?", filterID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("app_id, id").Find(&out).Error
	return out, err
}

// ListApprovedGuardrailExceptionsForApps lists the approved exceptions of
// the apps, by app and id (the edge configuration snapshot).
func ListApprovedGuardrailExceptionsForApps(db *gorm.DB, appIDs []uint) ([]GuardrailException, error) {
	var out []GuardrailException
	if len(appIDs) == 0 {
		return out, nil
	}
	err := db.Where("app_id IN ? AND status = ?", appIDs, GuardrailExceptionApproved).Order("app_id, id").Find(&out).Error
	return out, err
}

// CountApprovedGuardrailExceptions counts an app's approved exceptions.
func CountApprovedGuardrailExceptions(db *gorm.DB, appID uint) (int64, error) {
	var n int64
	err := db.Model(&GuardrailException{}).Where("app_id = ? AND status = ?", appID, GuardrailExceptionApproved).Count(&n).Error
	return n, err
}

// AddGuardrailExceptionHits records n more hits of an approved exception,
// the latest at at. Exceptions that are no longer approved are left alone.
func AddGuardrailExceptionHits(db *gorm.DB, id uint, n int64, at time.Time) error {
	if n <= 0 {
		return nil
	}
	return db.Model(&GuardrailException{}).Where("id = ? AND status = ?", id, GuardrailExceptionApproved).
		UpdateColumns(map[string]any{
			"hit_count":   gorm.Expr("hit_count + ?", n),
			"last_hit_at": at,
		}).Error
}

// LapseGuardrailException returns an approved exception to proposed,
// clearing its approval. It reports whether the row changed.
func LapseGuardrailException(db *gorm.DB, id uint) (bool, error) {
	res := db.Model(&GuardrailException{}).
		Where("id = ? AND status = ?", id, GuardrailExceptionApproved).
		Updates(map[string]any{"status": GuardrailExceptionProposed, "approved_by": nil, "approved_at": nil})
	return res.RowsAffected > 0, res.Error
}

// lockForUpdate locks the rows a query reads on PostgreSQL (SQLite
// serialises transactions anyway).
func lockForUpdate(db *gorm.DB) *gorm.DB {
	if db.Dialector.Name() == "postgres" {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return db
}

// GetGuardrailExceptionForUpdate loads one exception of an app inside a
// transaction, locking its row on PostgreSQL.
func GetGuardrailExceptionForUpdate(tx *gorm.DB, appID, id uint) (*GuardrailException, error) {
	return GetGuardrailException(lockForUpdate(tx), appID, id)
}
