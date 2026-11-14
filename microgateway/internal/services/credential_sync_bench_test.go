package services

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// The read every request makes with EDGE_CREDENTIAL_SYNC=full, against the
// on-demand edge's in-memory cache (bead midsommar-clean-4x5y.50). The
// database is SQLite as the gateway opens it (WAL, a pool of connections);
// requests run in parallel over 1,000 keys of 50 Apps.

const benchKeys = 1000

func benchEdgeDB(b *testing.B) *gorm.DB {
	b.Helper()
	cfg := database.DatabaseConfig{
		Type: "sqlite", DSN: "file:" + filepath.Join(b.TempDir(), "edge.db") + "?mode=rwc",
		MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent",
	}
	db, err := database.Connect(cfg)
	require.NoError(b, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	b.Cleanup(func() { database.Close(db) })
	require.NoError(b, database.Migrate(db))
	for i := 1; i <= 50; i++ {
		require.NoError(b, db.Create(&database.App{Model: gorm.Model{ID: uint(i)}, Name: fmt.Sprintf("app-%d", i), IsActive: true}).Error)
	}
	return db
}

func benchTokens() []string {
	tokens := make([]string, benchKeys)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("bench-token-%06d-%s", i, "0123456789abcdef0123456789abcdef")
	}
	return tokens
}

// quietLogs keeps the gateway's info level: debug lines would dominate.
func quietLogs(b *testing.B) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	b.Cleanup(func() { zerolog.SetGlobalLevel(prev) })
}

// runValidations validates tokens in parallel and reports the latency of a
// validation at p50 and p99 alongside ns/op.
func runValidations(b *testing.B, h *HybridGatewayService, tokens []string) {
	var mu sync.Mutex
	var all []time.Duration
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		local := make([]time.Duration, 0, 1<<16)
		i := 0
		for pb.Next() {
			i = (i + 7919) % len(tokens)
			start := time.Now()
			if _, err := h.ValidateAPIToken(tokens[i]); err != nil {
				b.Fatalf("validate %d: %v", i, err)
			}
			local = append(local, time.Since(start))
		}
		mu.Lock()
		all = append(all, local...)
		mu.Unlock()
	})
	b.StopTimer()
	slices.Sort(all)
	if len(all) > 0 {
		b.ReportMetric(float64(all[len(all)/2].Nanoseconds()), "p50-ns")
		b.ReportMetric(float64(all[len(all)*99/100].Nanoseconds()), "p99-ns")
	}
}

func BenchmarkValidateAPIToken(b *testing.B) {
	quietLogs(b)
	tokens := benchTokens()
	hs := config.HubSpokeConfig{TokenCacheEnabled: true, TokenCacheTTL: time.Hour, TokenCacheMaxSize: 10 * benchKeys, TokenCacheCleanupInt: time.Hour}

	b.Run("on_demand", func(b *testing.B) {
		db := benchEdgeDB(b)
		h := NewHybridGatewayService(db, database.NewRepository(db), "", hs, testCredentialHasher(b))
		b.Cleanup(h.Stop)
		h.SetEdgeClient(&fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: true, AppId: 1}})
		for _, tok := range tokens { // warm: the steady state is a cache hit
			_, err := h.ValidateAPIToken(tok)
			require.NoError(b, err)
		}
		runValidations(b, h, tokens)
	})

	b.Run("full_sync", func(b *testing.B) {
		db := benchEdgeDB(b)
		h := NewHybridGatewayService(db, database.NewRepository(db), "", hs, testCredentialHasher(b))
		b.Cleanup(h.Stop)
		list, err := NewCredentialSync(db, time.Hour)
		require.NoError(b, err)
		h.SetCredentialSync(list)
		h.SetEdgeClient(&fakeEdgeClient{err: fmt.Errorf("no control plane")})
		page := &CredentialSyncPage{Full: true, Last: true, Watermark: 1, HashVersion: "v1"}
		now := time.Now()
		for i, tok := range tokens {
			page.Credentials = append(page.Credentials, SyncedCredentialUpdate{CredentialID: uint(i + 1), SecretHash: h.cacheKey(tok), AppID: uint(i%50 + 1), Active: true, UpdatedAt: now})
		}
		for i := 1; i <= 50; i++ {
			page.Apps = append(page.Apps, pbApp(uint32(i), fmt.Sprintf("app-%d", i), true))
		}
		require.NoError(b, list.Sync(b.Context(), pagesOf{page}))
		for _, tok := range tokens { // as on demand: keys already in use
			_, err := h.ValidateAPIToken(tok)
			require.NoError(b, err)
		}
		runValidations(b, h, tokens)
	})
}

// BenchmarkCredentialLookupUnderWriters is the lookup alone, with and
// without the gateway's background writers on the same database: the
// analytics writer and the budget ledger flushing every 100 ms on their
// own connection, as the edge runs them, at 2,700 requests a second with
// 1 KB bodies. It reports the lookup's p50 and p99.
func BenchmarkCredentialLookupUnderWriters(b *testing.B) {
	quietLogs(b)
	for _, writers := range []bool{false, true} {
		name := "idle"
		if writers {
			name = "writers"
		}
		b.Run(name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "edge.db")
			cfg := database.DatabaseConfig{Type: "sqlite", DSN: "file:" + path + "?mode=rwc", MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent"}
			db, err := database.Connect(cfg)
			require.NoError(b, err)
			db.Logger = logger.Default.LogMode(logger.Silent)
			b.Cleanup(func() { database.Close(db) })
			require.NoError(b, database.Migrate(db))
			wdb, err := database.OpenWriter(cfg, db)
			require.NoError(b, err)
			wdb.Logger = logger.Default.LogMode(logger.Silent)
			b.Cleanup(func() { database.Close(wdb) })

			list, err := NewCredentialSync(db, time.Hour)
			require.NoError(b, err)
			page := &CredentialSyncPage{Full: true, Last: true, Watermark: 1, HashVersion: "v1"}
			hashes := make([]string, benchKeys)
			now := time.Now()
			for i := range hashes {
				hashes[i] = fmt.Sprintf("v1:%064x", i)
				page.Credentials = append(page.Credentials, SyncedCredentialUpdate{CredentialID: uint(i + 1), SecretHash: hashes[i], AppID: uint(i%50 + 1), Active: true, UpdatedAt: now})
			}
			require.NoError(b, list.Sync(b.Context(), pagesOf{page}))

			if writers {
				for i := 1; i <= 50; i++ {
					require.NoError(b, db.Create(&database.App{Model: gorm.Model{ID: uint(i)}, Name: fmt.Sprintf("app-%d", i), IsActive: true}).Error)
				}
				ledger := NewBudgetLedger(db)
				w := NewAnalyticsWriter(wdb, 20000, 1000, 100*time.Millisecond)
				w.SetLedger(ledger)
				w.Start()
				stop := make(chan struct{})
				done := make(chan struct{})
				go func() { // 2,700 a second, in ticks of 10 ms
					defer close(done)
					body := string(make([]byte, 1024))
					start, end := ledgerPeriod()
					tick := time.NewTicker(10 * time.Millisecond)
					defer tick.Stop()
					seq := 0
					for {
						select {
						case <-stop:
							return
						case <-tick.C:
						}
						for j := 0; j < 27; j++ {
							seq++
							app := uint(seq%50 + 1)
							w.Enqueue(&database.AnalyticsEvent{RequestID: fmt.Sprintf("r-%d", seq), AppID: app, TimeStamp: time.Now(), RequestBody: body, ResponseBody: body})
							ledger.Add(app, start, end, 100, 0.001, 60, 40)
						}
					}
				}()
				time.Sleep(time.Second) // writers in their stride
				b.Cleanup(func() { close(stop); <-done; w.Stop() })
			}

			var mu sync.Mutex
			var all []time.Duration
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				local := make([]time.Duration, 0, 1<<16)
				i := 0
				for pb.Next() {
					i = (i + 7919) % benchKeys
					t0 := time.Now()
					if _, found, err := list.Lookup(hashes[i]); err != nil || !found {
						b.Fatalf("lookup: found=%v err=%v", found, err)
					}
					local = append(local, time.Since(t0))
				}
				mu.Lock()
				all = append(all, local...)
				mu.Unlock()
			})
			b.StopTimer()
			slices.Sort(all)
			b.ReportMetric(float64(all[len(all)/2].Nanoseconds()), "p50-ns")
			b.ReportMetric(float64(all[len(all)*99/100].Nanoseconds()), "p99-ns")
			b.ReportMetric(float64(all[len(all)*999/1000].Nanoseconds()), "p999-ns")
		})
	}
}
