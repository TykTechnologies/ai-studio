//go:build enterprise
// +build enterprise

package services

import (
	"os"
	"testing"

	// Import enterprise features to register factories before tests run
	// Note: budget creates import cycle, so it's excluded here
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/edge_management"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/governed_metadata"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/group_access"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/licensing"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/marketplace_management"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/rbac"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/sso"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
