package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
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
