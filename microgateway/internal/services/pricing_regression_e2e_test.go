//go:build !enterprise

// The Studio control server needs the Enterprise registrations in an
// enterprise build, which this module's test binary does not link; the CE
// build covers the hand-off, which is the same in both.

package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	studiogrpc "github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	studio "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// hubPulseClient hands the edge's analytics pulses straight to a Studio
// control server, in place of the gRPC connection.
type hubPulseClient struct {
	pb.ConfigurationSyncServiceClient
	hub *studiogrpc.ControlServer
}

func (c hubPulseClient) SendAnalyticsPulse(ctx context.Context, in *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	return c.hub.SendAnalyticsPulse(ctx, in)
}

type hubEdgeClient struct{ client hubPulseClient }

func (e hubEdgeClient) GetGRPCClient() pb.ConfigurationSyncServiceClient { return e.client }
func (e hubEdgeClient) GetEdgeID() string                               { return "edge-pricing" }
func (e hubEdgeClient) GetEdgeNamespace() string                        { return "" }

// hubDB is the Studio database for the end-to-end test: SQLite on one
// connection, or Postgres (DATABASE_URL) in a schema of its own.
func hubDB(t *testing.T, kind string) *gorm.DB {
	t.Helper()
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var db *gorm.DB
	var err error
	if kind == "sqlite" {
		db, err = gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"_hub?mode=memory&cache=shared"), cfg)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
	} else {
		dsn := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(dsn, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		admin, err := gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("hub_pricing_%d", time.Now().UnixNano())
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
		db, err = gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, models.InitModels(db))
	require.NoError(t, analytics.Migrate(db))
	return db
}

// kq3b.12/kq3b.13 end to end, from the edge: a chat record the edge's
// gateway produced (requested gemini-2.0-flash, reported -001, EUR, a sync
// row and a 0.5x batch row) goes through the edge analytics handler and the
// built-in analytics pulse plugin to a Studio control server. The hub's rows
// keep the requested model, multiplier and currency, so a recalculation of
// gemini-2.0-flash reprices them and the batch row stays at 0.5x through a
// zero price. Edge and hub both on SQLite, and both on Postgres
// (MGW_TEST_POSTGRES_DSN, DATABASE_URL).
func TestPricingRegression_EdgeChatRecordToHubRecalculation(t *testing.T) {
	for _, kind := range []string{"sqlite", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			hub := hubDB(t, kind)
			var edge *gorm.DB
			if kind == "sqlite" {
				edge, _, _ = setupManagementTestDB(t)
			} else {
				edge = openPostgresSchema(t)
			}

			analytics.ResetHandler()
			ctx, cancel := context.WithCancel(context.Background())
			analytics.InitDefault(ctx, hub)
			t.Cleanup(func() { cancel(); analytics.ResetHandler() })
			server, err := studiogrpc.NewControlServer(&studiogrpc.Config{GRPCHost: "localhost", AuthToken: "t", EncryptionKey: "12345678901234567890123456789012"}, hub)
			require.NoError(t, err)
			t.Cleanup(server.Stop)

			const vendor = "google_ai"
			svc := &studio.Service{DB: hub}
			p := models.ModelPrice{CPT: 0.4e-6, CPIT: 0.1e-6}
			price, err := svc.CreateModelPrice("gemini-2.0-flash", vendor, p.CPT, p.CPIT, 0, 0, "EUR")
			require.NoError(t, err)
			_, err = svc.GetModelPriceByModelNameAndVendor("gemini-2.0-flash-001", vendor) // unpriced reported row
			require.NoError(t, err)

			pm := plugins.NewPluginManager(nil)
			pm.SetEdgeClient(hubEdgeClient{hubPulseClient{hub: server}})
			require.NoError(t, pm.LoadDeferredBuiltinPlugins([]plugins.DataCollectionPluginConfig{{
				Name: "analytics_pulse", Enabled: true, HookTypes: []string{"analytics"},
				Config: map[string]interface{}{"interval_seconds": 1},
			}}))
			t.Cleanup(func() { pm.Shutdown(context.Background()) })
			handler := NewMicrogatewaAnalyticsHandler(edge, &config.AnalyticsConfig{Enabled: true, BufferSize: 10, FlushInterval: time.Second}, pm, nil)
			t.Cleanup(handler.Stop)

			list := models.ChatRecordCost(vendor, p.CPT, p.CPIT, 0, 0, 1000, 200, 0, 0)
			now := time.Now()
			for i, mult := range []float64{1, 0.5} {
				m := mult
				handler.RecordChatRecord(context.Background(), &models.LLMChatRecord{
					LLMID: 1, AppID: 1, Name: "gemini-2.0-flash-001", RequestedModel: "gemini-2.0-flash", Vendor: vendor,
					PromptTokens: 1000, ResponseTokens: 200, TotalTokens: 1200, Cost: list * m, PriceMultiplier: &m,
					Currency: "EUR", TimeStamp: now.Add(time.Duration(i) * time.Second),
				})
			}

			var recs []models.LLMChatRecord
			require.Eventually(t, func() bool {
				recs = nil
				return hub.Order("time_stamp asc").Find(&recs).Error == nil && len(recs) == 2
			}, 15*time.Second, 50*time.Millisecond, "the edge's rows reach the hub")
			for _, r := range recs {
				assert.Equal(t, "gemini-2.0-flash", r.RequestedModel)
				assert.Equal(t, "EUR", r.Currency)
				require.NotNil(t, r.PriceMultiplier)
			}

			reprice := func(f float64) {
				t.Helper()
				_, err := svc.UpdateModelPriceAndRecalculate(price.ID, "gemini-2.0-flash", vendor, p.CPT*f, p.CPIT*f, 0, 0, "EUR")
				require.NoError(t, err)
			}
			costs := func() (float64, float64) {
				t.Helper()
				require.NoError(t, hub.Order("time_stamp asc").Find(&recs).Error)
				return recs[0].Cost, recs[1].Cost
			}
			reprice(2)
			s, b := costs()
			assert.InEpsilon(t, 2*list, s, 1e-9, "the edge row is repriced by its requested name")
			assert.InEpsilon(t, list, b, 1e-9, "the batch row at 0.5x of the doubled price")
			reprice(0)
			reprice(1)
			s, b = costs()
			assert.InEpsilon(t, list, s, 1e-9, "sync row after p -> 0 -> p")
			assert.InEpsilon(t, 0.5*list, b, 1e-9, "batch row keeps 0.5x through a zero price")
		})
	}
}
