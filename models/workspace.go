package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// DefaultWorkspaceName is the name the Default workspace is created with.
const DefaultWorkspaceName = "Default workspace"

// Workspace is an ownership boundary for objects:
// every stamped object (WorkspaceStampedModels) carries the id of the
// Workspace that owns it in workspace_id. Exactly one Workspace is the
// Default; it cannot be deleted or replaced, and every object an older
// build wrote (workspace_id 0) belongs to it.
//
// The table exists in Community Edition, holding only the Default
// workspace; managing Workspaces is Enterprise.
type Workspace struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:128;not null" json:"name"`
	// NameKey is Name trimmed and lower-cased: names are unique without
	// regard to case. Set on every save (BeforeSave).
	NameKey     string `gorm:"size:128;not null;uniqueIndex:idx_workspaces_name_key" json:"-"`
	Description string `gorm:"size:1024" json:"description"`
	// IsDefault marks the Default workspace; the partial unique index allows
	// one row with it set.
	IsDefault bool      `gorm:"not null;default:false;uniqueIndex:idx_workspaces_default,where:is_default = true" json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName pins the table name.
func (Workspace) TableName() string { return "workspaces" }

// WorkspaceNameKey is the case-insensitive form names are unique by.
func WorkspaceNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// BeforeSave keeps NameKey in step with Name.
func (w *Workspace) BeforeSave(*gorm.DB) error {
	w.Name = strings.TrimSpace(w.Name)
	w.NameKey = WorkspaceNameKey(w.Name)
	return nil
}

// GetDefaultWorkspace loads the Default workspace (gorm.ErrRecordNotFound
// before EnsureDefaultWorkspace has run).
func GetDefaultWorkspace(db *gorm.DB) (*Workspace, error) {
	var w Workspace
	if err := db.Where("is_default = ?", true).First(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// EffectiveWorkspaceID reads a stored workspace_id: 0, written by a build
// from before Workspaces (a rollback window, or an older replica during a
// rolling upgrade) or before the boot backfill reached the row, is the
// Default workspace.
func EffectiveWorkspaceID(workspaceID, defaultWorkspaceID uint) uint {
	if workspaceID == 0 {
		return defaultWorkspaceID
	}
	return workspaceID
}

// EnsureDefaultWorkspace creates the Default workspace when no row is
// marked default, and returns it. Idempotent; it runs at start-up under the
// migration lock (models.AcquireMigrationLock). If a Workspace already uses
// the default name (case-insensitively) without being the default, that
// row is an administrator's and the Default is refused rather than
// guessed at.
func EnsureDefaultWorkspace(db *gorm.DB) (*Workspace, error) {
	w, err := GetDefaultWorkspace(db)
	if err == nil {
		return w, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load the default workspace: %w", err)
	}
	w = &Workspace{
		Name:        DefaultWorkspaceName,
		Description: "Owns every object not placed in another Workspace.",
		IsDefault:   true,
	}
	if err := db.Create(w).Error; err != nil {
		return nil, fmt.Errorf("create the default workspace: %w", err)
	}
	return w, nil
}

// WorkspaceStampedModels are the models whose rows carry workspace_id.
// Users, teams, roles and other global configuration are not stamped.
func WorkspaceStampedModels() []interface{} {
	return []interface{}{
		&LLM{}, &App{}, &Tool{}, &Datasource{}, &Filter{}, &Embedder{},
		&ModelRouter{}, &SemanticRouter{}, &AgentConfig{},
		&Catalogue{}, &DataCatalogue{}, &ToolCatalogue{},
		&secrets.Secret{}, &FileStore{}, &MCPServer{}, &Plugin{}, &WebhookTarget{},
	}
}

// BackfillWorkspaceIDs stamps every row with workspace_id 0 (soft-deleted
// rows included) with the Default workspace. The selection terminates on
// its own, so it runs on every start: it also stamps rows an older build
// wrote during a rollback window. It writes the column alone (UpdateColumn:
// no hooks, no updated_at), so no object shows as changed since the last
// push because of it.
func BackfillWorkspaceIDs(db *gorm.DB) error {
	def, err := GetDefaultWorkspace(db)
	if err != nil {
		return fmt.Errorf("backfill workspace_id: %w", err)
	}
	for _, m := range WorkspaceStampedModels() {
		if err := db.Model(m).Unscoped().Where("workspace_id = ?", 0).
			UpdateColumn("workspace_id", def.ID).Error; err != nil {
			return fmt.Errorf("backfill workspace_id on %T: %w", m, err)
		}
	}
	return nil
}

// KeepWorkspaceID puts original's workspace_id back in modified (an object
// hook's result): moving an object between Workspaces is an authorized
// API operation, never a plugin's side effect. Objects without the stamp
// are left alone.
func KeepWorkspaceID(original, modified any) {
	switch m := modified.(type) {
	case *LLM:
		if o, ok := original.(*LLM); ok && o != nil && m != nil {
			m.WorkspaceID = o.WorkspaceID
		}
	case *Tool:
		if o, ok := original.(*Tool); ok && o != nil && m != nil {
			m.WorkspaceID = o.WorkspaceID
		}
	case *Datasource:
		if o, ok := original.(*Datasource); ok && o != nil && m != nil {
			m.WorkspaceID = o.WorkspaceID
		}
	}
}
