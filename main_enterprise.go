//go:build enterprise
// +build enterprise

package main

import (
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"                    // Register enterprise budget service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/compliance"                // Register enterprise compliance service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/edge_management"           // Register enterprise edge management service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/group_access"              // Register enterprise group access service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/licensing"                 // Register enterprise licensing service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/log_export"                // Register enterprise log export service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/marketplace_management"    // Register enterprise marketplace management service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/model_router"              // Register enterprise model router service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/plugin_security"           // Register enterprise plugin security service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/sso"                       // Register enterprise SSO service
)
