package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/aigateway"
	aiservices "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A batch an edge created and nobody downloaded is accounted by the edge's
// reconciler: it polls the vendor through the edge's own LLM config, reads
// the results and records the usage at batch price into the edge's analytics
// events (which the pulse sends to the hub) and its budget - once.
func TestNativeReconciler_EdgeAccountingReachesEventsAndBudget(t *testing.T) {
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "edge-vendor-key" || r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/messages/batches/msgbatch_EDGE":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msgbatch_EDGE","type":"message_batch","processing_status":"ended",` +
				`"created_at":"2026-10-12T10:00:00Z","ended_at":"2026-10-12T11:00:00Z",` +
				`"results_url":"https://api.anthropic.com/v1/messages/batches/msgbatch_EDGE/results"}`))
		case "/v1/messages/batches/msgbatch_EDGE/results":
			_, _ = w.Write([]byte(`{"custom_id":"a","result":{"type":"succeeded","message":{"id":"m1","type":"message","role":"assistant",` +
				`"model":"claude-luna","content":[],"usage":{"input_tokens":1500,"output_tokens":300,"service_tier":"batch"}}}}` + "\n" +
				`{"custom_id":"b","result":{"type":"errored","error":{"type":"error","error":{"type":"api_error","message":"x"}}}}` + "\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(vendor.Close)

	dbCfg := database.DatabaseConfig{Type: "sqlite", DSN: filepath.Join(t.TempDir(), "edge.db"), LogLevel: "silent"}
	db, err := database.Connect(dbCfg)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	writeDB, err := database.OpenWriter(dbCfg, db)
	require.NoError(t, err)

	cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: "test-encryption-key-32-chars-lng"}}
	sc, err := NewServiceContainer(db, cfg)
	require.NoError(t, err)
	t.Cleanup(sc.Cleanup)
	sc.SetWriteDB(writeDB)

	key, err := sc.Crypto.Encrypt("edge-vendor-key")
	require.NoError(t, err)
	llm := &database.LLM{Name: "Claude", Slug: "claude", Vendor: "anthropic", Endpoint: vendor.URL,
		APIKeyEncrypted: key, DefaultModel: "claude-luna", IsActive: true}
	require.NoError(t, db.Create(llm).Error)
	app := &database.App{Name: "Edge App", UserID: 9, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&database.ModelPrice{ModelName: "claude-luna", Vendor: "anthropic", CPT: 0.00001, CPIT: 0.000002, Currency: "USD"}).Error)

	budget := &usageRecorder{}
	analytics.SetHandler(NewMicrogatewaAnalyticsHandler(db, nil, nil, budget))
	t.Cleanup(func() { analytics.SetHandler(nil) })

	adapter := NewGatewayServiceAdapter(sc.GatewayService, sc.Management, sc.AnalyticsService,
		sc.Crypto, sc.FilterService, sc.PluginService, sc.PluginManager, sc.DB)
	store := aiservices.NewNativeResourceStore(sc.DB, writeDB)
	adapter.(interface {
		SetNativeResourceStore(aiservices.NativeResourceStore)
	}).SetNativeResourceStore(store)

	ctx := context.Background()
	row := &models.NativeResource{
		Vendor: "anthropic", LLMID: llm.ID, AppID: app.ID, UserID: 9,
		ResourceType: "anthropic.message_batch", VendorResourceID: "msgbatch_EDGE",
		State: "in_progress", AccountingState: models.NativeAccountingPending,
		OnBehalfOf: "user:alice", ActingAgent: "agent:bot",
	}
	require.NoError(t, store.Create(ctx, row))

	gw := aigateway.New(adapter, nil, &aigateway.Config{})
	require.NoError(t, gw.Reload())
	workers, ok := gw.(interface {
		StartNativeWorkers(context.Context, func() bool) func()
		WaitForAnalytics(context.Context) error
	})
	require.True(t, ok, "the edge gateway exposes the reconciler")
	stop := workers.StartNativeWorkers(ctx, nil) // the first pass runs at once

	// 1500 prompt + 300 completion tokens at half of the $2/$10 per MTok
	// price, in the gateway's ×10000 cost units.
	const batchCost = (1500*0.000002 + 300*0.00001) * 0.5 * 10000
	var events []database.AnalyticsEvent
	require.Eventually(t, func() bool {
		return db.Find(&events).Error == nil && len(events) == 1
	}, 5*time.Second, 20*time.Millisecond)
	stop()
	require.NoError(t, workers.WaitForAnalytics(ctx))

	ev := events[0]
	assert.Equal(t, app.ID, ev.AppID)
	assert.Equal(t, 1500, ev.PromptTokens)
	assert.Equal(t, 300, ev.ResponseTokens)
	assert.InDelta(t, batchCost, ev.Cost, 1e-9)
	assert.Equal(t, "user:alice", ev.OnBehalfOf)
	assert.Equal(t, "agent:bot", ev.ActingAgent)

	budget.mu.Lock()
	assert.Len(t, budget.costs, 1)
	if len(budget.costs) == 1 {
		assert.InDelta(t, batchCost, budget.costs[0], 1e-9)
	}
	budget.mu.Unlock()

	got, err := store.GetByOwner(ctx, llm.ID, app.ID, "anthropic.message_batch", "msgbatch_EDGE")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
	assert.Equal(t, "ended", got.State)
}
