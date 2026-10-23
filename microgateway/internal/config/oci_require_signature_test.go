package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_OCIRequireSignature(t *testing.T) {
	cases := map[string]bool{"": true, "true": true, "false": false}
	for value, want := range cases {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
			t.Setenv("OCI_PLUGINS_REQUIRE_SIGNATURE", value)
			cfg, err := Load("")
			require.NoError(t, err)
			assert.Equal(t, want, cfg.OCIPlugins.RequireSignature)
		})
	}
}

// A typo in a security setting fails startup instead of using the default.
func TestLoad_OCIRequireSignatureInvalidValueIsAnError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("OCI_PLUGINS_REQUIRE_SIGNATURE", "ture")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OCI_PLUGINS_REQUIRE_SIGNATURE")
}
