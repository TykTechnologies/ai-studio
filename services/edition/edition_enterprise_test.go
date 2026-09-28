//go:build enterprise

package edition_test

import (
	"testing"

	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/audit"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/compliance"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/edge_management"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/governed_metadata"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/group_access"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/licensing"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/log_export"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/marketplace_management"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/model_router"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/plugin_security"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/rbac"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/semantic_router"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/sso"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/team_budget"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/tykmcp"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/webhooks"

	"github.com/TykTechnologies/midsommar/v2/services/edition"
)

// TestCheckRegisteredPassesWithEveryFeatureImported fails when a new
// enterprise feature is added to the check but not to main_enterprise.go's
// import list (mirrored here).
func TestCheckRegisteredPassesWithEveryFeatureImported(t *testing.T) {
	if err := edition.CheckRegistered(); err != nil {
		t.Fatal(err)
	}
}
