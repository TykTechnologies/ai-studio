package analytics

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyChatRecord is llm_chat_records as it was before decision models: no
// request_kind, decision_questions or decision_refusals columns.
type legacyChatRecord struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	Vendor    string
	Cost      float64
	TimeStamp time.Time
	Choices   int
}

func (legacyChatRecord) TableName() string { return "llm_chat_records" }

// Rows written before the upgrade must read as generative ("" request kind),
// never NULL: every generative filter is request_kind = ”, and a NULL would
// drop a deployment's whole history out of it.
func assertUpgradeKeepsHistoryGenerative(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&legacyChatRecord{}))
	require.NoError(t, db.Create(&legacyChatRecord{Name: "gpt-4o", Vendor: "openai", Cost: 5, TimeStamp: time.Now()}).Error)

	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}))

	var generative, null int64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("request_kind = ?", models.GenerativeRequest).Count(&generative).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("request_kind IS NULL").Count(&null).Error)
	assert.Equal(t, int64(1), generative)
	assert.Zero(t, null)

	// A new chat record written by the upgraded code is generative too.
	require.NoError(t, db.Create(&models.LLMChatRecord{Name: "gpt-4o", Vendor: "openai", TimeStamp: time.Now()}).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("request_kind = ?", models.GenerativeRequest).Count(&generative).Error)
	assert.Equal(t, int64(2), generative)
}

func TestDecisionColumns_UpgradeKeepsHistoryGenerative(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	assertUpgradeKeepsHistoryGenerative(t, db)
}

// postgresSchemaDB opens DATABASE_URL in a schema of its own, skipping when no
// Postgres is configured (CI's "Go Postgres Tests" job sets it).
func postgresSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(dsn, "postgres") {
		t.Skip("DATABASE_URL is not a Postgres URL")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	schema := fmt.Sprintf("decisions_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestDecisionColumns_UpgradeKeepsHistoryGenerative_Postgres(t *testing.T) {
	assertUpgradeKeepsHistoryGenerative(t, postgresSchemaDB(t))
}

// The decision report's aggregates (SUM over integer columns returns numeric
// on Postgres, DATE() returns a date) must scan on Postgres as on SQLite.
func TestGetDecisionUsage_Postgres(t *testing.T) {
	db := postgresSchemaDB(t)
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}))

	day := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&[]models.LLMChatRecord{
		{LLMID: 1, AppID: 10, Vendor: "systemone", Name: "jev-1.13.0", PromptTokens: 200, Choices: 2, Cost: 0.084, Currency: "USD", TimeStamp: day,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3, DecisionRefusals: 1},
		{LLMID: 1, AppID: 10, Vendor: "openai", Name: "gpt-4o", PromptTokens: 50, Cost: 5, Currency: "USD", TimeStamp: day},
	}).Error)

	usage, err := GetDecisionUsage(db, day.Add(-time.Hour), day.Add(time.Hour), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), usage.Totals.Requests)
	assert.Equal(t, int64(3), usage.Totals.Questions)
	assert.Equal(t, int64(2), usage.Totals.Answered)
	assert.Equal(t, int64(1), usage.Totals.Refusals)
	assert.Equal(t, int64(200), usage.Totals.InputTokens)
	assert.InDelta(t, 0.084/10000, usage.Totals.CostByCurrency["USD"], 1e-12)
	require.Len(t, usage.ByDay.Labels, 1)
	assert.True(t, strings.HasPrefix(usage.ByDay.Labels[0], "2026-10-01"), usage.ByDay.Labels[0])
	require.Len(t, usage.ByModel, 1)
	assert.Equal(t, "jev-1.13.0", usage.ByModel[0].Model)
}
