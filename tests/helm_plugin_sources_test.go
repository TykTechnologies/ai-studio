// tests/helm_plugin_sources_test.go
//
// The charts expose the plugin source policy (midsommar-clean-c1g.14): a
// value in each chart, empty by default (all sources), rendered into the
// configmap only when set. Reads the files directly, so it runs without helm.
package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelmExposesPluginAllowedSources(t *testing.T) {
	umbrella := readValues(t, "../helm/values.yaml")
	assert.Equal(t, "", lookup(t, umbrella, "config", "pluginAllowedSources"), "Studio default allows all sources")
	assert.Equal(t, "", lookup(t, umbrella, "microgateway", "config", "pluginsAllowedSources"))
	gateway := readValues(t, "../helm/charts/microgateway/values.yaml")
	assert.Equal(t, "", lookup(t, gateway, "config", "pluginsAllowedSources"))

	studioTemplate, err := os.ReadFile("../helm/templates/configmap.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(studioTemplate), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES: {{ .Values.config.pluginAllowedSources | quote }}")
	gatewayTemplate, err := os.ReadFile("../helm/charts/microgateway/templates/configmap.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(gatewayTemplate), "PLUGINS_ALLOWED_SOURCES: {{ .Values.config.pluginsAllowedSources | quote }}")
}
