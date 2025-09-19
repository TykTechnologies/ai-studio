package config

import (
	"encoding/json"
	"fmt"

	"dario.cat/mergo"
)

// MergePluginConfigs merges base plugin config with per-LLM override using mergo.
// The override config takes precedence over the base config for conflicting keys.
func MergePluginConfigs(baseConfig, overrideConfig map[string]interface{}) (map[string]interface{}, error) {
	// Handle empty cases
	if len(overrideConfig) == 0 {
		// No override, return copy of base config
		if len(baseConfig) == 0 {
			return make(map[string]interface{}), nil
		}
		result := make(map[string]interface{})
		if err := mergo.Merge(&result, baseConfig); err != nil {
			return nil, fmt.Errorf("failed to copy base config: %w", err)
		}
		return result, nil
	}

	if len(baseConfig) == 0 {
		// No base config, return copy of override config
		result := make(map[string]interface{})
		if err := mergo.Merge(&result, overrideConfig); err != nil {
			return nil, fmt.Errorf("failed to copy override config: %w", err)
		}
		return result, nil
	}

	// Create result map
	result := make(map[string]interface{})

	// Copy base config first
	if err := mergo.Merge(&result, baseConfig); err != nil {
		return nil, fmt.Errorf("failed to copy base config: %w", err)
	}

	// Merge override with precedence (WithOverride means src overrides dst)
	if err := mergo.Merge(&result, overrideConfig, mergo.WithOverride); err != nil {
		return nil, fmt.Errorf("failed to merge config override: %w", err)
	}

	return result, nil
}

// MergePluginConfigsJSON handles JSON marshaling/unmarshaling for plugin configuration merging.
// Both baseConfigJSON and overrideConfigJSON can be nil or empty.
func MergePluginConfigsJSON(baseConfigJSON, overrideConfigJSON []byte) ([]byte, error) {
	var baseConfig, overrideConfig map[string]interface{}

	// Parse base config
	if len(baseConfigJSON) > 0 {
		if err := json.Unmarshal(baseConfigJSON, &baseConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base config: %w", err)
		}
	}

	// Parse override config
	if len(overrideConfigJSON) > 0 {
		if err := json.Unmarshal(overrideConfigJSON, &overrideConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal override config: %w", err)
		}
	}

	// Merge configurations
	merged, err := MergePluginConfigs(baseConfig, overrideConfig)
	if err != nil {
		return nil, err
	}

	// Marshal result back to JSON
	return json.Marshal(merged)
}

// MergePluginConfigMaps is a helper for merging Plugin.Config with LLMPlugin.ConfigOverride
// where both are stored as map[string]interface{} in the database.
func MergePluginConfigMaps(baseConfig, overrideConfig map[string]interface{}) (map[string]interface{}, error) {
	return MergePluginConfigs(baseConfig, overrideConfig)
}