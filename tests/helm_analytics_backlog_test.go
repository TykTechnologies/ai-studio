// tests/helm_analytics_backlog_test.go
//
// The microgateway chart's analytics backlog value (midsommar-clean-4x5y.36):
// analyticsPulse.maxBacklogBytes renders into the pulse file's
// max_backlog_bytes. The chart's default (64 MiB) is lower than the
// microgateway's own (256 MiB) because the chart's default memory limit is
// 512Mi. The first test reads the files and runs without helm; the other
// renders the chart and skips when helm is missing.
package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// pulseFileConfig is the analytics pulse plugin's config in a render of the
// umbrella chart.
func pulseFileConfig(t *testing.T, rendered string) map[string]any {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(rendered))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc == nil || doc["kind"] != "ConfigMap" {
			continue
		}
		if name, _ := dig(t, doc, "metadata", "name").(string); name != "test-microgateway-analytics" {
			continue
		}
		data, _ := doc["data"].(map[string]any)
		file, _ := data["analytics-pulse.yaml"].(string)
		var pulse map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(file), &pulse))
		plugins, _ := pulse["data_collection_plugins"].([]any)
		require.NotEmpty(t, plugins)
		first, _ := plugins[0].(map[string]any)
		config, _ := first["config"].(map[string]any)
		return config
	}
	t.Fatal("no analytics pulse ConfigMap in the render")
	return nil
}

func TestHelmAnalyticsBacklogDefault(t *testing.T) {
	gateway := readValues(t, "../helm/charts/microgateway/values.yaml")
	assert.EqualValues(t, 64<<20, lookup(t, gateway, "analyticsPulse", "maxBacklogBytes"),
		"64 MiB: the chart's default memory limit is 512Mi, too little for the microgateway's own 256 MiB default on top of an edge's working set")

	template, err := os.ReadFile("../helm/charts/microgateway/templates/analytics-configmap.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(template), "max_backlog_bytes: {{ .Values.analyticsPulse.maxBacklogBytes | int64 }}",
		"rendered as an integer: helm would print 67108864 as 6.7108864e+07")
}

func TestHelmAnalyticsBacklogRender(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		config := pulseFileConfig(t, helmTemplate(t))
		assert.Equal(t, 64<<20, config["max_backlog_bytes"])
		assert.Equal(t, 10000, config["max_buffer_size"], "max_buffer_size is unchanged")
	})
	t.Run("raised", func(t *testing.T) {
		config := pulseFileConfig(t, helmTemplate(t, "--set", "microgateway.analyticsPulse.maxBacklogBytes=268435456"))
		assert.Equal(t, 256<<20, config["max_backlog_bytes"])
	})
	t.Run("off", func(t *testing.T) {
		config := pulseFileConfig(t, helmTemplate(t, "--set", "microgateway.analyticsPulse.maxBacklogBytes=0"))
		assert.Equal(t, 0, config["max_backlog_bytes"])
	})
}
