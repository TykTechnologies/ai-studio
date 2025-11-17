//go:build enterprise
// +build enterprise

package main

import (
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"          // Register enterprise budget service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/licensing"       // Register enterprise licensing service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/plugin_security" // Register enterprise plugin security service
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/sso"             // Register enterprise SSO service
)
