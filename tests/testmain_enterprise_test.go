//go:build enterprise
// +build enterprise

package tests

import (
	"os"
	"testing"

	// Register every enterprise feature, as the enterprise Studio main
	// (main_enterprise.go) does.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/all"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
