package plugins

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// blackHole stands in for a grpc:// plugin host that drops packets: a dial
// to it returns only when its context ends or the test releases it.
type blackHole struct {
	dials   atomic.Int32
	release chan struct{}
}

func stubBlackHole(t *testing.T) *blackHole {
	t.Helper()
	h := &blackHole{release: make(chan struct{})}
	saved := dialGRPCPlugin
	dialGRPCPlugin = func(ctx context.Context, _ net.Addr) (net.Conn, error) {
		h.dials.Add(1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.release:
			return nil, errors.New("connection refused")
		}
	}
	t.Cleanup(func() { dialGRPCPlugin = saved })
	return h
}

func setGRPCConnectBounds(t *testing.T, attempts int, timeout, delay time.Duration) {
	t.Helper()
	a, c, d := grpcConnectAttempts, grpcConnectTimeout, grpcConnectRetryDelay
	grpcConnectAttempts, grpcConnectTimeout, grpcConnectRetryDelay = attempts, timeout, delay
	t.Cleanup(func() { grpcConnectAttempts, grpcConnectTimeout, grpcConnectRetryDelay = a, c, d })
}

// waitForLoad returns when no request-path load of the plugin is in
// progress, so a background attempt (and what it logs when it ends) does not
// outlive the test's stubs.
func waitForLoad(pm *PluginManager, pluginID uint) {
	_, _, _ = pm.requestLoads.Do(strconv.FormatUint(uint64(pluginID), 10), func() (interface{}, error) { return nil, nil })
	pm.pluginLoadLocks.lock(pluginID)()
}

// within fails the test if fn has not returned after d.
func within(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %s", what, d)
	}
}

// unreachableAndHealthy is a manager with LLM 1 on a grpc:// plugin whose
// host is unreachable and LLM 2 on a plugin that is loaded.
func unreachableAndHealthy(t *testing.T) (*PluginManager, *loadCountingService) {
	t.Helper()
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	dead := PluginData{ID: 1, Name: "ext-grpc-down", HookType: "post_auth", Command: "grpc://127.0.0.1:9", IsActive: true}
	healthy := PluginData{ID: 2, Name: "healthy", HookType: "post_auth", Command: "/bin/true", IsActive: true}
	svc := &loadCountingService{mockPluginService: &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {dead}, 2: {healthy}},
		getPluginByID: map[uint]PluginData{1: dead, 2: healthy},
	}}
	pm := NewPluginManager(svc)
	pm.loadedPlugins[2] = &LoadedPlugin{ID: 2, Name: "healthy", HookType: interfaces.HookTypePostAuth, IsHealthy: true}
	return pm, svc
}

// A connect to an unreachable grpc:// plugin must not hold the manager's
// lock: requests for other LLMs need it to find their plugins. It was held
// across the connect, so one such plugin stalled every request on the edge
// for minutes (2.3.0-rc4 finding M2).
func TestLoadPlugin_BlockedConnectDoesNotStallOtherPlugins(t *testing.T) {
	hole := stubBlackHole(t)
	setGRPCConnectBounds(t, 1, time.Minute, 0)
	pm, svc := unreachableAndHealthy(t)
	pm.loadWaitTimeout = time.Minute

	// Requests for the LLM with the unreachable plugin: all wait on one
	// connect attempt.
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
			errs <- err
		}()
	}
	require.Eventually(t, func() bool { return hole.dials.Load() == 1 }, 5*time.Second, 5*time.Millisecond,
		"the connect to the unreachable host is in progress")

	// Meanwhile everything that needs the manager's lock still answers.
	within(t, 2*time.Second, "a request for another LLM", func() {
		list, err := pm.GetPluginsForLLM(2, interfaces.HookTypePostAuth)
		assert.NoError(t, err)
		assert.Len(t, list, 1)
	})
	within(t, 2*time.Second, "IsPluginLoaded", func() { assert.True(t, pm.IsPluginLoaded(2)) })
	within(t, 2*time.Second, "GetPluginHealth", func() { pm.GetPluginHealth() })
	within(t, 2*time.Second, "a reconcile snapshot", func() { pm.GetLoadedPlugins() })

	close(hole.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePluginLoadError(t, err, 1)
	}
	assert.EqualValues(t, 1, hole.dials.Load(), "ten requests, one connect attempt")
	assert.EqualValues(t, 1, svc.loads.Load(), "ten requests, one load")

	// The failure is remembered: the next request is refused without a connect.
	_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	assert.True(t, requirePluginLoadError(t, err, 1).Remembered)
	assert.EqualValues(t, 1, hole.dials.Load())
}

// One attempt to connect to a host that drops packets ends at the connect
// timeout, and a load makes a bounded number of them. The 10-second context
// the code had was never passed to the dial.
func TestConnectWithRetry_UnreachableHostIsBounded(t *testing.T) {
	hole := stubBlackHole(t)
	setGRPCConnectBounds(t, 3, 30*time.Millisecond, 10*time.Millisecond)
	pm, _ := unreachableAndHealthy(t)

	start := time.Now()
	var err error
	within(t, 5*time.Second, "the load", func() { _, err = pm.LoadPlugin(1) })
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 3, hole.dials.Load(), "three attempts")
	assert.GreaterOrEqual(t, time.Since(start), 3*30*time.Millisecond)
	assert.Equal(t, PluginStatusFailed, pm.GetPluginHealth()[1].Status)
}

// A request does not wait for a load longer than the load wait timeout; the
// load goes on, and later requests do not start another.
func TestGetPluginsForLLM_RequestWaitIsBounded(t *testing.T) {
	hole := stubBlackHole(t)
	setGRPCConnectBounds(t, 1, time.Minute, 0)
	pm, svc := unreachableAndHealthy(t)
	pm.loadWaitTimeout = 30 * time.Millisecond

	for i := 0; i < 3; i++ {
		within(t, 2*time.Second, "a request for the unreachable plugin's LLM", func() {
			_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
			loadErr := requirePluginLoadError(t, err, 1)
			assert.ErrorIs(t, loadErr, errPluginLoadInProgress)
		})
	}
	assert.EqualValues(t, 1, hole.dials.Load(), "one connect attempt for all of them")

	close(hole.release)
	waitForLoad(pm, 1)
	require.Eventually(t, func() bool { return pm.rememberedLoadFailure(1) != nil }, 5*time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 1, svc.loads.Load())
}

// Once a failure is older than the retry interval, a request starts the
// retry and is refused at once instead of waiting for it; requests for a
// plugin that stays unreachable never queue behind its connect attempts.
func TestGetPluginsForLLM_RetryOfAnUnreachablePluginHoldsNoRequest(t *testing.T) {
	hole := stubBlackHole(t)
	setGRPCConnectBounds(t, 1, time.Minute, 0)
	pm, _ := unreachableAndHealthy(t)
	pm.loadWaitTimeout = time.Minute
	pm.rememberLoadFailure(1, "ext-grpc-down", errors.New("connection timed out"))
	pm.loadRetryInterval = time.Nanosecond // the failure is stale

	for i := 0; i < 5; i++ {
		within(t, 2*time.Second, "a request during the retry", func() {
			_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
			assert.True(t, requirePluginLoadError(t, err, 1).Remembered)
		})
	}
	require.Eventually(t, func() bool { return hole.dials.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 1, hole.dials.Load(), "one retry in flight, whatever the number of requests")
	close(hole.release)
	waitForLoad(pm, 1)
}

// When the cause of a failure goes away, the LLM serves again without a
// restart or a configuration push: the first request after the retry
// interval loads the plugin in the background.
func TestGetPluginsForLLM_RecoversAfterTheRetryInterval(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")

	p := PluginData{ID: 1, Name: "came-back", HookType: "post_auth", Command: self, IsActive: true}
	svc := &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {p}},
		getPluginByID: map[uint]PluginData{1: p},
	}
	pm := NewPluginManager(svc)
	pm.loadRetryInterval = 20 * time.Millisecond
	pm.SetSourcePolicy(mustSourcePolicy(t, "oci"))
	t.Cleanup(func() { _ = pm.UnloadPlugin(1) })

	_, err = pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	requirePluginLoadError(t, err, 1)

	pm.SetSourcePolicy(plugin_security.AllowAllSources())
	require.Eventually(t, func() bool {
		list, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
		return err == nil && len(list) == 1
	}, 20*time.Second, 20*time.Millisecond, "the plugin loads on a later request")
	assert.Nil(t, pm.rememberedLoadFailure(1))
}

// hangingShutdown is a loaded plugin whose host is gone: Shutdown returns
// only when its context ends.
type hangingShutdown struct {
	pb.PluginServiceClient
	called  chan struct{}
	release chan struct{}
}

func (h *hangingShutdown) Shutdown(ctx context.Context, _ *pb.ShutdownRequest, _ ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	close(h.called)
	select {
	case <-ctx.Done():
	case <-h.release:
	}
	return nil, errors.New("plugin host unreachable")
}

// Unloading a plugin whose host is gone (the health check's reload) waits on
// its Shutdown call without the manager's lock.
func TestUnloadPlugin_HangingShutdownDoesNotStallOtherPlugins(t *testing.T) {
	pm, _ := unreachableAndHealthy(t)
	gone := &hangingShutdown{called: make(chan struct{}), release: make(chan struct{})}
	pm.loadedPlugins[1] = &LoadedPlugin{ID: 1, Name: "ext-grpc-down", HookType: interfaces.HookTypePostAuth, IsHealthy: true, GRPCClient: gone}

	unloaded := make(chan error, 1)
	go func() { unloaded <- pm.UnloadPlugin(1) }()
	select {
	case <-gone.called:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown was not called")
	}

	within(t, 2*time.Second, "a request for another LLM", func() {
		list, err := pm.GetPluginsForLLM(2, interfaces.HookTypePostAuth)
		assert.NoError(t, err)
		assert.Len(t, list, 1)
	})
	assert.False(t, pm.IsPluginLoaded(1), "requests no longer get the plugin being unloaded")

	close(gone.release)
	select {
	case err := <-unloaded:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("UnloadPlugin did not return")
	}
}

// A load still running when the manager shuts down does not leave its plugin
// process behind: Shutdown stops what is in the maps, and the load, which no
// longer holds the manager's lock, would have stored its plugin afterwards.
func TestLoadPlugin_AfterShutdownStopsItsOwnProcess(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")

	p := PluginData{ID: 1, Name: "late", HookType: "post_auth", Command: self, IsActive: true}
	pm := NewPluginManager(&mockPluginService{getPluginByID: map[uint]PluginData{1: p}})
	require.NoError(t, pm.Shutdown(context.Background()))

	_, err = pm.LoadPlugin(1)
	require.Error(t, err)
	assert.False(t, pm.IsPluginLoaded(1))
	assert.Empty(t, pm.GetLoadedPlugins())
}
