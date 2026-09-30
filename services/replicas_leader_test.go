package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

type testLeadership struct{ leader atomic.Bool }

func (l *testLeadership) IsLeader() bool                       { return l.leader.Load() }
func (l *testLeadership) Signal(context.Context, string) error { return nil }

// One telemetry report per deployment: replicas that do not hold the
// leader lease send none.
func TestTelemetryManager_OnlyTheLeaderReports(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldURL := telemetryEndpoint
	telemetryEndpoint = server.URL
	t.Cleanup(func() { telemetryEndpoint = oldURL })

	lead := &testLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	_, tm := setupTelemetryManagerTest(t)
	tm.collectAndSend()
	assert.Zero(t, hits.Load(), "a follower does not report")

	lead.leader.Store(true)
	tm.collectAndSend()
	assert.Equal(t, int32(1), hits.Load(), "the leader reports")
}

func fileDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "s.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// A replica that is not the leader when it starts (a restarted process
// whose crashed predecessor held the lease a moment longer) sends its first
// report when it becomes the leader, once, rather than an hour later.
func TestTelemetryManager_FirstReportWhenItBecomesLeader(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldURL := telemetryEndpoint
	telemetryEndpoint = server.URL
	t.Cleanup(func() { telemetryEndpoint = oldURL })

	lead := &testLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	tm := NewTelemetryManager(fileDB(t), true, "1.0.0-test")
	tm.Start()
	t.Cleanup(tm.Stop)
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, hits.Load(), "a follower does not report")

	lead.leader.Store(true)
	replicas.BecameLeader()
	require.Eventually(t, func() bool { return hits.Load() == 1 }, 5*time.Second, 10*time.Millisecond, "reports on becoming the leader")

	replicas.BecameLeader() // leadership lost and regained: no extra report
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(1), hits.Load())
}

// The marketplace's start-up sync runs when the replica becomes the leader
// if it was skipped at start, instead of after MARKETPLACE_SYNC_INTERVAL
// (an hour by default).
func TestMarketplaceService_SyncsWhenItBecomesLeader(t *testing.T) {
	var fetches atomic.Int32
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		http.NotFound(w, r)
	}))
	defer index.Close()

	lead := &testLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	ms := NewMarketplaceService(fileDB(t), nil, nil, nil, t.TempDir(), index.URL+"/index.yaml", time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); ms.Start(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, fetches.Load(), "a follower does not sync")

	lead.leader.Store(true)
	replicas.BecameLeader()
	require.Eventually(t, func() bool { return fetches.Load() >= 1 }, 5*time.Second, 10*time.Millisecond, "syncs on becoming the leader")
}
