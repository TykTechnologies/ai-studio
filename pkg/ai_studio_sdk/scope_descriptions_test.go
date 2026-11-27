package ai_studio_sdk

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// o4a.19: every service scope a plugin can request has its own description
// in the install review (ScopeReviewSection.js SCOPE_DESCRIPTIONS), so the
// review never falls back to "Standard access permission". The scopes are
// read from their sources: AvailableScopes here and the scopes the gateway
// SDK's service API calls with (pkg/gatewayplugin/sdk/service_api.go).
func TestEveryScopeHasAnInstallReviewDescription(t *testing.T) {
	root := filepath.Join("..", "..")
	ui, err := os.ReadFile(filepath.Join(root, "ui", "admin-frontend", "src", "admin", "components", "plugins", "ScopeReviewSection.js"))
	if err != nil {
		t.Fatal(err)
	}
	described := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*'([a-z0-9._-]+)':\s*['"]`).FindAllSubmatch(ui, -1) {
		described[string(m[1])] = true
	}

	scopes := map[string]bool{}
	v := reflect.ValueOf(AvailableScopes)
	for i := 0; i < v.NumField(); i++ {
		if s := v.Field(i).String(); s != "" {
			scopes[s] = true
		}
	}
	gw, err := os.ReadFile(filepath.Join(root, "pkg", "gatewayplugin", "sdk", "service_api.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`createPluginContext\("([a-z0-9._-]+)"\)`).FindAllSubmatch(gw, -1) {
		scopes[string(m[1])] = true
	}
	if len(scopes) < 30 {
		t.Fatalf("found only %d scopes: the sources moved?", len(scopes))
	}

	var missing []string
	for s := range scopes {
		if !described[s] {
			missing = append(missing, s)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("scopes without a description in ScopeReviewSection.js SCOPE_DESCRIPTIONS: %v", missing)
	}
}
