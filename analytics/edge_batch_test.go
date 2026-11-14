package analytics

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func edgeBatchSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	// A file, not ":memory:": an in-memory database belongs to one
	// connection, and a cancelled transaction discards its connection.
	// _txlock=immediate as the microgateway's own SQLite DSN has: writers
	// queue for the lock instead of failing when they upgrade a read.
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func edgeBatchHandler(t *testing.T, db *gorm.DB) *DatabaseHandler {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}, &models.ComplianceEvent{},
		&models.ToolCallRecord{}, &models.EdgeAnalyticsBatch{}))
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)
	return h
}

func testEdgeBatch(key string) *EdgeBatch {
	now := time.Now().UTC().Truncate(time.Second)
	return &EdgeBatch{
		Key:    key,
		EdgeID: "edge-1",
		ProxyLogs: []*models.ProxyLog{
			{AppID: 1, LLMID: 1, Vendor: "openai", ModelName: "gpt-4o", ResponseCode: 200, TimeStamp: now},
			{AppID: 1, LLMID: 1, Vendor: "openai", ModelName: "gpt-4o", ResponseCode: 403, TimeStamp: now},
		},
		ChatRecords: []*models.LLMChatRecord{
			{AppID: 1, LLMID: 1, Vendor: "openai", Name: "gpt-4o", TotalTokens: 30, Cost: 1500, Currency: "USD", TimeStamp: now},
		},
		ComplianceEvents: []*models.ComplianceEvent{
			{AppID: 1, FilterName: "pii", EventType: "block", Severity: "high", Metadata: `{"owner_visible":true}`, TimeStamp: now},
		},
		ToolCalls: []*models.ToolCallRecord{
			{ToolID: 3, Name: "getWeather", ExecTime: 12, TimeStamp: now},
		},
	}
}

type edgeBatchCounts struct {
	logs, records, events, calls, batches int64
	cost                                  float64
}

func countEdgeBatchRows(t *testing.T, db *gorm.DB) edgeBatchCounts {
	t.Helper()
	var c edgeBatchCounts
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&c.logs).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&c.records).Error)
	require.NoError(t, db.Model(&models.ComplianceEvent{}).Count(&c.events).Error)
	require.NoError(t, db.Model(&models.ToolCallRecord{}).Count(&c.calls).Error)
	require.NoError(t, db.Model(&models.EdgeAnalyticsBatch{}).Count(&c.batches).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost), 0)").Scan(&c.cost).Error)
	return c
}

var oneEdgeBatch = edgeBatchCounts{logs: 2, records: 1, events: 1, calls: 1, batches: 1, cost: 1500}

// A batch is stored when RecordEdgeBatch returns, with its key; the same
// batch again stores nothing.
func testEdgeBatchStoredOnce(t *testing.T, db *gorm.DB) {
	h := edgeBatchHandler(t, db)
	ctx := context.Background()

	stored, err := h.RecordEdgeBatch(ctx, testEdgeBatch("k1"))
	require.NoError(t, err)
	require.True(t, stored)
	// No waiting for a background writer: it is committed.
	assert.Equal(t, oneEdgeBatch, countEdgeBatchRows(t, db))

	var event models.ComplianceEvent
	require.NoError(t, db.First(&event).Error)
	assert.True(t, event.OwnerVisible, "owner-visible events are marked as RecordComplianceEvents marks them")

	// The edge sends it again (fresh structs, as a second control plane
	// would build them from the same pulse).
	stored, err = h.RecordEdgeBatch(ctx, testEdgeBatch("k1"))
	require.NoError(t, err)
	assert.False(t, stored, "a repeated batch is reported as not stored")
	assert.Equal(t, oneEdgeBatch, countEdgeBatchRows(t, db), "and nothing of it is written")

	// Another batch is stored.
	stored, err = h.RecordEdgeBatch(ctx, testEdgeBatch("k2"))
	require.NoError(t, err)
	require.True(t, stored)
	c := countEdgeBatchRows(t, db)
	assert.EqualValues(t, 2, c.batches)
	assert.EqualValues(t, 2, c.records)
	assert.EqualValues(t, 3000, c.cost)
}

func TestRecordEdgeBatch_StoredOnce(t *testing.T) { testEdgeBatchStoredOnce(t, edgeBatchSQLite(t)) }
func TestRecordEdgeBatch_StoredOnce_Postgres(t *testing.T) {
	testEdgeBatchStoredOnce(t, postgresSchemaDB(t))
}

// A batch whose transaction fails part-way (here the request is cancelled
// after the proxy logs were written, as when the control plane is shutting
// down) leaves nothing behind, its key included, so the edge's next attempt
// stores it whole.
func testEdgeBatchFailureStoresNothing(t *testing.T, db *gorm.DB) {
	h := edgeBatchHandler(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:cancel_after_proxy_logs", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "proxy_logs" {
			cancel()
		}
	}))
	stored, err := h.RecordEdgeBatch(ctx, testEdgeBatch("k1"))
	require.NoError(t, db.Callback().Create().Remove("test:cancel_after_proxy_logs"))
	require.Error(t, err)
	assert.False(t, stored)
	assert.NotErrorIs(t, err, ErrEdgeBatchUnsupported)

	c := countEdgeBatchRows(t, db)
	assert.Zero(t, c.batches, "the key of a batch that was not stored is not recorded")
	assert.Zero(t, c.logs, "the records written before the failure are rolled back")
	assert.Zero(t, c.records)

	stored, err = h.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.NoError(t, err)
	require.True(t, stored, "the retry stores it")
	assert.Equal(t, oneEdgeBatch, countEdgeBatchRows(t, db))
}

func TestRecordEdgeBatch_FailureStoresNothing(t *testing.T) {
	testEdgeBatchFailureStoresNothing(t, edgeBatchSQLite(t))
}
func TestRecordEdgeBatch_FailureStoresNothing_Postgres(t *testing.T) {
	testEdgeBatchFailureStoresNothing(t, postgresSchemaDB(t))
}

// A failing key insert (the table is missing: a hub whose schema is behind)
// fails the batch; nothing is written without the key.
func TestRecordEdgeBatch_NoKeyTableStoresNothing(t *testing.T) {
	db := edgeBatchSQLite(t)
	h := edgeBatchHandler(t, db)
	require.NoError(t, db.Migrator().DropTable(&models.EdgeAnalyticsBatch{}))

	stored, err := h.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.Error(t, err)
	assert.False(t, stored)
	var n int64
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&n).Error)
	assert.Zero(t, n)
}

// One row the database refuses (an edge value over a column's size, which
// Postgres enforces) is left out; the rest of the batch and its key are
// stored, so the edge does not send the same batch for a day.
func TestRecordEdgeBatch_RefusedRowIsLeftOut_Postgres(t *testing.T) {
	db := postgresSchemaDB(t)
	h := edgeBatchHandler(t, db)
	b := testEdgeBatch("k1")
	b.ProxyLogs[1].RequestID = strings.Repeat("x", 200) // the column is 64 wide

	stored, err := h.RecordEdgeBatch(context.Background(), b)
	require.NoError(t, err)
	require.True(t, stored)
	c := countEdgeBatchRows(t, db)
	assert.EqualValues(t, 1, c.logs, "the refused row is left out, the other is written")
	assert.EqualValues(t, 1, c.records)
	assert.EqualValues(t, 1, c.batches)

	stored, err = h.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.NoError(t, err)
	assert.False(t, stored)
}

// The same batch arriving at two replicas at once (the edge gave up on the
// first and sent it to the second while the first was still writing) is
// stored by exactly one of them.
func testEdgeBatchConcurrentRepeat(t *testing.T, db *gorm.DB) {
	h := edgeBatchHandler(t, db)
	const senders = 8
	var wg sync.WaitGroup
	results := make([]bool, senders)
	errs := make([]error, senders)
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = h.RecordEdgeBatch(context.Background(), testEdgeBatch("same"))
		}(i)
	}
	wg.Wait()
	stored := 0
	for i := range results {
		require.NoError(t, errs[i])
		if results[i] {
			stored++
		}
	}
	assert.Equal(t, 1, stored, "exactly one of the concurrent attempts stored the batch")
	assert.Equal(t, oneEdgeBatch, countEdgeBatchRows(t, db))
}

func TestRecordEdgeBatch_ConcurrentRepeat(t *testing.T) {
	testEdgeBatchConcurrentRepeat(t, edgeBatchSQLite(t))
}
func TestRecordEdgeBatch_ConcurrentRepeat_Postgres(t *testing.T) {
	testEdgeBatchConcurrentRepeat(t, postgresSchemaDB(t))
}

func TestRecordEdgeBatch_NeedsAKeyAndARunningHandler(t *testing.T) {
	db := edgeBatchSQLite(t)
	h := edgeBatchHandler(t, db)
	_, err := h.RecordEdgeBatch(context.Background(), &EdgeBatch{})
	require.Error(t, err)

	h.Stop()
	_, err = h.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.Error(t, err, "a stopped handler stores nothing and says so, so the edge retries")
	assert.NotErrorIs(t, err, ErrEdgeBatchUnsupported)
}

// Batch records past their retention are pruned; recent ones stay.
func TestRecordEdgeBatch_PrunesOldKeys(t *testing.T) {
	db := edgeBatchSQLite(t)
	h := edgeBatchHandler(t, db)
	now := time.Now()
	require.NoError(t, db.Create(&[]models.EdgeAnalyticsBatch{
		{Key: "old", EdgeID: "e", AppliedAt: now.Add(-models.EdgeAnalyticsBatchRetention - time.Hour)},
		{Key: "recent", EdgeID: "e", AppliedAt: now.Add(-time.Hour)},
	}).Error)

	lastEdgeBatchPrune.Store(0)
	stored, err := h.RecordEdgeBatch(context.Background(), testEdgeBatch("new"))
	require.NoError(t, err)
	require.True(t, stored)

	var keys []string
	require.NoError(t, db.Model(&models.EdgeAnalyticsBatch{}).Order("key").Pluck("key", &keys).Error)
	assert.Equal(t, []string{"new", "recent"}, keys)

	// It does not prune again within the interval.
	require.NoError(t, db.Create(&models.EdgeAnalyticsBatch{Key: "old-2", EdgeID: "e", AppliedAt: now.Add(-100 * time.Hour)}).Error)
	_, err = h.RecordEdgeBatch(context.Background(), testEdgeBatch("newer"))
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Model(&models.EdgeAnalyticsBatch{}).Where("key = ?", "old-2").Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// edgeBatchSink records what a Tee hands its sinks.
type edgeBatchSink struct {
	teeRecorder
	smu    sync.Mutex
	events int
	calls  int
}

func (s *edgeBatchSink) RecordComplianceEvents(_ context.Context, es []*models.ComplianceEvent) {
	s.smu.Lock()
	s.events += len(es)
	s.smu.Unlock()
}
func (s *edgeBatchSink) RecordToolCall(context.Context, string, time.Time, int, uint) {
	s.smu.Lock()
	s.calls++
	s.smu.Unlock()
}

// Through a Tee the primary stores the batch once and the sinks get its
// records once: a repeated batch reaches neither.
func TestTee_RecordEdgeBatch(t *testing.T) {
	db := edgeBatchSQLite(t)
	h := edgeBatchHandler(t, db)
	sink := &edgeBatchSink{}
	tee := NewTee(h, sink)

	stored, err := tee.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.NoError(t, err)
	require.True(t, stored)
	stored, err = tee.RecordEdgeBatch(context.Background(), testEdgeBatch("k1"))
	require.NoError(t, err)
	require.False(t, stored)
	tee.Stop(5 * time.Second)

	chats, logs := sink.counts()
	assert.Equal(t, 1, chats)
	assert.Equal(t, 2, logs)
	sink.smu.Lock()
	assert.Equal(t, 1, sink.events)
	assert.Equal(t, 1, sink.calls)
	sink.smu.Unlock()
	assert.Equal(t, oneEdgeBatch, countEdgeBatchRows(t, db))

	// A primary that cannot store a batch once says so.
	plain := NewTee(&teeRecorder{})
	defer plain.Stop(time.Second)
	_, err = plain.RecordEdgeBatch(context.Background(), testEdgeBatch("k2"))
	assert.ErrorIs(t, err, ErrEdgeBatchUnsupported)
}

// The package function uses the handler for the context and reports a
// handler that cannot store a batch once.
func TestRecordEdgeBatch_HandlerForContext(t *testing.T) {
	db := edgeBatchSQLite(t)
	h := edgeBatchHandler(t, db)

	stored, err := RecordEdgeBatch(WithHandler(context.Background(), h), testEdgeBatch("k1"))
	require.NoError(t, err)
	assert.True(t, stored)

	_, err = RecordEdgeBatch(WithHandler(context.Background(), &teeRecorder{}), testEdgeBatch("k2"))
	assert.ErrorIs(t, err, ErrEdgeBatchUnsupported)
}
