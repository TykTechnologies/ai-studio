package services

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ErrWorkspaceNotFound is returned when a Workspace id names no Workspace.
// It is the models (wsstamp) sentinel, so the stamped models' create hooks
// and this package report the same error.
var ErrWorkspaceNotFound = models.ErrWorkspaceNotFound

// DefaultWorkspaceID returns the id of the Default workspace, loaded once
// per process. The Default is created at start-up under the migration lock
// and can never be deleted or replaced, so the id never changes and every
// replica reads the same value. A failed load is not cached.
func (s *Service) DefaultWorkspaceID() (uint, error) {
	if id := s.defaultWorkspaceID.Load(); id != 0 {
		return uint(id), nil
	}
	w, err := models.GetDefaultWorkspace(s.DB)
	if err != nil {
		return 0, fmt.Errorf("load the default workspace: %w", err)
	}
	s.defaultWorkspaceID.Store(uint64(w.ID))
	return w.ID, nil
}

// WorkspaceExists reports whether id names a Workspace.
func (s *Service) WorkspaceExists(id uint) (bool, error) {
	if id == 0 {
		return false, nil
	}
	var n int64
	if err := s.DB.Model(&models.Workspace{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// WorkspaceNames returns the names of the given Workspaces, read in one
// query; ids that name no Workspace are absent. On error it returns what
// it has (nothing) so callers show ids without names.
func (s *Service) WorkspaceNames(ctx context.Context, ids []uint) map[uint]string {
	names := map[uint]string{}
	if len(ids) == 0 {
		return names
	}
	var rows []models.Workspace
	if err := s.DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return names
	}
	for _, w := range rows {
		names[w.ID] = w.Name
	}
	return names
}

// LockWorkspaceForInsert checks, inside tx, that the Workspace exists and
// holds it until tx ends (FOR SHARE on Postgres; SQLite serializes writes),
// so an object or binding created in tx can never land in a Workspace that
// a concurrent delete (on any replica) is removing. Call it in the same
// transaction as the insert. ErrWorkspaceNotFound when it does not exist.
func LockWorkspaceForInsert(tx *gorm.DB, id uint) error {
	return models.LockWorkspaceForInsert(tx, id)
}
