package ociplugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTlogPolicy(t *testing.T) {
	assert.Equal(t, TlogPolicyRequired, (&OCIConfig{}).EffectiveTlogPolicy())
	assert.Equal(t, TlogPolicyRequired, (*OCIConfig)(nil).EffectiveTlogPolicy())
	assert.Equal(t, TlogPolicyDisabled, (&OCIConfig{TlogPolicy: TlogPolicyDisabled}).EffectiveTlogPolicy())
	assert.Equal(t, TlogPolicyRequired, DefaultOCIConfig().TlogPolicy)

	for in, want := range map[string]string{"": "required", "required": "required", " Optional ": "optional", "DISABLED": "disabled"} {
		got, err := ParseTlogPolicy(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"none", "off", "requried"} {
		_, err := ParseTlogPolicy(bad)
		assert.Error(t, err, bad)
	}
}

func TestValidateTrustedRootFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "trusted_root.json")
	require.NoError(t, os.WriteFile(file, []byte("{}"), 0o600))

	assert.NoError(t, ValidateTrustedRootFile(""))
	assert.NoError(t, ValidateTrustedRootFile(file))
	assert.Error(t, ValidateTrustedRootFile(filepath.Join(dir, "missing.json")))
	assert.Error(t, ValidateTrustedRootFile(dir), "a directory is not a trusted root")
}
