package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

// An OCI plugin manager cannot be built without a security service, so no
// service container can leave its OCI client unable to verify signatures.
func TestNewPluginManagerWithOCI_RequiresSecurityService(t *testing.T) {
	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0

	_, err := NewPluginManagerWithOCI(nil, cfg, nil)
	require.Error(t, err)

	pm, err := NewPluginManagerWithOCI(nil, nil, nil)
	require.NoError(t, err, "without OCI no security service is needed")
	assert.Nil(t, pm.GetOCIClient())
}
