package plugin_security_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

// models.Plugin's IsOCIPlugin/IsGRPCPlugin/IsLocalPlugin agree with SourceOf,
// so the source policy classifies a plugin record as the rest of Studio does.
func TestSourceOfMatchesPluginModel(t *testing.T) {
	for _, command := range []string{
		"oci://r.example.com/x:1", "grpc://h:1", "file:///x", "/x", "plugins/x",
		"https://h/x", "OCI://r/x", " oci://r/x", "Grpc://h:1", "oci:/x", "",
	} {
		p := &models.Plugin{Command: command}
		source := plugin_security.SourceOf(command)
		assert.Equal(t, p.IsOCIPlugin(), source == plugin_security.SourceOCI, command)
		assert.Equal(t, p.IsGRPCPlugin(), source == plugin_security.SourceGRPC, command)
		assert.Equal(t, p.IsLocalPlugin(), source == plugin_security.SourceFile, command)
	}
}
