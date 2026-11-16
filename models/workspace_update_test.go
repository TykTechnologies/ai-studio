package models

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models/wsstamp"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/callbacks"
)

// Every stamped model carries the BeforeUpdate guard, so a new stamped
// model cannot be added without it.
func TestWorkspaceStampedModels_HaveTheUpdateGuard(t *testing.T) {
	for _, m := range WorkspaceStampedModels() {
		_, ok := m.(callbacks.BeforeUpdateInterface)
		assert.True(t, ok, "%T has BeforeUpdate (wsstamp.KeepOnUpdate)", m)
	}
}

func storedWorkspaceID(t *testing.T, db *gorm.DB, m interface{}) uint {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(m))
	pk := reflect.ValueOf(m).Elem().FieldByName("ID").Interface()
	var ids []uint
	require.NoError(t, db.Table(stmt.Schema.Table).Where("id = ?", pk).Pluck("workspace_id", &ids).Error)
	require.Len(t, ids, 1)
	return ids[0]
}

// A struct rebuilt from a request (WorkspaceID 0) and saved, or an
// Updates(struct) carrying another WorkspaceID, leaves the stamp alone; a
// map update naming it is refused; a move names the column (Select) or
// writes it directly (UpdateColumn).
func TestWorkspaceID_OnlyAnExplicitUpdateChangesIt(t *testing.T) {
	db := setupTestDB(t)
	w1 := &Workspace{Name: "W1"}
	w2 := &Workspace{Name: "W2"}
	require.NoError(t, db.Create(w1).Error)
	require.NoError(t, db.Create(w2).Error)

	for _, row := range createStampedFixtures(t, db, 7) {
		name := reflect.TypeOf(row).Elem().Name()
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.Model(row).UpdateColumn("workspace_id", w1.ID).Error)
			require.Equal(t, w1.ID, storedWorkspaceID(t, db, row))

			// Save of a rebuilt struct: same id and required fields, no stamp.
			rebuilt := reflect.New(reflect.TypeOf(row).Elem())
			rebuilt.Elem().Set(reflect.ValueOf(row).Elem())
			rebuilt.Elem().FieldByName("WorkspaceID").SetUint(0)
			require.NoError(t, db.Session(&gorm.Session{SkipHooks: false}).Save(rebuilt.Interface()).Error)
			assert.Equal(t, w1.ID, storedWorkspaceID(t, db, row), "Save keeps the stamp")

			// Updates with another WorkspaceID, struct and map forms.
			other := reflect.New(reflect.TypeOf(row).Elem())
			other.Elem().FieldByName("WorkspaceID").SetUint(uint64(w2.ID))
			require.NoError(t, db.Model(row).Updates(other.Interface()).Error)
			assert.Equal(t, w1.ID, storedWorkspaceID(t, db, row), "Updates(struct) keeps the stamp")
			err := db.Model(row).Updates(map[string]interface{}{"workspace_id": w2.ID}).Error
			assert.ErrorIs(t, err, wsstamp.ErrImplicitMove, "a map update naming the stamp is refused loudly")
			err = db.Model(row).Updates(map[string]interface{}{"WorkspaceID": w2.ID}).Error
			assert.ErrorIs(t, err, wsstamp.ErrImplicitMove, "by field name too")
			assert.Equal(t, w1.ID, storedWorkspaceID(t, db, row))

			// An explicit move.
			require.NoError(t, db.Model(row).Select("workspace_id").Updates(map[string]interface{}{"workspace_id": w2.ID}).Error)
			assert.Equal(t, w2.ID, storedWorkspaceID(t, db, row), "Select(workspace_id) moves it")
			require.NoError(t, db.Model(row).UpdateColumn("workspace_id", w1.ID).Error)
			assert.Equal(t, w1.ID, storedWorkspaceID(t, db, row), "UpdateColumn moves it")
		})
	}
}

// The two router update paths that rebuild the router from the request
// (midsommar-clean-oztk.42) keep its Workspace.
func TestRouterUpdates_KeepWorkspace(t *testing.T) {
	db := setupTestDB(t)
	w1 := &Workspace{Name: "W1"}
	require.NoError(t, db.Create(w1).Error)

	mr := &ModelRouter{Name: "Router", Slug: "router"}
	require.NoError(t, db.Create(mr).Error)
	require.NoError(t, db.Model(mr).UpdateColumn("workspace_id", w1.ID).Error)
	rebuiltMR := &ModelRouter{ID: mr.ID, Name: "Router renamed", Slug: "router"}
	require.NoError(t, rebuiltMR.Update(db))
	var gotMR ModelRouter
	require.NoError(t, db.First(&gotMR, mr.ID).Error)
	assert.Equal(t, "Router renamed", gotMR.Name)
	assert.Equal(t, w1.ID, gotMR.WorkspaceID)

	sr := &SemanticRouter{Name: "Semantic", Slug: "semantic"}
	require.NoError(t, db.Create(sr).Error)
	require.NoError(t, db.Model(sr).UpdateColumn("workspace_id", w1.ID).Error)
	rebuiltSR := &SemanticRouter{ID: sr.ID, Name: "Semantic renamed", Slug: "semantic"}
	require.NoError(t, rebuiltSR.Update(db))
	var gotSR SemanticRouter
	require.NoError(t, db.First(&gotSR, sr.ID).Error)
	assert.Equal(t, "Semantic renamed", gotSR.Name)
	assert.Equal(t, w1.ID, gotSR.WorkspaceID)
}
