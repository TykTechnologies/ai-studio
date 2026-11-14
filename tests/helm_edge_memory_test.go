// tests/helm_edge_memory_test.go
//
// The microgateway chart's default memory: a 1Gi limit, so an edge's working
// set, the analytics backlog (64 MiB) and the memory around it fit under the
// edge's soft GOMEMLIMIT (90% of the limit) and its overload shedding (85%);
// 512Mi left 7 MB to spare in a 3.5-minute outage at 3,000 requests/s
// (bead midsommar-clean-4x5y.49, cluster run e5). The first test reads the
// values and runs without helm; the other renders the chart and skips when
// helm is missing.
package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelmEdgeMemoryDefault(t *testing.T) {
	gateway := readValues(t, "../helm/charts/microgateway/values.yaml")
	assert.Equal(t, "1Gi", lookup(t, gateway, "resources", "limits", "memory"))
	assert.Equal(t, "512Mi", lookup(t, gateway, "resources", "requests", "memory"))
	assert.Equal(t, "500m", lookup(t, gateway, "resources", "limits", "cpu"), "CPU unchanged")
	assert.Equal(t, "100m", lookup(t, gateway, "resources", "requests", "cpu"), "CPU unchanged")
}

// edgeMemory is the rendered edge container's memory request and limit.
func edgeMemory(t *testing.T, rendered string) (request, limit any) {
	t.Helper()
	workload := microgatewayWorkload(t, rendered)
	containers, _ := dig(t, workload, "spec", "template", "spec", "containers").([]any)
	require.NotEmpty(t, containers)
	container, _ := containers[0].(map[string]any)
	return dig(t, container, "resources", "requests", "memory"), dig(t, container, "resources", "limits", "memory")
}

func TestHelmEdgeMemoryRender(t *testing.T) {
	t.Run("bundled edge, default", func(t *testing.T) {
		request, limit := edgeMemory(t, helmTemplate(t))
		assert.Equal(t, "1Gi", limit)
		assert.Equal(t, "512Mi", request)
	})
	t.Run("StatefulSet", func(t *testing.T) {
		_, limit := edgeMemory(t, helmTemplate(t, "--set", "microgateway.kind=StatefulSet", "--set", "microgateway.persistence.enabled=true"))
		assert.Equal(t, "1Gi", limit)
	})
	t.Run("set by the operator", func(t *testing.T) {
		_, limit := edgeMemory(t, helmTemplate(t, "--set", "microgateway.resources.limits.memory=2Gi"))
		assert.Equal(t, "2Gi", limit)
	})
	examples, err := filepath.Glob("../helm/my-values-*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, examples)
	for _, file := range examples {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			abs, err := filepath.Abs(file)
			require.NoError(t, err)
			_, limit := edgeMemory(t, helmTemplate(t, "-f", abs))
			assert.Equal(t, "1Gi", limit, "the example does not set the edge's memory")
		})
	}
}
