package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
)

// LLM_UPSTREAM_BLOCK_INTERNAL is on unless set to false, applied to netguard
// at startup, and a value that is not a boolean fails startup (sf5).
func TestLoad_BlockInternal(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	prev := netguard.SetBlockInternal(true)
	t.Cleanup(func() { netguard.SetBlockInternal(prev) })

	t.Setenv(netguard.EnvBlockInternal, "false")
	_, err := Load("")
	require.NoError(t, err)
	assert.False(t, netguard.SetBlockInternal(false), "false turns blocking off")

	t.Setenv(netguard.EnvBlockInternal, "")
	_, err = Load("")
	require.NoError(t, err)
	assert.True(t, netguard.SetBlockInternal(false), "unset means on")

	for _, value := range []string{"yes", "ture", "off"} {
		t.Setenv(netguard.EnvBlockInternal, value)
		_, err := Load("")
		require.Error(t, err, value)
		assert.Contains(t, err.Error(), netguard.EnvBlockInternal)
	}
}
