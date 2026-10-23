package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// From the environment to startup: a typo in AI_STUDIO_OCI_REQUIRE_SIGNATURE
// loads into a configuration that New refuses, naming the variable
// (midsommar-clean-c1g.17).
func TestNewRefusesInvalidOCIRequireSignatureFromEnv(t *testing.T) {
	for _, value := range []string{"ture", "yes", "on", "enabled"} {
		t.Run(value, func(t *testing.T) {
			opts := newTestOptions(t)
			vals := map[string]string{
				"DATABASE_TYPE":                   "sqlite",
				"TELEMETRY_ENABLED":               "false",
				"MARKETPLACE_ENABLED":             "false",
				"DOCS_DISABLED":                   "true",
				"TYK_AI_SECRET_KEY":               "embedded-test-secret-key",
				"AI_STUDIO_OCI_CACHE_DIR":         t.TempDir(),
				"AI_STUDIO_OCI_REQUIRE_SIGNATURE": value,
			}
			opts.Config = config.LoadFrom(func(k string) string { return vals[k] })

			s, err := New(opts)
			if s != nil {
				stopStudio(t, s)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AI_STUDIO_OCI_REQUIRE_SIGNATURE")
			assert.Contains(t, err.Error(), value)
		})
	}
}
