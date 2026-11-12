//go:build enterprise && credsync_e2e

package cluster

import (
	// The control plane's SyncCredentials handler (Enterprise). Built in
	// with -tags 'enterprise credsync_e2e' while it is not in the pinned
	// enterprise module (ai-studio-enterprise#123); the credential sync
	// tests skip without it. Once the pin has it, it moves to
	// testmain_enterprise_test.go and the extra tag goes.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/credential_sync"
)
