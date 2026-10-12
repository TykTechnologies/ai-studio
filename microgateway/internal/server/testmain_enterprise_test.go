//go:build enterprise
// +build enterprise

package server

import (
	// The enterprise service container needs the enterprise factories
	// registered (by init) before tests build one, as the enterprise
	// microgateway main does; see internal/services/testmain_enterprise_test.go.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
)
