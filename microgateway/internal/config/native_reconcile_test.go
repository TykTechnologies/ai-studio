package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_NativeReconcilerTimings(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Zero(t, cfg.Gateway.NativeReconcileInterval, "0 keeps the gateway's default")
	assert.Zero(t, cfg.Gateway.NativeReconcileBatch)
	assert.Zero(t, cfg.Gateway.NativeReconcileWorkers)
	assert.Zero(t, cfg.Gateway.NativePollInterval)
	assert.Zero(t, cfg.Gateway.NativePollMaxBackoff)

	t.Setenv("NATIVE_API_RECONCILE_INTERVAL", "45s")
	t.Setenv("NATIVE_API_RECONCILE_BATCH", "200")
	t.Setenv("NATIVE_API_RECONCILE_WORKERS", "8")
	t.Setenv("NATIVE_API_POLL_INTERVAL", "5m")
	t.Setenv("NATIVE_API_POLL_MAX_BACKOFF", "2h")
	cfg, err = Load("")
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, cfg.Gateway.NativeReconcileInterval)
	assert.Equal(t, 200, cfg.Gateway.NativeReconcileBatch)
	assert.Equal(t, 8, cfg.Gateway.NativeReconcileWorkers)
	assert.Equal(t, 5*time.Minute, cfg.Gateway.NativePollInterval)
	assert.Equal(t, 2*time.Hour, cfg.Gateway.NativePollMaxBackoff)
}
