package nativeapi

import (
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/TykTechnologies/midsommar/v2"

// TestImportBoundary keeps the contract package free of the proxy, the vendor
// registry and vendor implementations (which import it), and of enterprise
// code. Checked transitively with go list.
func TestImportBoundary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	forbidden := []string{
		modulePath + "/proxy",
		modulePath + "/switches",
		modulePath + "/vendors",
		"github.com/TykTechnologies/ai-studio-enterprise",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("pkg/nativeapi depends on %s", dep)
			}
		}
	}
}

// TestDirectImports allows only the standard library and models.
func TestDirectImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, imp := range strings.Fields(string(out)) {
		first := strings.SplitN(imp, "/", 2)[0]
		if !strings.Contains(first, ".") {
			continue // standard library
		}
		if imp != modulePath+"/models" {
			t.Errorf("pkg/nativeapi imports %s; only the standard library and models are allowed", imp)
		}
	}
}
