package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowEdgeClient answers every validation after a delay, counting calls.
type slowEdgeClient struct {
	delay time.Duration
	calls atomic.Int32
	resp  atomic.Pointer[pb.TokenValidationResponse]
}

func (c *slowEdgeClient) ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) {
	c.calls.Add(1)
	time.Sleep(c.delay)
	return c.resp.Load(), nil
}

func newRefreshTestService(t *testing.T, ttl time.Duration) (*HybridGatewayService, *slowEdgeClient) {
	t.Helper()
	h := newStaleGraceService(t, 0)
	h.cacheConfig.TokenCacheTTL = ttl
	require.NoError(t, h.db.Create(&database.App{Name: "refresh-app", IsActive: true}).Error)
	var app database.App
	require.NoError(t, h.db.First(&app).Error)
	client := &slowEdgeClient{delay: 50 * time.Millisecond}
	client.resp.Store(&pb.TokenValidationResponse{Valid: true, AppId: uint32(app.ID), AppName: app.Name})
	h.SetEdgeClient(client)
	return h, client
}

// Requests that miss together share one call to the control instance. Every
// in-flight request used to call the hub (and rewrite the App) on its own
// whenever a busy token's entry expired: a stall every TTL under load.
func TestValidateAPIToken_ConcurrentMissesShareOneHubCall(t *testing.T) {
	h, client := newRefreshTestService(t, time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := h.ValidateAPIToken("tok-busy")
			assert.NoError(t, err)
			assert.NotNil(t, res)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, client.calls.Load())
}

// Near its expiry a cached result is revalidated in the background, once,
// while requests keep being served from the cache without waiting.
func TestValidateAPIToken_RefreshAhead(t *testing.T) {
	ttl := 400 * time.Millisecond
	h, client := newRefreshTestService(t, ttl)

	_, err := h.ValidateAPIToken("tok-ra")
	require.NoError(t, err)
	require.EqualValues(t, 1, client.calls.Load())

	time.Sleep(time.Duration(float64(ttl)*refreshAheadFraction) + 20*time.Millisecond)
	for i := 0; i < 50; i++ {
		start := time.Now()
		_, err := h.ValidateAPIToken("tok-ra")
		require.NoError(t, err)
		assert.Less(t, time.Since(start), client.delay, "served from the cache, not waiting for the refresh")
	}
	require.Eventually(t, func() bool { return client.calls.Load() == 2 }, time.Second, 5*time.Millisecond, "exactly one refresh")

	// The refreshed entry outlives the original expiry: no miss at the old TTL.
	time.Sleep(ttl - time.Duration(float64(ttl)*refreshAheadFraction))
	_, err = h.ValidateAPIToken("tok-ra")
	require.NoError(t, err)
	assert.EqualValues(t, 2, client.calls.Load(), "no validation at the original expiry")
}

// A refresh that the control instance rejects removes the entry at once, so a
// revoked token stops working no later than its TTL (sooner, in fact).
func TestValidateAPIToken_RefreshRejectionEvicts(t *testing.T) {
	ttl := 400 * time.Millisecond
	h, client := newRefreshTestService(t, ttl)
	_, err := h.ValidateAPIToken("tok-rev")
	require.NoError(t, err)

	client.resp.Store(&pb.TokenValidationResponse{Valid: false, ErrorMessage: "revoked"})
	time.Sleep(time.Duration(float64(ttl)*refreshAheadFraction) + 20*time.Millisecond)
	_, err = h.ValidateAPIToken("tok-rev") // still served; starts the refresh
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := h.ValidateAPIToken("tok-rev")
		return err != nil
	}, time.Second, 10*time.Millisecond, "the rejected token is refused once the refresh has run")
}
