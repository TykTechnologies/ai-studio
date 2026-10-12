package config

import (
	"bytes"
	"strings"
	"testing"

	aiconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

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

// NATIVE_API_MAX_BODY_BYTES is capped at 256 MiB at load, with one warning
// naming it; values at or below the cap, and unset, are kept.
func TestLoad_NativeMaxBodyBytesCapped(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        int64
		warned      bool
	}{
		{"over the cap", "1073741824", aiconfig.NativeMaxBodyBytesLimit, true},
		{"just over the cap", "268435457", aiconfig.NativeMaxBodyBytesLimit, true},
		{"at the cap", "268435456", aiconfig.NativeMaxBodyBytesLimit, false},
		{"below the cap", "1048576", 1048576, false},
		{"unset", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
			t.Setenv("NATIVE_API_MAX_BODY_BYTES", tc.value)
			var logs bytes.Buffer
			saved := log.Logger
			log.Logger = zerolog.New(&logs)
			defer func() { log.Logger = saved }()

			cfg, err := Load("")
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Gateway.NativeMaxBodyBytes)
			want := 0
			if tc.warned {
				want = 1
			}
			assert.Equal(t, want, strings.Count(logs.String(), "NATIVE_API_MAX_BODY_BYTES"), logs.String())
		})
	}
}
