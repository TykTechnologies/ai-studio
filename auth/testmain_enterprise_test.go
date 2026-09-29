//go:build enterprise
// +build enterprise

package auth

import (
	"os"
	"testing"

	// Import enterprise features to register factories before tests run
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/edge_management"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/group_access"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/sso"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
