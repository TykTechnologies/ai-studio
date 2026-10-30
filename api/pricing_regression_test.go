package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pricing regression suite, API side (the core suite is
// proxy/pricing_regression_test.go). Runs on SQLite and, when DATABASE_URL
// is a Postgres URL, on Postgres in a schema of its own.

func forEachPricingAPIDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, apitest.SetupTestDB(t)) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(dsn, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
		admin, err := gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("pricing_api_regression_%d", time.Now().UnixNano())
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
		require.NoError(t, models.InitModels(db))
		fn(t, db)
	})
}

// kq3b.8, end to end: the control server tracks configuration changes on the
// event bus the API's system events go to, as in Studio. Updating a price
// with recalculation changes the namespace checksum and marks an in-sync edge
// Pending, as a standard update does, so the new price reaches the edges.
func TestPricingRegression_RecalcMarksEdgesPending(t *testing.T) {
	forEachPricingAPIDB(t, func(t *testing.T, db *gorm.DB) {
		server, err := grpc.NewControlServer(&grpc.Config{GRPCHost: "localhost", AuthToken: "pricing-regression", EncryptionKey: "pricing-regression-key-32-chars!"}, db)
		require.NoError(t, err)
		t.Cleanup(server.Stop)

		service := apitest.SetupTestService(db)
		service.SystemEvents = services.NewSystemEventEmitter(server.GetEventBus(), "")
		api := NewAPI(service, true, apitest.SetupTestAuthService(db, service), apitest.SetupTestAuthConfig(db, service), nil, emptyFile, nil)

		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}).Error)
		syncStatus := func() string {
			var e models.EdgeInstance
			require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&e).Error)
			return e.SyncStatus
		}
		checksum := func() string {
			var s models.NamespaceSyncStatus
			if err := s.GetByNamespace(db, "default"); err != nil {
				return ""
			}
			return s.ExpectedChecksum
		}
		patch := func(path string, cpt float64) {
			t.Helper()
			body := map[string]interface{}{"data": map[string]interface{}{
				"type": "model-prices", "attributes": map[string]interface{}{"cpt": cpt},
			}}
			w := performRequest(api.router, http.MethodPatch, path, body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		}

		price, err := service.CreateModelPrice("gemini-2.5-flash", "google_ai", 0.0000025, 0.0000003, 0, 0, "USD")
		require.NoError(t, err)
		// The edge loaded the configuration with this price: a standard update
		// records the checksum, then the edge reports it.
		patch(fmt.Sprintf("/api/v1/model-prices/%d", price.ID), 0.0000025)
		require.Eventually(t, func() bool { return checksum() != "" }, 5*time.Second, 10*time.Millisecond, "the standard update records a checksum")
		before := checksum()
		require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)

		patch(fmt.Sprintf("/api/v1/model-prices/%d/recalculate", price.ID), 0.000005)
		assert.Eventually(t, func() bool { return syncStatus() == models.EdgeSyncStatusPending }, 5*time.Second, 10*time.Millisecond,
			"the edge is Pending after the recalculation")
		assert.NotEqual(t, before, checksum(), "the recalculated price changes the namespace checksum")
	})
}

// kq3b.6 over HTTP: POST /model-prices for a model traffic has already seen
// fills the auto-created unpriced row (200), a second POST is a 409, and a
// new model is created (201). Each used to be a 500 unique violation.
func TestPricingRegression_PostPriceForModelSeenInTraffic(t *testing.T) {
	forEachPricingAPIDB(t, func(t *testing.T, db *gorm.DB) {
		service := apitest.SetupTestService(db)
		api := NewAPI(service, true, apitest.SetupTestAuthService(db, service), apitest.SetupTestAuthConfig(db, service), nil, emptyFile, nil)
		_, err := service.GetModelPriceByModelNameAndVendor("gemini-2.0-flash", string(models.GOOGLEAI)) // the gateway's first request
		require.NoError(t, err)

		post := func(model string, cpit float64) int {
			body := map[string]interface{}{"data": map[string]interface{}{"type": "ModelPrice", "attributes": map[string]interface{}{
				"model_name": model, "vendor": string(models.GOOGLEAI), "cpt": 0.0000004, "cpit": cpit, "currency": "USD",
			}}}
			return performRequest(api.router, http.MethodPost, "/api/v1/model-prices", body).Code
		}
		assert.Equal(t, http.StatusOK, post("gemini-2.0-flash", 0.0000001), "fills the unpriced row")
		assert.Equal(t, http.StatusConflict, post("gemini-2.0-flash", 0.0000002), "a priced row is a conflict")
		assert.Equal(t, http.StatusCreated, post("gemini-unseen", 0.0000001), "a new model is created")

		var rows []models.ModelPrice
		require.NoError(t, db.Where("model_name = ?", "gemini-2.0-flash").Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.Equal(t, 0.0000001, rows[0].CPIT)
	})
}

// kq3b.6 under concurrency (the case Postgres exercises; SQLite serialises
// it): the gateway's first lookups and several price creates for an unseen
// model race. Exactly one create wins, the rest are told the price exists,
// the lookups never fail, and one priced row remains.
func TestPricingRegression_ConcurrentCreateAndAutoCreate(t *testing.T) {
	forEachPricingAPIDB(t, func(t *testing.T, db *gorm.DB) {
		service := apitest.SetupTestService(db)
		const lookups, creates = 6, 6
		var wg sync.WaitGroup
		errs := make(chan error, lookups+creates)
		start := make(chan struct{})
		for i := 0; i < lookups+creates; i++ {
			wg.Add(1)
			go func(create bool) {
				defer wg.Done()
				<-start
				var err error
				if create {
					_, err = service.CreateModelPrice("gemini-race", string(models.GOOGLEAI), 0.0000004, 0.0000001, 0, 0, "USD")
					if err == nil {
						err = errWon
					}
				} else {
					_, err = service.GetModelPriceByModelNameAndVendor("gemini-race", string(models.GOOGLEAI))
				}
				errs <- err
			}(i%2 == 0)
		}
		close(start)
		wg.Wait()
		close(errs)
		won := 0
		for err := range errs {
			switch {
			case err == nil: // a lookup
			case err == errWon:
				won++
			default:
				assert.Contains(t, err.Error(), "already exists", "only a conflict is expected")
			}
		}
		assert.Equal(t, 1, won, "exactly one create wins")
		var rows []models.ModelPrice
		require.NoError(t, db.Where("model_name = ?", "gemini-race").Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.Equal(t, 0.0000001, rows[0].CPIT, "the row is priced")
	})
}

var errWon = fmt.Errorf("created or filled")

// kq3b.2 over HTTP: POST /model-prices/:id/backfill takes inclusive UTC
// dates. Rows from the first instant of from to the last of to are priced,
// the instant after is not, on both databases.
func TestPricingRegression_BackfillDateRange(t *testing.T) {
	forEachPricingAPIDB(t, func(t *testing.T, db *gorm.DB) {
		service := apitest.SetupTestService(db)
		api := NewAPI(service, true, apitest.SetupTestAuthService(db, service), apitest.SetupTestAuthConfig(db, service), nil, emptyFile, nil)
		price, err := service.CreateModelPrice("gemini-2.0-flash", string(models.GOOGLEAI), 0.0000004, 0.0000001, 0, 0, "USD")
		require.NoError(t, err)
		at := func(day, h, m, s, ns int) time.Time { return time.Date(2026, 9, day, h, m, s, ns, time.UTC) }
		stamps := []time.Time{
			at(9, 23, 59, 59, 999_000_000),  // before from
			at(10, 0, 0, 0, 0),              // first instant of from
			at(12, 23, 59, 59, 999_000_000), // last instant of to
			at(13, 0, 0, 0, 0),              // after to
		}
		ids := make([]uint, len(stamps))
		for i, ts := range stamps {
			rec := models.LLMChatRecord{Name: "gemini-2.0-flash", Vendor: string(models.GOOGLEAI), PromptTokens: 1000, ResponseTokens: 100, TotalTokens: 1100, Currency: "USD", TimeStamp: ts}
			require.NoError(t, db.Create(&rec).Error)
			ids[i] = rec.ID
		}

		body := map[string]interface{}{"from": "2026-09-10", "to": "2026-09-12"}
		w := performRequest(api.router, http.MethodPost, fmt.Sprintf("/api/v1/model-prices/%d/backfill", price.ID), body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), `"rows_priced":2`)

		want := models.ChatRecordCost(string(models.GOOGLEAI), 0.0000004, 0.0000001, 0, 0, 1000, 100, 0, 0)
		for i, id := range ids {
			var rec models.LLMChatRecord
			require.NoError(t, db.First(&rec, id).Error)
			if i == 1 || i == 2 {
				assert.InDelta(t, want, rec.Cost, 1e-9, "row at %s is in range", stamps[i])
			} else {
				assert.Zero(t, rec.Cost, "row at %s is out of range", stamps[i])
			}
		}
	})
}

// kq3b.9, the app budget views: /analytics/budget-usage-for-app and the
// portal usage summary report the currency the period's spend implies (EUR
// for EUR-only, USD for USD and rows with no currency), and for a mixed
// period the spend by currency with nothing added up. Enterprise only (CE
// tracks no spend and reports no currency); on SQLite and Postgres.
func TestPricingRegression_AppBudgetViewsImpliedCurrency(t *testing.T) {
	forEachPricingAPIDB(t, func(t *testing.T, db *gorm.DB) {
		service := services.NewService(db)
		api := NewAPI(service, true, apitest.SetupTestAuthService(db, service), apitest.SetupTestAuthConfig(db, service), nil, emptyFile, nil)
		user := createTestUser(t, service)
		cat := createTestCatalogue(t, service)
		addCatalogueToUserGroup(t, service, user.ID, cat.ID)
		llm := createTestLLM(t, service, "LLM")
		require.NoError(t, service.AddLLMToCatalogue(llm.ID, cat.ID))
		limit := 100.0
		apps := map[string]*models.App{}
		for _, name := range []string{"EUR", "USD", "Mixed"} {
			app, err := service.CreateApp(name, "", user.ID, nil, []uint{llm.ID}, nil, &limit, nil, nil)
			require.NoError(t, err)
			apps[name] = app
		}
		now := time.Now()
		for _, r := range []struct {
			app      string
			cost     float64
			currency string
		}{
			{"EUR", 20000, "EUR"}, {"EUR", 5000, "eur"},
			{"USD", 10000, ""}, {"USD", 20000, "USD"},
			{"Mixed", 10000, ""}, {"Mixed", 20000, "EUR"},
		} {
			require.NoError(t, db.Create(&models.LLMChatRecord{AppID: apps[r.app].ID, LLMID: llm.ID, Cost: r.cost, Currency: r.currency, TimeStamp: now.Add(-time.Minute)}).Error)
		}

		w := portalGet(t, api.getUserAppsUsageSummary, user)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var summary AppUsageSummaryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
		forApp := func(id uint) map[string]interface{} {
			w := portalGetQuery(t, api.getBudgetUsageForApp, user, fmt.Sprintf("app_id=%d", id))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var out map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
			return out
		}

		if !budget.IsEnterpriseAvailable() {
			assert.Empty(t, summary.Data[idOf(apps["EUR"].ID)].Currency, "CE reports no currency")
			return
		}
		for name, want := range map[string]struct {
			currency string
			spend    float64
		}{"EUR": {"EUR", 2.5}, "USD": {"USD", 3}} {
			s := summary.Data[idOf(apps[name].ID)]
			assert.True(t, strings.EqualFold(want.currency, s.Currency), "%s summary currency %q", name, s.Currency)
			assert.InDelta(t, want.spend, s.CurrentSpend, 1e-9, "%s summary spend", name)
			assert.Nil(t, s.MixedCurrencySpend)
			v := forApp(apps[name].ID)
			assert.True(t, strings.EqualFold(want.currency, fmt.Sprint(v["currency"])), "%s view currency %v", name, v["currency"])
			assert.Nil(t, v["mixed_currency_spend"])
		}
		m := summary.Data[idOf(apps["Mixed"].ID)]
		assert.Empty(t, m.Currency)
		assert.Zero(t, m.CurrentSpend, "a mixed period is not added up")
		assert.Equal(t, map[string]float64{"USD": 1, "EUR": 2}, m.MixedCurrencySpend)
		v := forApp(apps["Mixed"].ID)
		assert.Nil(t, v["currency"])
		assert.Equal(t, 0.0, v["current_usage"])
		assert.Equal(t, map[string]interface{}{"USD": 1.0, "EUR": 2.0}, v["mixed_currency_spend"])
	})
}
