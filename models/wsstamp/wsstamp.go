// Package wsstamp keeps an object's Workspace stamp (workspace_id) out of
// ordinary updates. It is its own package so that models and secrets (which
// models imports) share it.
package wsstamp

import (
	"errors"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Column is the stamp's column.
const Column = "workspace_id"

// ErrImplicitMove is returned for an update that names workspace_id in its
// values (Updates(map{"workspace_id": ...})) without selecting it: moving
// an object is done deliberately, with Select("workspace_id") or
// UpdateColumn, so a stray one fails loudly instead of being dropped.
var ErrImplicitMove = errors.New(`workspace_id is only written by an explicit move: use Select("workspace_id") or UpdateColumn`)

// KeepOnUpdate is called from a stamped model's BeforeUpdate hook: unless
// the statement selects workspace_id by name (Select("workspace_id"), the
// form an authorized move uses), the update leaves the column alone. A
// handler that rebuilds a struct from a request and saves it, whose
// WorkspaceID is 0, therefore cannot move the object to the Default
// workspace by accident (Select("*"), what Save uses, is not naming it).
// A map update naming the column without selecting it is refused with
// ErrImplicitMove. UpdateColumn runs no hooks, so the start-up backfill and
// moves written with it are unaffected.
func KeepOnUpdate(tx *gorm.DB) error {
	for _, s := range tx.Statement.Selects {
		if s == Column || s == "WorkspaceID" {
			return nil
		}
	}
	if values, ok := tx.Statement.Dest.(map[string]interface{}); ok {
		for k := range values {
			// gorm takes the column or the field name as a map key.
			if k == Column || k == "WorkspaceID" {
				return ErrImplicitMove
			}
		}
	}
	tx.Statement.Omits = append(tx.Statement.Omits, Column)
	return nil
}
