// tests/helm_edge_resilience_test.go
//
// The microgateway chart's settings for an edge with several control planes
// and a start without one (midsommar-clean-4x5y.23): what the values render
// into the edge's environment. The first test reads the files and runs
// without helm; the others render the chart and skip when helm is missing.
package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// edgeConfig is the data of the microgateway's own ConfigMap in a render of
// the umbrella chart.
func edgeConfig(t *testing.T, rendered string) map[string]any {
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
		if name, _ := dig(t, doc, "metadata", "name").(string); name == "test-microgateway-config" {
			data, _ := doc["data"].(map[string]any)
			return data
		}
	}
	t.Fatal("no microgateway ConfigMap in the render")
	return nil
}

func TestHelmEdgeResilienceDefaults(t *testing.T) {
	gateway := readValues(t, "../helm/charts/microgateway/values.yaml")
	assert.Empty(t, lookup(t, gateway, "config", "controlEndpoints"), "one control endpoint unless a list is given")
	assert.Equal(t, "", lookup(t, gateway, "config", "controlFailbackInterval"))
	assert.Equal(t, false, lookup(t, gateway, "config", "startFromLastKnownGood"), "an edge serves stored configuration only when asked to")
	assert.Equal(t, "", lookup(t, gateway, "config", "startupControlTimeout"))
	assert.Equal(t, "", lookup(t, gateway, "config", "lastKnownGoodMaxAge"))

	template, err := os.ReadFile("../helm/charts/microgateway/templates/configmap.yaml")
	require.NoError(t, err)
	for _, name := range []string{
		"CONTROL_ENDPOINTS:", "EDGE_CONTROL_FAILBACK_INTERVAL:", "EDGE_START_FROM_LAST_KNOWN_GOOD:",
		"EDGE_STARTUP_CONTROL_TIMEOUT:", "EDGE_LAST_KNOWN_GOOD_MAX_AGE:",
	} {
		assert.Contains(t, string(template), name)
	}

	notes, err := os.ReadFile("../helm/charts/microgateway/templates/NOTES.txt")
	require.NoError(t, err)
	assert.Contains(t, string(notes), `include "microgateway.notes"`)
}

// With default values the edge gets one control endpoint and none of the new
// settings, as before the settings existed.
func TestHelmEdgeDefaultRenderHasNoResilienceSettings(t *testing.T) {
	data := edgeConfig(t, helmTemplate(t))
	assert.Equal(t, "test:50051", data["CONTROL_ENDPOINT"])
	for _, name := range []string{
		"CONTROL_ENDPOINTS", "EDGE_CONTROL_FAILBACK_INTERVAL", "EDGE_START_FROM_LAST_KNOWN_GOOD",
		"EDGE_STARTUP_CONTROL_TIMEOUT", "EDGE_LAST_KNOWN_GOOD_MAX_AGE",
	} {
		assert.NotContains(t, data, name)
	}
}

// The edge refuses CONTROL_ENDPOINT and CONTROL_ENDPOINTS together, so a list
// must replace the single endpoint, in the order given (own site first).
func TestHelmEdgeControlEndpointsReplaceTheSingleEndpoint(t *testing.T) {
	data := edgeConfig(t, helmTemplate(t,
		"--set", "microgateway.config.controlEndpoint=ignored:50051",
		"--set", "microgateway.config.controlEndpoints={studio.site-a:50051,studio.site-b:50051}",
		"--set", "microgateway.config.controlFailbackInterval=5m"))
	assert.Equal(t, "studio.site-a:50051,studio.site-b:50051", data["CONTROL_ENDPOINTS"])
	assert.NotContains(t, data, "CONTROL_ENDPOINT")
	assert.Equal(t, "5m", data["EDGE_CONTROL_FAILBACK_INTERVAL"])
}

func TestHelmEdgeStartFromLastKnownGood(t *testing.T) {
	t.Run("on, with its limits", func(t *testing.T) {
		data := edgeConfig(t, helmTemplate(t,
			"--set", "microgateway.config.startFromLastKnownGood=true",
			"--set", "microgateway.config.startupControlTimeout=45s",
			"--set", "microgateway.config.lastKnownGoodMaxAge=24h"))
		assert.Equal(t, "true", data["EDGE_START_FROM_LAST_KNOWN_GOOD"])
		assert.Equal(t, "45s", data["EDGE_STARTUP_CONTROL_TIMEOUT"])
		assert.Equal(t, "24h", data["EDGE_LAST_KNOWN_GOOD_MAX_AGE"])
	})

	// "0" means no limit; --set makes it the number 0, which a template reads
	// as unset unless it is compared as a string.
	t.Run("a max age of 0 reaches the edge", func(t *testing.T) {
		data := edgeConfig(t, helmTemplate(t, "--set", "microgateway.config.lastKnownGoodMaxAge=0"))
		assert.Equal(t, "0", data["EDGE_LAST_KNOWN_GOOD_MAX_AGE"])
	})

	t.Run("the string false leaves it off", func(t *testing.T) {
		data := edgeConfig(t, helmTemplate(t, "--set-string", "microgateway.config.startFromLastKnownGood=false"))
		assert.NotContains(t, data, "EDGE_START_FROM_LAST_KNOWN_GOOD")
	})

	// The stored configuration is in the edge's database: with the
	// StatefulSet form it is on the volume claim, mounted where the chart's
	// database path points.
	t.Run("the database is on the volume claim of a StatefulSet", func(t *testing.T) {
		rendered := helmTemplate(t,
			"--set", "microgateway.config.startFromLastKnownGood=true",
			"--set", "microgateway.kind=StatefulSet",
			"--set", "microgateway.persistence.enabled=true")
		workload := microgatewayWorkload(t, rendered)
		claims, _ := dig(t, workload, "spec", "volumeClaimTemplates").([]any)
		require.Len(t, claims, 1)
		assert.Equal(t, "data", dig(t, claims[0].(map[string]any), "metadata", "name"))
		containers, _ := dig(t, workload, "spec", "template", "spec", "containers").([]any)
		var mounted bool
		for _, m := range containers[0].(map[string]any)["volumeMounts"].([]any) {
			mount := m.(map[string]any)
			if mount["name"] == "data" && mount["mountPath"] == "/opt/tyk-microgateway/data" {
				mounted = true
			}
		}
		assert.True(t, mounted, "the claim is mounted at the data directory")
	})
}
