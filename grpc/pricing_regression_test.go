package grpc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services"
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

// kq3b.3 at the hub, end to end: a running budget sync, then a price
// recalculation through the service. The next sync carries each App's spend
// at the new prices (a raise from unpriced and a lowering from overpriced)
// and tells edges they may lower; the settled figures used to stand until
// the next period.
func TestPricingRegression_HubBudgetSyncFollowsRecalculation(t *testing.T) {
	forEachBudgetSyncDB(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(&models.ModelPrice{}))
		bus := eventbridge.NewBus()
		payloads := make(chan map[string]json.RawMessage, 16)
		bus.Subscribe(BudgetSyncTopic, func(e eventbridge.Event) {
			var p map[string]json.RawMessage
			if json.Unmarshal(e.Payload, &p) == nil {
				payloads <- p
			}
		})
		next := func() map[string]json.RawMessage {
			t.Helper()
			select {
			case p := <-payloads:
				return p
			case <-time.After(5 * time.Second):
				t.Fatal("no budget sync published")
				return nil
			}
		}
		usage := func(p map[string]json.RawMessage, appID uint) float64 {
			t.Helper()
			var apps map[string]struct {
				Usage float64 `json:"usage"`
			}
			require.NoError(t, json.Unmarshal(p["app_budgets"], &apps))
			a, ok := apps[fmt.Sprint(appID)]
			require.True(t, ok, "app %d in the sync", appID)
			return a.Usage
		}

		svc := &services.Service{DB: db}
		type side struct {
			app      models.App
			price    models.ModelPrice
			from, to float64 // per input token
		}
		sides := []*side{
			{app: models.App{Name: "raise"}, price: models.ModelPrice{ModelName: "gemini-raise", Vendor: "google_ai", Currency: "USD"}, from: 0, to: 1e-5},
			{app: models.App{Name: "lower"}, price: models.ModelPrice{ModelName: "gemini-lower", Vendor: "google_ai", CPIT: 1e-4, Currency: "USD"}, from: 1e-4, to: 1e-6},
		}
		at := time.Now().Add(-time.Hour)
		for _, sd := range sides {
			require.NoError(t, db.Create(&sd.app).Error)
			require.NoError(t, sd.price.Create(db))
			for i := 0; i < 20; i++ {
				require.NoError(t, db.Create(&models.LLMChatRecord{AppID: sd.app.ID, LLMID: 1, Name: sd.price.ModelName, Vendor: "google_ai",
					PromptTokens: 10000, TotalTokens: 10000, Cost: 10000 * sd.from * 10000, Currency: "USD", TimeStamp: at.Add(time.Duration(i) * time.Second)}).Error)
			}
		}

		s := NewBudgetSyncService(db, bus)
		s.syncInterval = time.Hour
		s.Start()
		t.Cleanup(s.Stop)
		next() // the start-up cycle
		for i := 0; i < budgetSettleCycles; i++ {
			s.aggregateAndPublish()
			next()
		}

		for _, sd := range sides {
			_, err := svc.UpdateModelPriceAndRecalculate(sd.price.ID, sd.price.ModelName, "google_ai", 0, sd.to, 0, 0, "USD")
			require.NoError(t, err)
		}
		s.aggregateAndPublish()
		p := next()
		assert.JSONEq(t, "true", string(p["spend_recalculated"]), "edges are told they may lower")
		for _, sd := range sides {
			assert.InDelta(t, 20*10000*sd.to, usage(p, sd.app.ID), 1e-9, "%s: spend at the new price", sd.app.Name)
		}
	})
}

// kq3b.9 at the hub: the budget usage tracker counts USD spend (and rows with
// no currency) only; EUR spend is not added to the dollars.
func TestPricingRegression_HubBudgetCountsUSDOnly(t *testing.T) {
	forEachBudgetSyncDB(t, func(t *testing.T, db *gorm.DB) {
		app := models.App{Name: "mixed"}
		require.NoError(t, db.Create(&app).Error)
		now := time.Now()
		for _, r := range []struct {
			cost     float64
			currency string
		}{{30000, "USD"}, {10000, ""}, {580000, "EUR"}, {20000, "usd"}} {
			require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: r.cost, Currency: r.currency, TimeStamp: now.Add(-time.Minute)}).Error)
		}
		got, err := newBudgetUsageTracker().usage(db, []models.App{app}, now)
		require.NoError(t, err)
		assert.InDelta(t, 60000, got[app.ID].cost, 1e-9, "USD, lower-case usd and no currency; not EUR")
	})
}
