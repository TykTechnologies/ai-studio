package services

import (
	"encoding/json"
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

// An event that cannot be read changes nothing.
func TestCredentialChanged_UnreadableEventIsIgnored(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	h.storeInCache(h.cacheKey("tok"), &TokenValidationResult{AppID: 1}, time.Now())
	for _, payload := range []string{`not json`, `{}`, `{"credential_id": 11}`} {
		h.HandleCredentialChanged(eventbridge.Event{Topic: eventbridge.CredentialChangedTopic, Payload: json.RawMessage(payload)})
	}
	assert.True(t, cached(h, "tok"))
}

// Evictions are remembered only as long as a call can take.
func TestCredentialChanged_EvictionMemoryIsPruned(t *testing.T) {
	h := newStaleGraceService(t, time.Hour)
	h.EvictCredential(h.cacheKey("tok-old"))
	h.EvictCredential(h.cacheKey("tok-new"))
	h.cacheMutex.Lock()
	h.evictedAt[h.cacheKey("tok-old")] = time.Now().Add(-evictionMemory - time.Minute)
	h.cacheMutex.Unlock()
	h.cleanupExpiredEntries()
	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()
	assert.NotContains(t, h.evictedAt, h.cacheKey("tok-old"))
	assert.Contains(t, h.evictedAt, h.cacheKey("tok-new"))
}
