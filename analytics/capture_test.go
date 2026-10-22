package analytics

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureDBs runs fn on a migrated SQLite file database and, when
// DATABASE_URL is set, on Postgres in a schema of its own.
func captureDBs(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	cfg := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "capture.db")), cfg)
		require.NoError(t, err)
		require.NoError(t, Migrate(db))
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			t.Skip("DATABASE_URL not set")
		}
		admin, err := gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("capture_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
		t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
		require.NoError(t, err)
		require.NoError(t, Migrate(db))
		fn(t, db)
	})
}

func captureEvent(appID uint, data string) *agenttrace.Event {
	return &agenttrace.Event{
		Kind:     agenttrace.KindLLMInference,
		SpanID:   agenttrace.NewSpanID(),
		Identity: agenttrace.Identity{AppID: appID},
		Parts: []agenttrace.PayloadPart{{
			Part: agenttrace.PartRequest, ContentState: agenttrace.StateFull, Data: []byte(data),
		}},
		GatewayEvents: []agenttrace.GatewayEvent{{Type: agenttrace.GatewayEventAttempt, Detail: "attempt=1"}},
	}
}

func stagedRows(t *testing.T, db *gorm.DB) []models.AgentTraceCaptureEvent {
	t.Helper()
	rows, err := models.ListAgentTraceCaptureEventsAfter(db, 0, 1000)
	require.NoError(t, err)
	return rows
}

func TestCaptureStagedExactlyOnce(t *testing.T) {
	captureDBs(t, func(t *testing.T, db *gorm.DB) {
		h := NewDatabaseHandler(context.Background(), db)
		t.Cleanup(h.Stop)

		a, b := captureEvent(7, "first"), captureEvent(7, "second")
		dup := cloneCaptureEvent(a) // the same span and seq: a duplicate submit
		h.RecordCaptureEvents(context.Background(), []*agenttrace.Event{a, b})
		h.RecordCaptureEvents(context.Background(), []*agenttrace.Event{dup})

		require.Eventually(t, func() bool { return h.capture.Depth() == 0 && len(stagedRows(t, db)) == 2 },
			5*time.Second, 20*time.Millisecond)
		time.Sleep(2 * captureFlushInterval) // nothing more arrives
		rows := stagedRows(t, db)
		require.Len(t, rows, 2)
		for _, r := range rows {
			assert.Equal(t, agenttrace.SourceEmbedded, r.Source)
			assert.Equal(t, uint(7), r.AppID)
			assert.Equal(t, string(agenttrace.KindLLMInference), r.Kind)
			assert.Len(t, r.SpanID, 8)
			assert.True(t, strings.HasPrefix(r.IdempotencyKey, "embedded:"))
			// No sealer installed: content never reaches the table (jyx.54).
			assert.False(t, bytes.Contains(r.Event, []byte("first")) || bytes.Contains(r.Event, []byte("second")))
			ev, err := agenttrace.DecodeStaged(r.Event, r.IdempotencyKey)
			require.NoError(t, err)
			assert.Equal(t, agenttrace.StateDropped, ev.Parts[0].ContentState)
		}
		assert.Equal(t, uint64(3), h.capture.Dropped(agenttrace.DropNoSealer))
	})
}

// With the writer stalled, offering far more than the caps never blocks a
// request goroutine, and every event is either held or counted as dropped.
func TestCaptureNeverBlocksWithStalledWriter(t *testing.T) {
	SetCaptureLimits(1000, 64<<20)
	t.Cleanup(func() { SetCaptureLimits(0, 0) })

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stall.db")),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	stall := make(chan struct{})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("stall_capture", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_trace_capture_events" {
			<-stall
		}
	}))
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(func() { close(stall); h.Stop() })

	const offered = 10000
	var wg sync.WaitGroup
	done := make(chan struct{})
	start := time.Now()
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < offered/10; i++ {
				h.RecordCaptureEvents(context.Background(), []*agenttrace.Event{captureEvent(1, "payload")})
			}
		}()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RecordCaptureEvents blocked behind a stalled writer")
	}
	t.Logf("offered %d events in %s", offered, time.Since(start))

	held := uint64(h.capture.Depth())
	assert.LessOrEqual(t, held, uint64(1000))
	assert.Equal(t, uint64(offered), held+h.capture.Dropped(agenttrace.DropSpan), "every event is held or counted")
}

func TestCaptureOffHandlerIsInert(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "off.db")),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)
	// No capture event ever arrived: no queue, no flusher.
	assert.Nil(t, h.capture)

	// The agenttrace recorder with the flag off never reaches the handler.
	agenttrace.SetEnabled(false)
	SetHandler(h)
	t.Cleanup(ResetHandler)
	agenttrace.SetSink(CaptureSink())
	t.Cleanup(func() { agenttrace.SetSink(nil) })
	ctx, rec := agenttrace.Begin(context.Background())
	rec.Update(func(ev *agenttrace.Event) { ev.Kind = agenttrace.KindLLMInference })
	agenttrace.AddPart(ctx, agenttrace.PayloadPart{Part: agenttrace.PartRequest})
	rec.Finish()
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, h.capture)
	var n int64
	require.NoError(t, db.Model(&models.AgentTraceCaptureEvent{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestCaptureSinkThroughRecorder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sink.db")),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)
	SetHandler(h)
	t.Cleanup(ResetHandler)
	agenttrace.SetEnabled(true)
	t.Cleanup(func() { agenttrace.SetEnabled(false) })
	agenttrace.SetSink(CaptureSink())
	t.Cleanup(func() { agenttrace.SetSink(nil) })

	_, rec := agenttrace.Begin(context.Background())
	rec.Update(func(ev *agenttrace.Event) { ev.Kind = agenttrace.KindToolCall; ev.Identity.AppID = 3 })
	rec.Finish()
	require.Eventually(t, func() bool { return len(stagedRows(t, db)) == 1 }, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, rec.SpanID().Bytes(), stagedRows(t, db)[0].SpanID)
}

// noopHandler is an AnalyticsHandler that ignores everything and does not
// stage capture events.
type noopHandler struct{}

func (noopHandler) RecordChatRecord(context.Context, *models.LLMChatRecord)           {}
func (noopHandler) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry)       {}
func (noopHandler) RecordProxyLog(context.Context, *models.ProxyLog)                  {}
func (noopHandler) RecordToolCall(context.Context, string, time.Time, int, uint)      {}
func (noopHandler) SetAsGlobalHandler()                                               {}
func (noopHandler) RecordChatRecordsBatch(context.Context, []*models.LLMChatRecord)   {}
func (noopHandler) RecordProxyLogsBatch(context.Context, []*models.ProxyLog)          {}
func (noopHandler) RecordComplianceEvents(context.Context, []*models.ComplianceEvent) {}

type captureCollector struct {
	noopHandler
	mu     sync.Mutex
	events []*agenttrace.Event
}

func (c *captureCollector) RecordCaptureEvents(_ context.Context, events []*agenttrace.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, events...)
}

func (c *captureCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestTeeForwardsCaptureEvents(t *testing.T) {
	primary, sink, plain := &captureCollector{}, &captureCollector{}, &noopHandler{}
	tee := NewTee(primary, sink, plain)
	ev := captureEvent(1, "content")
	tee.RecordCaptureEvents(context.Background(), []*agenttrace.Event{ev})
	tee.Stop(time.Second)

	require.Equal(t, 1, primary.count())
	require.Equal(t, 1, sink.count())
	assert.Same(t, ev, primary.events[0])
	assert.NotSame(t, ev, sink.events[0])
	agenttrace.StripPayloadData(primary.events[0])
	assert.Equal(t, []byte("content"), sink.events[0].Parts[0].Data, "the sink's copy is independent")
}
