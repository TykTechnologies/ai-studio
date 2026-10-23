package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

func TestLoad_PluginsAllowedSources(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.True(t, cfg.Plugins.SourcePolicy.AllowsAll(), "unset must allow every source")

	t.Setenv("PLUGINS_ALLOWED_SOURCES", "oci")
	cfg, err = Load("")
	require.NoError(t, err)
	assert.Equal(t, []plugin_security.PluginSource{plugin_security.SourceOCI}, cfg.Plugins.SourcePolicy.Allowed())
	err = cfg.Plugins.SourcePolicy.Check("file:///opt/plugins/demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGINS_ALLOWED_SOURCES")

	for _, bad := range []string{"ocii", ","} {
		t.Setenv("PLUGINS_ALLOWED_SOURCES", bad)
		_, err = Load("")
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "PLUGINS_ALLOWED_SOURCES")
	}
}
