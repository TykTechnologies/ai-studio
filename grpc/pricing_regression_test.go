package grpc

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pricing regression suite, hub budget side (the core suite is
// proxy/pricing_regression_test.go). Runs on SQLite and, when DATABASE_URL
// is a Postgres URL, on Postgres in a schema of its own.

func forEachBudgetSyncDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, setupBudgetSyncTestDB(t)) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(dsn, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		// Only the budget queries are under test, not the App's foreign keys.
		cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
		admin, err := gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("budget_sync_regression_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
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
			admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			if sqlDB, err := admin.DB(); err == nil {
				sqlDB.Close()
			}
		})
		require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.App{}))
		fn(t, db)
	})
}

// kq3b.4 at the hub: Apps anchored on the 29th, 30th and 31st, checked on
// Mar 1 of a common and a leap year. Each period started on Feb 28 or 29,
// so that day's and Mar 1's spend count and the day before does not. Before
// the fix the periods started on Mar 1-3 and missed February's last day (or
// started in the future and read 0).
func TestPricingRegression_HubShortMonthAnchors(t *testing.T) {
	for _, year := range []int{2026, 2028} {
		t.Run(fmt.Sprint(year), func(t *testing.T) {
			forEachBudgetSyncDB(t, func(t *testing.T, db *gorm.DB) {
				feb := time.Date(year, time.March, 0, 0, 0, 0, 0, time.UTC).Day()
				var apps []models.App
				for _, day := range []int{29, 30, 31} {
					anchor := time.Date(year-1, time.January, day, 0, 0, 0, 0, time.UTC)
					app := models.App{Name: fmt.Sprintf("anchor%d", day), BudgetStartDate: &anchor}
					require.NoError(t, db.Create(&app).Error)
					apps = append(apps, app)
					addSpend(t, db, app.ID, 100, time.Date(year, time.February, feb-1, 23, 0, 0, 0, time.UTC)) // previous period
					addSpend(t, db, app.ID, 200, time.Date(year, time.February, feb, 0, 0, 0, 0, time.UTC))
					addSpend(t, db, app.ID, 400, time.Date(year, time.March, 1, 9, 0, 0, 0, time.UTC))
				}
				now := time.Date(year, time.March, 1, 12, 0, 0, 0, time.UTC)
				got, err := newBudgetUsageTracker().usage(db, apps, now)
				require.NoError(t, err)
				for _, app := range apps {
					u := got[app.ID]
					assert.Equal(t, time.Date(year, time.February, feb, 0, 0, 0, 0, time.UTC), u.start, "%s period start", app.Name)
					assert.InDelta(t, 600, u.cost, 1e-9, "%s spend", app.Name)
				}
			})
		})
	}
}
