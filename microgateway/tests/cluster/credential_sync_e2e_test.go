package cluster

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/pkg/credsync"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// Full credential sync between real control-plane replicas and a real edge
// (EDGE_CREDENTIAL_SYNC=full, Enterprise; features/ActiveActive.md,
// Phase 4). The control plane's handler is Enterprise: these tests run when
// it is registered (enterprise build with the credential_sync package, see
// testmain_credsync_enterprise_test.go) and skip otherwise.

func requireSyncHandler(t *testing.T) {
	t.Helper()
	if credsync.Registered() == nil {
		t.Skip("no SyncCredentials handler in this build: needs -tags 'enterprise credsync_e2e' and the Enterprise credential_sync package")
	}
}

// syncEdge is an edge with full credential sync, wired as cmd/microgateway
// wires it: the edge client, the hybrid gateway service validating from the
// synced list, and the list syncing on every contact.
type syncEdge struct {
	db     *gorm.DB
	client *edgegrpc.SimpleEdgeClient
	h      *services.HybridGatewayService
	list   *services.CredentialSync
	stop   context.CancelFunc
	once   sync.Once
}

type syncEdgeOptions struct {
	maxAge        time.Duration
	lastKnownGood bool
}

func runSyncEdge(t *testing.T, path, id string, opts syncEdgeOptions, fwds ...*forwarder) *syncEdge {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	if opts.maxAge == 0 {
		opts.maxAge = 24 * time.Hour
	}
	hs := config.HubSpokeConfig{
		Mode: "edge", EdgeID: id,
		ReconnectInterval: 100 * time.Millisecond, HeartbeatInterval: 300 * time.Millisecond,
		SyncTimeout: 5 * time.Second, ClientToken: authToken, AllowInsecure: true,
		StartFromLastKnownGood: opts.lastKnownGood, StartupControlTimeout: 500 * time.Millisecond, LastKnownGoodMaxAge: 72 * time.Hour,
		TokenCacheEnabled: true, TokenCacheTTL: time.Minute, TokenCacheMaxSize: 100, TokenCacheCleanupInt: time.Hour,
		CredentialSync: config.CredentialSyncFull, CredentialSyncMaxAge: opts.maxAge,
	}
	for _, f := range fwds {
		hs.ControlEndpoints = append(hs.ControlEndpoints, f.addr())
	}
	cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: encryptionKey}, HubSpoke: hs}
	require.NoError(t, services.PersistAppliedSnapshots(db, encryptionKey))

	hasher, err := credhash.New(encryptionKey)
	require.NoError(t, err)
	h := services.NewHybridGatewayService(db, database.NewRepository(db), "", hs, hasher)
	list, err := services.NewCredentialSync(db, opts.maxAge)
	require.NoError(t, err)
	h.SetCredentialSync(list)

	client := edgegrpc.NewSimpleEdgeClient(cfg, "e2e", "hash", "now")
	client.SetRegisteredHook(list.Reset)
	client.SetControlContactHook(func(time.Time) { list.Kick() })
	syncService := services.NewEdgeSyncService(db, "")
	client.SetOnConfigChange(func(snap *pb.ConfigurationSnapshot) { _ = syncService.SyncConfiguration(snap) })
	lkg, err := client.StartWithLastKnownGood(db)
	require.NoError(t, err)
	if initial := client.GetCurrentConfiguration(); initial != nil {
		require.NoError(t, syncService.SyncConfiguration(initial))
	}
	if lkg != nil {
		client.StartReconnecting()
	}
	h.SetEdgeClient(client)
	client.GetEventBus().Subscribe(eventbridge.CredentialChangedTopic, h.HandleCredentialChanged)
	ctx, cancel := context.WithCancel(context.Background())
	go list.Run(ctx, client, time.Minute)
	list.Kick()
	e := &syncEdge{db: db, client: client, h: h, list: list, stop: cancel}
	t.Cleanup(func() { e.close() })
	return e
}

func (e *syncEdge) close() {
	e.once.Do(func() {
		e.stop()
		_ = e.client.Stop()
		e.h.Stop()
		_ = database.Close(e.db)
	})
}

func (e *syncEdge) waitSynced(t *testing.T, count int64) {
	t.Helper()
	require.Eventually(t, func() bool { return e.list.Count() == count && !e.list.LastSuccess().IsZero() },
		20*time.Second, 25*time.Millisecond, "the list holds %d usable credentials (has %d)", count, e.list.Count())
}

// hubKey creates an App with its key on the hub.
func hubKey(t *testing.T, db *gorm.DB, name, key string, active bool) *models.App {
	t.Helper()
	cred := &models.Credential{KeyID: name + "-key-id", Secret: key, Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: name, IsActive: true, CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)
	if !active {
		require.NoError(t, db.Model(&models.Credential{}).Where("id = ?", cred.ID).Update("active", false).Error)
	}
	return app
}

func key(n int) string { return fmt.Sprintf("%064x", 0xc0ffee00+n) }

// With every control plane cut, a key this edge has never served is
// accepted from the synced list, an inactive one is refused, and one the
// list does not have cannot be checked (503). A key created on the hub
// while the edge was cut off is accepted after it reconnects, from the
// list: no on-demand call. Past EDGE_CREDENTIAL_SYNC_MAX_AGE the edge
// stops trusting the list and asks, as on demand, until a sync succeeds.
func TestE2E_CredentialSync_ValidatesWithEveryControlPlaneCut(t *testing.T) {
	requireSyncHandler(t)
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		hubKey(t, a.db, "served", key(1), true)
		hubKey(t, a.db, "paused", key(2), false)
		fwdA, fwdB := newForwarder(t, a.addr), newForwarder(t, b.addr)
		const maxAge = 3 * time.Second
		e := runSyncEdge(t, filepath.Join(t.TempDir(), "edge.db"), "edge-1", syncEdgeOptions{maxAge: maxAge}, fwdA, fwdB)
		e.waitSynced(t, 1)

		fwdA.fail()
		fwdB.fail()
		before := e.list.Validations()
		res, err := e.h.ValidateAPIToken(key(1))
		require.NoError(t, err, "a key never served, accepted from the list")
		assert.NotZero(t, res.AppID)
		_, err = e.h.ValidateAPIToken(key(2))
		require.Error(t, err, "an inactive key is refused")
		assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "refused, not unavailable")
		_, err = e.h.ValidateAPIToken(key(99))
		require.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "an unknown key cannot be checked: 503")
		after := e.list.Validations()
		assert.Equal(t, before[services.ValidationLocal]+2, after[services.ValidationLocal])

		// Created while the edge is cut off; accepted after reconnect
		// without asking the control plane about it.
		hubKey(t, b.db, "created-offline", key(3), true)
		fwdB.setUp()
		e.waitSynced(t, 2)
		before = e.list.Validations()
		_, err = e.h.ValidateAPIToken(key(3))
		require.NoError(t, err)
		after = e.list.Validations()
		assert.Equal(t, before[services.ValidationLocal]+1, after[services.ValidationLocal], "from the list")
		assert.Equal(t, before[services.ValidationControl], after[services.ValidationControl], "no on-demand call")

		// Past the max age with no sync, the list is not trusted: the edge
		// asks, and with no control plane that is a 503.
		fwdB.fail()
		require.Eventually(t, func() bool { return !e.list.Trusted(time.Now()) }, maxAge+5*time.Second, 50*time.Millisecond)
		_, err = e.h.ValidateAPIToken(key(1))
		require.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "as on demand once the list is too old")
		assert.NotZero(t, e.list.Validations()[services.ValidationStale])
		fwdB.setUp()
		require.Eventually(t, func() bool { return e.list.Trusted(time.Now()) }, 20*time.Second, 25*time.Millisecond, "trusted again after a sync")
		fwdB.fail()
		_, err = e.h.ValidateAPIToken(key(1))
		require.NoError(t, err, "from the list again")
	})
}

// An edge restarted with every control plane cut and last-known-good on
// validates from the list it stored.
func TestE2E_CredentialSync_RestartWithEveryControlPlaneCut(t *testing.T) {
	requireSyncHandler(t)
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		hubKey(t, a.db, "served", key(1), true)
		fwd := newForwarder(t, a.addr)
		path := filepath.Join(t.TempDir(), "edge.db")
		first := runSyncEdge(t, path, "edge-1", syncEdgeOptions{lastKnownGood: true}, fwd)
		first.waitSynced(t, 1)
		require.Eventually(t, func() bool {
			_, err := services.LoadAppliedSnapshot(first.db, encryptionKey)
			return err == nil
		}, 10*time.Second, 25*time.Millisecond, "a snapshot is stored")
		first.close()

		fwd.fail()
		second := runSyncEdge(t, path, "edge-1", syncEdgeOptions{lastKnownGood: true}, fwd)
		assert.EqualValues(t, 1, second.list.Count(), "the stored list")
		_, err := second.h.ValidateAPIToken(key(1))
		require.NoError(t, err, "a synced key is accepted after a restart with no control plane")
		_, err = second.h.ValidateAPIToken(key(99))
		require.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable)
	})
}

// The edge's database holds no key it could give back: every text column
// of every table is searched for the keys the test used.
func TestE2E_CredentialSync_EdgeDatabaseHoldsNoKey(t *testing.T) {
	requireSyncHandler(t)
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		keys := []string{key(1), key(2), key(3)}
		hubKey(t, a.db, "one", keys[0], true)
		hubKey(t, a.db, "two", keys[1], false)
		hubKey(t, a.db, "three", keys[2], true)
		fwd := newForwarder(t, a.addr)
		e := runSyncEdge(t, filepath.Join(t.TempDir(), "edge.db"), "edge-1", syncEdgeOptions{}, fwd)
		e.waitSynced(t, 2)
		for _, k := range keys {
			_, _ = e.h.ValidateAPIToken(k) // and through validation
		}

		var tables []string
		require.NoError(t, e.db.Raw("SELECT name FROM sqlite_master WHERE type = 'table'").Scan(&tables).Error)
		scanned := 0
		for _, table := range tables {
			rows, err := e.db.Raw(`SELECT * FROM "` + table + `"`).Rows()
			require.NoError(t, err)
			cols, err := rows.Columns()
			require.NoError(t, err)
			for rows.Next() {
				vals := make([]interface{}, len(cols))
				ptrs := make([]interface{}, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				require.NoError(t, rows.Scan(ptrs...))
				for i, v := range vals {
					var s string
					switch x := v.(type) {
					case string:
						s = x
					case []byte:
						s = string(x)
					default:
						continue
					}
					scanned++
					for _, k := range keys {
						assert.NotContains(t, s, k, "table %s column %s holds a key", table, cols[i])
						assert.False(t, strings.Contains(strings.ToLower(s), strings.ToLower(k)), "table %s column %s", table, cols[i])
					}
				}
			}
			require.NoError(t, rows.Close())
		}
		assert.NotZero(t, scanned)
	})
}

// A control plane without the sync (an older one, or Community) answers
// Unimplemented: the edge says so and validates on demand.
func TestE2E_CredentialSync_ControlPlaneWithoutTheSync(t *testing.T) {
	requireSyncHandler(t)
	registered := credsync.Registered()
	credsync.Register(nil)
	t.Cleanup(func() { credsync.Register(registered) })
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		hubKey(t, a.db, "served", key(1), true)
		fwd := newForwarder(t, a.addr)
		e := runSyncEdge(t, filepath.Join(t.TempDir(), "edge.db"), "edge-1", syncEdgeOptions{}, fwd)
		waitOwner(t, a.db, "edge-1", "node-a")
		require.Eventually(t, func() bool { return e.list.Unavailable() }, 10*time.Second, 25*time.Millisecond, "the edge saw Unimplemented")
		assert.Zero(t, e.list.Count())
		res, err := e.h.ValidateAPIToken(key(1))
		require.NoError(t, err, "validated on demand")
		assert.NotZero(t, res.AppID)
		// A list that never synced is not trusted: counted as "stale", the
		// on-demand fallback.
		assert.NotZero(t, e.list.Validations()[services.ValidationStale])
	})
}
