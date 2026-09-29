// Package edition checks that a build carries every enterprise feature it
// was compiled for.
package edition

import (
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/scripting/engine"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/services/compliance"
	"github.com/TykTechnologies/midsommar/v2/services/edge_management"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/TykTechnologies/midsommar/v2/services/licensing"
	"github.com/TykTechnologies/midsommar/v2/services/log_export"
	"github.com/TykTechnologies/midsommar/v2/services/marketplace_management"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	"github.com/TykTechnologies/midsommar/v2/services/team_budget"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/TykTechnologies/midsommar/v2/services/webhooks"
)

// enterpriseFeatures maps each enterprise feature to whether its
// implementation registered itself, which its package's init() does when
// imported.
type feature struct {
	name       string
	registered func() bool
}

var enterpriseFeatures = []feature{
	{"audit", audit.IsEnterpriseAvailable},
	{"budget", budget.IsEnterpriseAvailable},
	{"compliance", compliance.IsEnterpriseAvailable},
	{"edge_management", edge_management.FactoryRegistered},
	{"governed_metadata", governed_metadata.IsEnterpriseAvailable},
	{"group_access", group_access.FactoryRegistered},
	{"guardrails", guardrails.Available},
	{"licensing", licensing.IsEnterpriseAvailable},
	{"log_export", log_export.IsEnterpriseAvailable},
	{"marketplace_management", marketplace_management.IsEnterpriseAvailable},
	{"model_router", model_router.IsEnterpriseAvailable},
	{"plugin_security", plugin_security.IsEnterpriseAvailable},
	{"rbac", rbac.IsEnterpriseAvailable},
	{"scripting", engine.Available},
	{"semantic_router", semantic_router.IsEnterpriseAvailable},
	{"sso", sso.IsEnterpriseAvailable},
	{"team_budget", team_budget.IsEnterpriseAvailable},
	{"tykmcp", tykmcp.IsEnterpriseAvailable},
	{"webhooks", webhooks.IsEnterpriseAvailable},
}

// CheckRegistered returns an error naming each enterprise feature whose
// implementation is missing from an enterprise build. Without this check the
// build would panic on the feature's first use. Community builds always pass.
func CheckRegistered() error {
	if !config.IsEnterprise() {
		return nil
	}
	return checkFeatures(enterpriseFeatures)
}

func checkFeatures(features []feature) error {
	var missing []string
	for _, f := range features {
		if !f.registered() {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("enterprise build is missing features %s: import github.com/TykTechnologies/ai-studio-enterprise/v2/all",
			strings.Join(missing, ", "))
	}
	return nil
}
