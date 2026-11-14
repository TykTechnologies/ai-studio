package cluster

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// credential.changed across replicas (features/ActiveActive.md, Phase 4
// item 4): a credential changed through one replica reaches an edge held
// by another through the cluster relay, and the edge drops its cached
// answer at once instead of at its cache TTL.

// relayed starts the cluster event log and relay of replica r, as Studio
// does, so its edge-bound events reach the other replicas' edges.
func relayed(t *testing.T, r *replica) {
	t.Helper()
	log := cluster.NewLog(r.db, r.id, cluster.LogOptions{PollInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, log.Start(ctx))
	relay := cluster.NewRelay(log, r.server.GetEventBus(), cluster.RelayOptions{})
	relay.Start()
	t.Cleanup(func() {
		relay.Stop()
		log.Stop()
		cancel()
	})
}

// studioService is the part of replica r's Studio service the test uses:
// its credential writes, raising their events on r's bus. Built without
// NewService, which an Enterprise build of this package cannot run (it
// needs Enterprise registrations this test module does not link).
func studioService(r *replica) *coresvc.Service {
	return &coresvc.Service{DB: r.db, EventBus: r.server.GetEventBus()}
}

// onDemandEdge is an edge validating on demand with a token cache, wired as
// cmd/microgateway wires it, credential.changed included.
func onDemandEdge(t *testing.T, id string, fwd *forwarder) *services.HybridGatewayService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { database.Close(db) })
	hs := config.HubSpokeConfig{
		Mode: "edge", EdgeID: id, ControlEndpoints: []string{fwd.addr()},
		ReconnectInterval: 100 * time.Millisecond, HeartbeatInterval: 300 * time.Millisecond,
		SyncTimeout: 5 * time.Second, ClientToken: authToken, AllowInsecure: true,
		TokenCacheEnabled: true, TokenCacheTTL: 10 * time.Minute, TokenCacheMaxSize: 100, TokenCacheCleanupInt: time.Hour,
	}
	client := edgegrpc.NewSimpleEdgeClient(&config.Config{Security: config.SecurityConfig{EncryptionKey: encryptionKey}, HubSpoke: hs}, "e2e", "hash", "now")
	require.NoError(t, client.Start())
	t.Cleanup(func() { _ = client.Stop() })
	hasher, err := credhash.New(encryptionKey)
	require.NoError(t, err)
	h := services.NewHybridGatewayService(db, database.NewRepository(db), "", hs, hasher)
	t.Cleanup(h.Stop)
	h.SetEdgeClient(client)
	client.GetEventBus().Subscribe(eventbridge.CredentialChangedTopic, h.HandleCredentialChanged)
	return h
}

const revokedKey = "7e57c0ffee7e57c0ffee7e57c0ffee7e57c0ffee7e57c0ffee7e57c0ffee7e57"

// revocableKey creates an App with an active key on the hub.
func revocableKey(t *testing.T, db *gorm.DB, secret string) *models.App {
	t.Helper()
	cred := &models.Credential{KeyID: "revocable-key-id", Secret: secret, Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "revocable", IsActive: true, CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)
	return app
}

// A key deactivated through replica B is refused by an edge held by
// replica A within 2 seconds, although the edge had cached it as valid for
// ten minutes. On SQLite (one process, no cluster event log) the change is
// made through the edge's own replica.
func TestE2E_CredentialChanged_RevocationReachesAnEdgeOfAnotherReplica(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		coresvc.SetEdgeCredentialHasher(encryptionKey)
		t.Cleanup(func() { coresvc.SetEdgeCredentialHasher("") })
		a := startReplica(t, sdb, "node-a")
		writer := a
		if sdb.postgres {
			writer = startReplica(t, sdb, "node-b")
			relayed(t, a)
			relayed(t, writer)
		}
		app := revocableKey(t, a.db, revokedKey)
		fwd := newForwarder(t, a.addr)
		edge := onDemandEdge(t, "edge-1", fwd)
		waitOwner(t, a.db, "edge-1", "node-a")

		_, err := edge.ValidateAPIToken(revokedKey)
		require.NoError(t, err, "valid, and cached for ten minutes")

		start := time.Now()
		require.NoError(t, studioService(writer).DeactivateCredential(app.CredentialID))
		require.Eventually(t, func() bool {
			_, err := edge.ValidateAPIToken(revokedKey)
			return err != nil
		}, 2*time.Second, 20*time.Millisecond, "refused within 2 s of the deactivation")
		t.Logf("%s: refused %s after the deactivation through %s", sdb.name, time.Since(start).Round(time.Millisecond), writer.id)

		var cred models.Credential
		require.NoError(t, a.db.First(&cred, app.CredentialID).Error)
		assert.False(t, cred.Active)
	})
}

// With full credential sync, a key deactivated through replica B is
// refused from the list of an edge held by replica A within 2 seconds: the
// event starts a sync at once. A lost event (here: Studio raises none) is
// repaired by the next sync, which the edge runs on every heartbeat.
func TestE2E_CredentialChanged_FullSyncEdgeOfAnotherReplica(t *testing.T) {
	requireSyncHandler(t)
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		if !sdb.postgres {
			t.Skip("two replicas need the cluster event log (PostgreSQL)")
		}
		coresvc.SetEdgeCredentialHasher(encryptionKey)
		t.Cleanup(func() { coresvc.SetEdgeCredentialHasher("") })
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		relayed(t, a)
		relayed(t, b)
		one := hubKey(t, a.db, "one", key(1), true)
		two := hubKey(t, a.db, "two", key(2), true)
		fwd := newForwarder(t, a.addr)
		e := runSyncEdge(t, filepath.Join(t.TempDir(), "edge.db"), "edge-1", syncEdgeOptions{}, fwd)
		waitOwner(t, a.db, "edge-1", "node-a")
		e.waitSynced(t, 2)

		start := time.Now()
		require.NoError(t, studioService(b).DeactivateCredential(one.CredentialID))
		require.Eventually(t, func() bool {
			_, err := e.h.ValidateAPIToken(key(1))
			return err != nil
		}, 2*time.Second, 20*time.Millisecond, "refused within 2 s of the deactivation through node-b")
		t.Logf("full sync: refused %s after the deactivation", time.Since(start).Round(time.Millisecond))

		// No event this time: the heartbeat's sync repairs it.
		coresvc.SetEdgeCredentialHasher("")
		start = time.Now()
		require.NoError(t, studioService(b).DeactivateCredential(two.CredentialID))
		require.Eventually(t, func() bool {
			_, err := e.h.ValidateAPIToken(key(2))
			return err != nil
		}, 5*time.Second, 20*time.Millisecond, "refused after the next heartbeat's sync")
		t.Logf("lost event: refused %s after the deactivation", time.Since(start).Round(time.Millisecond))
	})
}
