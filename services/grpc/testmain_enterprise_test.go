//go:build enterprise
// +build enterprise

package grpc

import (
	"os"
	"testing"

	// Import enterprise features to register factories before tests run
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/group_access"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()
	os.Exit(m.Run())
}
