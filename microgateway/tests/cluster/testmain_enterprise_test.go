//go:build enterprise
// +build enterprise

package cluster

import (
	// Enterprise builds of the Studio control server need the enterprise
	// edge management service (multi-tenant namespaces) registered.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/edge_management"
)
