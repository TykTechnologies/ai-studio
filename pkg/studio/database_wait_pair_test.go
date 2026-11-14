package studio

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Start-up while the database is failing over (bead midsommar-clean-4x5y.5):
// OpenDatabase waits, up to DatabaseStartupTimeout, for a server that takes
// writes. On real pairs (containers.StartPostgresPair); skipped under -short.

// A replica that starts while the pair has no primary waits, and is ready
// once the standby is promoted inside the timeout: the database it returns
// takes writes, so New's migrations can run.
func TestFailover_StartupWaitsForAPromotedStandby(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	ctx := context.Background()
	sink := captureLog(t)
	require.NoError(t, pair.StopPrimary(ctx))

	conf := failoverConf(pair)
	conf.DatabaseStartupTimeout = time.Minute
	type result struct {
		db  *gorm.DB
		err error
	}
	done := make(chan result, 1)
	go func() {
		db, err := OpenDatabase(conf)
		done <- result{db, err}
	}()

	select {
	case r := <-done:
		t.Fatalf("OpenDatabase returned with no primary: %v", r.err)
	case <-time.After(3 * time.Second):
	}
	require.NoError(t, pair.Promote(ctx))

	var r result
	select {
	case r = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("OpenDatabase did not return after the promotion")
	}
	require.NoError(t, r.err)
	t.Cleanup(func() {
		if pool, err := r.db.DB(); err == nil {
			pool.Close()
		}
	})
	require.NoError(t, r.db.AutoMigrate(&failoverNote{}))
	require.NoError(t, r.db.Create(&failoverNote{Note: "after start-up"}).Error)

	log := sink.String()
	assert.Equal(t, 1, strings.Count(log, "Waiting for the database"), "one line per distinct reason, not one per attempt:\n%s", log)
	assert.Contains(t, log, "The database is reachable and writable, after")
}

// With no promotion inside the timeout the start fails, with a message that
// names the setting, once the timeout has passed (not at the first attempt).
func TestFailover_StartupGivesUpAfterTheTimeout(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	require.NoError(t, pair.StopPrimary(context.Background()))

	conf := failoverConf(pair)
	conf.DatabaseStartupTimeout = 3 * time.Second
	started := time.Now()
	_, err := OpenDatabase(conf)
	took := time.Since(started)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no writable database within DATABASE_STARTUP_TIMEOUT (3s")
	assert.GreaterOrEqual(t, took, 3*time.Second)
	assert.Less(t, took, 20*time.Second)
}

// A start against a hot standby alone (no target_session_attrs, so the
// connection succeeds) never returns the standby: OpenDatabase fails with
// the read-only reason, so New's migrations are never attempted on it. With
// a timeout of 0 it fails at once.
func TestFailover_StartupDoesNotSettleOnAStandby(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	for _, timeout := range []time.Duration{0, 2 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			conf := failoverConf(pair)
			conf.DatabaseURL = pair.Standby().DSN()
			conf.DatabaseStartupTimeout = timeout
			started := time.Now()
			db, err := OpenDatabase(conf)
			require.Error(t, err)
			assert.Nil(t, db)
			assert.True(t, errors.Is(err, errReadOnlyServer), "%v", err)
			assert.Contains(t, err.Error(), "read-only")
			assert.GreaterOrEqual(t, time.Since(started), timeout)
		})
	}
}

func captureLog(t *testing.T) *lockedLog {
	t.Helper()
	prev := logger.Log
	t.Cleanup(func() { logger.Use(prev) })
	sink := &lockedLog{}
	logger.Use(zerolog.New(sink).Level(zerolog.DebugLevel))
	return sink
}
