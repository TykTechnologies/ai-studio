package api

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// useTempBrandingStorage stores uploaded branding assets in a temporary
// directory for the test. The default, ./data/branding, is created in the
// package directory, which changes what go test keys its cached results on
// (the directory's mtime and listing) on every run.
func useTempBrandingStorage(t *testing.T) {
	t.Helper()
	services.SetBrandingStoragePath(t.TempDir())
	t.Cleanup(func() { services.SetBrandingStoragePath("") })
}
