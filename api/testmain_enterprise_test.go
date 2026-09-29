//go:build enterprise
// +build enterprise

package api

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	// Import enterprise features to register factories before tests run
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/edge_management"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/governed_metadata"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/group_access"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/rbac"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/sso"
)

func TestMain(m *testing.M) {
	// Set gin to test mode
	gin.SetMode(gin.TestMode)

	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
