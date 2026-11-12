package services

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sizeLimitedEdgeDB is an edge database on one connection that can be
// stopped from growing: limit(true) sets SQLite's page limit to the pages
// it has now, limit(false) lifts it. Writes that need new pages then fail
// with "database or disk is full", the error of a full volume. (On a volume
// that is really full, writes that replace rows in place fail too:
// TestFullDisk_WhatAnEdgeCanStillStore shows that, and needs a filesystem
// of its own.)
func sizeLimitedEdgeDB(t *testing.T) (db *gorm.DB, limit func(on bool)) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=2000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1) // the page limit belongs to the connection
	t.Cleanup(func() { database.Close(db) })
	require.NoError(t, database.Migrate(db))
	return db, func(on bool) {
		t.Helper()
		pages := 1 << 30
		if on {
			require.NoError(t, db.Raw("PRAGMA page_count").Scan(&pages).Error)
		}
		var got int
		require.NoError(t, db.Raw(fmt.Sprintf("PRAGMA max_page_count = %d", pages)).Scan(&got).Error)
	}
}

func manyLLMs(n int, at time.Time) []*pb.LLMConfig {
	var llms []*pb.LLMConfig
	for i := 1; i <= n; i++ {
		llms = append(llms, syncLLM(uint32(i), fmt.Sprintf("llm-%d-%s", i, strings.Repeat("x", 200)), at.Add(-time.Hour)))
	}
	return llms
}

// An edge whose database has no room left cannot apply a configuration it
// has no room for. The configuration and the snapshot the edge would start
// from are written in one transaction, so neither is: the edge keeps
// serving the configuration it had, its heartbeats go on reporting that one
// (the control plane shows the edge as not in sync and the push as failed),
// and the stored snapshot is still the one it runs. The condition is
// visible while it lasts and clears when a configuration is stored again.
func TestDatabaseFull_ConfigurationIsNotAppliedNorReportedAsApplied(t *testing.T) {
	db, limit := sizeLimitedEdgeDB(t)
	require.NoError(t, PersistAppliedSnapshots(db, snapshotTestKey))
	t.Cleanup(func() { _ = database.NoteWrite(database.WriteConfiguration, nil) })
	at := time.Now().UTC().Truncate(time.Second)
	svc := NewEdgeSyncService(db, "ns")
	running := strings.Repeat("a", 64)
	_, err := svc.SyncConfigurationReport(richSnapshot(at, running))
	require.NoError(t, err)

	limit(true)
	pushed := snapshotAt(at.Add(time.Minute), strings.Repeat("b", 64), manyLLMs(3000, at), []*pb.AppConfig{syncApp(1, []uint32{1}, at.Add(-time.Hour))})
	_, err = svc.SyncConfigurationReport(pushed)
	require.Error(t, err)
	assert.True(t, database.IsStorageFull(err), "the push fails with the reason: %v", err)

	reported, _, ok := svc.ReportedConfig()
	require.True(t, ok)
	assert.Equal(t, running, reported, "heartbeats report the configuration the edge runs, not the one it could not store")
	assert.Equal(t, running, storedRow(t, db).Checksum, "what the edge would start from is what it runs")
	var llms int64
	require.NoError(t, db.Model(&database.LLM{}).Count(&llms).Error)
	assert.EqualValues(t, 1, llms, "nothing of the new configuration is in the tables")

	full, since, failing := database.StorageStatus()
	assert.True(t, full)
	assert.WithinDuration(t, time.Now(), since, time.Minute)
	assert.Contains(t, failing, database.WriteConfiguration)

	// Room again (the volume was enlarged, old analytics were removed): the
	// same push is applied, reported and stored, and the condition clears.
	limit(false)
	_, err = svc.SyncConfigurationReport(pushed)
	require.NoError(t, err)
	reported, _, _ = svc.ReportedConfig()
	assert.Equal(t, strings.Repeat("b", 64), reported)
	assert.Equal(t, strings.Repeat("b", 64), storedRow(t, db).Checksum)
	_, _, failing = database.StorageStatus()
	assert.NotContains(t, failing, database.WriteConfiguration)
}

// Budget blocks the control plane sends are enforced from memory whether or
// not they can be stored. On a database with no room left they are not
// stored: after a restart the edge would enforce the set it last stored.
// That is reported once through the storage status, not at every sync.
func TestDatabaseFull_BudgetBlocksAreEnforcedButNotStored(t *testing.T) {
	db, limit := sizeLimitedEdgeDB(t)
	t.Cleanup(func() {
		_ = database.NoteWrite(database.WriteBudgetBlocks, nil)
		_ = database.NoteWrite(database.WriteBudgetSyncPosition, nil)
	})
	h := NewBudgetSyncHandler(db)
	sync := func(seq uint64, blocks map[uint32]string) {
		payload, err := json.Marshal(BudgetSyncPayload{SequenceNumber: seq, ControlTimestamp: time.Now(), Blocks: blocks, BlocksIncluded: true})
		require.NoError(t, err)
		h.HandleBudgetSync(eventbridge.Event{Topic: BudgetSyncTopic, Payload: payload})
	}
	sync(1, map[uint32]string{1: "over budget"})
	var stored int64
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&stored).Error)
	require.EqualValues(t, 1, stored)

	limit(true)
	many := map[uint32]string{}
	for i := uint32(1); i <= 5000; i++ {
		many[i] = "over budget: " + strings.Repeat("r", 200)
	}
	sync(2, many)
	sync(3, many)

	_, enforced := h.budgetBlocks().Reason(4999)
	assert.True(t, enforced, "the new blocks are enforced from memory")
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&stored).Error)
	assert.Less(t, stored, int64(5000), "they could not all be stored")
	_, _, failing := database.StorageStatus()
	assert.Contains(t, failing, database.WriteBudgetBlocks)

	limit(false)
	sync(4, many)
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&stored).Error)
	assert.EqualValues(t, 5000, stored, "the next sync stores what the failed ones could not")
	_, _, failing = database.StorageStatus()
	assert.NotContains(t, failing, database.WriteBudgetBlocks)
}
