package plugins

import (
	"testing"
	"time"
)

// A failing data collection plugin is marked unhealthy under the write lock.
// That used to happen while ExecuteDataCollectionPlugins still held the read
// lock, which deadlocked the calling goroutine and, with a writer queued,
// every later hook call.
func TestExecuteDataCollectionPluginsFailureDoesNotDeadlock(t *testing.T) {
	failing := &GlobalPlugin{IsHealthy: true, LoadedPlugin: &LoadedPlugin{}}
	pm := &PluginManager{
		globalDataPlugins:       map[string]*GlobalPlugin{"failing": failing},
		dataCollectionHookTypes: map[string][]string{"failing": {"analytics"}},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Wrong payload type for the hook: the plugin call fails.
		_ = pm.ExecuteDataCollectionPlugins("analytics", "not analytics data")
		_ = pm.ExecuteDataCollectionPlugins("analytics", "not analytics data")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteDataCollectionPlugins deadlocked on a plugin failure")
	}

	pm.mu.RLock()
	healthy := failing.IsHealthy
	pm.mu.RUnlock()
	if healthy {
		t.Fatal("a failing plugin should be marked unhealthy")
	}
}
