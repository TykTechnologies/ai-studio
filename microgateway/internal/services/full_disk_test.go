package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// fullDiskDirEnv names a directory on a small filesystem of its own (a few
// megabytes) for TestFullDisk_WhatAnEdgeCanStillStore to fill. Without it
// the test is skipped: filling a disk is not something a test does to the
// machine it runs on. See the test for how to give it one.
const fullDiskDirEnv = "MGW_TEST_FULL_DISK_DIR"

// fillDisk writes a file in dir until the filesystem has no room left, and
// returns its path.
func fillDisk(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "filler")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	defer f.Close()
	for _, chunk := range []int{1 << 20, 64 << 10, 4 << 10, 512, 1} {
		buf := make([]byte, chunk)
		for {
			if _, err := f.Write(buf); err != nil {
				require.True(t, errors.Is(err, syscall.ENOSPC), "filling the disk ended with %v, not with no space left", err)
				break
			}
		}
	}
	return path
}

// What an edge whose volume is full can still store, on a filesystem that
// really has no room (SQLite at a page limit is not the same thing: writes
// that replace rows in place still succeed there, and on a full disk they
// may not, because the write-ahead log has to grow).
//
// Run it with a directory on a small filesystem, for example in a container:
//
//	go test -c -o /tmp/services.test ./internal/services/
//	docker run --rm --tmpfs /data:size=16m -v /tmp/services.test:/t:ro \
//	    -e MGW_TEST_FULL_DISK_DIR=/data <any glibc image> /t -test.run TestFullDisk -test.v
func TestFullDisk_WhatAnEdgeCanStillStore(t *testing.T) {
	dir := os.Getenv(fullDiskDirEnv)
	if dir == "" {
		t.Skipf("%s is not set (it must name a directory on a small filesystem of its own)", fullDiskDirEnv)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "edge.db")+"?_busy_timeout=2000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() { database.Close(db) })
	require.NoError(t, database.Migrate(db))
	require.NoError(t, PersistAppliedSnapshots(db, snapshotTestKey))

	// A healthy edge: a configuration applied, a contact recorded, budget
	// blocks stored.
	at := time.Now().UTC().Truncate(time.Second)
	svc := NewEdgeSyncService(db, "ns")
	first := strings.Repeat("a", 64)
	_, err = svc.SyncConfigurationReport(richSnapshot(at, first))
	require.NoError(t, err)
	require.NoError(t, RecordControlContact(db, at))
	budgets := NewBudgetSyncHandler(db)
	sync := func(seq uint64, blocks map[uint32]string) {
		payload, err := json.Marshal(BudgetSyncPayload{SequenceNumber: seq, ControlTimestamp: time.Now(), Blocks: blocks, BlocksIncluded: true})
		require.NoError(t, err)
		budgets.HandleBudgetSync(eventbridge.Event{Topic: BudgetSyncTopic, Payload: payload})
	}
	sync(1, map[uint32]string{1: "over budget"})

	// The volume fills (on an edge: with its own copy of the analytics).
	fillDisk(t, dir)
	growth := db.Exec("CREATE TABLE filler (v text)").Error
	if growth == nil {
		growth = db.Exec("INSERT INTO filler VALUES (?)", strings.Repeat("x", 4<<20)).Error
	}
	require.Error(t, growth, "the database can still grow: the disk is not full")
	t.Logf("growth on the full disk: %v", growth)
	require.True(t, database.IsStorageFull(growth), "a full disk is recognised as one: %v", growth)

	// 1. A new configuration snapshot, about the size of the stored one.
	second := strings.Repeat("b", 64)
	_, err = svc.SyncConfigurationReport(richSnapshot(at.Add(time.Minute), second))
	t.Logf("1. apply a configuration of the same size: %v", err)
	reported, _, _ := svc.ReportedConfig()
	stored := storedRow(t, db)
	require.NotNil(t, stored)
	assert.Equal(t, stored.Checksum, reported, "the edge reports the configuration whose snapshot is stored, whichever that is")

	// 2. A much larger configuration: its snapshot cannot fit.
	var llms []*pb.LLMConfig
	for i := uint32(1); i <= 4000; i++ {
		llms = append(llms, syncLLM(i, fmt.Sprintf("llm-%d-%s", i, strings.Repeat("x", 200)), at.Add(-time.Hour)))
	}
	big := snapshotAt(at.Add(2*time.Minute), strings.Repeat("c", 64), llms, []*pb.AppConfig{syncApp(1, []uint32{1}, at.Add(-time.Hour))})
	_, err = svc.SyncConfigurationReport(big)
	t.Logf("2. apply a configuration with 4,000 LLMs (about 1 MB more): %v", err)
	reported, _, _ = svc.ReportedConfig()
	stored = storedRow(t, db)
	assert.Equal(t, stored.Checksum, reported, "a configuration whose snapshot could not be stored is not reported as applied")
	if err != nil {
		assert.NotEqual(t, strings.Repeat("c", 64), reported)
		assert.True(t, database.IsStorageFull(err), "the failure says the disk is full: %v", err)
	}

	// 3. The control contact.
	controlContactWrites.Range(func(k, _ any) bool { controlContactWrites.Delete(k); return true })
	err = RecordControlContact(db, at.Add(time.Hour))
	t.Logf("3. record the control contact: %v", err)

	// 4. Budget blocks and the budget sync's sequence number.
	sync(2, map[uint32]string{1: "over budget", 2: "over budget"})
	var blocks int64
	require.NoError(t, db.Model(&database.BudgetBlock{}).Count(&blocks).Error)
	_, enforced := budgets.budgetBlocks().Reason(2)
	t.Logf("4. budget blocks stored after a sync with 2: %d (the new block is enforced from memory: %v)", blocks, enforced)

	// 5. The edge's own analytics rows.
	err = db.Create(&database.AnalyticsEvent{RequestID: "after-full", TimeStamp: time.Now()}).Error
	t.Logf("5. one analytics row: %v", err)

	full, since, failing := database.StorageStatus()
	t.Logf("storage status: full=%v since=%s failing=%v", full, since.Format(time.RFC3339), failing)
}
