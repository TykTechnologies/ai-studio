package grpc

import (
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestBudgetUsageTracker_Postgres runs the tracker's queries on Postgres. It
// drops and recreates the two tables it uses: point BUDGET_TEST_POSTGRES_DSN at
// a scratch database to run it.
func TestBudgetUsageTracker_Postgres(t *testing.T) {
	dsn := os.Getenv("BUDGET_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("BUDGET_TEST_POSTGRES_DSN not set")
	}
	// Only the budget queries are under test; the App's foreign keys (to
	// credentials and so on) are not.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&models.LLMChatRecord{}, &models.App{}))
	require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.App{}))
	t.Cleanup(func() {
		db.Exec("DELETE FROM llm_chat_records")
		db.Exec("DELETE FROM apps")
	})

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	custom := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	apps := []models.App{{Name: "pg-calendar"}, {Name: "pg-custom", BudgetStartDate: &custom}}
	for i := range apps {
		require.NoError(t, db.Create(&apps[i]).Error)
	}
	for _, a := range apps {
		addSpend(t, db, a.ID, 100, time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC))
		addSpend(t, db, a.ID, 200, time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC))
		addSpend(t, db, a.ID, 400, time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC))
	}

	tr := newBudgetUsageTracker()
	for cycle := 0; cycle < 6; cycle++ {
		for _, a := range apps {
			addSpend(t, db, a.ID, float64(10*(cycle+1)), now.Add(-time.Duration(cycle)*time.Minute))
		}
		got, err := tr.usage(db, apps, now)
		require.NoError(t, err)
		for _, a := range apps {
			assert.InDelta(t, naiveSpend(t, db, a, now), got[a.ID].cost, 1e-9, "cycle %d app %s", cycle, a.Name)
		}
	}
}
