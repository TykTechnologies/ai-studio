package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Pricing regression suite, hub budget side (the core suite is
// proxy/pricing_regression_test.go). Runs on SQLite and, when DATABASE_URL
// is a Postgres URL, on Postgres in a schema of its own.

func forEachBudgetSyncDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, setupBudgetSyncTestDB(t)) })
	t.Run("postgres", func(t *testing.T) {
		db := budgetSyncPostgresDB(t)
		require.NoError(t, db.AutoMigrate(&models.LLMChatRecord{}, &models.App{}))
		fn(t, db)
	})
}

// budgetSyncPostgresDB opens DATABASE_URL in a schema of its own, skipping
// unless it is a Postgres URL. Only the budget and pricing queries are under
// test, not the foreign keys, so migrations skip them.
func budgetSyncPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(dsn, "postgres") {
		t.Skip("DATABASE_URL is not a Postgres URL")
	}
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
	return db
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

// kq3b.9 at the hub: a budget's currency is implied by its period's spend.
// An App whose period is all EUR (any case) is counted in EUR and not
// blocked, rows with no currency count as USD alongside USD, and only an App
// whose period mixes EUR and USD is blocked, with the reason sent to edges.
func TestPricingRegression_HubBudgetImpliedCurrency(t *testing.T) {
	forEachBudgetSyncDB(t, func(t *testing.T, db *gorm.DB) {
		bus := eventbridge.NewBus()
		got := make(chan BudgetSyncPayload, 4)
		bus.Subscribe(BudgetSyncTopic, func(e eventbridge.Event) {
			var p BudgetSyncPayload
			if json.Unmarshal(e.Payload, &p) == nil {
				got <- p
			}
		})
		svc := NewBudgetSyncService(db, bus)
		svc.SetEdgeBudgetSource(&fakeEdgeBudget{blocks: map[uint]string{}})

		limit := 100.0
		apps := map[string]*models.App{"eur": {Name: "eur"}, "usd": {Name: "usd"}, "mixed": {Name: "mixed"}}
		rows := map[string][]struct {
			cost     float64
			currency string
		}{
			"eur":   {{10000, "EUR"}, {10000, "eur"}},
			"usd":   {{10000, "USD"}, {10000, ""}},
			"mixed": {{30000, "USD"}, {580000, "EUR"}},
		}
		now := time.Now()
		for name, app := range apps {
			app.MonthlyBudget = &limit
			require.NoError(t, db.Create(app).Error)
			for _, r := range rows[name] {
				require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: r.cost, Currency: r.currency, TimeStamp: now.Add(-time.Minute)}).Error)
			}
		}

		svc.aggregateAndPublish()
		var p BudgetSyncPayload
		select {
		case p = <-got:
		case <-time.After(5 * time.Second):
			t.Fatal("no budget sync published")
		}
		assert.InDelta(t, 2, p.AppBudgets[uint32(apps["eur"].ID)].Usage, 1e-9, "EUR-only (EUR and eur) counts in EUR")
		assert.NotContains(t, p.Blocks, uint32(apps["eur"].ID))
		assert.InDelta(t, 2, p.AppBudgets[uint32(apps["usd"].ID)].Usage, 1e-9, "no currency counts as USD")
		assert.NotContains(t, p.Blocks, uint32(apps["usd"].ID))
		assert.Contains(t, p.Blocks[uint32(apps["mixed"].ID)], "spend in EUR, USD", "a mixed period is blocked")
	})
}

// forEachPulseDB runs fn on a database set up for edge analytics pulses
// (SQLite on one connection, for the analytics worker; Postgres in a schema
// of its own), with model prices.
func forEachPulseDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	tables := []any{&models.LLMChatRecord{}, &models.ProxyLog{}, &models.EdgeInstance{}, &models.ComplianceEvent{}, &models.ToolCallRecord{}, &models.ModelPrice{}, &models.App{}}
	t.Run("sqlite", func(t *testing.T) {
		db := setupPulseTestDB(t)
		require.NoError(t, db.AutoMigrate(tables...))
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := budgetSyncPostgresDB(t)
		require.NoError(t, db.AutoMigrate(tables...))
		fn(t, db)
	})
}

// kq3b.12/kq3b.13 end to end: rows an edge sends in an analytics pulse keep
// the model asked for, the tier multiplier and the currency. So a
// recalculation of the requested name's price reprices the versioned row,
// and a batch row stays at 0.5x through a recalculation via a zero price.
func TestPricingRegression_EdgeRowsRepricedLikeEmbedded(t *testing.T) {
	forEachPulseDB(t, func(t *testing.T, db *gorm.DB) {
		server := setupControlServer(t, db)
		t.Cleanup(server.Stop)
		svc := &services.Service{DB: db}
		const vendor = "google_ai"
		p := models.ModelPrice{CPT: 0.4e-6, CPIT: 0.1e-6, Currency: "EUR"}
		price, err := svc.CreateModelPrice("gemini-2.0-flash", vendor, p.CPT, p.CPIT, 0, 0, "EUR")
		require.NoError(t, err)
		_, err = svc.GetModelPriceByModelNameAndVendor("gemini-2.0-flash-001", vendor) // the edge's unpriced reported row
		require.NoError(t, err)

		list := models.ChatRecordCost(vendor, p.CPT, p.CPIT, 0, 0, 1000, 200, 0, 0)
		now := time.Now()
		event := func(id string, mult *float64, at time.Duration) *pb.AnalyticsEvent {
			cost := list
			if mult != nil {
				cost *= *mult
			}
			return &pb.AnalyticsEvent{RequestId: id, AppId: 1, LlmId: 1, ModelName: "gemini-2.0-flash-001", RequestedModel: "gemini-2.0-flash",
				Vendor: vendor, StatusCode: 200, RequestTokens: 1000, ResponseTokens: 200, TotalTokens: 1200,
				Cost: cost, Currency: "EUR", PriceMultiplier: mult, Timestamp: timestamppb.New(now.Add(at))}
		}
		resp, err := server.SendAnalyticsPulse(context.Background(), &pb.AnalyticsPulse{
			EdgeId: "edge-1", EdgeNamespace: "default", SequenceNumber: 1, TotalRecords: 2,
			AnalyticsEvents: []*pb.AnalyticsEvent{event("sync", protobuf.Float64(1), 0), event("batch", protobuf.Float64(0.5), time.Second)},
		})
		require.NoError(t, err)
		require.True(t, resp.Success)
		var recs []models.LLMChatRecord
		require.Eventually(t, func() bool {
			recs = nil
			return db.Order("time_stamp asc").Find(&recs).Error == nil && len(recs) == 2
		}, 5*time.Second, 20*time.Millisecond)
		for _, r := range recs {
			assert.Equal(t, "gemini-2.0-flash", r.RequestedModel)
			assert.Equal(t, "EUR", r.Currency, "the edge's currency is kept")
		}

		reprice := func(f float64) {
			t.Helper()
			_, err := svc.UpdateModelPriceAndRecalculate(price.ID, "gemini-2.0-flash", vendor, p.CPT*f, p.CPIT*f, 0, 0, "EUR")
			require.NoError(t, err)
		}
		costs := func() (sync, batch float64) {
			t.Helper()
			require.NoError(t, db.Order("time_stamp asc").Find(&recs).Error)
			return recs[0].Cost, recs[1].Cost
		}
		reprice(2)
		s, b := costs()
		assertRel(t, 2*list, s, 1e-9, "the edge row requested by the repriced name is repriced")
		assertRel(t, list, b, 1e-9, "the edge batch row at 0.5x of the doubled price")
		reprice(0)
		reprice(1)
		s, b = costs()
		assertRel(t, list, s, 1e-9, "sync row after p -> 0 -> p")
		assertRel(t, 0.5*list, b, 1e-9, "batch row keeps 0.5x through a zero price")
	})
}

func assertRel(t *testing.T, want, got, rel float64, msg string) {
	t.Helper()
	assert.InEpsilon(t, want, got, rel, msg)
}
