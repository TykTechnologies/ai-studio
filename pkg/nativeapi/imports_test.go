package nativeapi

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/TykTechnologies/midsommar/v2"

// enterpriseModule is the private enterprise module (and, by prefix, its /v2
// path). It is split so the quoted module path never appears in this file:
// make enterprise-import-guard greps public packages for it and would take
// this deny-list entry for an import.
const enterpriseModule = "github.com/TykTechnologies/" + "ai-studio-enterprise"

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
		enterpriseModule,
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

// TestEnterpriseModuleNamesTheRealModule keeps the split deny-list entry
// live: the root go.mod requires the enterprise module under that path, so a
// typo in the pieces cannot quietly stop TestImportBoundary catching it.
func TestEnterpriseModuleNamesTheRealModule(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Skipf("root go.mod unavailable: %v", err)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "require" {
			fields = fields[1:]
		}
		if len(fields) > 0 && (fields[0] == enterpriseModule || strings.HasPrefix(fields[0], enterpriseModule+"/")) {
			return
		}
	}
	t.Fatalf("go.mod requires no module at %s; the deny-list entry is stale", enterpriseModule)
}
