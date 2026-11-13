package grpc

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const lkgTestKey = "0123456789abcdef0123456789abcdef"

// configControl is a control plane that answers a registration with a
// configuration, as AI Studio does; hang makes it accept the connection and
// never answer.
type configControl struct {
	siteControl
	snapshot atomic.Pointer[pb.ConfigurationSnapshot]
	hang     atomic.Bool
}

func (c *configControl) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	if c.hang.Load() {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	resp, err := c.siteControl.RegisterEdge(ctx, req)
	if err == nil {
		resp.InitialConfig = c.snapshot.Load()
	}
	return resp, err
}

func (c *configControl) serveConfig(t *testing.T, addr string) (string, *grpc.Server) {
	t.Helper()
	var lis net.Listener
	var err error
	require.Eventually(t, func() bool {
		lis, err = net.Listen("tcp", addr)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "listen on %s", addr)
	srv := grpc.NewServer()
	pb.RegisterConfigurationSyncServiceServer(srv, c)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), srv
}

func lkgSnapshot(checksum string) *pb.ConfigurationSnapshot {
	now := time.Now()
	return &pb.ConfigurationSnapshot{
		Version:      checksum + "-version",
		Checksum:     checksum,
		SnapshotTime: timestamppb.New(now),
		Llms: []*pb.LLMConfig{{Id: 1, Name: "LLM " + checksum, Slug: "llm-" + checksum, Vendor: "openai", IsActive: true,
			CreatedAt: timestamppb.New(now.Add(-time.Hour)), UpdatedAt: timestamppb.New(now.Add(-time.Hour))}},
	}
}

// lkgEdgeDB opens (or reopens) an edge database file, with applied
// snapshots persisted as the hub-spoke container does on an edge.
func lkgEdgeDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	require.NoError(t, services.PersistAppliedSnapshots(db, lkgTestKey))
	t.Cleanup(func() { database.Close(db) })
	return db
}

func lkgClient(t *testing.T, db *gorm.DB, mutate func(*config.HubSpokeConfig), endpoints ...string) *SimpleEdgeClient {
	t.Helper()
	hs := config.HubSpokeConfig{
		EdgeID:                 "lkg-edge",
		EdgeNamespace:          "emea",
		ControlEndpoints:       endpoints,
		AllowInsecure:          true,
		HeartbeatInterval:      50 * time.Millisecond,
		ReconnectInterval:      50 * time.Millisecond,
		StartFromLastKnownGood: true,
		StartupControlTimeout:  300 * time.Millisecond,
		LastKnownGoodMaxAge:    72 * time.Hour,
	}
	if mutate != nil {
		mutate(&hs)
	}
	client := NewSimpleEdgeClient(&config.Config{HubSpoke: hs, Security: config.SecurityConfig{EncryptionKey: lkgTestKey}}, "test", "hash", "time")
	// Wired as cmd/microgateway wires it.
	sync := services.NewEdgeSyncService(db, hs.EdgeNamespace)
	client.RecordControlContactsIn(db)
	client.SetOnConfigChange(func(snap *pb.ConfigurationSnapshot) {
		if err := sync.SyncConfiguration(snap); err != nil {
			t.Logf("sync failed: %v", err)
		}
	})
	client.SetAppliedConfigSource(sync.ReportedConfig)
	t.Cleanup(func() { _ = client.Stop() })
	return client
}

func storedChecksum(t *testing.T, db *gorm.DB) string {
	t.Helper()
	stored, err := services.LoadAppliedSnapshot(db, lkgTestKey)
	if errors.Is(err, services.ErrNoStoredSnapshot) {
		return ""
	}
	require.NoError(t, err)
	return stored.Checksum
}

// With the option off an edge that finds no control plane fails as it
// always has: one attempt, Start's error, no look at the database.
func TestLastKnownGood_OffByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge.db")
	db := lkgEdgeDB(t, path)
	// Even with a usable snapshot stored.
	require.NoError(t, services.NewEdgeSyncService(db, "emea").SyncConfiguration(lkgSnapshot("stored")))

	client := lkgClient(t, db, func(hs *config.HubSpokeConfig) {
		hs.StartFromLastKnownGood = false
		hs.StartupControlTimeout = time.Hour // not used when the option is off
	}, closedAddr(t))
	started := time.Now()
	lkg, err := client.StartWithLastKnownGood(db)
	require.Error(t, err)
	assert.Nil(t, lkg)
	assert.Less(t, time.Since(started), 5*time.Second, "no waiting for a control plane when the option is off")
	var refusal *services.LastKnownGoodRefusal
	assert.False(t, errors.As(err, &refusal), "the error is the connection's, as before: %v", err)
	assert.Contains(t, err.Error(), "failed to")
	source, _ := client.ConfigSource()
	assert.Equal(t, ConfigSourceControl, source)
}

// With a control plane there, the option changes nothing: the edge
// connects, takes its configuration and records the contact.
func TestLastKnownGood_ConnectsWhenAControlPlaneAnswers(t *testing.T) {
	db := lkgEdgeDB(t, filepath.Join(t.TempDir(), "edge.db"))
	cp := &configControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("live"))
	addr, _ := cp.serveConfig(t, "127.0.0.1:0")

	client := lkgClient(t, db, nil, addr)
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.Nil(t, lkg, "connected: nothing to restore")
	source, contact := client.ConfigSource()
	assert.Equal(t, ConfigSourceControl, source)
	assert.WithinDuration(t, time.Now(), contact, 5*time.Second)
	at, ok, err := services.LastControlContact(db)
	require.NoError(t, err)
	require.True(t, ok, "the contact with the control plane is recorded in the edge's database")
	assert.WithinDuration(t, time.Now(), at, 5*time.Second)
}

// No control plane answers within EDGE_STARTUP_CONTROL_TIMEOUT: the edge
// starts from the snapshot it stored on its last run, keeps trying, and
// takes the live configuration from the first control plane that accepts
// it.
func TestLastKnownGood_StartsFromTheStoredSnapshotAndReconnects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge.db")
	cp := &configControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("first"))
	addr, srv := cp.serveConfig(t, "127.0.0.1:0")

	// First run: connected. The snapshot is applied and stored.
	db := lkgEdgeDB(t, path)
	first := lkgClient(t, db, nil, addr)
	lkg, err := first.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.Nil(t, lkg)
	first.onConfigChange(first.GetCurrentConfiguration()) // cmd/microgateway applies the initial configuration
	require.Equal(t, "first", storedChecksum(t, db))
	require.NoError(t, first.Stop())
	require.NoError(t, database.Close(db))

	// The control plane goes away; the edge restarts.
	srv.Stop()
	db2 := lkgEdgeDB(t, path)
	second := lkgClient(t, db2, nil, addr)
	started := time.Now()
	lkg, err = second.StartWithLastKnownGood(db2)
	require.NoError(t, err)
	require.NotNil(t, lkg, "the edge starts from its stored snapshot")
	waited := time.Since(started)
	assert.GreaterOrEqual(t, waited, 200*time.Millisecond, "it kept trying the control plane for about the start-up timeout first")
	assert.Less(t, waited, 5*time.Second)
	assert.Equal(t, "first", lkg.Checksum)
	assert.Equal(t, "emea", lkg.Namespace)
	require.NotNil(t, second.GetCurrentConfiguration())
	assert.Equal(t, "first", second.GetCurrentConfiguration().Checksum)
	source, contact := second.ConfigSource()
	assert.Equal(t, ConfigSourceLastKnownGood, source)
	assert.WithinDuration(t, time.Now(), contact, time.Minute, "the last contact is the one of the first run")
	assert.False(t, second.ControlLink().Connected)

	// cmd/microgateway applies the loaded configuration, then starts the
	// reconnect loop.
	rowBefore := appliedRow(t, db2)
	second.onConfigChange(second.GetCurrentConfiguration())
	sum, _ := second.loadedConfig()
	assert.Equal(t, "first", sum, "heartbeats will report the restored configuration")
	rowAfter := appliedRow(t, db2)
	assert.Equal(t, rowBefore.Snapshot, rowAfter.Snapshot, "restoring does not rewrite the stored row")
	second.StartReconnecting()

	// Still no control plane: it stays on last-known-good.
	time.Sleep(300 * time.Millisecond)
	source, _ = second.ConfigSource()
	assert.Equal(t, ConfigSourceLastKnownGood, source)

	// The control plane returns with a newer configuration.
	cp.snapshot.Store(lkgSnapshot("live"))
	cp.serveConfig(t, addr)
	require.Eventually(t, func() bool {
		source, _ := second.ConfigSource()
		return source == ConfigSourceControl && second.ControlLink().Connected
	}, 10*time.Second, 20*time.Millisecond, "the edge never reconnected")
	require.Eventually(t, func() bool { return storedChecksum(t, db2) == "live" }, 5*time.Second, 20*time.Millisecond,
		"the live snapshot replaces the stored one")
	sum, _ = second.loadedConfig()
	assert.Equal(t, "live", sum)
}

func appliedRow(t *testing.T, db *gorm.DB) database.AppliedSnapshot {
	t.Helper()
	var row database.AppliedSnapshot
	require.NoError(t, db.First(&row, database.AppliedSnapshotID).Error)
	return row
}

// Each refusal: the edge must exit, and the error says why.
func TestLastKnownGood_Refusals(t *testing.T) {
	dir := t.TempDir()
	seeded := func(t *testing.T, name, namespace string) (string, *gorm.DB) {
		path := filepath.Join(dir, name)
		db := lkgEdgeDB(t, path)
		require.NoError(t, services.NewEdgeSyncService(db, namespace).SyncConfiguration(lkgSnapshot("stored")))
		return path, db
	}
	start := func(t *testing.T, db *gorm.DB, mutate func(*config.HubSpokeConfig)) error {
		client := lkgClient(t, db, func(hs *config.HubSpokeConfig) {
			hs.StartupControlTimeout = 0 // one attempt
			if mutate != nil {
				mutate(hs)
			}
		}, closedAddr(t))
		lkg, err := client.StartWithLastKnownGood(db)
		require.Error(t, err)
		require.Nil(t, lkg)
		var refusal *services.LastKnownGoodRefusal
		require.True(t, errors.As(err, &refusal), "not a refusal: %v", err)
		assert.Contains(t, err.Error(), "no control plane answered", "the error also says what was tried")
		source, _ := client.ConfigSource()
		assert.Equal(t, ConfigSourceControl, source, "a refused edge is not marked as serving last-known-good")
		assert.Nil(t, client.GetCurrentConfiguration())
		return err
	}

	t.Run("no stored snapshot", func(t *testing.T) {
		db := lkgEdgeDB(t, filepath.Join(dir, "empty.db"))
		assert.Contains(t, start(t, db, nil).Error(), "no configuration snapshot is stored")
	})
	t.Run("stored for another namespace", func(t *testing.T) {
		_, db := seeded(t, "ns.db", "apac")
		err := start(t, db, nil) // this edge is in emea
		assert.Contains(t, err.Error(), `for namespace "apac"`)
		assert.Contains(t, err.Error(), `EDGE_NAMESPACE is "emea"`)
	})
	t.Run("does not decrypt", func(t *testing.T) {
		_, db := seeded(t, "key.db", "emea")
		client := lkgClient(t, db, func(hs *config.HubSpokeConfig) { hs.StartupControlTimeout = 0 }, closedAddr(t))
		client.config.Security.EncryptionKey = "fedcba9876543210fedcba9876543210"
		_, err := client.StartWithLastKnownGood(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not decrypt with this ENCRYPTION_KEY")
	})
	t.Run("without a control plane for too long", func(t *testing.T) {
		_, db := seeded(t, "old.db", "emea")
		err := start(t, db, func(hs *config.HubSpokeConfig) { hs.LastKnownGoodMaxAge = time.Nanosecond })
		assert.Contains(t, err.Error(), "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE")
	})
	t.Run("no limit with a max age of 0", func(t *testing.T) {
		_, db := seeded(t, "nolimit.db", "emea")
		client := lkgClient(t, db, func(hs *config.HubSpokeConfig) {
			hs.StartupControlTimeout = 0
			hs.LastKnownGoodMaxAge = 0
		}, closedAddr(t))
		lkg, err := client.StartWithLastKnownGood(db)
		require.NoError(t, err)
		require.NotNil(t, lkg)
	})
}

// StartWithin keeps trying for the time it is given: a control plane that
// appears inside it is used, and the stored snapshot is not.
func TestStartWithin_TakesAControlPlaneThatAppearsInTime(t *testing.T) {
	db := lkgEdgeDB(t, filepath.Join(t.TempDir(), "edge.db"))
	require.NoError(t, services.NewEdgeSyncService(db, "emea").SyncConfiguration(lkgSnapshot("stored")))
	cp := &configControl{}
	cp.name = "a"
	cp.snapshot.Store(lkgSnapshot("live"))
	addr := closedAddr(t)
	var once sync.Once
	time.AfterFunc(250*time.Millisecond, func() { once.Do(func() { cp.serveConfig(t, addr) }) })

	client := lkgClient(t, db, func(hs *config.HubSpokeConfig) { hs.StartupControlTimeout = 10 * time.Second }, addr)
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	assert.Nil(t, lkg, "a control plane answered in time")
	assert.Equal(t, "live", client.GetCurrentConfiguration().Checksum)
}

// A control plane that accepts the connection and never answers the
// registration does not hold the start-up past its limit.
func TestStartWithin_HangingControlPlaneIsBounded(t *testing.T) {
	db := lkgEdgeDB(t, filepath.Join(t.TempDir(), "edge.db"))
	require.NoError(t, services.NewEdgeSyncService(db, "emea").SyncConfiguration(lkgSnapshot("stored")))
	cp := &configControl{}
	cp.name = "hanging"
	cp.hang.Store(true)
	addr, _ := cp.serveConfig(t, "127.0.0.1:0")

	client := lkgClient(t, db, func(hs *config.HubSpokeConfig) { hs.StartupControlTimeout = 500 * time.Millisecond }, addr)
	started := time.Now()
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	require.NotNil(t, lkg, "the edge falls back to its stored snapshot")
	assert.Less(t, time.Since(started), 5*time.Second)
	conn, _ := client.connection()
	assert.Nil(t, conn, "no half-open connection is left as the current one")
}
