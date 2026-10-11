package services

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The edge writes decision events through the batched writer on Postgres as
// on SQLite: the request kind and question counts survive the batch insert,
// and an event without them (every chat event, every event from before
// decision models) stores the defaults. Set MGW_TEST_POSTGRES_DSN to run it.
func TestAnalyticsWriterDecisionEventsPostgres(t *testing.T) {
	dsn := os.Getenv("MGW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MGW_TEST_POSTGRES_DSN not set")
	}
	cfg := database.DatabaseConfig{Type: "postgres", DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent"}
	db, err := database.Connect(cfg)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(t, database.Migrate(db))
	wdb, err := database.OpenWriter(cfg, db)
	require.NoError(t, err)

	run := fmt.Sprint(time.Now().UnixNano())
	app := &database.App{Name: "pg-decisions-" + run, IsActive: true}
	require.NoError(t, db.Create(app).Error)

	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.Enqueue(&database.AnalyticsEvent{RequestID: "dec-" + run, AppID: app.ID, TimeStamp: time.Now(),
		Vendor: "systemone", Name: "jev-1.13.0", Choices: 2,
		RequestKind: "decision", DecisionQuestions: 3, DecisionRefusals: 1})
	w.Enqueue(&database.AnalyticsEvent{RequestID: "chat-" + run, AppID: app.ID, TimeStamp: time.Now(), Vendor: "openai"})
	w.Start()
	w.Stop()

	var events []database.AnalyticsEvent
	require.NoError(t, db.Where("app_id = ?", app.ID).Order("request_id").Find(&events).Error)
	require.Len(t, events, 2)
	chat, dec := events[0], events[1]
	assert.Equal(t, "", chat.RequestKind)
	assert.Zero(t, chat.DecisionQuestions)
	assert.Equal(t, "decision", dec.RequestKind)
	assert.Equal(t, 3, dec.DecisionQuestions)
	assert.Equal(t, 1, dec.DecisionRefusals)
	assert.Equal(t, 2, dec.Choices)

	var null int64
	require.NoError(t, db.Model(&database.AnalyticsEvent{}).Where("app_id = ? AND request_kind IS NULL", app.ID).Count(&null).Error)
	assert.Zero(t, null)
}
