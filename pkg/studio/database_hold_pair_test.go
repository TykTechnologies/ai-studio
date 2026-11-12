package studio

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Statements across a failover (bead midsommar-clean-4x5y.6), on real pairs;
// skipped under -short.

func openHeldDB(t *testing.T, pair *containers.PostgresPair, window time.Duration) *gorm.DB {
	t.Helper()
	conf := failoverConf(pair)
	conf.DatabaseWriteRetryWindow = window
	db, err := OpenDatabase(conf)
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			pool.Close()
		}
	})
	_, held := db.ConnPool.(*dbhold.Pool)
	require.True(t, held, "the pool holds statements")
	require.NoError(t, db.AutoMigrate(&failoverNote{}))
	return db
}

// A write issued while the pair has no primary is held, and is applied once
// (exactly once) when the standby is promoted inside the window. So is a
// transaction begun meanwhile.
func TestFailover_WriteIsHeldUntilPromotion(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx := context.Background()
	db := openHeldDB(t, pair, 30*time.Second)
	require.NoError(t, pair.StopPrimary(ctx))

	promoted := make(chan error, 1)
	go func() {
		time.Sleep(3 * time.Second)
		promoted <- pair.Promote(ctx)
	}()
	started := time.Now()
	require.NoError(t, db.Create(&failoverNote{Note: "held"}).Error)
	assert.GreaterOrEqual(t, time.Since(started), 2*time.Second, "it waited for the promotion")
	require.NoError(t, <-promoted)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&failoverNote{Note: "in a transaction"}).Error
	}))
	var n int64
	require.NoError(t, db.Model(&failoverNote{}).Where("note = ?", "held").Count(&n).Error)
	assert.EqualValues(t, 1, n, "applied once")
}

// With no promotion inside the window the statement fails as unavailable,
// once the window has passed, and nothing is written.
func TestFailover_WriteGivesUpAfterTheWindow(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	db := openHeldDB(t, pair, 2*time.Second)
	require.NoError(t, pair.StopPrimary(context.Background()))

	started := time.Now()
	err := db.Create(&failoverNote{Note: "never"}).Error
	took := time.Since(started)
	require.Error(t, err)
	var unavailable *dbhold.UnavailableError
	assert.True(t, errors.As(err, &unavailable), "%v", err)
	assert.True(t, dbhold.IsUnavailable(err))
	assert.GreaterOrEqual(t, took, 2*time.Second)
	assert.Less(t, took, 10*time.Second)
}

// A cancelled request stops waiting.
func TestFailover_HeldWriteStopsWithTheRequest(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	db := openHeldDB(t, pair, time.Minute)
	require.NoError(t, pair.StopPrimary(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	err := db.WithContext(ctx).Create(&failoverNote{Note: "cancelled"}).Error
	require.Error(t, err)
	assert.Less(t, time.Since(started), 10*time.Second)
}

// A statement inside a transaction that was open when the primary died is
// not retried: it fails at once, the transaction leaves nothing, and the
// error counts as unavailable. A statement error that is not a failover's
// (a unique violation) is returned at once.
func TestFailover_NoRetryInsideATransactionOrForOtherErrors(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	ctx := context.Background()
	db := openHeldDB(t, pair, 30*time.Second)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX failover_notes_note ON failover_notes (note)").Error)
	require.NoError(t, db.Create(&failoverNote{Note: "unique"}).Error)

	started := time.Now()
	err := db.Create(&failoverNote{Note: "unique"}).Error
	require.Error(t, err)
	assert.False(t, dbhold.IsUnavailable(err), "%v", err)
	assert.Less(t, time.Since(started), time.Second, "not held")

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&failoverNote{Note: "partial"}).Error; err != nil {
			return err
		}
		require.NoError(t, pair.StopPrimary(ctx))
		started = time.Now()
		return tx.Create(&failoverNote{Note: "after the loss"}).Error
	})
	require.Error(t, err)
	assert.True(t, dbhold.IsUnavailable(err), "%v", err)
	assert.Less(t, time.Since(started), 5*time.Second, "not held inside a transaction")

	require.NoError(t, pair.Promote(ctx))
	var n int64
	require.NoError(t, db.Model(&failoverNote{}).Where("note IN ?", []string{"partial", "after the loss"}).Count(&n).Error)
	assert.Zero(t, n, "no partial rows")
}
