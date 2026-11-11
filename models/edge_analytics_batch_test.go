package models

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The key is fixed-length whatever the edge sends, and each part counts:
// another edge, another run of the same edge (boot ID) and another pulse
// (sequence number) are different batches.
func TestEdgeAnalyticsBatchKey(t *testing.T) {
	key := EdgeAnalyticsBatchKey("edge-1", "boot-1", 7)
	assert.Len(t, key, 64)
	assert.Equal(t, key, EdgeAnalyticsBatchKey("edge-1", "boot-1", 7))

	for name, other := range map[string]string{
		"another edge":               EdgeAnalyticsBatchKey("edge-2", "boot-1", 7),
		"another run":                EdgeAnalyticsBatchKey("edge-1", "boot-2", 7),
		"another pulse":              EdgeAnalyticsBatchKey("edge-1", "boot-1", 8),
		"parts are not run together": EdgeAnalyticsBatchKey("edge-1b", "oot-1", 7),
		"nor the number":             EdgeAnalyticsBatchKey("edge-1", "boot-17", 0),
	} {
		assert.NotEqual(t, key, other, name)
		assert.Len(t, other, 64)
	}
	long := make([]byte, 10000)
	for i := range long {
		long[i] = 'x'
	}
	assert.Len(t, EdgeAnalyticsBatchKey(string(long), string(long), 1), 64, "an edge cannot make the key longer than its column")
}

func TestClaimAndPruneEdgeAnalyticsBatches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&EdgeAnalyticsBatch{}))
	now := time.Now()

	claimed, err := ClaimEdgeAnalyticsBatch(db, "k1", "edge-1", now)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = ClaimEdgeAnalyticsBatch(db, "k1", "edge-1", now.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, claimed, "a key already recorded is not claimed again")

	var row EdgeAnalyticsBatch
	require.NoError(t, db.First(&row, "key = ?", "k1").Error)
	assert.Equal(t, "edge-1", row.EdgeID)
	assert.WithinDuration(t, now, row.AppliedAt, time.Second, "the first claim's time is kept")

	// A claim rolled back with its transaction leaves the key free.
	require.Error(t, db.Transaction(func(tx *gorm.DB) error {
		claimed, err := ClaimEdgeAnalyticsBatch(tx, "k2", "edge-1", now)
		require.NoError(t, err)
		require.True(t, claimed)
		return assert.AnError
	}))
	claimed, err = ClaimEdgeAnalyticsBatch(db, "k2", "edge-1", now)
	require.NoError(t, err)
	assert.True(t, claimed)

	// Pruning removes records past the retention and keeps the rest.
	_, err = ClaimEdgeAnalyticsBatch(db, "old", "edge-1", now.Add(-EdgeAnalyticsBatchRetention-time.Minute))
	require.NoError(t, err)
	_, err = ClaimEdgeAnalyticsBatch(db, "kept", "edge-1", now.Add(-EdgeAnalyticsBatchRetention+time.Hour))
	require.NoError(t, err)
	n, err := PruneEdgeAnalyticsBatches(db, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	n, err = PruneEdgeAnalyticsBatches(db, now)
	require.NoError(t, err)
	assert.Zero(t, n, "pruning again deletes nothing")
	var keys []string
	require.NoError(t, db.Model(&EdgeAnalyticsBatch{}).Order("key").Pluck("key", &keys).Error)
	assert.Equal(t, []string{"k1", "k2", "kept"}, keys)
}
