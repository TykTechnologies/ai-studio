package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openEnforcingEdgeDB returns a migrated in-memory SQLite database that
// enforces foreign keys, as Postgres does. One connection, so the pragma holds.
func openEnforcingEdgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	return db
}

func syncLLM(id uint32, slug string, created time.Time) *pb.LLMConfig {
	return &pb.LLMConfig{
		Id: id, Name: "LLM " + slug, Slug: slug, Vendor: "openai", IsActive: true,
		CreatedAt: timestamppb.New(created), UpdatedAt: timestamppb.New(created),
	}
}

func syncApp(id uint32, llmIDs []uint32, created time.Time) *pb.AppConfig {
	return &pb.AppConfig{
		Id: id, Name: fmt.Sprintf("App %d", id), IsActive: true, LlmIds: llmIDs,
		CreatedAt: timestamppb.New(created), UpdatedAt: timestamppb.New(created),
	}
}

func snapshotAt(at time.Time, checksum string, llms []*pb.LLMConfig, apps []*pb.AppConfig) *pb.ConfigurationSnapshot {
	return &pb.ConfigurationSnapshot{
		Version:      fmt.Sprint(at.Unix()),
		Checksum:     checksum,
		SnapshotTime: timestamppb.New(at),
		Llms:         llms,
		Apps:         apps,
	}
}

func appVisible(t *testing.T, db *gorm.DB, id uint) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&database.App{}).Where("id = ?", id).Count(&n).Error)
	return n == 1
}

// A push must not lower the edge's spend to the control plane's figure: the
// snapshot's usage can be minutes old (it arrives with the analytics pulse),
// and taking it reopened budgets the edge had already spent.
func TestEdgeSyncNeverLowersSpend(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := NewEdgeSyncService(db, "")
	now := time.Now()
	start, end := calculateBudgetPeriod(nil, now)
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 10800}).Error)

	app := syncApp(1, nil, now.Add(-time.Hour))
	app.MonthlyBudget = 1
	app.CurrentPeriodUsage = 0 // the hub has not seen the edge's spend yet
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "c1", nil, []*pb.AppConfig{app})))

	var u database.BudgetUsage
	require.NoError(t, db.Where("app_id = ? AND period_start = ?", 1, start).First(&u).Error)
	assert.Equal(t, 10800.0, u.TotalCost, "a stale control-plane figure never lowers stored spend")

	app.CurrentPeriodUsage = 2 // the hub knows more: raise
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "c2", nil, []*pb.AppConfig{app})))
	require.NoError(t, db.Where("app_id = ? AND period_start = ?", 1, start).First(&u).Error)
	assert.Equal(t, 20000.0, u.TotalCost)
}

// The ledger's periodic re-read never lowers the spend it enforces.
func TestBudgetLedgerRefreshNeverLowers(t *testing.T) {
	db, _ := openWriterTestDB(t)
	start, end := ledgerPeriod()
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 10800}).Error)
	ledger := NewBudgetLedger(db)
	spent, err := ledger.Spent(1, start, end)
	require.NoError(t, err)
	require.Equal(t, 10800.0, spent)

	require.NoError(t, db.Model(&database.BudgetUsage{}).Where("app_id = ?", 1).Update("total_cost", 0).Error)
	ledger.refresh()
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 10800.0, spent)
}

// On a database that enforces foreign keys (Postgres), a resync used to fail
// once any analytics or budget row referenced an App or LLM: the sync deleted
// every App and LLM and re-inserted them. The failed sync left the edge
// refusing every App.
func TestEdgeSyncResyncWithReferencingRows(t *testing.T) {
	db := openEnforcingEdgeDB(t)
	testEdgeSyncResyncWithReferencingRows(t, db, 1)
}

func testEdgeSyncResyncWithReferencingRows(t *testing.T, db *gorm.DB, base uint32) {
	s := NewEdgeSyncService(db, "")
	now := time.Now()
	llmA, llmB := base, base+1
	appA, appB := base, base+1
	llms := []*pb.LLMConfig{syncLLM(llmA, fmt.Sprintf("fk-a-%d", base), now.Add(-time.Hour)), syncLLM(llmB, fmt.Sprintf("fk-b-%d", base), now.Add(-time.Hour))}
	apps := []*pb.AppConfig{syncApp(appA, []uint32{llmA}, now.Add(-time.Hour)), syncApp(appB, []uint32{llmB}, now.Add(-time.Hour))}
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, fmt.Sprintf("fk1-%d", base), llms, apps)))

	llmID := uint(llmB)
	start, end := calculateBudgetPeriod(nil, now)
	require.NoError(t, db.Create(&database.AnalyticsEvent{RequestID: fmt.Sprintf("fk-req-%d-%d", base, now.UnixNano()), AppID: uint(appB), LLMID: &llmID, TimeStamp: now}).Error)
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: uint(appB), PeriodStart: start, PeriodEnd: end, TotalCost: 5}).Error)

	// The same objects again: nothing may fail.
	require.NoError(t, s.SyncConfiguration(snapshotAt(now.Add(time.Minute), fmt.Sprintf("fk2-%d", base), llms, apps)))
	assert.True(t, appVisible(t, db, uint(appB)))

	// App B and LLM B deleted on the hub: they leave the edge's view, while
	// the rows that reference them stay.
	require.NoError(t, s.SyncConfiguration(snapshotAt(now.Add(2*time.Minute), fmt.Sprintf("fk3-%d", base), llms[:1], apps[:1])))
	assert.True(t, appVisible(t, db, uint(appA)))
	assert.False(t, appVisible(t, db, uint(appB)), "a deleted App is not served")
	var llm database.LLM
	assert.Error(t, db.First(&llm, llmB).Error, "a deleted LLM is not served")
	var n int64
	require.NoError(t, db.Model(&database.AnalyticsEvent{}).Where("app_id = ?", appB).Count(&n).Error)
	assert.EqualValues(t, 1, n, "analytics of a deleted App are kept for the pulse")

	// Both come back (the hub may reuse IDs): served again.
	require.NoError(t, s.SyncConfiguration(snapshotAt(now.Add(3*time.Minute), fmt.Sprintf("fk4-%d", base), llms, apps)))
	assert.True(t, appVisible(t, db, uint(appB)))
	require.NoError(t, db.First(&llm, llmB).Error)
	assert.Equal(t, fmt.Sprintf("fk-b-%d", base), llm.Slug)
	var grants int64
	require.NoError(t, db.Model(&database.AppLLM{}).Where("app_id = ? AND llm_id = ?", appB, llmB).Count(&grants).Error)
	assert.EqualValues(t, 1, grants)
}

// A snapshot taken before an App existed must not remove it: the edge learnt
// of the App from the hub (pull-on-miss) after the snapshot was built, and
// removing it made every request with its cached key answer 401 until the key
// was revalidated.
func TestEdgeSyncKeepsAppsNewerThanTheSnapshot(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := NewEdgeSyncService(db, "")
	snapTime := time.Now().Add(-10 * time.Second)
	llms := []*pb.LLMConfig{syncLLM(1, "fast", snapTime.Add(-time.Hour))}
	require.NoError(t, s.SyncConfiguration(snapshotAt(snapTime.Add(-time.Minute), "p1", llms, []*pb.AppConfig{syncApp(1, []uint32{1}, snapTime.Add(-time.Hour))})))

	// App 2 is created on the hub after the next snapshot is taken, and the
	// edge stores it when its key is first validated.
	h := &HybridGatewayService{DatabaseGatewayService: &DatabaseGatewayService{db: db}}
	require.NoError(t, h.storeAppFromPullOnMiss(syncApp(2, []uint32{1}, snapTime.Add(2*time.Second))))

	// The older snapshot lands afterwards.
	require.NoError(t, s.SyncConfiguration(snapshotAt(snapTime, "p2", llms, []*pb.AppConfig{syncApp(1, []uint32{1}, snapTime.Add(-time.Hour))})))
	assert.True(t, appVisible(t, db, 2), "an App newer than the snapshot stays")
	var grants int64
	require.NoError(t, db.Model(&database.AppLLM{}).Where("app_id = ?", 2).Count(&grants).Error)
	assert.EqualValues(t, 1, grants, "and keeps its grants")

	// A snapshot taken after the App was created, without it: the App was
	// deleted on the hub.
	require.NoError(t, s.SyncConfiguration(snapshotAt(time.Now(), "p3", llms, []*pb.AppConfig{syncApp(1, []uint32{1}, snapTime.Add(-time.Hour))})))
	assert.False(t, appVisible(t, db, 2))

	// Hubs that do not send snapshot_time: the version is the snapshot's
	// Unix time.
	require.NoError(t, h.storeAppFromPullOnMiss(syncApp(3, []uint32{1}, time.Now().Add(2*time.Second))))
	old := snapshotAt(time.Now(), "p4", llms, []*pb.AppConfig{syncApp(1, []uint32{1}, snapTime.Add(-time.Hour))})
	old.SnapshotTime = nil
	require.NoError(t, s.SyncConfiguration(old))
	assert.True(t, appVisible(t, db, 3))
}

// A retired App the hub vouches for again (token validation) is restored,
// not refused by its retired row.
func TestPullOnMissRestoresARetiredApp(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := NewEdgeSyncService(db, "")
	c := time.Now().Add(-time.Hour)
	llms := []*pb.LLMConfig{syncLLM(1, "fast", c)}
	require.NoError(t, s.SyncConfiguration(snapshotAt(time.Now(), "r1", llms, []*pb.AppConfig{syncApp(1, []uint32{1}, c)})))
	require.NoError(t, s.SyncConfiguration(snapshotAt(time.Now(), "r2", llms, nil)))
	require.False(t, appVisible(t, db, 1))

	h := &HybridGatewayService{DatabaseGatewayService: &DatabaseGatewayService{db: db}}
	require.NoError(t, h.storeAppFromPullOnMiss(syncApp(1, []uint32{1}, c)))
	assert.True(t, appVisible(t, db, 1))
}

// Two LLMs may swap slugs between snapshots, and a new LLM may take the slug
// of a deleted one; slugs are unique.
func TestEdgeSyncLLMSlugsMove(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := NewEdgeSyncService(db, "")
	now := time.Now()
	c := now.Add(-time.Hour)
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "s1", []*pb.LLMConfig{syncLLM(1, "a", c), syncLLM(2, "b", c)}, nil)))
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "s2", []*pb.LLMConfig{syncLLM(1, "b", c), syncLLM(2, "a", c)}, nil)))
	var llm database.LLM
	require.NoError(t, db.First(&llm, 1).Error)
	assert.Equal(t, "b", llm.Slug)

	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "s3", []*pb.LLMConfig{syncLLM(2, "a", c), syncLLM(3, "b", c)}, nil)))
	var bySlug database.LLM
	require.NoError(t, db.Where("slug = ?", "b").First(&bySlug).Error)
	assert.EqualValues(t, 3, bySlug.ID)
}

// An App or LLM the hub sends as inactive stays inactive on the edge (the
// column defaults to true).
func TestEdgeSyncKeepsInactiveFlags(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	s := NewEdgeSyncService(db, "")
	now := time.Now()
	app := syncApp(1, nil, now.Add(-time.Hour))
	app.IsActive = false
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "i1", nil, []*pb.AppConfig{app})))
	var got database.App
	require.NoError(t, db.First(&got, 1).Error)
	assert.False(t, got.IsActive)

	app.IsActive = true
	require.NoError(t, s.SyncConfiguration(snapshotAt(now, "i2", nil, []*pb.AppConfig{app})))
	require.NoError(t, db.First(&got, 1).Error)
	assert.True(t, got.IsActive)
}

// A reload applied the pulled snapshot twice, about 2 s apart: once from the
// config callback and again in the reload handler, which get the same
// snapshot. It is applied once. Another snapshot is always applied, even with
// the same checksum: the checksum leaves out Apps.
func TestEdgeSyncAppliesASnapshotOnce(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	now := time.Now()
	c := now.Add(-time.Hour)
	snap := snapshotAt(now, "same", nil, []*pb.AppConfig{syncApp(1, nil, c), syncApp(2, nil, c)})
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snap))

	gen := database.ConfigGeneration()
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snap))
	assert.Equal(t, gen, database.ConfigGeneration(), "the second application is skipped")

	withoutApp2 := snapshotAt(now, "same", nil, []*pb.AppConfig{syncApp(1, nil, c)})
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(withoutApp2))
	assert.NotEqual(t, gen, database.ConfigGeneration())
	assert.False(t, appVisible(t, db, 2), "an App deleted on the hub leaves although the checksum did not change")
}

// One App's budget usage that cannot be written must not hold back the
// others'. It used to fail every flush after it, so no App's spend was
// persisted until restart.
func TestAnalyticsWriterLedgerIsolatesUnwritableUsage(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_usage BEFORE INSERT ON budget_usage
		WHEN NEW.app_id IN (7, 999) BEGIN SELECT RAISE(ABORT, 'rejected usage'); END`).Error)
	require.NoError(t, db.Create(&database.App{Model: gorm.Model{ID: 7}, Name: "present"}).Error)
	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	_, err := ledger.Spent(7, start, end) // the budget check reads before it records
	require.NoError(t, err)
	ledger.Add(1, start, end, 10, 100, 5, 5)
	ledger.Add(7, start, end, 10, 70, 5, 5)   // an App the database refuses for another reason
	ledger.Add(999, start, end, 10, 90, 5, 5) // an App deleted meanwhile

	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Start()
	w.Stop()

	assert.Equal(t, 100.0, storedUsage(t, db, 1).TotalCost, "the writable App is written")

	f := ledger.take()
	pending := map[uint]float64{}
	for i, e := range f.entries {
		pending[e.appID] = f.amounts[i].cost
	}
	assert.Equal(t, map[uint]float64{7: 70}, pending, "a deleted App's usage is dropped; an existing App's is retried")
	ledger.finish(f, false)

	// An existing App's usage that keeps failing is given up on after a
	// while; the ledger still counts it towards the budget.
	ledger.now = func() time.Time { return time.Now().Add(ledgerGiveUpAfter + time.Minute) }
	f = ledger.take()
	ledger.finishEach(f, []bool{false})
	assert.True(t, ledger.take().empty(), "no longer retried")
	spent, err := ledger.Spent(7, start, end)
	require.NoError(t, err)
	assert.Equal(t, 70.0, spent, "still enforced")
	ledger.refresh()
	spent, _ = ledger.Spent(7, start, end)
	assert.Equal(t, 70.0, spent, "a re-read does not undo it")
}
