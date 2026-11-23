package plugins

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// PluginLoadRetryInterval is how long a failed plugin load is remembered.
// A request that needs the plugin within that time is refused at once, with
// no new OCI fetch, signature check, exec or connect; the first request after
// it starts a new attempt (see loadPluginForRequest). A configuration sync
// (ReconcilePlugins) and an explicit LoadPlugin or ReloadPlugin always try at
// once.
const PluginLoadRetryInterval = 30 * time.Second

// PluginLoadError says a plugin a request needs could not be loaded. Callers
// on the request path must refuse the request: the plugin may be the one
// enforcing policy (a firewall, a rate limiter), so the others on the LLM
// must not run without it and the request must not go upstream.
type PluginLoadError struct {
	PluginID   uint
	PluginName string
	// Remembered is true when no load was tried for this request: an earlier
	// one failed less than PluginLoadRetryInterval ago.
	Remembered bool
	Err        error
}

func (e *PluginLoadError) Error() string {
	return fmt.Sprintf("plugin %d (%s) could not be loaded: %v", e.PluginID, e.PluginName, e.Err)
}

func (e *PluginLoadError) Unwrap() error { return e.Err }

// loadFailure is the last failed load of one plugin.
type loadFailure struct {
	name string
	err  error
	at   time.Time
}

func (pm *PluginManager) loadRetryIntervalOrDefault() time.Duration {
	if pm.loadRetryInterval > 0 {
		return pm.loadRetryInterval
	}
	return PluginLoadRetryInterval
}

// rememberedLoadFailure returns the plugin's last load failure while it is
// still fresh, else nil.
func (pm *PluginManager) rememberedLoadFailure(pluginID uint) *PluginLoadError {
	failed, fresh := pm.lastLoadFailure(pluginID)
	if !fresh {
		return nil
	}
	return failed
}

// lastLoadFailure returns the plugin's last load failure, if it has not
// loaded since, and whether it is fresh: less than the retry interval old.
func (pm *PluginManager) lastLoadFailure(pluginID uint) (failed *PluginLoadError, fresh bool) {
	pm.loadFailuresMu.RLock()
	defer pm.loadFailuresMu.RUnlock()
	f, ok := pm.loadFailures[pluginID]
	if !ok {
		return nil, false
	}
	return &PluginLoadError{PluginID: pluginID, PluginName: f.name, Remembered: true, Err: f.err},
		time.Since(f.at) < pm.loadRetryIntervalOrDefault()
}

// rememberLoadFailure records a failed load. It logs the plugin and the cause
// when the state changes (the first failure, or a different cause), not on
// every retry and never per request.
func (pm *PluginManager) rememberLoadFailure(pluginID uint, name string, err error) {
	pm.loadFailuresMu.Lock()
	prev, had := pm.loadFailures[pluginID]
	if pm.loadFailures == nil {
		pm.loadFailures = make(map[uint]*loadFailure)
	}
	if name == "" && had {
		name = prev.name
	}
	pm.loadFailures[pluginID] = &loadFailure{name: name, err: err, at: time.Now()}
	pm.loadFailuresMu.Unlock()

	if had && prev.err.Error() == err.Error() {
		log.Debug().Uint("plugin_id", pluginID).Str("plugin_name", name).Err(err).Msg("Plugin still cannot be loaded")
		return
	}
	log.Error().
		Uint("plugin_id", pluginID).
		Str("plugin_name", name).
		Err(err).
		Dur("retry_interval", pm.loadRetryIntervalOrDefault()).
		Msg("Plugin could not be loaded; requests to the LLMs it is attached to are refused until it loads")
}

// forgetLoadFailure clears a plugin's remembered failure once it has loaded.
func (pm *PluginManager) forgetLoadFailure(pluginID uint) {
	pm.loadFailuresMu.Lock()
	prev, had := pm.loadFailures[pluginID]
	delete(pm.loadFailures, pluginID)
	pm.loadFailuresMu.Unlock()
	if had {
		log.Info().Uint("plugin_id", pluginID).Str("plugin_name", prev.name).
			Msg("Plugin loaded after earlier failures; requests to its LLMs are served again")
	}
}

// pruneLoadFailures drops remembered failures of plugins that are no longer
// configured on this gateway.
func (pm *PluginManager) pruneLoadFailures(desired map[uint]PluginData) {
	pm.loadFailuresMu.Lock()
	defer pm.loadFailuresMu.Unlock()
	for id := range pm.loadFailures {
		if _, ok := desired[id]; !ok {
			delete(pm.loadFailures, id)
		}
	}
}

// pluginLoadWaitTimeout bounds how long a request waits for the load of a
// plugin it needs. A load that takes longer goes on, and the request is
// refused.
const pluginLoadWaitTimeout = 20 * time.Second

// errPluginLoadInProgress is the cause given to a request that did not wait
// for a load to finish.
var errPluginLoadInProgress = errors.New("the plugin is still loading")

func (pm *PluginManager) loadWaitTimeoutOrDefault() time.Duration {
	if pm.loadWaitTimeout > 0 {
		return pm.loadWaitTimeout
	}
	return pluginLoadWaitTimeout
}

// loadPluginForRequest loads a plugin a request needs. Every error is a
// *PluginLoadError. Unlike LoadPlugin:
//
//   - while an earlier failure is remembered it does not try again;
//   - once that failure is older than the retry interval it starts one new
//     attempt in the background and still refuses this request at once, so
//     requests for a plugin that keeps failing never queue behind its retries;
//   - concurrent requests for the same plugin share one attempt, and wait for
//     it (a plugin loading for the first time) no longer than the load wait
//     timeout.
func (pm *PluginManager) loadPluginForRequest(pluginID uint, name string) (*LoadedPlugin, error) {
	failed, fresh := pm.lastLoadFailure(pluginID)
	if fresh {
		return nil, failed
	}
	result := pm.requestLoads.DoChan(strconv.FormatUint(uint64(pluginID), 10), func() (interface{}, error) {
		// A request that waited behind another's attempt takes its result.
		if failed := pm.rememberedLoadFailure(pluginID); failed != nil {
			return nil, failed
		}
		lp, err := pm.loadPluginNamed(pluginID, name)
		if err != nil {
			return nil, &PluginLoadError{PluginID: pluginID, PluginName: name, Err: err}
		}
		return lp, nil
	})
	if failed != nil {
		// The retry runs without this request.
		return nil, failed
	}

	timer := time.NewTimer(pm.loadWaitTimeoutOrDefault())
	defer timer.Stop()
	select {
	case r := <-result:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*LoadedPlugin), nil
	case <-timer.C:
		return nil, &PluginLoadError{PluginID: pluginID, PluginName: name, Err: errPluginLoadInProgress}
	}
}
