package services

import (
	"crypto/sha256"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openCountingDB returns a migrated in-memory database with the config
// generation callbacks the gateway registers, and a counter of SELECTs per
// table.
func openCountingDB(t *testing.T) (*gorm.DB, func(table string) int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, database.EnsureConfigGenerationCallbacks(db))
	require.NoError(t, database.Migrate(db))

	counts := map[string]*atomic.Int64{}
	for _, table := range []string{"access_tokens", "model_prices", "apps"} {
		counts[table] = &atomic.Int64{}
	}
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_selects", func(tx *gorm.DB) {
		if c, ok := counts[tx.Statement.Table]; ok {
			c.Add(1)
		}
	}))
	return db, func(table string) int64 { return counts[table].Load() }
}

func newCachingAdapter(db *gorm.DB) *GatewayServiceAdapter {
	return &GatewayServiceAdapter{
		db:                  db,
		management:          NewManagementService(db, database.NewRepository(db), nil),
		modelPrices:         database.NewGenCache[string, models.ModelPrice](),
		accessTokensPresent: database.NewGenCache[struct{}, bool](),
	}
}

// Every Bearer request is tried as an OAuth token first. An edge without
// OAuth tokens must answer that from the cache, and a sync that adds a token
// must make the lookup find it.
func TestAccessTokenLookupSkippedWhenEdgeHasNone(t *testing.T) {
	db, selects := openCountingDB(t)
	a := newCachingAdapter(db)

	for i := 0; i < 5; i++ {
		_, err := a.GetValidAccessTokenByToken(fmt.Sprintf("app-secret-%d", i))
		require.Error(t, err)
	}
	assert.EqualValues(t, 1, selects("access_tokens"), "only the presence check should reach the database")

	h := sha256.Sum256([]byte("oauth-token"))
	require.NoError(t, db.Create(&database.AccessTokenEdge{
		TokenHash: fmt.Sprintf("%x", h[:]), TokenEncrypted: "x", ClientID: "c", UserID: 1, AppID: 7,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	at, err := a.GetValidAccessTokenByToken("oauth-token")
	require.NoError(t, err)
	require.NotNil(t, at.AppID)
	assert.EqualValues(t, 7, *at.AppID)

	_, err = a.GetValidAccessTokenByToken("app-secret-x")
	assert.Error(t, err)
}

// The price is read after every response; a missing price must be cached as
// the zero price too, since the model name comes from the response.
func TestModelPriceCachedIncludingMissing(t *testing.T) {
	db, selects := openCountingDB(t)
	a := newCachingAdapter(db)
	require.NoError(t, db.Create(&database.ModelPrice{ModelName: "gpt-x", Vendor: "openai", CPT: 0.002, CPIT: 0.001, Currency: "USD"}).Error)
	before := selects("model_prices")

	for i := 0; i < 5; i++ {
		p, err := a.GetModelPriceByModelNameAndVendor("gpt-x", "openai")
		require.NoError(t, err)
		assert.Equal(t, 0.002, p.CPT)
		p.CPT = 99 // callers get their own copy

		z, err := a.GetModelPriceByModelNameAndVendor("unpriced", "openai")
		require.NoError(t, err)
		assert.Zero(t, z.CPT)
		assert.Equal(t, "USD", z.Currency)
	}
	assert.EqualValues(t, 2, selects("model_prices")-before, "one read per model")

	require.NoError(t, db.Model(&database.ModelPrice{}).Where("model_name = ?", "gpt-x").Update("cpt", 0.005).Error)
	p, err := a.GetModelPriceByModelNameAndVendor("gpt-x", "openai")
	require.NoError(t, err)
	assert.Equal(t, 0.005, p.CPT, "a price change invalidates the cache")
}

// Recording a proxy log and its chat record keeps no timer goroutine per
// request; expired entries are swept on a later store.
func TestPendingEventsSweptWithoutGoroutines(t *testing.T) {
	h := &MicrogatewaAnalyticsHandler{pendingEvents: map[string]pendingEvent{}}

	h.storeEventForMatching("a", 1)
	id, ok := h.findEventForMerge("a")
	require.True(t, ok)
	assert.EqualValues(t, 1, id)
	_, ok = h.findEventForMerge("a")
	assert.False(t, ok, "a pending event merges once")

	h.pendingEvents["old"] = pendingEvent{id: 2, at: time.Now().Add(-2 * pendingEventTTL)}
	_, ok = h.findEventForMerge("old")
	assert.False(t, ok, "expired events do not merge")

	h.lastPendingSweep = time.Time{}
	h.storeEventForMatching("b", 3)
	_, present := h.pendingEvents["old"]
	assert.False(t, present, "expired events are swept")
}
