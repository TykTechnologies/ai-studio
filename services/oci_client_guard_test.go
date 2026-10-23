package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Studio code must build OCI plugin clients through Service.NewOCIPluginClient
// (or inside NewServiceWithOCI), which wires the shared plugin security
// service. A client built directly with ociplugins.NewOCIPluginClient has no
// security service and, before c1g.1, silently skipped the enterprise
// verifier (midsommar-clean-c1g.1).
func TestStudioBuildsOCIClientsOnlyThroughTheSecuredConstructors(t *testing.T) {
	root := filepath.Join("..")
	skipDirs := map[string]bool{
		"third_party": true, "node_modules": true, "examples": true, "community": true,
		"enterprise": true, "tyk-internal": true, "tests": true,
		// The gateway is a separate module with its own wiring (c1g.11).
		"microgateway": true,
		// The client package itself.
		filepath.Join("pkg", "ociplugins"): true,
	}
	// Each allowed call site wires the security service right after.
	allowed := map[string]bool{
		filepath.Join("services", "service.go"):        true, // NewOCIPluginClient, newSecuredOCIClient
		filepath.Join("services", "plugin_service.go"): true, // NewServiceWithOCI calls PluginService.SetSecurityService
	}

	calls := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || skipDirs[rel]) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil // not this test's concern
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewOCIPluginClient" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "ociplugins" {
				return true
			}
			calls++
			if !allowed[rel] {
				t.Errorf("%s:%d: builds an OCI client with ociplugins.NewOCIPluginClient; use Service.NewOCIPluginClient so it verifies through the plugin security service",
					rel, fset.Position(call.Pos()).Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("found no ociplugins.NewOCIPluginClient call; the walk is broken")
	}
}
