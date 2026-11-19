// tests/helm_studio_metrics_test.go
//
// The Studio chart's metrics values: what reaches the pod, the ServiceMonitor
// and the Service it selects, and the renders the chart refuses. AI Studio
// registers /metrics only with a token or the unauthenticated opt-out; without
// either the path answers with the web UI page, which is not metrics.
//
// Skips when helm is not installed.
package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// studioEnv returns the named entry of the Studio container's env, nil when
// there is none.
func studioEnv(t *testing.T, rendered, name string) map[string]any {
	t.Helper()
	d := renderedDoc(t, rendered, "Deployment", "test-midsommar")
	require.NotNil(t, d)
	env, _ := studioContainer(t, d)["env"].([]any)
	for _, e := range env {
		if entry, _ := e.(map[string]any); entry["name"] == name {
			return entry
		}
	}
	return nil
}

// The defaults leave /metrics unregistered, as before: metrics on, no token,
// no opt-out, no monitor.
func TestHelmStudioMetricsDefaults(t *testing.T) {
	values := readValues(t, "../helm/values.yaml")
	assert.Equal(t, true, lookup(t, values, "metrics", "enabled"))
	assert.Equal(t, false, lookup(t, values, "metrics", "allowUnauthenticated"))
	assert.Equal(t, "", lookup(t, values, "metrics", "bearerTokenSecret", "name"))
	assert.Equal(t, false, lookup(t, values, "metrics", "serviceMonitor", "enabled"))

	rendered := helmTemplate(t)
	assert.Nil(t, renderedDoc(t, rendered, "ServiceMonitor", "test-midsommar"))
	assert.Nil(t, studioEnv(t, rendered, "METRICS_AUTH_TOKEN"))

	config := studioConfig(t, rendered)
	assert.Equal(t, "true", config["METRICS_ENABLED"])
	assert.Equal(t, "/metrics", config["METRICS_PATH"])
	assert.NotContains(t, config, "METRICS_ALLOW_UNAUTHENTICATED")
}

// With a token Secret the pod reads the token and the monitor sends the same
// one, and the monitor selects the Studio Service only.
func TestHelmStudioServiceMonitorWithToken(t *testing.T) {
	rendered := helmTemplate(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.serviceMonitor.scrapeTimeout=10s",
		"--set", "metrics.serviceMonitor.labels.release=prometheus",
		"--set", "metrics.bearerTokenSecret.name=studio-metrics",
		"--set", "metrics.bearerTokenSecret.key=bearer",
		"--set", "metrics.path=/internal/metrics",
	)

	token := studioEnv(t, rendered, "METRICS_AUTH_TOKEN")
	require.NotNil(t, token, "the pod needs the token the monitor sends")
	ref, _ := dig(t, token, "valueFrom", "secretKeyRef").(map[string]any)
	assert.Equal(t, "studio-metrics", ref["name"])
	assert.Equal(t, "bearer", ref["key"])

	config := studioConfig(t, rendered)
	assert.Equal(t, "/internal/metrics", config["METRICS_PATH"])
	assert.NotContains(t, config, "METRICS_ALLOW_UNAUTHENTICATED")

	sm := renderedDoc(t, rendered, "ServiceMonitor", "test-midsommar")
	require.NotNil(t, sm)
	assert.Equal(t, "prometheus", dig(t, sm, "metadata", "labels", "release"))

	endpoints, _ := dig(t, sm, "spec", "endpoints").([]any)
	require.Len(t, endpoints, 1)
	endpoint, _ := endpoints[0].(map[string]any)
	assert.Equal(t, "http", endpoint["port"])
	assert.Equal(t, "/internal/metrics", endpoint["path"])
	assert.Equal(t, "30s", endpoint["interval"])
	assert.Equal(t, "10s", endpoint["scrapeTimeout"])
	credentials, _ := dig(t, endpoint, "authorization", "credentials").(map[string]any)
	assert.Equal(t, "studio-metrics", credentials["name"])
	assert.Equal(t, "bearer", credentials["key"])

	// Every label the monitor selects on is on the Studio Service, and the
	// Service has the port the monitor scrapes.
	selector, _ := dig(t, sm, "spec", "selector", "matchLabels").(map[string]any)
	require.NotEmpty(t, selector)
	service := renderedDoc(t, rendered, "Service", "test-midsommar")
	require.NotNil(t, service)
	serviceLabels, _ := dig(t, service, "metadata", "labels").(map[string]any)
	for key, value := range selector {
		assert.Equal(t, value, serviceLabels[key], "Service label %s", key)
	}
	var portNames []any
	ports, _ := dig(t, service, "spec", "ports").([]any)
	for _, p := range ports {
		port, _ := p.(map[string]any)
		portNames = append(portNames, port["name"])
	}
	assert.Contains(t, portNames, "http")

	// The bundled PostgreSQL Service is not a scrape target.
	postgres := renderedDoc(t, rendered, "Service", "test-midsommar-postgres")
	require.NotNil(t, postgres)
	postgresLabels, _ := dig(t, postgres, "metadata", "labels").(map[string]any)
	assert.NotEqual(t, selector["app.kubernetes.io/component"], postgresLabels["app.kubernetes.io/component"])
}

// The opt-out reaches the pod, and the monitor then sends no token.
func TestHelmStudioServiceMonitorUnauthenticated(t *testing.T) {
	rendered := helmTemplate(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.allowUnauthenticated=true",
	)

	assert.Equal(t, "true", studioConfig(t, rendered)["METRICS_ALLOW_UNAUTHENTICATED"])
	assert.Nil(t, studioEnv(t, rendered, "METRICS_AUTH_TOKEN"))

	sm := renderedDoc(t, rendered, "ServiceMonitor", "test-midsommar")
	require.NotNil(t, sm)
	endpoints, _ := dig(t, sm, "spec", "endpoints").([]any)
	require.Len(t, endpoints, 1)
	endpoint, _ := endpoints[0].(map[string]any)
	assert.Equal(t, "/metrics", endpoint["path"])
	assert.NotContains(t, endpoint, "authorization")
}

// metrics.enabled=false reaches the pod, as a boolean or as a string.
func TestHelmStudioMetricsOff(t *testing.T) {
	for _, flag := range []string{"--set", "--set-string"} {
		config := studioConfig(t, helmTemplate(t, flag, "metrics.enabled=false"))
		assert.Equal(t, "false", config["METRICS_ENABLED"], flag)
	}
}

func TestHelmStudioServiceMonitorRefused(t *testing.T) {
	out := helmTemplateExpectingError(t, "--set", "metrics.serviceMonitor.enabled=true")
	assert.Contains(t, out, "metrics.bearerTokenSecret")
	assert.Contains(t, out, "metrics.allowUnauthenticated")

	out = helmTemplateExpectingError(t,
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.allowUnauthenticated=true",
		"--set", "metrics.enabled=false",
	)
	assert.Contains(t, out, "metrics.enabled")
}
