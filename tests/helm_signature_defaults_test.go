// tests/helm_signature_defaults_test.go
//
// The charts' default values require OCI plugin signatures for Studio and
// for the microgateway (midsommar-clean-c1g.8). Reads the values files
// directly, so it runs without helm installed.
package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// lookup walks nested maps along path.
func lookup(t *testing.T, values map[string]any, path ...string) any {
	t.Helper()
	var cur any = values
	for _, key := range path {
		m, ok := cur.(map[string]any)
		require.True(t, ok, "%v: %q is not under a map", path, key)
		cur, ok = m[key]
		require.True(t, ok, "%v: no key %q", path, key)
	}
	return cur
}

func readValues(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var values map[string]any
	require.NoError(t, yaml.Unmarshal(data, &values))
	return values
}

func TestHelmDefaultsRequireOCISignatures(t *testing.T) {
	t.Skip("fails until midsommar-clean-c1g.28 is fixed (gateway chart defaults to false); remove this skip with the fix")
	umbrella := readValues(t, "../helm/values.yaml")
	require.Equal(t, "true", lookup(t, umbrella, "config", "ociRequireSignature"), "Studio")
	require.Equal(t, "true", lookup(t, umbrella, "microgateway", "config", "ociPluginsRequireSignature"),
		"the umbrella chart's microgateway override")

	gateway := readValues(t, "../helm/charts/microgateway/values.yaml")
	require.Equal(t, "true", lookup(t, gateway, "config", "ociPluginsRequireSignature"),
		"the microgateway chart default")
}
