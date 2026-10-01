package studio

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

func TestNewControlPlaneRequiresConfigAndDB(t *testing.T) {
	_, err := NewControlPlane(ControlPlaneOptions{})
	require.Error(t, err)
	assert.False(t, running.Load(), "a refused start leaves the process free for another instance")
}

// A headless control plane shares a full Studio's database and coordinates
// with it through Postgres; SQLite serves one process.
func TestNewControlPlaneRefusesSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "studio.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	_, err = NewControlPlane(ControlPlaneOptions{Config: &config.AppConf{}, DB: db})
	assert.ErrorIs(t, err, ErrControlPlaneNeedsPostgres)
	assert.False(t, running.Load())
}

// A replica without a leader lease never leads. pkg/replicas takes no
// backend at all to mean the only replica, which leads, so the headless
// control plane's backend must say so explicitly.
func TestReplicaWithoutALeaseNeverLeads(t *testing.T) {
	c := &clusterParts{}
	b := replicaBackend{c: c}
	assert.False(t, b.IsLeader())

	replicas.SetBackend(b)
	t.Cleanup(func() { replicas.SetBackend(nil) })
	assert.False(t, replicas.IsLeader())
}

func TestDefaultControlPlaneLogger(t *testing.T) {
	assert.Equal(t, zerolog.InfoLevel, defaultControlPlaneLogger("").GetLevel())
	assert.Equal(t, zerolog.InfoLevel, defaultControlPlaneLogger("nonsense").GetLevel())
	assert.Equal(t, zerolog.DebugLevel, defaultControlPlaneLogger("DEBUG").GetLevel())
	assert.Equal(t, zerolog.WarnLevel, defaultControlPlaneLogger(" warn ").GetLevel())
}
