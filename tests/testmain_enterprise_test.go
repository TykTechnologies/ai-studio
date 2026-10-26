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
