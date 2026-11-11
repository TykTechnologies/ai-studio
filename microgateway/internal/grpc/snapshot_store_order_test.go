package grpc

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An edge with the default settings (EDGE_START_FROM_LAST_KNOWN_GOOD off)
// registers with its control plane before its services exist, and the
// registration is its first control contact. The database's snapshot store,
// whose key authenticates the recorded contact, is set up before that
// registration: the first contact is recorded (nothing else sets the store
// up in this test, so a recorded contact means the start did), and the edge
// has no occasion to report a database without a store, which on an edge is
// an error.
func TestEdgeStartSetsUpTheSnapshotStoreBeforeItsFirstContact(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db)) // no store set up: the start does that
	t.Cleanup(func() { database.Close(db) })

	cp := &configControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("live"))
	addr, _ := cp.serveConfig(t, "127.0.0.1:0")
	client := lkgClient(t, db, func(hs *config.HubSpokeConfig) { hs.StartFromLastKnownGood = false }, addr)

	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.Nil(t, lkg)

	at, ok, err := services.LastControlContact(db)
	require.NoError(t, err)
	require.True(t, ok, "the registration, the edge's first contact, is recorded")
	assert.WithinDuration(t, time.Now(), at, 5*time.Second)
}
