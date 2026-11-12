package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialChanged is a credential.changed event for token, as the control
// plane raises it.
func credentialChanged(t *testing.T, h *HybridGatewayService, token string, active, deleted bool) eventbridge.Event {
	t.Helper()
	payload, err := json.Marshal(eventbridge.CredentialChanged{
		CredentialID: 11, Hash: h.cacheKey(token), AppID: 42, Namespace: "other-namespace", Active: active, Deleted: deleted,
	})
	require.NoError(t, err)
	return eventbridge.Event{ID: "evt-1", Topic: eventbridge.CredentialChangedTopic, Origin: "control", Dir: eventbridge.DirDown, Payload: payload}
}

func cached(h *HybridGatewayService, token string) bool {
	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()
	_, ok := h.tokenCache[h.cacheKey(token)]
	return ok
}

// An edge validating on demand drops what it cached for a credential when
// the control plane says it changed: a revoked key is refused at the next
// request, not at the cache TTL. Other keys keep their entries.
func TestCredentialChanged_EvictsTheCachedValidation(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	revoked := &TokenValidationResult{TokenID: 42, AppID: 42}
	other := &TokenValidationResult{TokenID: 7, AppID: 7}
	h.storeInCache(h.cacheKey("tok-revoked"), revoked, time.Now())
	h.storeInCache(h.cacheKey("tok-other"), other, time.Now())
	hub := &fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: false, ErrorMessage: "credential is not active"}}
	h.SetEdgeClient(hub)

	got, err := h.ValidateAPIToken("tok-revoked")
	require.NoError(t, err)
	assert.Same(t, revoked, got, "served from the cache before the event")

	// Deactivated on the control plane; the event names another namespace
	// than this edge's: it still only drops, which is always safe.
	h.HandleCredentialChanged(credentialChanged(t, h, "tok-revoked", false, false))
	assert.False(t, cached(h, "tok-revoked"))
	assert.True(t, cached(h, "tok-other"), "only that credential")

	_, err = h.ValidateAPIToken("tok-revoked")
	require.Error(t, err, "refused at once")
	assert.Contains(t, err.Error(), "credential is not active")
	assert.Equal(t, 1, hub.calls, "the control plane was asked")

	got, err = h.ValidateAPIToken("tok-other")
	require.NoError(t, err)
	assert.Same(t, other, got)
	assert.Equal(t, 1, hub.calls)
}

// The stale grace does not bring an evicted key back: with no control
// plane to ask, a key whose credential changed gets the retryable 503, not
// its old answer.
func TestCredentialChanged_EvictsTheStaleEntryToo(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	seedExpiredEntry(h, "tok-deleted", 10*time.Minute)
	h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})

	h.HandleCredentialChanged(credentialChanged(t, h, "tok-deleted", false, true))
	_, err := h.ValidateAPIToken("tok-deleted")
	require.Error(t, err)
	assert.ErrorIs(t, err, coresvc.ErrCredentialCheckUnavailable)
}

// blockingEdgeClient answers valid, after the test lets it.
type blockingEdgeClient struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
	resp    *pb.TokenValidationResponse
}

func (b *blockingEdgeClient) ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) {
	b.mu.Lock()
	b.calls++
	first := b.calls == 1
	b.mu.Unlock()
	if first {
		close(b.started)
		<-b.release
	}
	return b.resp, nil
}

// A validation that was already asking the control plane when the event
// arrived may carry the answer from before the change: it answers its own
// request, but its result is not cached, and a request after the event
// asks again instead of joining it.
func TestCredentialChanged_ValidationInFlightIsNotCached(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	require.NoError(t, h.db.Create(&database.App{Name: "app", IsActive: true}).Error)
	var app database.App
	require.NoError(t, h.db.First(&app).Error)
	hub := &blockingEdgeClient{started: make(chan struct{}), release: make(chan struct{}),
		resp: &pb.TokenValidationResponse{Valid: true, AppId: uint32(app.ID), AppName: "app"}}
	h.SetEdgeClient(hub)

	done := make(chan error, 1)
	go func() {
		_, err := h.ValidateAPIToken("tok-racing")
		done <- err
	}()
	<-hub.started
	h.HandleCredentialChanged(credentialChanged(t, h, "tok-racing", false, false))
	close(hub.release)
	require.NoError(t, <-done)
	assert.False(t, cached(h, "tok-racing"), "an answer from before the change is not cached")

	_, err := h.ValidateAPIToken("tok-racing")
	require.NoError(t, err)
	hub.mu.Lock()
	assert.Equal(t, 2, hub.calls, "asked again")
	hub.mu.Unlock()
	assert.True(t, cached(h, "tok-racing"), "a validation after the change is cached as usual")
}

// An event that cannot be read, or whose hash is not a credential hash,
// changes nothing.
func TestCredentialChanged_UnreadableEventIsIgnored(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	h.storeInCache(h.cacheKey("tok"), &TokenValidationResult{AppID: 1}, time.Now())
	short := h.cacheKey("tok")[:20]
	for _, payload := range []string{`not json`, `{}`, `{"credential_id": 11}`, `{"credential_id": 11, "hash": "` + short + `"}`} {
		h.HandleCredentialChanged(eventbridge.Event{Topic: eventbridge.CredentialChangedTopic, Payload: json.RawMessage(payload)})
	}
	assert.True(t, cached(h, "tok"))
}

// Evictions are remembered only as long as a call can take: older ones
// are forgotten when a later one is added, at most once a minute.
func TestCredentialChanged_EvictionMemoryIsPruned(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	h.EvictCredential(h.cacheKey("tok-old"))
	h.evictionsMu.Lock()
	h.evictedAt[h.cacheKey("tok-old")] = time.Now().Add(-evictionMemory - time.Minute)
	h.evictionsPruned = time.Now().Add(-time.Minute)
	h.evictionsMu.Unlock()
	h.EvictCredential(h.cacheKey("tok-new"))
	h.evictionsMu.Lock()
	defer h.evictionsMu.Unlock()
	assert.NotContains(t, h.evictedAt, h.cacheKey("tok-old"))
	assert.Contains(t, h.evictedAt, h.cacheKey("tok-new"))
}

// A flood of events does not grow the remembered evictions without bound:
// past maxRememberedEvictions they become one eviction of every key, so a
// validation that started before still does not cache, and one that
// starts after does.
func TestCredentialChanged_RememberedEvictionsAreBounded(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	before := time.Now()
	for i := 0; i <= maxRememberedEvictions; i++ {
		h.EvictCredential(fmt.Sprintf("v1:%064d", i))
	}
	h.evictionsMu.Lock()
	n := len(h.evictedAt)
	h.evictionsMu.Unlock()
	assert.LessOrEqual(t, n, maxRememberedEvictions)

	key := h.cacheKey("tok-unrelated")
	h.storeInCache(key, &TokenValidationResult{AppID: 1}, before)
	assert.False(t, cached(h, "tok-unrelated"), "started before the flood: not cached")
	h.storeInCache(key, &TokenValidationResult{AppID: 1}, time.Now().Add(time.Millisecond))
	assert.True(t, cached(h, "tok-unrelated"), "started after it: cached as usual")
}

// With full credential sync the event is a hint for the list: for a hash
// the list holds, the edge syncs (within about half a second), so a key
// deactivated on the control plane is refused from the list well before
// the next heartbeat. A hash the list does not hold starts no sync, and
// nothing is taken from the payload.
func TestCredentialChanged_FullSyncEdgeSyncsAtOnce(t *testing.T) {
	sqliteFile := func(t *testing.T) *gorm.DB {
		// A file, not :memory:: the sync runs on its own connection.
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, database.Migrate(db))
		t.Cleanup(func() { database.Close(db) })
		return db
	}
	for name, open := range map[string]func(*testing.T) *gorm.DB{"sqlite": sqliteFile, "postgres": openPostgresSchema} {
		t.Run(name, func(t *testing.T) { fullSyncEdgeSyncsAtOnce(t, open(t)) })
	}
}

func fullSyncEdgeSyncsAtOnce(t *testing.T, db *gorm.DB) {
	e := newSyncedEdge(t, db, 24*time.Hour)
	e.hub.apps[1] = pbApp(1, "app", true)
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	_, err := e.h.ValidateAPIToken(keyA)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go e.list.Run(ctx, e.hub, time.Minute)

	// Deactivated on the control plane; no heartbeat, only the event.
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: false, UpdatedAt: time.Now()})
	e.h.HandleCredentialChanged(credentialChanged(t, e.h, keyA, false, false))
	require.Eventually(t, func() bool {
		_, err := e.h.ValidateAPIToken(keyA)
		// Refused from the list, not 503 for want of a control plane.
		return err != nil && !errors.Is(err, coresvc.ErrCredentialCheckUnavailable)
	}, 2*time.Second, 10*time.Millisecond, "refused from the list after the sync the event started")

	// A hash this edge's list does not hold (another namespace's key, or
	// one created since the last sync): no sync, and nothing added.
	calls := e.hub.callCount()
	e.h.HandleCredentialChanged(credentialChanged(t, e.h, keyC, true, false))
	time.Sleep(1500 * time.Millisecond) // past the longest delay and interval
	assert.Equal(t, calls, e.hub.callCount(), "no sync for a hash the list does not hold")
	_, found, err := e.list.Lookup(e.hash(keyC))
	require.NoError(t, err)
	assert.False(t, found, "nothing is added from the payload")
}

// Under a stream of 100 events a second for 10 seconds, all for a key the
// edge holds, the edge makes about one event-driven sync a second, not one
// per event.
func TestCredentialChanged_EventSyncsAreSpaced(t *testing.T) {
	if testing.Short() {
		t.Skip("streams events for 10 seconds")
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "edge.db")+"?_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { database.Close(db) })
	e := newSyncedEdge(t, db, 24*time.Hour)
	e.hub.apps[1] = pbApp(1, "app", true)
	e.hub.set(SyncedCredentialUpdate{CredentialID: 11, SecretHash: e.hash(keyA), AppID: 1, Active: true, UpdatedAt: time.Now()})
	require.NoError(t, e.list.Sync(context.Background(), e.hub))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go e.list.Run(ctx, e.hub, time.Minute)

	before := e.hub.callCount()
	event := credentialChanged(t, e.h, keyA, true, false)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); <-tick.C {
		e.h.HandleCredentialChanged(event)
	}
	time.Sleep(1500 * time.Millisecond) // the last scheduled one
	syncs := e.hub.callCount() - before
	t.Logf("1000 events over 10 s: %d event-driven syncs", syncs)
	assert.GreaterOrEqual(t, syncs, 8, "still syncing on events")
	assert.LessOrEqual(t, syncs, 12, "about one a second, not one per event")
}
