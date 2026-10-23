//go:build enterprise
// +build enterprise

package tests

import (
	"os"
	"testing"

	// Import enterprise features to register factories before tests run.
	// The hub-spoke container builds the plugin security service, which
	// panics in an enterprise build without the enterprise factory.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
