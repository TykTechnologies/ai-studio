package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fullStream is a control plane's full credential sync: n credentials over
// apps Apps, each with one LLM, in pages of pageSize, each page carrying the
// Apps of its credentials once. Every changedEvery-th App has a new budget
// (0: none).
type fullStream struct {
	n, apps, pageSize int
	hash              func(i int) string
	watermark         uint64
	appTime           time.Time
	changedEvery      int
}

func (f fullStream) SyncCredentials(_ context.Context, _ uint64, onPage func(*CredentialSyncPage) error) error {
	now := time.Now()
	ts := timestamppb.New(f.appTime)
	for start := 0; start < f.n; start += f.pageSize {
		end := min(start+f.pageSize, f.n)
		page := &CredentialSyncPage{Full: true, HashVersion: "v1", Last: end == f.n}
		seen := map[int]bool{}
		for i := start; i < end; i++ {
			app := i%f.apps + 1
			if !seen[app] {
				seen[app] = true
				a := &pb.AppConfig{Id: uint32(app), Name: fmt.Sprintf("app-%d", app), IsActive: true, OwnerEmail: "owner@example.com",
					UserId: 7, MonthlyBudget: 100, RateLimitRpm: 60, LlmIds: []uint32{1}, CreatedAt: ts, UpdatedAt: ts}
				if f.changedEvery > 0 && (app-1)%f.changedEvery == 0 {
					a.MonthlyBudget = 250
				}
				page.Apps = append(page.Apps, a)
			}
			page.Credentials = append(page.Credentials, SyncedCredentialUpdate{
				CredentialID: uint(i + 1), SecretHash: f.hash(i), AppID: uint(app), Active: true, UpdatedAt: now,
			})
		}
		if page.Last {
			page.Watermark = f.watermark
		}
		if err := onPage(page); err != nil {
			return err
		}
	}
	return nil
}

// What an edge does while it applies a full credential sync of 50,000
// credentials, each the key of its own App with one LLM (the shape the
// control plane's own measurement used, the most Apps a page carries), in
// pages of 500: how long it takes, what it costs in memory, and whether
// requests wait. Requests must not: keys are validated from the list while
// it is being replaced, the list as it was until each page commits. A repeat
// full sync with nothing changed writes no App; one with 1% of the Apps
// changed writes those only.
//
// It runs on a file-backed SQLite database in WAL mode, as an edge has, and
// on Postgres when MGW_TEST_POSTGRES_DSN is set. Skipped under -short.
func TestCredentialSync_FullSyncOf50000WhileValidating(t *testing.T) {
	if testing.Short() {
		t.Skip("applies 50,000 credentials twice")
	}
	const n, apps, pageSize = 50_000, 50_000, 500
	run := func(t *testing.T, db *gorm.DB) {
		h := NewHybridGatewayService(db, database.NewRepository(db), "", config.HubSpokeConfig{
			TokenCacheEnabled: false, // every validation goes to the list
		}, testCredentialHasher(t))
		t.Cleanup(h.Stop)
		list, err := NewCredentialSync(db, 24*time.Hour)
		require.NoError(t, err)
		h.SetCredentialSync(list)
		// Apps written at or after a moment.
		appsWrittenSince := func(at time.Time) int64 {
			var c int64
			require.NoError(t, db.Model(&database.App{}).Where("updated_at >= ?", at).Count(&c).Error)
			return c
		}
		h.SetEdgeClient(&fakeEdgeClient{err: fmt.Errorf("no control plane")})
		key := func(i int) string { return fmt.Sprintf("key-%038d", i) }
		stream := fullStream{n: n, apps: apps, pageSize: pageSize, hash: func(i int) string { return h.cacheKey(key(i)) }, watermark: 1, appTime: time.Now()}

		// The first full sync, on an empty list.
		var before runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		start := time.Now()
		require.NoError(t, list.Sync(context.Background(), stream))
		first := time.Since(start)
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		require.EqualValues(t, n, list.Count())

		// The statistics refresh each full sync ends with on PostgreSQL:
		// bounded by ANALYZE's sample, not by the table sizes.
		t0 := time.Now()
		refreshSyncStatistics(db)
		analyze := time.Since(t0)
		t.Logf("%s: statistics refresh after a full sync: %s", db.Dialector.Name(), analyze.Round(time.Millisecond))
		assert.Less(t, analyze, 2*time.Second, "ANALYZE after a full sync stays cheap")

		// The second, as every 15 minutes and after every reconnect to
		// another replica: the whole list again, while requests come in.
		var (
			stop      atomic.Bool
			wg        sync.WaitGroup
			mu        sync.Mutex
			latencies []time.Duration
			failed    atomic.Int64
		)
		for w := 0; w < 4; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := w; !stop.Load(); i += 7 {
					t0 := time.Now()
					if _, err := h.ValidateAPIToken(key(i % n)); err != nil {
						failed.Add(1)
					}
					d := time.Since(t0)
					mu.Lock()
					latencies = append(latencies, d)
					mu.Unlock()
				}
			}(w)
		}
		time.Sleep(100 * time.Millisecond)
		stream.watermark = 2
		start = time.Now()
		require.NoError(t, list.Sync(context.Background(), stream))
		second := time.Since(start)
		stop.Store(true)
		wg.Wait()
		assert.Zero(t, appsWrittenSince(start), "nothing changed: no App written")

		// 1% of the Apps changed (a budget edit): those are written, no other.
		stream.watermark, stream.changedEvery = 3, 100
		start = time.Now()
		require.NoError(t, list.Sync(context.Background(), stream))
		third := time.Since(start)
		assert.EqualValues(t, apps/100, appsWrittenSince(start))
		var budget float64
		require.NoError(t, db.Model(&database.App{}).Select("monthly_budget").Where("id = ?", 1).Scan(&budget).Error)
		assert.EqualValues(t, 250, budget)

		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		p := func(q float64) time.Duration { return latencies[int(float64(len(latencies)-1)*q)] }
		t.Logf("%s: first full sync of %d credentials (%d Apps, pages of %d): %s, %.0f MB allocated in total, heap after %.0f MB",
			db.Dialector.Name(), n, apps, pageSize, first.Round(time.Millisecond),
			float64(after.TotalAlloc-before.TotalAlloc)/1e6, float64(after.HeapAlloc)/1e6)
		t.Logf("%s: repeat full sync with 4 validating goroutines: %s; %d validations, %d failed; latency p50 %s, p99 %s, max %s",
			db.Dialector.Name(), second.Round(time.Millisecond), len(latencies), failed.Load(), p(0.5), p(0.99), latencies[len(latencies)-1])
		t.Logf("%s: repeat full sync with 1%% of the Apps changed: %s", db.Dialector.Name(), third.Round(time.Millisecond))

		// The per-request cost of the list at rest: no sync running, the
		// token cache off, so every validation reads the list (the hash's
		// row by its primary key) and the App.
		var rest []time.Duration
		for i := 0; i < 5000; i++ {
			t0 := time.Now()
			_, err := h.ValidateAPIToken(key((i * 37) % n))
			require.NoError(t, err)
			rest = append(rest, time.Since(t0))
		}
		sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
		t.Logf("%s: validation from the list at rest (no sync, no token cache): p50 %s, p99 %s, max %s over %d",
			db.Dialector.Name(), rest[len(rest)/2], rest[len(rest)*99/100], rest[len(rest)-1], len(rest))

		assert.Zero(t, failed.Load(), "a key on the list is accepted throughout a full sync")
		assert.EqualValues(t, n, list.Count())
		assert.Less(t, p(0.99), 250*time.Millisecond, "validation does not wait for the sync")
	}

	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		t.Cleanup(func() { database.Close(db) })
		require.NoError(t, database.Migrate(db))
		run(t, db)
	})
	if os.Getenv("MGW_TEST_POSTGRES_DSN") != "" {
		t.Run("postgres", func(t *testing.T) { run(t, openPostgresSchema(t)) })
	}
}
