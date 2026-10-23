package studio

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// An unknown transparency-log policy or an unreadable trusted root refuses
// to start, naming the variable (midsommar-clean-c1g.5).
func TestNewRefusesInvalidOCIVerificationSettingsFromEnv(t *testing.T) {
	cases := map[string]map[string]string{
		"AI_STUDIO_OCI_TLOG_POLICY":       {"AI_STUDIO_OCI_TLOG_POLICY": "requried"},
		"AI_STUDIO_OCI_TRUSTED_ROOT_FILE": {"AI_STUDIO_OCI_TRUSTED_ROOT_FILE": filepath.Join(t.TempDir(), "missing.json")},
	}
	for envVar, extra := range cases {
		t.Run(envVar, func(t *testing.T) {
			opts := newTestOptions(t)
			vals := map[string]string{
				"DATABASE_TYPE":           "sqlite",
				"TELEMETRY_ENABLED":       "false",
				"MARKETPLACE_ENABLED":     "false",
				"DOCS_DISABLED":           "true",
				"TYK_AI_SECRET_KEY":       "embedded-test-secret-key",
				"AI_STUDIO_OCI_CACHE_DIR": t.TempDir(),
			}
			for k, v := range extra {
				vals[k] = v
			}
			opts.Config = config.LoadFrom(func(k string) string { return vals[k] })

			s, err := New(opts)
			if s != nil {
				stopStudio(t, s)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), envVar)
		})
	}
}
