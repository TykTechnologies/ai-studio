package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Values that read as "on" to a person but are not booleans fail startup too,
// rather than leaving the default in place (midsommar-clean-c1g.17).
func TestLoad_OCIRequireSignatureNonBooleanValuesAreErrors(t *testing.T) {
	for _, value := range []string{"yes", "on", "enabled", "flase"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
			t.Setenv("OCI_PLUGINS_REQUIRE_SIGNATURE", value)
			_, err := Load("")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "OCI_PLUGINS_REQUIRE_SIGNATURE")
		})
	}
}
