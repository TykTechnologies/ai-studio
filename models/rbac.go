package models

import (
	"database/sql/driver"
	"time"
)

// StringList is a JSON-encoded []string column. Nil scans to an empty list
// so callers never see a NULL.
type StringList []string

// Scan implements sql.Scanner.
func (s *StringList) Scan(value interface{}) error {
	if value == nil {
		*s = StringList{}
		return nil
	}
	return JSONScan(value, s)
}

// Value implements driver.Valuer.
func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return JSONValue([]string{})
	}
	return JSONValue([]string(s))
}

// System role slugs seeded by the Enterprise RBAC service. System roles are
// immutable; administrators clone them to customise.
const (
	SystemRoleOwner         = "owner"
	SystemRoleAdministrator = "administrator"
	SystemRoleEditor        = "editor"
	SystemRoleViewer        = "viewer"
	SystemRoleAuditor       = "auditor"
)

// Role binding subject types.
const (
	RoleBindingSubjectUser  = "user"
	RoleBindingSubjectGroup = "group"
)

// Role is a named bundle of permissions. Permissions are stored as the
// catalogue strings ("llms:read"); the single wildcard "*" is reserved for
// the Owner and Administrator system roles.
//
// The table exists in Community Edition but is unused there.
type Role struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Slug        string     `gorm:"size:64;uniqueIndex:idx_roles_slug" json:"slug"`
	Name        string     `gorm:"size:128;uniqueIndex:idx_roles_name" json:"name"`
	Description string     `gorm:"size:1024" json:"description"`
	IsSystem    bool       `json:"is_system"`
	Permissions StringList `gorm:"type:text" json:"permissions"`
	CreatedBy   uint       `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName pins the table so the slug/name indexes are stable.
func (Role) TableName() string { return "roles" }

// IsWildcard reports whether the role grants full administrator access.
func (r *Role) IsWildcard() bool {
	for _, p := range r.Permissions {
		if p == "*" {
			return true
		}
	}
	return false
}

// RoleBinding assigns a role to a subject (a user or a group). ScopeType and
// ScopeID are reserved for scoped bindings (namespace, catalogue); an empty
// string means the binding is global. They are empty strings rather than
// NULLs so the composite unique index behaves the same on Postgres and SQLite.
type RoleBinding struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SubjectType string    `gorm:"size:16;uniqueIndex:idx_role_bindings_unique,priority:1;index:idx_role_bindings_subject,priority:1" json:"subject_type"`
	SubjectID   uint      `gorm:"uniqueIndex:idx_role_bindings_unique,priority:2;index:idx_role_bindings_subject,priority:2" json:"subject_id"`
	RoleID      uint      `gorm:"uniqueIndex:idx_role_bindings_unique,priority:3;index:idx_role_bindings_role" json:"role_id"`
	ScopeType   string    `gorm:"size:32;uniqueIndex:idx_role_bindings_unique,priority:4" json:"scope_type"`
	ScopeID     string    `gorm:"size:64;uniqueIndex:idx_role_bindings_unique,priority:5" json:"scope_id"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	Role        *Role     `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// TableName pins the table name.
func (RoleBinding) TableName() string { return "role_bindings" }
