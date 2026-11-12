//go:build !enterprise

package studio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
)

// Requests through Studio's API across a failover (bead
// midsommar-clean-4x5y.6), on real pairs; skipped under -short. Community
// build only: an Enterprise Studio needs a license to start.

// heldStudio starts a Studio on the pair with a hold window, and returns its
// HTTP server and an administrator's API key.
func heldStudio(t *testing.T, pair *containers.PostgresPair, window time.Duration) (*Studio, string, string, uint) {
	t.Helper()
	opts := newTestOptions(t)
	opts.Config.DatabaseType = "postgres"
	opts.Config.DatabaseURL = pair.DSN()
	opts.Config.DatabaseWriteRetryWindow = window
	db := openHeldDB(t, pair, window)
	opts.DB = db
	s := newWithin(t, opts, 2*time.Minute)
	srv := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(srv.Close)
	admin := models.NewUser()
	admin.Email, admin.Name, admin.IsAdmin, admin.EmailVerified = "hold-admin@example.com", "Hold", true, true
	require.NoError(t, admin.Create(db))
	return s, srv.URL, admin.APIKey, admin.ID
}

func postJSON(t *testing.T, url, key, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// POST /api/v1/apps and POST /api/v1/credentials made while the primary is
// being replaced answer 201 once the standby is promoted inside the window,
// and exactly one App and one credential each are created. (The audit entry
// of the acceptance criteria is Enterprise: tests' rig run covers it.)
func TestFailover_CreatedAcrossAFailover(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx := context.Background()
	s, url, key, adminID := heldStudio(t, pair, 30*time.Second)
	var credsBefore, appsBefore int64
	require.NoError(t, s.db.Model(&models.Credential{}).Count(&credsBefore).Error)
	require.NoError(t, s.db.Model(&models.App{}).Count(&appsBefore).Error)

	require.NoError(t, pair.StopPrimary(ctx))
	type answer struct {
		code int
		body string
	}
	credential, app := make(chan answer, 1), make(chan answer, 1)
	go func() {
		resp, body := postJSON(t, url+"/api/v1/credentials", key, "{}")
		credential <- answer{resp.StatusCode, body}
	}()
	go func() {
		resp, body := postJSON(t, url+"/api/v1/apps", key, fmt.Sprintf(`{"data":{"type":"apps","attributes":{"name":"held-app","user_id":%d}}}`, adminID))
		app <- answer{resp.StatusCode, body}
	}()
	time.Sleep(3 * time.Second)
	require.NoError(t, pair.Promote(ctx))
	c, a := <-credential, <-app
	assert.Equal(t, http.StatusCreated, c.code, c.body)
	assert.Equal(t, http.StatusCreated, a.code, a.body)

	var creds, apps, named int64
	require.NoError(t, s.db.Model(&models.Credential{}).Count(&creds).Error)
	require.NoError(t, s.db.Model(&models.App{}).Count(&apps).Error)
	require.NoError(t, s.db.Model(&models.App{}).Where("name = ?", "held-app").Count(&named).Error)
	assert.Equal(t, appsBefore+1, apps, "exactly one App")
	assert.EqualValues(t, 1, named)
	assert.Equal(t, credsBefore+2, creds, "exactly one credential from the POST, and the App's own")
	stopStudio(t, s)
}

// With no promotion inside the window: 503 with Retry-After and code
// database_unavailable, and nothing is written.
func TestFailover_CredentialRefusedWith503AfterTheWindow(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	ctx := context.Background()
	s, url, key, _ := heldStudio(t, pair, 2*time.Second)
	var before int64
	require.NoError(t, s.db.Model(&models.Credential{}).Count(&before).Error)

	require.NoError(t, pair.StopPrimary(ctx))
	started := time.Now()
	resp, body := postJSON(t, url+"/api/v1/credentials", key, "{}")
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, body)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
	assert.Contains(t, body, "database_unavailable")
	assert.Less(t, time.Since(started), 15*time.Second, "answered inside a bounded time, not hung")

	require.NoError(t, pair.Promote(ctx))
	var after int64
	require.NoError(t, s.db.Model(&models.Credential{}).Count(&after).Error)
	assert.Equal(t, before, after, "nothing written")
	stopStudio(t, s)
}

// Admin requests in flight during an outage against the cap on held
// statements: at most dbhold.MaxHeld wait, the rest are answered 503 at
// once. Logs the numbers (held at most, answers by kind, goroutines and
// heap) for the PR.
func TestFailover_HeldStatementsAreCapped(t *testing.T) {
	// HOLD_CAP_REQUESTS overrides the number (3000 for the PR's figures).
	requests := 1500
	if n, err := strconv.Atoi(os.Getenv("HOLD_CAP_REQUESTS")); err == nil && n > 0 {
		requests = n
	}
	window := 10 * time.Second
	pair := containers.StartPostgresPair(t, nil)
	ctx := context.Background()
	s, url, key, _ := heldStudio(t, pair, window)
	pool, ok := s.db.ConnPool.(*dbhold.Pool)
	require.True(t, ok)

	// Keep-alive connections for all the clients at once.
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = requests
	var heapBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&heapBefore)
	goroutinesBefore := runtime.NumGoroutine()

	require.NoError(t, pair.StopPrimary(ctx))
	var maxHeld, maxGoroutines atomic.Int64
	var heapPeak atomic.Uint64
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			if h := pool.Held(); h > maxHeld.Load() {
				maxHeld.Store(h)
			}
			if g := int64(runtime.NumGoroutine()); g > maxGoroutines.Load() {
				maxGoroutines.Store(g)
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > heapPeak.Load() {
				heapPeak.Store(m.HeapAlloc)
			}
		}
	}()

	var fast503, slow503, other atomic.Int64
	var wg sync.WaitGroup
	started := time.Now()
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t0 := time.Now()
			resp, body := postJSON(t, url+"/api/v1/credentials", key, "{}")
			switch {
			case resp.StatusCode == http.StatusServiceUnavailable && time.Since(t0) < window/2:
				fast503.Add(1)
			case resp.StatusCode == http.StatusServiceUnavailable:
				slow503.Add(1)
			default:
				other.Add(1)
				t.Logf("unexpected %d: %s", resp.StatusCode, body)
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-sampled
	took := time.Since(started)

	t.Logf("%d concurrent POST /api/v1/credentials with no primary, window %s, MaxHeld %d: statements held at most %d; 503 at once %d, 503 after the window %d, other %d; all answered in %s; goroutines %d before, %d at most; heap %.1f MB before, %.1f MB at most",
		requests, window, dbhold.MaxHeld, maxHeld.Load(), fast503.Load(), slow503.Load(), other.Load(), took.Round(time.Millisecond),
		goroutinesBefore, maxGoroutines.Load(), float64(heapBefore.HeapAlloc)/(1<<20), float64(heapPeak.Load())/(1<<20))
	assert.LessOrEqual(t, maxHeld.Load(), int64(dbhold.MaxHeld))
	assert.Zero(t, other.Load(), "every answer is 503 database_unavailable")
	// Loose: the time also counts the test's own 1,500 clients and the
	// pool's queue for its 8 connections on a busy machine.
	assert.Less(t, took, 6*window, "bounded")

	require.NoError(t, pair.Promote(ctx))
	stopStudio(t, s)
}
