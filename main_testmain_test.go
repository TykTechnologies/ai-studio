package main

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps log exports out of the repository. A Service built by these
// tests (services.NewService) writes them to EXPORT_STORAGE_PATH, by default
// ./data/exports: in the repository root, where a new directory changes
// what go test keys every source-walking test's cached result on. A
// caller's own setting (the main() child processes) is left alone.
func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("EXPORT_STORAGE_PATH"); ok {
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "studio-test-exports-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("EXPORT_STORAGE_PATH", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
