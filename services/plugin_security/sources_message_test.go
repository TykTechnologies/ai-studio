package plugin_security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin UI refuses a disallowed plugin command on the client with this
// exact wording (pluginSourceError in
// ui/admin-frontend/src/admin/services/pluginService.js, pinned by
// pluginSources.test.js). Keep the two in step: change both or neither
// (midsommar-clean-c1g.12).
func TestErrPluginSourceNotAllowed_MessageMatchesTheUI(t *testing.T) {
	policy, err := ParseSourcePolicy("oci")
	require.NoError(t, err)
	err = policy.WithSetting("AI_STUDIO_PLUGIN_ALLOWED_SOURCES").Check("/opt/plugins/demo")
	require.Error(t, err)
	assert.Equal(t,
		`🔒 SECURITY: plugin source "file" is not allowed by this installation (allowed: oci). Set AI_STUDIO_PLUGIN_ALLOWED_SOURCES to change this.`,
		err.Error())
}
