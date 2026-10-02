package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A governance plugin flags Apps while Studio writes elsewhere (audit
// records, plugin KV, webhook events). On SQLite a transaction that reads and
// then writes fails at once with "database is locked" if another connection
// wrote in between, so the flag update must take the write lock first and
// wait its turn.
func TestSetAppGovernanceStateWaitsForAConcurrentWriter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "studio.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	svc := NewService(db)
	app := &models.App{Name: "Support bot", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	other := &models.App{Name: "Other"}
	require.NoError(t, db.Create(other).Error)

	// Another connection holds a write transaction for a moment.
	holding := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&models.App{}).Where("id = ?", other.ID).Update("description", "busy").Error; err != nil {
				return err
			}
			close(holding)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()
	<-holding

	_, diff, err := svc.SetAppGovernanceState(app.ID, nil, map[string]string{"ownerless": "2026-10-02"}, "owner left", "plugin:1")
	require.NoError(t, err, "waits for the other writer instead of failing")
	assert.Contains(t, diff, "flag:ownerless")
	require.NoError(t, <-done)

	got, err := svc.GetAppByID(app.ID)
	require.NoError(t, err)
	flags, _ := got.Metadata[AppGovernanceFlagsKey].(map[string]interface{})
	assert.Contains(t, flags, "ownerless")
}

// Governance flags belong to SetAppGovernanceState: App edits through the
// generic paths can neither erase nor forge them.
func TestAppEditsKeepGovernanceFlags(t *testing.T) {
	svc, db := setupAppTest(t)
	user := createTestAppUser(t, svc, "owner@test.com", "Owner")
	app, err := svc.CreateApp("Support bot", "", user.ID, nil, nil, nil, nil, nil,
		map[string]interface{}{"team": "ops", AppGovernanceFlagsKey: map[string]interface{}{"forged": map[string]interface{}{"value": "x"}}})
	require.NoError(t, err)
	assert.NotContains(t, app.Metadata, AppGovernanceFlagsKey, "create drops supplied flags")

	_, _, err = svc.SetAppGovernanceState(app.ID, nil, map[string]string{"ownerless": "2026-10-02"}, "owner left", "plugin:1")
	require.NoError(t, err)

	// An edit that omits metadata, or sends stale metadata, keeps the flags.
	_, err = svc.UpdateApp(app.ID, "Support bot v2", "", user.ID, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	_, err = svc.UpdateApp(app.ID, "Support bot v2", "", user.ID, nil, nil, nil, nil, nil,
		map[string]interface{}{"team": "platform", AppGovernanceFlagsKey: map[string]interface{}{}})
	require.NoError(t, err)

	var stored models.App
	require.NoError(t, db.First(&stored, app.ID).Error)
	assert.Equal(t, "platform", stored.Metadata["team"])
	flags, _ := stored.Metadata[AppGovernanceFlagsKey].(map[string]interface{})
	assert.Contains(t, flags, "ownerless")
	assert.NotContains(t, flags, "forged")

	_, err = svc.PatchAppMetadata(app.ID, AppGovernanceFlagsKey, "{}", false)
	assert.ErrorIs(t, err, ErrReservedAppMetadataKey)
	_, err = svc.PatchAppMetadata(app.ID, AppGovernanceFlagsKey, "", true)
	assert.ErrorIs(t, err, ErrReservedAppMetadataKey)
}
