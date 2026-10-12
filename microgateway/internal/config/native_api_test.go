package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_NativeAPISettings(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Empty(t, cfg.Gateway.PublicURL)
	assert.Zero(t, cfg.Gateway.NativeMaxBodyBytes, "0 keeps the gateway's default")
	assert.Zero(t, cfg.Gateway.NativeMaxConcurrentCreates)
	assert.Zero(t, cfg.Gateway.NativeMaxFilteredItems)
	assert.Zero(t, cfg.Gateway.NativeMaxInProgressPerApp)

	t.Setenv("GATEWAY_PUBLIC_URL", "https://edge.example.com")
	t.Setenv("NATIVE_API_MAX_BODY_BYTES", "134217728")
	t.Setenv("NATIVE_API_MAX_CONCURRENT_CREATES", "8")
	t.Setenv("NATIVE_API_MAX_FILTERED_ITEMS", "500")
	t.Setenv("NATIVE_API_MAX_IN_PROGRESS_PER_APP", "3")
	cfg, err = Load("")
	require.NoError(t, err)
	assert.Equal(t, "https://edge.example.com", cfg.Gateway.PublicURL)
	assert.Equal(t, int64(134217728), cfg.Gateway.NativeMaxBodyBytes)
	assert.Equal(t, 8, cfg.Gateway.NativeMaxConcurrentCreates)
	assert.Equal(t, 500, cfg.Gateway.NativeMaxFilteredItems)
	assert.Equal(t, 3, cfg.Gateway.NativeMaxInProgressPerApp)
}
