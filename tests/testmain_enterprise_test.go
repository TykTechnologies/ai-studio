//go:build enterprise
// +build enterprise

package tests

import (
	"os"
	"testing"

	// Import enterprise features to register factories before tests run
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/edge_management"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/group_access"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/licensing"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/marketplace_management"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/plugin_security"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/sso"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
