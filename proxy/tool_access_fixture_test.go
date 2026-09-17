package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/require"
)

// exposeTestTool makes a tool created through the service reachable on the
// gateway. A new tool is chat only and CreateTool carries no operation
// whitelist, so a test that calls /tools/{slug} has to turn the access methods
// on and whitelist the operations it uses, as an admin would.
func exposeTestTool(t *testing.T, service *services.Service, toolID uint, operations string) {
	t.Helper()
	require.NoError(t, service.DB.Model(&models.Tool{}).
		Where("id = ?", toolID).
		Updates(map[string]interface{}{
			"rest_access_disabled": false,
			"mcp_access_disabled":  false,
			"available_operations": operations,
		}).Error)
}

// setTestToolAccess flips the two access-method switches of a tool.
func setTestToolAccess(t *testing.T, service *services.Service, toolID uint, rest, mcp bool) {
	t.Helper()
	require.NoError(t, service.DB.Model(&models.Tool{}).
		Where("id = ?", toolID).
		Updates(map[string]interface{}{
			"rest_access_disabled": !rest,
			"mcp_access_disabled":  !mcp,
		}).Error)
}
