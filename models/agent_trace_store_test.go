package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentTraceDBs runs fn on a migrated SQLite database and, when DATABASE_URL
// is set, on Postgres in a schema of its own.
func agentTraceDBs(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "trace.db")), cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.Close()
			}
		})
		require.NoError(t, db.AutoMigrate(AnalyticsModels()...))
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			t.Skip("DATABASE_URL not set")
		}
		admin, err := gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("agent_trace_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
		t.Cleanup(func() {
			admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			if sqlDB, err := admin.DB(); err == nil {
				sqlDB.Close()
			}
		})
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.Close()
			}
		})
		require.NoError(t, db.AutoMigrate(AnalyticsModels()...))
		fn(t, db)
	})
}

func spanID(b byte) []byte  { return []byte{0, 0, 0, 0, 0, 0, 0, b} }
func traceID(b byte) []byte { return append(make([]byte, 15), b) }

func TestAgentTraceCaptureEvents_IdempotentInsertAndCursor(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		now := time.Now()
		ev := func(key string) AgentTraceCaptureEvent {
			return AgentTraceCaptureEvent{IdempotencyKey: key, Source: "edge-1", Kind: "llm_inference",
				AppID: 7, SpanID: spanID(1), ReceivedAt: now, Event: []byte{1, 2, 3}}
		}
		n, err := InsertAgentTraceCaptureEvents(db, []AgentTraceCaptureEvent{ev("edge-1:01:0"), ev("edge-1:01:1")})
		require.NoError(t, err)
		assert.EqualValues(t, 2, n)

		// A redelivered pulse: one duplicate, one new.
		n, err = InsertAgentTraceCaptureEvents(db, []AgentTraceCaptureEvent{ev("edge-1:01:1"), ev("edge-1:01:2")})
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)

		// All duplicates: nothing inserted, no error.
		n, err = InsertAgentTraceCaptureEvents(db, []AgentTraceCaptureEvent{ev("edge-1:01:0")})
		require.NoError(t, err)
		assert.EqualValues(t, 0, n)

		var count int64
		require.NoError(t, db.Model(&AgentTraceCaptureEvent{}).Count(&count).Error)
		assert.EqualValues(t, 3, count)

		page, err := ListAgentTraceCaptureEventsAfter(db, 0, 2)
		require.NoError(t, err)
		require.Len(t, page, 2)
		assert.Equal(t, "edge-1:01:0", page[0].IdempotencyKey)
		assert.Equal(t, 3, page[0].Size)
		assert.Equal(t, []byte{1, 2, 3}, page[0].Event)
		rest, err := ListAgentTraceCaptureEventsAfter(db, page[1].ID, 10)
		require.NoError(t, err)
		require.Len(t, rest, 1)
		assert.Equal(t, "edge-1:01:2", rest[0].IdempotencyKey)
	})
}

func TestAgentTraceSpans_KeysetOrder(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		mk := func(id byte, start time.Time, app uint, session string) AgentTraceSpan {
			return AgentTraceSpan{SpanID: spanID(id), TraceID: traceID(1), SessionID: session, AppID: app,
				Kind: "llm_inference", Name: "chat", StartTime: start, Status: "ok", ContentState: "metadata",
				FinishReasons: []string{"tool_calls"}, ToolCallIDs: []string{"call_1"}}
		}
		// Ties on start_time are broken by span_id; times include a
		// non-UTC zone and sub-second precision.
		spans := []AgentTraceSpan{
			mk(5, base.Add(time.Second), 1, "s1"),
			mk(3, base, 1, "s1"),
			mk(1, base, 1, "s1"),
			mk(2, base.Add(500*time.Millisecond).In(time.FixedZone("x", 3600)), 1, "s1"),
			mk(4, base.Add(time.Second), 1, "s1"),
			mk(9, base, 2, "s1"), // another App: never returned
			mk(8, base, 1, "s2"),
		}
		require.NoError(t, InsertAgentTraceSpans(db, spans, 3))

		var got [][]byte
		var cur *AgentTraceSpanCursor
		for {
			page, err := ListAgentTraceSessionSpans(db, 1, "s1", cur, 2)
			require.NoError(t, err)
			for _, s := range page {
				got = append(got, s.SpanID)
			}
			if len(page) < 2 {
				break
			}
			last := page[len(page)-1]
			cur = &AgentTraceSpanCursor{StartTime: last.StartTime, SpanID: last.SpanID}
		}
		assert.Equal(t, [][]byte{spanID(1), spanID(3), spanID(2), spanID(4), spanID(5)}, got)

		page, err := ListAgentTraceSessionSpans(db, 1, "s1", nil, 1)
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, []string{"tool_calls"}, page[0].FinishReasons)
		assert.Equal(t, []string{"call_1"}, page[0].ToolCallIDs)
		assert.Equal(t, traceID(1), page[0].TraceID)
	})
}

func TestAgentTraceSessionRollupUpsert(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		t0 := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		team := uint(4)
		first := &AgentTraceSession{ID: "0192b6a0-0000-7000-8000-000000000001", AppID: 1, TeamID: &team,
			Source: "inferred", Status: AgentTraceSessionActive, StartedAt: t0, LastActivityAt: t0.Add(time.Minute),
			SpanCount: 2, LLMCallCount: 1, InputTokens: 100, CostMicros: 50, ContentMode: "off",
			Sampled: false, RetentionUntil: t0.Add(30 * 24 * time.Hour)}
		require.NoError(t, UpsertAgentTraceSessionRollup(db, first))

		// Deltas: counts add, an older activity time does not move it back,
		// other columns keep their stored value.
		delta := &AgentTraceSession{ID: first.ID, AppID: 1, Source: "declared", Status: AgentTraceSessionActive,
			StartedAt: t0, LastActivityAt: t0, SpanCount: 3, ToolCallCount: 2, InputTokens: 10, CostMicros: 5,
			ContentMode: "full", RetentionUntil: t0}
		require.NoError(t, UpsertAgentTraceSessionRollup(db, delta))
		later := &AgentTraceSession{ID: first.ID, AppID: 1, Source: "inferred", Status: AgentTraceSessionActive,
			StartedAt: t0, LastActivityAt: t0.Add(time.Hour), ErrorCount: 1, ContentMode: "off", RetentionUntil: t0}
		require.NoError(t, UpsertAgentTraceSessionRollup(db, later))

		var got AgentTraceSession
		require.NoError(t, db.First(&got, "id = ?", first.ID).Error)
		assert.Equal(t, 5, got.SpanCount)
		assert.Equal(t, 1, got.LLMCallCount)
		assert.Equal(t, 2, got.ToolCallCount)
		assert.Equal(t, 1, got.ErrorCount)
		assert.EqualValues(t, 110, got.InputTokens)
		assert.EqualValues(t, 55, got.CostMicros)
		assert.True(t, got.LastActivityAt.Equal(t0.Add(time.Hour)), "last_activity_at %v", got.LastActivityAt)
		assert.Equal(t, "inferred", got.Source)
		assert.Equal(t, "off", got.ContentMode)
		assert.False(t, got.Sampled, "an explicit false must stay false")
		require.NotNil(t, got.TeamID)
		assert.EqualValues(t, 4, *got.TeamID)

		tr := &AgentTraceTrace{TraceID: traceID(9), SessionID: first.ID, AppID: 1, Source: "tyk",
			StartedAt: t0, Status: "in_progress", SpanCount: 1, CostMicros: 7}
		require.NoError(t, UpsertAgentTraceTraceRollup(db, tr))
		require.NoError(t, UpsertAgentTraceTraceRollup(db, &AgentTraceTrace{TraceID: traceID(9), SessionID: first.ID,
			AppID: 1, Source: "tyk", StartedAt: t0, Status: "ok", SpanCount: 2, CostMicros: 3}))
		var gotTr AgentTraceTrace
		require.NoError(t, db.First(&gotTr, "session_id = ?", first.ID).Error)
		assert.Equal(t, 3, gotTr.SpanCount)
		assert.EqualValues(t, 10, gotTr.CostMicros)
		assert.Equal(t, "in_progress", gotTr.Status)
	})
}

func TestAgentTraceRetention_ChunkedDelete(t *testing.T) {
	agentTraceDBs(t, func(t *testing.T, db *gorm.DB) {
		now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			app := uint(1)
			if i == 4 {
				app = 2
			}
			id := fmt.Sprintf("0192b6a0-0000-7000-8000-00000000000%d", i)
			require.NoError(t, UpsertAgentTraceSessionRollup(db, &AgentTraceSession{ID: id, AppID: app,
				Source: "inferred", Status: AgentTraceSessionClosed, StartedAt: now, LastActivityAt: now,
				ContentMode: "off", RetentionUntil: now.Add(time.Duration(i-3) * time.Hour)}))
			require.NoError(t, db.Create(&AgentTraceSessionModel{SessionID: id, Name: "gpt-4o"}).Error)
			require.NoError(t, db.Create(&AgentTraceSessionTool{SessionID: id, Name: "search"}).Error)
		}
		// Expired: i=0,1,2 (app 1). i=3 is at the cutoff, i=4 later (app 2).

		// App-scoped delete for app 2 removes nothing (not expired).
		app2 := uint(2)
		n, err := DeleteAgentTraceRowsBefore(db, AgentTraceRetentionSessions, now, &app2, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 0, n)

		var total int64
		for {
			n, err := DeleteAgentTraceRowsBefore(db, AgentTraceRetentionSessions, now, nil, 2)
			require.NoError(t, err)
			total += n
			if n < 2 {
				break
			}
		}
		assert.EqualValues(t, 3, total)
		var left, models, tools int64
		require.NoError(t, db.Model(&AgentTraceSession{}).Count(&left).Error)
		require.NoError(t, db.Model(&AgentTraceSessionModel{}).Count(&models).Error)
		require.NoError(t, db.Model(&AgentTraceSessionTool{}).Count(&tools).Error)
		assert.EqualValues(t, 2, left)
		assert.EqualValues(t, 2, models, "filter rows go with their session")
		assert.EqualValues(t, 2, tools)

		// Generic path: payloads by expires_at, in chunks.
		for i := 0; i < 5; i++ {
			require.NoError(t, db.Create(&AgentTracePayload{ID: fmt.Sprintf("p%d", i), SpanID: spanID(byte(i)), AppID: 1,
				Part: "request", ContentState: "full", MediaType: "application/json", Encoding: "identity",
				CreatedAt: now, ExpiresAt: now.Add(time.Duration(i-3) * time.Minute)}).Error)
		}
		n, err = DeleteAgentTraceRowsBefore(db, AgentTraceRetentionPayloads, now, nil, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 2, n)
		n, err = DeleteAgentTraceRowsBefore(db, AgentTraceRetentionPayloads, now, nil, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)
		var payloads int64
		require.NoError(t, db.Model(&AgentTracePayload{}).Count(&payloads).Error)
		assert.EqualValues(t, 2, payloads)

		// A byte-keyed table (spans) also deletes through the subquery.
		require.NoError(t, InsertAgentTraceSpans(db, []AgentTraceSpan{
			{SpanID: spanID(1), TraceID: traceID(1), SessionID: "s", AppID: 1, Kind: "llm_inference", Name: "a", StartTime: now.Add(-time.Hour), Status: "ok", ContentState: "off"},
			{SpanID: spanID(2), TraceID: traceID(1), SessionID: "s", AppID: 1, Kind: "llm_inference", Name: "b", StartTime: now.Add(time.Hour), Status: "ok", ContentState: "off"},
		}, 0))
		n, err = DeleteAgentTraceRowsBefore(db, AgentTraceRetentionSpans, now, nil, 10)
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)

		_, err = DeleteAgentTraceRowsBefore(db, AgentTraceRetentionAgents, now, &app2, 10)
		assert.ErrorIs(t, err, ErrAgentTraceNoAppScope)
	})
}
