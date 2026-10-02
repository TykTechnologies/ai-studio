package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/rs/zerolog/log"
)

// AuthPluginTimeout bounds one auth plugin's answer. Auth runs on every
// request to an endpoint with auth plugins, before anything else, and a
// plugin that hangs must not hold requests for the 30 seconds other hooks get.
const AuthPluginTimeout = 5 * time.Second

// Endpoint kinds for GetAuthPlugins; they match the hub's endpoint types and
// proxy.AuthTarget kinds.
const (
	AuthEndpointLLM = "llm"
)

// GetAuthPlugins returns the auth plugins attached to an endpoint (an LLM, or
// a datasource, tool, router or custom-endpoint plugin by its endpoint type),
// in execution order. attached counts every auth plugin attached and active
// on this edge; loaded holds those that are loaded and healthy. A caller that
// finds attached > 0 and nothing loaded must refuse the request: the endpoint
// is meant to be authenticated by plugins, and app keys must not stand in.
func (pm *PluginManager) GetAuthPlugins(endpointType string, endpointID uint) (attached int, loaded []*LoadedPlugin, err error) {
	var candidates []PluginData
	if endpointType == AuthEndpointLLM {
		all, err := pm.service.GetPluginsForLLM(endpointID)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get plugins for LLM %d: %w", endpointID, err)
		}
		for _, p := range all {
			if p.SupportsHookType(string(interfaces.HookTypeAuth)) {
				candidates = append(candidates, p)
			}
		}
	} else {
		candidates, err = pm.service.GetAuthPluginsForEndpoint(endpointType, endpointID)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get auth plugins for %s %d: %w", endpointType, endpointID, err)
		}
	}

	for _, p := range candidates {
		if !p.SupportsHookType(string(interfaces.HookTypeAuth)) {
			continue
		}
		attached++

		pm.mu.RLock()
		lp, ok := pm.loadedPlugins[p.ID]
		pm.mu.RUnlock()
		if !ok {
			lp, err = pm.LoadPlugin(p.ID)
			if err != nil {
				log.Error().Err(err).Uint("plugin_id", p.ID).Str("plugin_name", p.Name).
					Msg("Failed to load auth plugin; it cannot authenticate requests")
				continue
			}
		}

		pm.mu.RLock()
		healthy := lp.IsHealthy
		pm.mu.RUnlock()
		if !healthy {
			log.Warn().Uint("plugin_id", p.ID).Str("plugin_name", p.Name).Msg("Skipping unhealthy auth plugin")
			continue
		}
		loaded = append(loaded, lp)
	}
	return attached, loaded, nil
}

// CallAuth asks one auth plugin about a request.
func (pm *PluginManager) CallAuth(lp *LoadedPlugin, req *interfaces.AuthRequest, pluginCtx *interfaces.PluginContext) (*interfaces.AuthResponse, error) {
	if lp == nil || lp.GRPCClient == nil {
		return nil, fmt.Errorf("auth plugin is not loaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), AuthPluginTimeout)
	defer cancel()

	pbCtx := convertPluginContext(pluginCtx)
	resp, err := lp.GRPCClient.Authenticate(ctx, convertAuthRequest(req, pbCtx))
	if err != nil {
		return nil, fmt.Errorf("auth plugin %s: %w", lp.Name, err)
	}
	return convertAuthResponse(resp), nil
}
