package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// A plugin stored before the source policy was restricted does not stop
// Studio from starting: it is not loaded, and it can still be read
// (midsommar-clean-c1g.10).
func TestNewStartsWithAStoredPluginWhoseSourceIsDisallowed(t *testing.T) {
	opts := newTestOptions(t)
	vals := map[string]string{
		"DATABASE_TYPE":                    "sqlite",
		"TELEMETRY_ENABLED":                "false",
		"MARKETPLACE_ENABLED":              "false",
		"DOCS_DISABLED":                    "true",
		"TYK_AI_SECRET_KEY":                "embedded-test-secret-key",
		"SKIP_FILTER_DEFAULTS":             "true",
		"AI_STUDIO_PLUGIN_ALLOWED_SOURCES": "oci",
	}
	opts.Config = config.LoadFrom(func(k string) string { return vals[k] })
	require.NoError(t, opts.Config.Err())

	require.NoError(t, models.InitModels(opts.DB))
	stored := &models.Plugin{Name: "legacy", Command: "/bin/true", HookType: "post_auth", IsActive: true}
	require.NoError(t, opts.DB.Create(stored).Error)

	s, err := New(opts)
	require.NoError(t, err, "a disallowed stored plugin must not stop startup")
	defer stopStudio(t, s)

	assert.False(t, s.service.AIStudioPluginManager.IsPluginLoaded(stored.ID))
	var count int64
	require.NoError(t, opts.DB.Model(&models.Plugin{}).Where("id = ?", stored.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the stored plugin is kept")
}
