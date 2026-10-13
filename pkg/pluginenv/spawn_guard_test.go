package pluginenv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Every go-plugin ClientConfig in the repository that starts a process (sets
// Cmd) must build it with pluginenv.Command and set SkipHostEnv: true, or the
// plugin inherits the host environment.
func TestPluginSpawners_UseCommandAndSkipHostEnv(t *testing.T) {
	root := filepath.Join("..", "..")
	skipDirs := map[string]bool{
		"third_party": true, "node_modules": true, "examples": true, "community": true,
		filepath.Join("pkg", "testinfra"): true, filepath.Join("enterprise", "plugins"): true,
	}

	spawners := 0
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
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isClientConfig(lit.Type) {
				return true
			}
			fields := map[string]ast.Expr{}
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = kv.Value
					}
				}
			}
			cmd, ok := fields["Cmd"]
			if !ok {
				return true // reattach client: no process
			}
			spawners++
			pos := fset.Position(lit.Pos())
			if !isSelectorCall(cmd, "pluginenv", "Command") {
				t.Errorf("%s:%d: plugin Cmd is not built with pluginenv.Command", rel, pos.Line)
			}
			if v, ok := fields["SkipHostEnv"].(*ast.Ident); !ok || v.Name != "true" {
				t.Errorf("%s:%d: plugin ClientConfig does not set SkipHostEnv: true", rel, pos.Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if spawners == 0 {
		t.Fatal("found no plugin ClientConfig with Cmd; the walk is broken")
	}
}

// isClientConfig matches <pkg>.ClientConfig, the go-plugin client config
// (imported as plugin or goplugin).
func isClientConfig(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ClientConfig" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && (pkg.Name == "plugin" || pkg.Name == "goplugin")
}

func isSelectorCall(expr ast.Expr, pkg, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != fn {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}
