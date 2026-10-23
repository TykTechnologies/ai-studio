package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

func TestLoad_OCIVerificationSettings(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	rootFile := filepath.Join(t.TempDir(), "trusted_root.json")
	require.NoError(t, os.WriteFile(rootFile, []byte("{}"), 0o600))

	cfg, err := Load("")
	require.NoError(t, err)
	lib := cfg.OCIPlugins.ToOCIConfig()
	assert.Equal(t, ociplugins.TlogPolicyRequired, lib.TlogPolicy)
	assert.Empty(t, lib.TrustedRootFile)

	t.Setenv("OCI_PLUGINS_TLOG_POLICY", "Optional")
	t.Setenv("OCI_PLUGINS_TRUSTED_ROOT_FILE", rootFile)
	cfg, err = Load("")
	require.NoError(t, err)
	lib = cfg.OCIPlugins.ToOCIConfig()
	assert.Equal(t, ociplugins.TlogPolicyOptional, lib.TlogPolicy)
	assert.Equal(t, rootFile, lib.TrustedRootFile)
}

func TestLoad_OCIVerificationSettingsInvalid(t *testing.T) {
	cases := map[string]string{
		"OCI_PLUGINS_TLOG_POLICY":       "off",
		"OCI_PLUGINS_TRUSTED_ROOT_FILE": filepath.Join(t.TempDir(), "missing.json"),
	}
	for envVar, value := range cases {
		t.Run(envVar, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
			t.Setenv(envVar, value)
			_, err := Load("")
			require.Error(t, err)
			assert.Contains(t, err.Error(), envVar)
		})
	}
}

func TestLoad_OCITlogPolicyDisabledWarnsOnce(t *testing.T) {
	var logs bytes.Buffer
	saved := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = saved })

	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("OCI_PLUGINS_TLOG_POLICY", "disabled")
	_, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(logs.String(), "OCI_PLUGINS_TLOG_POLICY=disabled"))
}
