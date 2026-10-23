package studio

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// A configuration with invalid security settings refuses to start.
func TestNewRefusesInvalidConfiguration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "studio.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	conf := &config.AppConf{ConfigErrors: []error{errors.New(`invalid AI_STUDIO_OCI_REQUIRE_SIGNATURE value "ture"`)}}

	_, err = New(Options{Config: conf, DB: db})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE")
	assert.False(t, running.Load(), "a refused start leaves the process free for another instance")

	_, err = NewControlPlane(ControlPlaneOptions{Config: conf, DB: db})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE")
	assert.False(t, running.Load())
}
