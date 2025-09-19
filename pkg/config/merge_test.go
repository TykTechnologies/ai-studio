package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergePluginConfigs(t *testing.T) {
	tests := []struct {
		name           string
		baseConfig     map[string]interface{}
		overrideConfig map[string]interface{}
		expected       map[string]interface{}
	}{
		{
			name:           "both configs empty",
			baseConfig:     map[string]interface{}{},
			overrideConfig: map[string]interface{}{},
			expected:       map[string]interface{}{},
		},
		{
			name:           "both configs nil",
			baseConfig:     nil,
			overrideConfig: nil,
			expected:       map[string]interface{}{},
		},
		{
			name: "only base config",
			baseConfig: map[string]interface{}{
				"timeout": 30,
				"retries": 3,
			},
			overrideConfig: nil,
			expected: map[string]interface{}{
				"timeout": 30,
				"retries": 3,
			},
		},
		{
			name:       "only override config",
			baseConfig: nil,
			overrideConfig: map[string]interface{}{
				"timeout": 60,
			},
			expected: map[string]interface{}{
				"timeout": 60,
			},
		},
		{
			name: "simple override",
			baseConfig: map[string]interface{}{
				"timeout": 30,
				"retries": 3,
				"debug":   false,
			},
			overrideConfig: map[string]interface{}{
				"timeout": 60, // Override
			},
			expected: map[string]interface{}{
				"timeout": 60, // Overridden
				"retries": 3,  // From base
				"debug":   false,
			},
		},
		{
			name: "add new fields",
			baseConfig: map[string]interface{}{
				"timeout": 30,
				"retries": 3,
			},
			overrideConfig: map[string]interface{}{
				"debug":   true, // New field
				"verbose": true, // New field
			},
			expected: map[string]interface{}{
				"timeout": 30,
				"retries": 3,
				"debug":   true,
				"verbose": true,
			},
		},
		{
			name: "nested config merge",
			baseConfig: map[string]interface{}{
				"auth": map[string]interface{}{
					"method":  "bearer",
					"timeout": 10,
					"retries": 2,
				},
				"logging": map[string]interface{}{
					"level": "info",
				},
			},
			overrideConfig: map[string]interface{}{
				"auth": map[string]interface{}{
					"timeout": 20, // Override nested value
				},
			},
			expected: map[string]interface{}{
				"auth": map[string]interface{}{
					"method":  "bearer", // From base
					"timeout": 20,       // Overridden
					"retries": 2,        // From base
				},
				"logging": map[string]interface{}{
					"level": "info", // From base
				},
			},
		},
		{
			name: "complex nested merge with arrays",
			baseConfig: map[string]interface{}{
				"endpoints": []interface{}{
					"https://api1.example.com",
					"https://api2.example.com",
				},
				"config": map[string]interface{}{
					"retries": 3,
					"timeout": 30,
				},
			},
			overrideConfig: map[string]interface{}{
				"endpoints": []interface{}{
					"https://override.example.com",
				},
				"config": map[string]interface{}{
					"timeout": 60,
				},
			},
			expected: map[string]interface{}{
				"endpoints": []interface{}{
					"https://override.example.com", // Arrays are replaced, not merged
				},
				"config": map[string]interface{}{
					"retries": 3,  // From base
					"timeout": 60, // Overridden
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergePluginConfigs(tt.baseConfig, tt.overrideConfig)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergePluginConfigsJSON(t *testing.T) {
	tests := []struct {
		name               string
		baseConfigJSON     []byte
		overrideConfigJSON []byte
		expectedJSON       string
	}{
		{
			name:               "both empty",
			baseConfigJSON:     nil,
			overrideConfigJSON: nil,
			expectedJSON:       "{}",
		},
		{
			name:               "empty JSON objects",
			baseConfigJSON:     []byte("{}"),
			overrideConfigJSON: []byte("{}"),
			expectedJSON:       "{}",
		},
		{
			name:               "only base config",
			baseConfigJSON:     []byte(`{"timeout":30,"retries":3}`),
			overrideConfigJSON: nil,
			expectedJSON:       `{"retries":3,"timeout":30}`,
		},
		{
			name:               "simple override",
			baseConfigJSON:     []byte(`{"timeout":30,"retries":3,"debug":false}`),
			overrideConfigJSON: []byte(`{"timeout":60}`),
			expectedJSON:       `{"debug":false,"retries":3,"timeout":60}`,
		},
		{
			name:               "nested config merge",
			baseConfigJSON:     []byte(`{"auth":{"method":"bearer","timeout":10},"logging":{"level":"info"}}`),
			overrideConfigJSON: []byte(`{"auth":{"timeout":20}}`),
			expectedJSON:       `{"auth":{"method":"bearer","timeout":20},"logging":{"level":"info"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergePluginConfigsJSON(tt.baseConfigJSON, tt.overrideConfigJSON)
			require.NoError(t, err)

			// Parse both result and expected to compare as objects (to ignore key ordering)
			var resultObj, expectedObj map[string]interface{}
			require.NoError(t, json.Unmarshal(result, &resultObj))
			require.NoError(t, json.Unmarshal([]byte(tt.expectedJSON), &expectedObj))

			assert.Equal(t, expectedObj, resultObj)
		})
	}
}

func TestMergePluginConfigsJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name               string
		baseConfigJSON     []byte
		overrideConfigJSON []byte
		expectedError      string
	}{
		{
			name:               "invalid base config",
			baseConfigJSON:     []byte(`{"invalid": json}`),
			overrideConfigJSON: []byte(`{"timeout": 60}`),
			expectedError:      "failed to unmarshal base config",
		},
		{
			name:               "invalid override config",
			baseConfigJSON:     []byte(`{"timeout": 30}`),
			overrideConfigJSON: []byte(`{"invalid": json}`),
			expectedError:      "failed to unmarshal override config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergePluginConfigsJSON(tt.baseConfigJSON, tt.overrideConfigJSON)
			assert.Nil(t, result)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestMergePluginConfigMaps(t *testing.T) {
	// This is just an alias function, so we test it works the same as MergePluginConfigs
	baseConfig := map[string]interface{}{
		"timeout": 30,
		"retries": 3,
	}
	overrideConfig := map[string]interface{}{
		"timeout": 60,
	}
	expected := map[string]interface{}{
		"timeout": 60,
		"retries": 3,
	}

	result, err := MergePluginConfigMaps(baseConfig, overrideConfig)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// Example of real plugin configuration merging scenario
func TestRealWorldExample(t *testing.T) {
	// Base message modifier plugin config
	baseConfig := map[string]interface{}{
		"instruction": "Say Moo! at the end of your response",
		"enabled":     true,
		"priority":    100,
	}

	// LLM-specific override for a different instruction
	overrideConfig := map[string]interface{}{
		"instruction": "Say Moo Too! at the end of your response",
		"priority":    200,
	}

	expected := map[string]interface{}{
		"instruction": "Say Moo Too! at the end of your response", // Overridden
		"enabled":     true,                                      // From base
		"priority":    200,                                       // Overridden
	}

	result, err := MergePluginConfigs(baseConfig, overrideConfig)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}