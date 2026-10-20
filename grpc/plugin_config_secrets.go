package grpc

import (
	"reflect"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// pluginConfigSecretsForEdge resolves the top-level $SECRET/ references in a
// plugin's config for the edges, encrypted like an LLM's API key, by config
// key. The plain config keeps the reference: edges released before this field
// ignore it and still get the reference, never a value.
//
// Only what an edge runs gets values: a plugin with no gateway hook (a Studio
// UI or agent plugin) gets none, nor does an inactive one. Only references
// from the plugin's own config are resolved, since writing one there needs
// the right to read secrets; a reference that a per-LLM config override put
// in cfg is not (writing an override has no such check). A reference that
// cannot be resolved goes out as secrets.RefusedCredential, and $ENV/ is never
// resolved here: edges get nothing from the hub's environment.
func (s *ControlServer) pluginConfigSecretsForEdge(plugin *models.Plugin, cfg map[string]interface{}) map[string]string {
	if !plugin.IsActive || !pluginRunsOnGateway(plugin) {
		return nil
	}
	var out map[string]string
	for key, v := range cfg {
		ref, ok := v.(string)
		if !ok || !strings.HasPrefix(ref, secrets.SecretRefPrefix) {
			continue
		}
		if base, ok := plugin.Config[key]; !ok || !reflect.DeepEqual(base, v) {
			logger.Log.Warn().Uint("plugin_id", plugin.ID).Str("key", key).
				Msg("Plugin config secret reference comes from a per-LLM override; edges get it unresolved")
			continue
		}
		encrypted, err := s.encryptForMicrogateway(secrets.ForEdgeCredential(secrets.ResolveRef(ref)))
		if err != nil {
			// The plugin still runs, with the reference: leaving it out
			// would drop it from every LLM chain it enforces policy on.
			logger.Log.Error().Err(err).Uint("plugin_id", plugin.ID).Str("key", key).
				Msg("Failed to encrypt plugin config secret; edges get the reference unresolved")
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[key] = encrypted
	}
	return out
}

// pluginRunsOnGateway reports whether an edge would run the plugin at all.
func pluginRunsOnGateway(plugin *models.Plugin) bool {
	if interfaces.IsSupportedGatewayHookType(plugin.HookType) {
		return true
	}
	for _, ht := range plugin.HookTypes {
		if interfaces.IsSupportedGatewayHookType(ht) {
			return true
		}
	}
	return false
}
