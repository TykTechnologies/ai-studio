//go:build enterprise
// +build enterprise

package cluster

import (
	// Enterprise builds of the Studio control server need the enterprise
	// edge management service (multi-tenant namespaces) registered.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/edge_management"

	// The control plane's SyncCredentials handler (full credential sync),
	// for the credential sync and credential.changed end-to-end tests.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/credential_sync"
)
