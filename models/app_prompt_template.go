package models

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// DefaultPromptTemplateExpiryDays is how long an approved template stays
// approved without a match before it drops back to proposed.
const DefaultPromptTemplateExpiryDays = 30

// AppPromptTemplate is a trusted prompt template: an app's own prompt text
// that, once an administrator approves it, request guardrails do not score
// (features/TrustedPromptTemplates.md). Each row is one version of a named
// template; changing its constants or placement makes a new version that
// needs approving again. Not to be confused with PromptTemplate, the chat
// room's prompt shortcuts.
type AppPromptTemplate struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	AppID   uint   `json:"app_id" gorm:"not null;uniqueIndex:idx_app_prompt_template_version;index:idx_app_prompt_template_status"`
	Name    string `json:"name" gorm:"size:128;not null;uniqueIndex:idx_app_prompt_template_version"`
	Version int    `json:"version" gorm:"not null;uniqueIndex:idx_app_prompt_template_version"`

	PlacementRole    string `json:"-" gorm:"size:16;not null"`
	PlacementMessage string `json:"-" gorm:"size:16;not null"`
	PlacementAnchor  string `json:"-" gorm:"size:16;not null"`
	// Parts are the template's constants and slots, in order. Constants
	// are stored as written: they are the app's own prompt, not a secret.
	Parts []templates.Part `json:"parts" gorm:"type:text;serializer:json"`

	Status string `json:"status" gorm:"size:16;not null;default:proposed;index:idx_app_prompt_template_status"`
	// CreatedBy proposed this version; ApprovedBy and ApprovedAt record its
	// approval (cleared when it lapses or is retired).
	CreatedBy  uint       `json:"created_by"`
	ApprovedBy *uint      `json:"approved_by"`
	ApprovedAt *time.Time `json:"approved_at"`
	// ExpiresAfterDays is how long the template stays approved without a
	// match before it drops back to proposed.
	ExpiresAfterDays int        `json:"expires_after_days" gorm:"not null;default:30"`
	LastMatchedAt    *time.Time `json:"last_matched_at"`
}

// TableName is the table's name.
func (AppPromptTemplate) TableName() string { return "app_prompt_templates" }

// Placement is the template's placement.
func (t *AppPromptTemplate) Placement() templates.Placement {
	return templates.Placement{Role: t.PlacementRole, Message: t.PlacementMessage, Anchor: t.PlacementAnchor}
}

// SetPlacement stores p.
func (t *AppPromptTemplate) SetPlacement(p templates.Placement) {
	t.PlacementRole, t.PlacementMessage, t.PlacementAnchor = p.Role, p.Message, p.Anchor
}

// Lapsed reports an approved template that has gone ExpiresAfterDays
// without a match (counted from its approval when it has never matched).
func (t *AppPromptTemplate) Lapsed(now time.Time) bool {
	if t.Status != templates.StatusApproved || t.ExpiresAfterDays <= 0 {
		return false
	}
	since := t.LastMatchedAt
	if since == nil || (t.ApprovedAt != nil && t.ApprovedAt.After(*since)) {
		since = t.ApprovedAt
	}
	if since == nil {
		return false
	}
	return now.Sub(*since) > time.Duration(t.ExpiresAfterDays)*24*time.Hour
}

// Matcher is the template as the guardrail matcher uses it.
func (t *AppPromptTemplate) Matcher() *templates.Template {
	return &templates.Template{RecordID: t.ID, Name: t.Name, Version: t.Version, Placement: t.Placement(), Parts: t.Parts}
}

// Create stores a new template version.
func (t *AppPromptTemplate) Create(db *gorm.DB) error {
	return db.Create(t).Error
}

// Update saves the template.
func (t *AppPromptTemplate) Update(db *gorm.DB) error {
	return db.Save(t).Error
}

// Delete removes the template version.
func (t *AppPromptTemplate) Delete(db *gorm.DB) error {
	return db.Delete(t).Error
}

// GetAppPromptTemplate loads one template version of an app.
func GetAppPromptTemplate(db *gorm.DB, appID, id uint) (*AppPromptTemplate, error) {
	var t AppPromptTemplate
	if err := db.Where("app_id = ?", appID).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListAppPromptTemplates lists an app's template versions, by name and
// version; status filters when not empty.
func ListAppPromptTemplates(db *gorm.DB, appID uint, status string) ([]AppPromptTemplate, error) {
	var out []AppPromptTemplate
	q := db.Where("app_id = ?", appID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("name, version").Find(&out).Error
	return out, err
}

// ListApprovedPromptTemplatesForApps lists the approved template versions of
// the apps, by app, name and version (the edge configuration snapshot).
func ListApprovedPromptTemplatesForApps(db *gorm.DB, appIDs []uint) ([]AppPromptTemplate, error) {
	var out []AppPromptTemplate
	if len(appIDs) == 0 {
		return out, nil
	}
	err := db.Where("app_id IN ? AND status = ?", appIDs, templates.StatusApproved).Order("app_id, name, version").Find(&out).Error
	return out, err
}

// TouchApprovedAppPromptTemplates records a match of each approved template
// among ids (edge gateways report theirs in heartbeats). Templates that are
// no longer approved are left alone.
func TouchApprovedAppPromptTemplates(db *gorm.DB, ids []uint, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Model(&AppPromptTemplate{}).Where("id IN ? AND status = ?", ids, templates.StatusApproved).UpdateColumn("last_matched_at", at).Error
}

// CountApprovedAppPromptTemplates counts an app's approved templates.
func CountApprovedAppPromptTemplates(db *gorm.DB, appID uint) (int64, error) {
	var n int64
	err := db.Model(&AppPromptTemplate{}).Where("app_id = ? AND status = ?", appID, templates.StatusApproved).Count(&n).Error
	return n, err
}

// LatestAppPromptTemplateVersion is the highest version of an app's named
// template, 0 when there is none.
func LatestAppPromptTemplateVersion(db *gorm.DB, appID uint, name string) (int, error) {
	var v *int
	err := db.Model(&AppPromptTemplate{}).Where("app_id = ? AND name = ?", appID, name).Select("MAX(version)").Scan(&v).Error
	if err != nil || v == nil {
		return 0, err
	}
	return *v, nil
}

// TouchAppPromptTemplate records a match of the template.
func TouchAppPromptTemplate(db *gorm.DB, id uint, at time.Time) error {
	return db.Model(&AppPromptTemplate{}).Where("id = ?", id).UpdateColumn("last_matched_at", at).Error
}

// DeleteAppPromptTemplates removes every template of an app.
func DeleteAppPromptTemplates(db *gorm.DB, appID uint) error {
	return db.Where("app_id = ?", appID).Delete(&AppPromptTemplate{}).Error
}
