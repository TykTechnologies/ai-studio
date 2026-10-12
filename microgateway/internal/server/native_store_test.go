package server

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/models"
	aiservices "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The edge's native resource ledger is reachable through the gateway
// adapter (the way the proxy finds it) and its writes go through the
// container's writer handle, not the request-path pool.
func TestAttachNativeResourceStore_WritesThroughWriteDB(t *testing.T) {
	dbCfg := database.DatabaseConfig{Type: "sqlite", DSN: filepath.Join(t.TempDir(), "edge.db"), LogLevel: "silent"}
	db, err := database.Connect(dbCfg)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	writeDB, err := database.OpenWriter(dbCfg, db)
	require.NoError(t, err)
	require.NotSame(t, db, writeDB, "a file database gets its own writer")

	// Registered before the container starts its background goroutines
	// (registering later races them).
	var poolWrites, writerWrites atomic.Int32
	count := func(g *gorm.DB, n *atomic.Int32, name string) {
		require.NoError(t, g.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "native_resources" {
				n.Add(1)
			}
		}))
	}
	count(db, &poolWrites, "test:native_pool_writes")
	count(writeDB, &writerWrites, "test:native_writer_writes")

	cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: "test-encryption-key-32-chars-lng"}}
	sc, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)
	t.Cleanup(sc.Cleanup)
	sc.SetWriteDB(writeDB)

	adapter := services.NewGatewayServiceAdapter(sc.GatewayService, sc.Management, sc.AnalyticsService,
		sc.Crypto, sc.FilterService, sc.PluginService, sc.PluginManager, sc.DB)
	accessor, ok := adapter.(interface {
		NativeResourceStore() aiservices.NativeResourceStore
	})
	require.True(t, ok, "the adapter exposes the ledger to the proxy")
	assert.Nil(t, accessor.NativeResourceStore(), "no ledger until server.New attaches it")

	attachNativeResourceStore(adapter, sc)
	store := accessor.NativeResourceStore()
	require.NotNil(t, store)

	ctx := context.Background()
	r := &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: 1, AppID: 2,
		ResourceType: "anthropic.message_batch", VendorResourceID: "msgbatch_edge",
		AccountingState: models.NativeAccountingPending,
	}
	require.NoError(t, store.Create(ctx, r))
	got, err := store.GetByOwner(ctx, 1, 2, "anthropic.message_batch", "msgbatch_edge")
	require.NoError(t, err)
	assert.Equal(t, r.ID, got.ID)

	assert.Equal(t, int32(1), writerWrites.Load(), "the create went through WriteDB")
	assert.Equal(t, int32(0), poolWrites.Load(), "nothing was written through the request-path pool")
}
