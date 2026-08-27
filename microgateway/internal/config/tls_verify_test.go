package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #400: EDGE_SKIP_TLS_VERIFY must default to false, and enabling it
// must produce a prominent startup warning rather than being silent.

func TestSkipTLSVerifyDefaultsToFalse(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.False(t, cfg.HubSpoke.SkipTLSVerify, "EDGE_SKIP_TLS_VERIFY must default to false")
	assert.False(t, cfg.HubSpoke.AllowInsecure, "EDGE_ALLOW_INSECURE must default to false")
	assert.True(t, cfg.HubSpoke.ClientTLSEnabled, "EDGE_TLS_ENABLED must default to true")
}

func TestSkipTLSVerifyEnabledStillValidatesButWarns(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("EDGE_SKIP_TLS_VERIFY", "true")

	cfg, err := Load("")
	require.NoError(t, err, "enabling skip-verify remains operator-controlled and must not hard-fail")
	assert.True(t, cfg.HubSpoke.SkipTLSVerify)

	// The warning is emitted during Validate; assert the helper reports the
	// insecure state so callers surface it prominently.
	assert.True(t, cfg.HubSpoke.IsTLSVerificationDisabled())
}

func TestIsTLSVerificationDisabled(t *testing.T) {
	c := HubSpokeConfig{ClientTLSEnabled: true, SkipTLSVerify: true}
	assert.True(t, c.IsTLSVerificationDisabled())

	c = HubSpokeConfig{ClientTLSEnabled: true, SkipTLSVerify: false}
	assert.False(t, c.IsTLSVerificationDisabled())

	// With TLS disabled entirely, skip-verify is irrelevant (a separate
	// warning path covers plaintext connections).
	c = HubSpokeConfig{ClientTLSEnabled: false, SkipTLSVerify: true}
	assert.False(t, c.IsTLSVerificationDisabled())
}
