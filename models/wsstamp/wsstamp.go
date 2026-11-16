// Package wsstamp keeps an object's Workspace stamp (workspace_id) out of
// ordinary updates. It is its own package so that models and secrets (which
// models imports) share it.
package wsstamp

import "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

// Column is the stamp's column.
const Column = "workspace_id"

// KeepOnUpdate is called from a stamped model's BeforeUpdate hook: unless
// the statement selects workspace_id by name (Select("workspace_id"), the
// form an authorized move uses), the update leaves the column alone. A
// handler that rebuilds a struct from a request and saves it, whose
// WorkspaceID is 0, therefore cannot move the object to the Default
// workspace by accident. UpdateColumn runs no hooks, so the start-up
// backfill and moves written with it are unaffected. Select("*") (what
// Save uses) is not naming it.
func KeepOnUpdate(tx *gorm.DB) {
	for _, s := range tx.Statement.Selects {
		if s == Column || s == "WorkspaceID" {
			return
		}
	}
	tx.Statement.Omits = append(tx.Statement.Omits, Column)
}
