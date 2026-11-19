// Package wsstamp keeps an object's Workspace stamp (workspace_id) out of
// ordinary updates. It is its own package so that models and secrets (which
// models imports) share it.
package wsstamp

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
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

type contextKey struct{}

// WithWorkspace returns a context carrying the Workspace new stamped objects
// created through a statement with it land in, when their WorkspaceID is
// not set otherwise (OnCreate). Handlers use it for services that create
// on a connection the caller passes (db.WithContext).
func WithWorkspace(ctx context.Context, workspaceID uint) context.Context {
	return context.WithValue(ctx, contextKey{}, workspaceID)
}

// FromContext reads WithWorkspace's Workspace.
func FromContext(ctx context.Context) (uint, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(contextKey{}).(uint)
	return id, ok && id != 0
}

// ErrWorkspaceNotFound is returned when a Workspace an insert targets does
// not exist (LockForInsert).
var ErrWorkspaceNotFound = errors.New("workspace not found")

// LockForInsert checks, inside tx, that the Workspace exists and holds it
// until tx ends (FOR SHARE on Postgres; SQLite serializes writes), so an
// object or binding created in tx can never land in a Workspace that a
// concurrent delete (on any replica) is removing. ErrWorkspaceNotFound when
// it does not exist.
func LockForInsert(tx *gorm.DB, id uint) error {
	if id == 0 {
		return ErrWorkspaceNotFound
	}
	q := tx.Session(&gorm.Session{NewDB: true}).Table("workspaces").Where("id = ?", id)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var ids []uint
	if err := q.Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// OnCreate is every stamped model's BeforeCreate hook: a new object with no
// Workspace takes the one its statement's context carries (WithWorkspace),
// and an object landing in a Workspace locks it for the insert's
// transaction (LockForInsert). An object with neither is written with 0,
// which reads as the Default workspace.
func OnCreate(tx *gorm.DB, workspaceID *uint) error {
	if *workspaceID == 0 {
		if id, ok := FromContext(tx.Statement.Context); ok {
			*workspaceID = id
		}
	}
	if *workspaceID == 0 {
		return nil
	}
	return LockForInsert(tx, *workspaceID)
}
