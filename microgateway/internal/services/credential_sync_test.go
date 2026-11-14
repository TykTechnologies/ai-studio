package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeCredentialHub is the control plane's side of the credential sync: it
// serves the credentials it holds, everything or those changed since a
// watermark, in pages of two.
type fakeCredentialHub struct {
	credentials []SyncedCredentialUpdate
	apps        map[uint]*pb.AppConfig
	// version is the watermark of the newest change; each credential has
	// the version it last changed at.
	version  uint64
	versions map[string]uint64
	// forceFull makes the hub answer a delta request with everything.
	forceFull   bool
	unavailable bool
	failAfter   int // fail the stream after this many pages (0: never)
	calls       int
	sinceSeen   []uint64
	// mu lets a test change the hub while the edge's sync runs in another
	// goroutine (set, SyncCredentials and callCount take it).
	mu sync.Mutex
}

func (f *fakeCredentialHub) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newFakeCredentialHub() *fakeCredentialHub {
	return &fakeCredentialHub{apps: map[uint]*pb.AppConfig{}, versions: map[string]uint64{}}
}

func (f *fakeCredentialHub) set(c SyncedCredentialUpdate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version++
	f.versions[c.SecretHash] = f.version
	for i := range f.credentials {
		if f.credentials[i].SecretHash == c.SecretHash {
			f.credentials[i] = c
			return
		}
	}
	f.credentials = append(f.credentials, c)
}

// purge removes a credential without a trace, as a hard delete does.
func (f *fakeCredentialHub) purge(hash string) {
	for i := range f.credentials {
		if f.credentials[i].SecretHash == hash {
			f.credentials = append(f.credentials[:i], f.credentials[i+1:]...)
			return
		}
	}
}

func (f *fakeCredentialHub) SyncCredentials(_ context.Context, since uint64, onPage func(*CredentialSyncPage) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.sinceSeen = append(f.sinceSeen, since)
	if f.unavailable {
		return ErrCredentialSyncUnavailable
	}
	full := since == 0 || f.forceFull
	var send []SyncedCredentialUpdate
	for _, c := range f.credentials {
		if full || f.versions[c.SecretHash] > since {
			send = append(send, c)
		}
	}
	pages := 0
	for len(send) > 0 || pages == 0 {
		n := 2
		if n > len(send) {
			n = len(send)
		}
		page := &CredentialSyncPage{Credentials: send[:n], Full: full}
		for _, c := range page.Credentials {
			if app := f.apps[c.AppID]; app != nil {
				page.Apps = append(page.Apps, app)
			}
		}
		send = send[n:]
		page.Last = len(send) == 0
		if page.Last {
			page.Watermark = f.version
		}
		pages++
		if f.failAfter > 0 && pages > f.failAfter {
			return errors.New("stream broke")
		}
		if err := onPage(page); err != nil {
			return err
		}
	}
	return nil
}

const (
	keyA = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
	keyB = "0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d"
	keyC = "1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff"
)

// syncedEdge is an edge with full credential sync: the hybrid service, its
// list and a control plane to sync from and to ask.
type syncedEdge struct {
	h    *HybridGatewayService
	list *CredentialSync
	hub  *fakeCredentialHub
	// control answers on-demand validations; calls counts them.
	control *fakeEdgeClient
}

func newSyncedEdge(t *testing.T, db *gorm.DB, maxAge time.Duration) *syncedEdge {
	t.Helper()
	h := NewHybridGatewayService(db, database.NewRepository(db), "", config.HubSpokeConfig{
		TokenCacheEnabled: true, TokenCacheTTL: time.Minute, TokenCacheMaxSize: 100, TokenCacheCleanupInt: time.Hour,
	}, testCredentialHasher(t))
	t.Cleanup(h.Stop)
	list, err := NewCredentialSync(db, maxAge)
	require.NoError(t, err)
	h.SetCredentialSync(list)
	control := &fakeEdgeClient{err: errors.New("no control plane")}
	h.SetEdgeClient(control)
	return &syncedEdge{h: h, list: list, hub: newFakeCredentialHub(), control: control}
}

func (e *syncedEdge) hash(token string) string { return e.h.cacheKey(token) }

func pbApp(id uint32, name string, active bool) *pb.AppConfig {
	now := timestamppb.Now()
	return &pb.AppConfig{Id: id, Name: name, IsActive: active, CreatedAt: now, UpdatedAt: now}
}

// The list answers with no control plane: a key the edge has never served
// is accepted, a deactivated one and one of an inactive App are refused,
// and a key that is not on the list cannot be checked.
func TestCredentialSync_ValidatesFromTheListWithoutAControlPlane(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		e.hub.apps[1] = pbApp(1, "live-app", true)
		e.hub.apps[2] = pbApp(2, "paused-app", false)
		now := time.Now()
		e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now})
		e.hub.set(SyncedCredentialUpdate{CredentialID: 12, SecretHash: e.hash(keyB), AppID: 1, Active: false, UpdatedAt: now})
		e.hub.set(SyncedCredentialUpdate{CredentialID: 13, SecretHash: e.hash(keyC), AppID: 2, Active: true, UpdatedAt: now})
		require.NoError(t, e.list.Sync(context.Background(), e.hub))
		assert.EqualValues(t, 2, e.list.Count(), "usable credentials on the list")

		// Every control plane is cut from here on (e.control fails).
		res, err := e.h.ValidateAPIToken(keyA)
		require.NoError(t, err, "a key the edge never served is accepted from the list")
		assert.EqualValues(t, 1, res.AppID)
		require.NotNil(t, res.App)
		assert.Equal(t, "live-app", res.App.Name)

		_, err = e.h.ValidateAPIToken(keyB)
		require.Error(t, err, "a deactivated key is refused")
		assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "refused, not unknown")

		_, err = e.h.ValidateAPIToken(keyC)
		require.Error(t, err)
		assert.ErrorIs(t, err, coresvc.ErrAppInactive, "403 app is inactive, as on demand")

		assert.Zero(t, e.control.calls, "none of these asked the control plane")

		_, err = e.h.ValidateAPIToken("a-key-the-list-does-not-have")
		require.Error(t, err)
		assert.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "an unknown key with no control plane is a 503, not a 401")
		assert.Equal(t, 1, e.control.calls)

		v := e.list.Validations()
		assert.EqualValues(t, 3, v[ValidationLocal])
		assert.EqualValues(t, 1, v[ValidationControl])
		assert.Zero(t, v[ValidationStale])
	})
}

// A delta brings what changed while the edge was away: a key created then
// is accepted without asking, a revoked one is refused. The watermark is
// kept, so the next sync asks only for what is newer.
func TestCredentialSync_DeltaAfterReconnect(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		e.hub.apps[1] = pbApp(1, "app", true)
		now := time.Now()
		e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now})
		require.NoError(t, e.list.Sync(context.Background(), e.hub))
		_, err := e.h.ValidateAPIToken(keyA)
		require.NoError(t, err)

		// While the edge is offline: a key is created and another revoked.
		e.hub.set(SyncedCredentialUpdate{CredentialID: 12, SecretHash: e.hash(keyB), AppID: 1, Active: true, UpdatedAt: now})
		e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, Deleted: true, UpdatedAt: now})
		_, err = e.h.ValidateAPIToken(keyA)
		require.NoError(t, err, "until the edge syncs, it knows what it knew")

		// It reconnects and syncs.
		require.NoError(t, e.list.Sync(context.Background(), e.hub))
		require.Len(t, e.hub.sinceSeen, 2)
		assert.Zero(t, e.hub.sinceSeen[0], "the first sync asks for everything")
		assert.EqualValues(t, 1, e.hub.sinceSeen[1], "the second asks for the changes since its watermark")

		_, err = e.h.ValidateAPIToken(keyB)
		require.NoError(t, err, "the key created while the edge was offline is accepted")
		_, err = e.h.ValidateAPIToken(keyA)
		require.Error(t, err, "the key deleted meanwhile is refused")
		assert.Zero(t, e.control.calls, "without a pull-on-miss call")

		// Applying the same delta again changes nothing.
		before := e.list.Count()
		e.hub.forceFull = false
		require.NoError(t, e.list.Sync(context.Background(), e.hub))
		assert.Equal(t, before, e.list.Count())
	})
}

// A full sync removes what it does not bring (a credential deleted without
// a trace on the control plane), and a delta never does.
func TestCredentialSync_FullSyncSweeps(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 0)
	e.hub.apps[1] = pbApp(1, "app", true)
	now := time.Now()
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now})
	e.hub.set(SyncedCredentialUpdate{CredentialID: 12, SecretHash: e.hash(keyB), AppID: 1, Active: true, UpdatedAt: now})
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	require.EqualValues(t, 2, e.list.Count())

	e.hub.purge(e.hash(keyB))
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, e.list.Sync(context.Background(), e.hub), "a delta")
	assert.EqualValues(t, 2, e.list.Count(), "a delta cannot say a row is gone")

	e.hub.forceFull = true
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	assert.EqualValues(t, 1, e.list.Count(), "the full sync removed the credential it did not bring")
	_, found, err := e.list.Lookup(e.hash(keyB))
	require.NoError(t, err)
	assert.False(t, found)
}

// A sync that breaks part-way keeps the pages it applied (they are true)
// but not the watermark: the next sync asks from the old one again.
func TestCredentialSync_BrokenStreamKeepsTheWatermark(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 0)
	e.hub.apps[1] = pbApp(1, "app", true)
	now := time.Now()
	for i, key := range []string{keyA, keyB, keyC} {
		e.hub.set(SyncedCredentialUpdate{CredentialID: uint(i + 1), SecretHash: e.hash(key), AppID: 1, Active: true, UpdatedAt: now})
	}
	e.hub.failAfter = 1
	require.Error(t, e.list.Sync(context.Background(), e.hub))
	assert.True(t, e.list.LastSuccess().IsZero(), "a broken sync is not a successful one")
	assert.False(t, e.list.Trusted(time.Now()))

	e.hub.failAfter = 0
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	assert.Equal(t, []uint64{0, 0}, e.hub.sinceSeen, "the watermark was not advanced by the broken sync")
	assert.EqualValues(t, 3, e.list.Count())
	assert.True(t, e.list.Trusted(time.Now()))
}

// Past EDGE_CREDENTIAL_SYNC_MAX_AGE the list is not trusted: the edge asks
// the control plane, as on demand, and trusts the list again after the next
// successful sync.
func TestCredentialSync_MaxAge(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, time.Hour)
	e.hub.apps[1] = pbApp(1, "app", true)
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	_, err := e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)
	require.Zero(t, e.control.calls)

	// The last sync is two hours old.
	e.list.lastSuccess.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	assert.False(t, e.list.Trusted(time.Now()))
	_, err = e.h.ValidateAPIToken(keyA)
	require.Error(t, err)
	assert.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "with no control plane a stale list answers nothing: 503")
	assert.Equal(t, 1, e.control.calls, "the edge asked, as on demand")
	assert.EqualValues(t, 1, e.list.Validations()[ValidationStale])

	// The control plane is back: asked, it accepts the key.
	e.control.err = nil
	// The control plane sends the App with its answer, as it does for a
	// key of this edge's namespace.
	e.control.resp = &pb.TokenValidationResponse{Valid: true, AppId: 1, AppName: "app", App: pbApp(1, "app", true)}
	_, err = e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)

	// A sync succeeds: the list is trusted again and answers by itself.
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	calls := e.control.calls
	e.h.removeFromCache(e.hash(keyA))
	_, err = e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)
	assert.Equal(t, calls, e.control.calls)

	// 0 means no limit.
	nolimit := newSyncedEdge(t, setupEdgeSyncTestDB(t), 0)
	nolimit.list.lastSuccess.Store(time.Now().Add(-1000 * time.Hour).UnixNano())
	assert.True(t, nolimit.list.Trusted(time.Now()))
}

// A key the list does not have is asked about (it may have been created a
// second ago); the answer is kept. A rejection from the control plane wins
// over the list.
func TestCredentialSync_ControlPlaneAnswersAreKept(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 0)
	e.hub.apps[1] = pbApp(1, "app", true)
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
	require.NoError(t, e.list.Sync(context.Background(), e.hub))

	// keyB was created a moment ago: not on the list yet, the control plane
	// knows it.
	e.control.err = nil
	// The control plane sends the App with its answer, as it does for a
	// key of this edge's namespace.
	e.control.resp = &pb.TokenValidationResponse{Valid: true, AppId: 1, AppName: "app", App: pbApp(1, "app", true)}
	_, err := e.h.ValidateAPIToken(keyB)
	require.NoError(t, err)
	require.Equal(t, 1, e.control.calls)
	row, found, err := e.list.Lookup(e.hash(keyB))
	require.NoError(t, err)
	require.True(t, found, "the accepted key is on the list now")
	assert.True(t, row.Active)
	assert.EqualValues(t, 1, row.AppID)

	// From then on the list answers for it, control plane or not.
	e.h.removeFromCache(e.hash(keyB))
	e.control.err = errors.New("no control plane")
	_, err = e.h.ValidateAPIToken(keyB)
	require.NoError(t, err)
	assert.Equal(t, 1, e.control.calls)

	// A key the control plane rejects is marked unusable on the list.
	e.list.Learned(e.hash(keyA), 0, false)
	_, err = e.h.ValidateAPIToken(keyA)
	require.Error(t, err)
	assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable)
}

// An old or Community control plane does not offer the sync: the edge says
// so once, stops asking, and validates on demand. After a reconnect it asks
// again.
func TestCredentialSync_ControlPlaneWithoutTheSync(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	e.hub.unavailable = true

	err := e.list.Sync(context.Background(), e.hub)
	require.ErrorIs(t, err, ErrCredentialSyncUnavailable)
	require.ErrorIs(t, e.list.Sync(context.Background(), e.hub), ErrCredentialSyncUnavailable)
	assert.Equal(t, 1, e.hub.calls, "it is not asked again")
	assert.False(t, e.list.Trusted(time.Now()))

	// On demand: the control plane decides.
	e.control.err = nil
	e.control.resp = &pb.TokenValidationResponse{Valid: false, ErrorMessage: "Invalid token"}
	_, err = e.h.ValidateAPIToken(keyA)
	require.Error(t, err)
	assert.Equal(t, 1, e.control.calls)

	// The edge reconnects, to a control plane that offers it.
	e.list.Reset()
	e.hub.unavailable = false
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	assert.Equal(t, 2, e.hub.calls)
}

// A restarted edge reads how old its list is from its database: with every
// control plane cut it validates a synced key at once.
func TestCredentialSync_SurvivesARestart(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		e.hub.apps[1] = pbApp(1, "app", true)
		e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
		require.NoError(t, e.list.Sync(context.Background(), e.hub))

		restarted := newSyncedEdge(t, db, 24*time.Hour)
		assert.True(t, restarted.list.Trusted(time.Now()))
		assert.EqualValues(t, 1, restarted.list.Count())
		_, err := restarted.h.ValidateAPIToken(keyA)
		require.NoError(t, err)
		assert.Zero(t, restarted.control.calls)
	})
}

// The edge database holds no key: every text column of every table is
// searched for the keys used here.
func TestCredentialSync_EdgeDatabaseHoldsNoKey(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 0)
		e.hub.apps[1] = pbApp(1, "app", true)
		now := time.Now()
		keys := []string{keyA, keyB, keyC}
		for i, key := range keys[:2] {
			e.hub.set(SyncedCredentialUpdate{CredentialID: uint(i + 1), SecretHash: e.hash(key), AppID: 1, Active: true, UpdatedAt: now})
		}
		require.NoError(t, e.list.Sync(context.Background(), e.hub))
		for _, key := range keys[:2] {
			_, err := e.h.ValidateAPIToken(key)
			require.NoError(t, err)
		}
		// One learned from the control plane.
		e.control.err = nil
		// The control plane sends the App with its answer, as it does for a
		// key of this edge's namespace.
		e.control.resp = &pb.TokenValidationResponse{Valid: true, AppId: 1, AppName: "app", App: pbApp(1, "app", true)}
		_, err := e.h.ValidateAPIToken(keyC)
		require.NoError(t, err)

		tables, err := db.Migrator().GetTables()
		require.NoError(t, err)
		require.Contains(t, tables, "synced_credentials")
		searched := 0
		for _, table := range tables {
			var rows []map[string]interface{}
			require.NoError(t, db.Table(table).Find(&rows).Error, table)
			for _, row := range rows {
				for col, v := range row {
					text := fmt.Sprint(v)
					if b, ok := v.([]byte); ok {
						text = string(b)
					}
					searched++
					for _, key := range keys {
						if strings.Contains(text, key) {
							t.Errorf("table %s column %s holds a key", table, col)
						}
					}
				}
			}
		}
		assert.Greater(t, searched, 10, "the scan looked at the database")
	})
}

// EDGE_CREDENTIAL_SYNC=full is an Enterprise option: a Community build
// stays on demand.
func TestCredentialSyncEnabled(t *testing.T) {
	was := fullCredentialSyncAvailable
	t.Cleanup(func() { fullCredentialSyncAvailable = was })

	fullCredentialSyncAvailable = true
	assert.True(t, CredentialSyncEnabled(config.HubSpokeConfig{CredentialSync: "full"}))
	assert.False(t, CredentialSyncEnabled(config.HubSpokeConfig{CredentialSync: "on_demand"}))
	assert.False(t, CredentialSyncEnabled(config.HubSpokeConfig{}))

	fullCredentialSyncAvailable = false
	assert.False(t, CredentialSyncEnabled(config.HubSpokeConfig{CredentialSync: "full"}), "a Community edge given full stays on demand")
	assert.Equal(t, fullCredentialSyncBuild, was, "the default follows the build")
}

// countingSource is a control plane for the Run tests: it counts the syncs
// asked of it and can hold one open, fail, or not offer the sync.
type countingSource struct {
	calls       atomic.Int32
	finished    atomic.Int32
	hold        chan struct{} // when not nil, a sync waits for it
	fail        atomic.Bool
	unavailable atomic.Bool
}

func (c *countingSource) SyncCredentials(ctx context.Context, _ uint64, onPage func(*CredentialSyncPage) error) error {
	c.calls.Add(1)
	defer c.finished.Add(1)
	if c.hold != nil {
		select {
		case <-c.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.unavailable.Load() {
		return ErrCredentialSyncUnavailable
	}
	if c.fail.Load() {
		return errors.New("stream broke")
	}
	return onPage(&CredentialSyncPage{Full: true, Last: true, Watermark: 1})
}

// Run syncs once per kick, folds the kicks that arrive during a sync into
// one more, survives a failed sync, and stops with its context.
func TestCredentialSync_RunSyncsOnKicks(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	list, err := NewCredentialSync(db, time.Hour)
	require.NoError(t, err)
	source := &countingSource{hold: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { list.Run(ctx, source, time.Minute); close(done) }()

	calls := func(n int32) func() bool { return func() bool { return source.calls.Load() == n } }
	require.Never(t, calls(1), 50*time.Millisecond, 10*time.Millisecond, "nothing is asked before the first kick")

	// The first sync is held open; three kicks arrive meanwhile.
	list.Kick()
	require.Eventually(t, calls(1), 2*time.Second, 5*time.Millisecond)
	list.Kick()
	list.Kick()
	list.Kick()
	source.hold <- struct{}{}
	require.Eventually(t, calls(2), 2*time.Second, 5*time.Millisecond, "the kicks during a sync are one more sync")
	source.hold <- struct{}{}
	require.Eventually(t, func() bool { return list.Trusted(time.Now()) }, 2*time.Second, 5*time.Millisecond)
	require.Never(t, calls(3), 50*time.Millisecond, 10*time.Millisecond)

	// A failed sync leaves the list as it was, and the next kick tries again.
	before := list.LastSuccess()
	source.fail.Store(true)
	list.Kick()
	require.Eventually(t, calls(3), 2*time.Second, 5*time.Millisecond)
	source.hold <- struct{}{}
	require.Eventually(t, func() bool { return source.finished.Load() == 3 }, 2*time.Second, 5*time.Millisecond)
	source.fail.Store(false)
	list.Kick()
	require.Eventually(t, calls(4), 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, before, list.LastSuccess(), "a failed sync is not a success")
	source.hold <- struct{}{}
	require.Eventually(t, func() bool { return list.LastSuccess().After(before) }, 2*time.Second, 5*time.Millisecond)

	// A sync in flight ends with the context, and so does Run.
	list.Kick()
	require.Eventually(t, calls(5), 2*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop with its context")
	}
}

// A sync that outlasts its timeout is given up, so a control plane that
// accepts the call and never answers does not stop later syncs.
func TestCredentialSync_RunGivesUpASyncThatHangs(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	list, err := NewCredentialSync(db, time.Hour)
	require.NoError(t, err)
	source := &countingSource{hold: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go list.Run(ctx, source, 30*time.Millisecond)

	list.Kick()
	require.Eventually(t, func() bool { return source.calls.Load() == 1 }, 2*time.Second, 5*time.Millisecond)
	// Nothing releases the first sync: only its timeout lets the second start.
	list.Kick()
	require.Eventually(t, func() bool { return source.calls.Load() == 2 }, 2*time.Second, 5*time.Millisecond)
	assert.False(t, list.Trusted(time.Now()))
}

// A control plane that does not offer the sync is not asked on every
// heartbeat: only again after the edge registered anew (Reset).
func TestCredentialSync_RunStopsAskingAControlPlaneWithoutTheSync(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	list, err := NewCredentialSync(db, time.Hour)
	require.NoError(t, err)
	source := &countingSource{}
	source.unavailable.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go list.Run(ctx, source, time.Minute)

	list.Kick()
	require.Eventually(t, func() bool { return source.calls.Load() == 1 }, 2*time.Second, 5*time.Millisecond)
	for i := 0; i < 3; i++ {
		list.Kick()
		time.Sleep(20 * time.Millisecond)
	}
	assert.EqualValues(t, 1, source.calls.Load(), "heartbeats do not ask it again")

	source.unavailable.Store(false)
	list.Reset()
	list.Kick()
	require.Eventually(t, func() bool { return list.Trusted(time.Now()) }, 2*time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 2, source.calls.Load())
}

// pagesOf is a control plane that answers a sync with the pages given.
type pagesOf []*CredentialSyncPage

func (p pagesOf) SyncCredentials(_ context.Context, _ uint64, onPage func(*CredentialSyncPage) error) error {
	for _, page := range p {
		if err := onPage(page); err != nil {
			return err
		}
	}
	return nil
}

func delta(watermark uint64, creds ...SyncedCredentialUpdate) pagesOf {
	return pagesOf{{Credentials: creds, Last: true, Watermark: watermark, HashVersion: "v1"}}
}

// A credential is applied by its ID. A regenerated key arrives as the same
// ID with a new hash: the old key stops being accepted with that page, not
// at the next full sync, and the new one is accepted. Applying the page
// again changes nothing.
func TestCredentialSync_RegeneratedKeyReplacesTheOldHash(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		now := time.Now()
		full := pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
			Apps:        []*pb.AppConfig{pbApp(1, "app", true)},
			Credentials: []SyncedCredentialUpdate{{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now}}}}
		require.NoError(t, e.list.Sync(context.Background(), full))
		_, err := e.h.ValidateAPIToken(keyA)
		require.NoError(t, err)

		regenerated := delta(6, SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyB), AppID: 1, Active: true, UpdatedAt: now})
		for i := 0; i < 2; i++ {
			require.NoError(t, e.list.Sync(context.Background(), regenerated))
			var rows []database.SyncedCredential
			require.NoError(t, db.Where("credential_id = ?", 11).Find(&rows).Error)
			require.Len(t, rows, 1, "one row for the credential")
			assert.Equal(t, e.hash(keyB), rows[0].SecretHash)
		}

		_, err = e.h.ValidateAPIToken(keyB)
		require.NoError(t, err, "the new key is accepted from the list")
		_, err = e.h.ValidateAPIToken(keyA)
		require.Error(t, err, "the old key is no longer accepted")
		assert.EqualValues(t, 1, e.list.Count())
	})
}

// A deletion in a delta is a bare tombstone: the credential's ID and
// nothing else. Every row of that credential goes; a tombstone for a
// credential this edge never held changes nothing.
func TestCredentialSync_TombstoneRemovesTheCredential(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		now := time.Now()
		full := pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
			Apps: []*pb.AppConfig{pbApp(1, "app", true)},
			Credentials: []SyncedCredentialUpdate{
				{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now},
				{CredentialID: 12, SecretHash: e.hash(keyB), AppID: 1, Active: true, UpdatedAt: now},
			}}}
		require.NoError(t, e.list.Sync(context.Background(), full))
		require.EqualValues(t, 2, e.list.Count())

		require.NoError(t, e.list.Sync(context.Background(), delta(6,
			SyncedCredentialUpdate{CredentialID: 11, Deleted: true},
			SyncedCredentialUpdate{CredentialID: 999, Deleted: true})))

		assert.EqualValues(t, 1, e.list.Count())
		_, found, err := e.list.Lookup(e.hash(keyA))
		require.NoError(t, err)
		assert.False(t, found, "the deleted credential is off the list")
		_, err = e.h.ValidateAPIToken(keyB)
		require.NoError(t, err, "the other one is untouched")
	})
}

// A credential the control plane sends without an App (no App of this
// edge's namespace uses it) is refused from the list, without asking.
func TestCredentialSync_CredentialWithoutAnAppIsRefusedLocally(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
		Credentials: []SyncedCredentialUpdate{{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 0, Active: true, UpdatedAt: time.Now()}}}}))

	_, err := e.h.ValidateAPIToken(keyA)
	require.Error(t, err)
	assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "refused, not unknown")
	assert.Zero(t, e.control.calls, "the control plane is not asked")
}

// Hashes in a version this edge does not compute are not applied: the sync
// fails, the list, the watermark and the time of the last success stay as
// they were.
func TestCredentialSync_UnknownHashVersionIsNotApplied(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	now := time.Now()
	require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
		Apps:        []*pb.AppConfig{pbApp(1, "app", true)},
		Credentials: []SyncedCredentialUpdate{{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now}}}}))
	before := e.list.LastSuccess()

	err := e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 9, HashVersion: "v2",
		Credentials: []SyncedCredentialUpdate{{CredentialID: 12, SecretHash: "v2:" + strings.Repeat("0", 64), AppID: 1, Active: true, UpdatedAt: now}}}})
	require.ErrorIs(t, err, ErrCredentialHashVersion)

	assert.Equal(t, before, e.list.LastSuccess())
	assert.EqualValues(t, 1, e.list.Count(), "nothing of that sync was applied, and nothing was swept")
	var state database.SyncState
	require.NoError(t, db.Where("topic = ?", credentialSyncTopic).First(&state).Error)
	assert.EqualValues(t, 5, state.SequenceNumber, "the watermark is the one of the last sync that was applied")
	_, err = e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)
}

// With full credential sync, a key this edge may serve is one the control
// plane names with its App. When the control plane answers that a key is
// valid and sends no App, the App is not one this edge's namespace serves:
// the request is refused, although this edge's database still holds a row
// for that App ID (the App was in its namespace once), and nothing is
// cached.
func TestCredentialSync_ValidAnswerWithoutAnAppIsRefused(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1"}}))
	// A row for App 7 from an earlier configuration.
	require.NoError(t, e.h.storeAppFromPullOnMiss(pbApp(7, "moved-app", true)))

	e.control.err = nil
	e.control.resp = &pb.TokenValidationResponse{Valid: true, AppId: 7, AppName: "moved-app"}
	for i := 1; i <= 2; i++ {
		_, err := e.h.ValidateAPIToken(keyA)
		require.Error(t, err, "refused")
		assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "a refusal, not a retryable 503")
		assert.Equal(t, i, e.control.calls, "not cached as valid: the control plane is asked again")
	}
	assert.Zero(t, e.list.Count(), "and not added to the list")

	// With the App sent, the same key is served (and learned).
	e.control.resp = &pb.TokenValidationResponse{Valid: true, AppId: 7, AppName: "moved-app", App: pbApp(7, "moved-app", true)}
	res, err := e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)
	assert.EqualValues(t, 7, res.AppID)
}

// A key the edge learned from a validation (no credential ID) cannot be
// removed or replaced by a delta, which works by ID. Each completed sync
// therefore drops the learned rows from before it: a key used once at the
// edge and then deleted, or regenerated, before a delta carries it is not
// accepted from the list afterwards.
func TestCredentialSync_LearnedKeysDoNotOutliveASync(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		for _, tc := range []struct {
			name   string
			change func(e *syncedEdge)
		}{
			{"deleted", func(e *syncedEdge) {
				// Created and deleted on the hub between two syncs: no
				// delta ever carries it.
			}},
			{"regenerated", func(e *syncedEdge) {
				e.hub.set(SyncedCredentialUpdate{CredentialID: 12, SecretHash: e.hash(keyC), AppID: 1, Active: true, UpdatedAt: time.Now()})
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				e := newSyncedEdge(t, db, 24*time.Hour)
				require.NoError(t, db.Where("1 = 1").Delete(&database.SyncedCredential{}).Error)
				require.NoError(t, db.Where("topic = ?", credentialSyncTopic).Delete(&database.SyncState{}).Error)
				e.hub.apps[1] = pbApp(1, "app", true)
				e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
				require.NoError(t, e.list.Sync(context.Background(), e.hub))

				// keyB is used at the edge: the control plane says valid,
				// and the edge records it, without an ID.
				e.list.Learned(e.hash(keyB), 1, true)
				_, found, err := e.list.Lookup(e.hash(keyB))
				require.NoError(t, err)
				require.True(t, found)

				tc.change(e)
				require.NoError(t, e.list.Sync(context.Background(), e.hub), "a delta")
				_, found, err = e.list.Lookup(e.hash(keyB))
				require.NoError(t, err)
				assert.False(t, found, "the learned row is gone after the sync")

				// With no control plane, the old key cannot be checked: 503,
				// not accepted from the list.
				e.control.err = errors.New("no control plane")
				_, err = e.h.ValidateAPIToken(keyB)
				require.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable)
				// keyA, which the sync brought with its ID, stays.
				_, err = e.h.ValidateAPIToken(keyA)
				require.NoError(t, err)
			})
		}
	})
}

// A key learned while a sync runs is kept: only rows from before the sync
// started are dropped.
func TestCredentialSync_KeyLearnedDuringASyncIsKept(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	e.hub.apps[1] = pbApp(1, "app", true)
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
	require.NoError(t, e.list.Sync(context.Background(), pagesDuring{e.hub, func() { e.list.Learned(e.hash(keyB), 1, true) }}))
	_, found, err := e.list.Lookup(e.hash(keyB))
	require.NoError(t, err)
	assert.True(t, found)
}

// pagesDuring runs during once, after the first page of source's stream.
type pagesDuring struct {
	source CredentialSyncSource
	during func()
}

func (p pagesDuring) SyncCredentials(ctx context.Context, since uint64, onPage func(*CredentialSyncPage) error) error {
	once := false
	return p.source.SyncCredentials(ctx, since, func(page *CredentialSyncPage) error {
		if err := onPage(page); err != nil {
			return err
		}
		if !once {
			once = true
			p.during()
		}
		return nil
	})
}

// A rejected key the list does not hold changes nothing: no recount.
func TestCredentialSync_RejectedUnknownKeyChangesNothing(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	e := newSyncedEdge(t, db, 24*time.Hour)
	before := e.list.Count()
	e.list.Learned(e.hash(keyC), 0, false)
	assert.Equal(t, before, e.list.Count())
	var n int64
	require.NoError(t, db.Model(&database.SyncedCredential{}).Count(&n).Error)
	assert.Zero(t, n, "nothing added for a rejected key")
}

// Lookup caches the rows it found, so a key in use costs no database read
// per request. Every way the list changes clears that cache, so the next
// request sees the change (bead midsommar-clean-4x5y.50). Each path on its
// own, with both keys looked up (cached) first.
func TestCredentialSync_LookupCacheFollowsEveryWrite(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		start := func(t *testing.T) (*syncedEdge, func(key string) (database.SyncedCredential, bool)) {
			t.Helper()
			require.NoError(t, db.Exec("DELETE FROM synced_credentials").Error)
			require.NoError(t, db.Exec("DELETE FROM sync_states").Error)
			e := newSyncedEdge(t, db, 24*time.Hour)
			lookup := func(key string) (database.SyncedCredential, bool) {
				t.Helper()
				row, found, err := e.list.Lookup(e.hash(key))
				require.NoError(t, err)
				return row, found
			}
			now := time.Now()
			require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
				Apps: []*pb.AppConfig{pbApp(1, "app", true)},
				Credentials: []SyncedCredentialUpdate{
					{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now},
					{CredentialID: 12, SecretHash: e.hash(keyB), AppID: 1, Active: true, UpdatedAt: now},
				}}}))
			for _, key := range []string{keyA, keyB} {
				row, found := lookup(key)
				require.True(t, found)
				require.True(t, row.Active)
			}
			return e, lookup
		}

		t.Run("a page that deactivates a key", func(t *testing.T) {
			e, lookup := start(t)
			require.NoError(t, e.list.Sync(context.Background(), delta(6,
				SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: false, UpdatedAt: time.Now()})))
			row, found := lookup(keyA)
			require.True(t, found)
			assert.False(t, row.Active)
		})

		t.Run("a page that regenerates a key", func(t *testing.T) {
			e, lookup := start(t)
			require.NoError(t, e.list.Sync(context.Background(), delta(6,
				SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyC), AppID: 1, Active: true, UpdatedAt: time.Now()})))
			_, found := lookup(keyA)
			assert.False(t, found, "the old hash is gone")
		})

		t.Run("a tombstone", func(t *testing.T) {
			e, lookup := start(t)
			require.NoError(t, e.list.Sync(context.Background(), delta(6, SyncedCredentialUpdate{CredentialID: 12, Deleted: true})))
			_, found := lookup(keyB)
			assert.False(t, found)
		})

		t.Run("a full sync without the key", func(t *testing.T) {
			e, lookup := start(t)
			time.Sleep(10 * time.Millisecond) // the rows were synced before this sync starts
			require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 7, HashVersion: "v1",
				Apps:        []*pb.AppConfig{pbApp(1, "app", true)},
				Credentials: []SyncedCredentialUpdate{{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()}}}}))
			_, found := lookup(keyB)
			assert.False(t, found, "what the full sync did not bring is gone")
		})

		t.Run("the control plane refuses a key (Learned)", func(t *testing.T) {
			e, lookup := start(t)
			e.list.Learned(e.hash(keyA), 1, false)
			row, found := lookup(keyA)
			require.True(t, found)
			assert.False(t, row.Active)
		})

		t.Run("the control plane accepts a key the list had refused (Learned)", func(t *testing.T) {
			e, lookup := start(t)
			e.list.Learned(e.hash(keyA), 1, false)
			row, _ := lookup(keyA)
			require.False(t, row.Active, "the refusal is what is cached now")
			e.list.Learned(e.hash(keyA), 1, true)
			row, _ = lookup(keyA)
			assert.True(t, row.Active)
		})

		t.Run("the cleanup of keys learned without an ID", func(t *testing.T) {
			e, lookup := start(t)
			e.list.Learned(e.hash(keyC), 1, true)
			_, found := lookup(keyC)
			require.True(t, found)
			time.Sleep(10 * time.Millisecond) // learned before the sync starts
			require.NoError(t, e.list.Sync(context.Background(), delta(7)))
			_, found = lookup(keyC)
			assert.False(t, found)
		})

		t.Run("a credential.changed eviction", func(t *testing.T) {
			e, lookup := start(t)
			// Changed behind the cache (by hand): served from the cache until
			// an event names the key.
			require.NoError(t, db.Model(&database.SyncedCredential{}).Where("secret_hash = ?", e.hash(keyA)).Update("active", false).Error)
			row, _ := lookup(keyA)
			require.True(t, row.Active, "the cache answered")
			e.h.EvictCredential(e.hash(keyA))
			row, _ = lookup(keyA)
			assert.False(t, row.Active)
		})
	})
}

// A key in use, deactivated by a sync, is refused on the very next request.
func TestCredentialSync_KeyDeactivatedBySyncIsRefusedOnTheNextRequest(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		now := time.Now()
		require.NoError(t, e.list.Sync(context.Background(), pagesOf{{Full: true, Last: true, Watermark: 5, HashVersion: "v1",
			Apps:        []*pb.AppConfig{pbApp(1, "app", true)},
			Credentials: []SyncedCredentialUpdate{{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: now}}}}))
		for i := 0; i < 3; i++ { // in use: cached
			_, err := e.h.ValidateAPIToken(keyA)
			require.NoError(t, err)
		}
		require.NoError(t, e.list.Sync(context.Background(), delta(6,
			SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: false, UpdatedAt: now})))
		_, err := e.h.ValidateAPIToken(keyA)
		require.Error(t, err, "refused on the next request")
		assert.NotErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable, "refused from the list, not for want of a control plane")
		assert.Zero(t, e.control.calls, "without asking the control plane")
	})
}
