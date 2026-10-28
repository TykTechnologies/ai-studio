//go:build enterprise
// +build enterprise

package tests

import (
	"os"
	"testing"
)

// Every enterprise feature is registered by enterprise_entlink_test.go,
// linked in from enterprise/_coretests by `make ent-link`, as the enterprise
// Studio main does. Without it this fails to compile, rather than running
// the Community Edition under the enterprise tag.
var _ = enterpriseFeaturesLinked

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()

	// The enterprise log export service creates EXPORT_STORAGE_PATH, by
	// default ./data/exports in this package's directory, where a new
	// directory changes what go test keys every source-walking test's cached
	// result on. A temporary directory instead.
	if _, ok := os.LookupEnv("EXPORT_STORAGE_PATH"); !ok {
		dir, err := os.MkdirTemp("", "studio-test-exports-")
		if err != nil {
			panic(err)
		}
		os.Setenv("EXPORT_STORAGE_PATH", dir)
		code := m.Run()
		os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}
