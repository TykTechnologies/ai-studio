package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// A mistyped AI_STUDIO_PLUGIN_ALLOWED_SOURCES refuses to start, whether or
// not OCI plugins are enabled: lockdown is independent of OCI
// (midsommar-clean-c1g.9).
func TestNewRefusesInvalidPluginAllowedSourcesFromEnv(t *testing.T) {
	for _, value := range []string{"ocii", ",", "oci,files"} {
		for _, oci := range []bool{false, true} {
			name := value
			if oci {
				name += "/oci-enabled"
			}
			t.Run(name, func(t *testing.T) {
				opts := newTestOptions(t)
				vals := map[string]string{
					"DATABASE_TYPE":                    "sqlite",
					"TELEMETRY_ENABLED":                "false",
					"MARKETPLACE_ENABLED":              "false",
					"DOCS_DISABLED":                    "true",
					"TYK_AI_SECRET_KEY":                "embedded-test-secret-key",
					"BRANDING_STORAGE_PATH":            t.TempDir(),
					"AI_STUDIO_PLUGIN_ALLOWED_SOURCES": value,
				}
				if oci {
					vals["AI_STUDIO_OCI_CACHE_DIR"] = t.TempDir()
				}
				opts.Config = config.LoadFrom(func(k string) string { return vals[k] })

				s, err := New(opts)
				if s != nil {
					stopStudio(t, s)
				}
				require.Error(t, err)
				assert.Contains(t, err.Error(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES")
			})
		}
	}
}
