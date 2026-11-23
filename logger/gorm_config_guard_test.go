package logger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gormConfigAllowed are the non-test files that may build a gorm.Config
// themselves instead of taking GetGormConfig(), each with its reason. A file
// that is not here and writes gorm.Config{...}, or calls gorm.Open without a
// configuration, gets gorm's default logger unless it remembers to set one,
// and that logger prints a slow or failed statement with its values.
var gormConfigAllowed = map[string]string{
	"logger/gorm_logger.go":                          "GetGormConfig itself",
	"microgateway/internal/database/connection.go":   "the Microgateway's connections; its logger drops values (getGormLogger) and its own tests cover it",
	"pkg/testinfra/sqlitetemplate/sqlitetemplate.go": "test infrastructure: an in-memory template database for tests, in no shipped binary",
	"performance/framework/testutil.go":              "benchmark helper with its own statement-counting logger, in no shipped binary",
	"api/testing/testing.go":                         "test helper: an in-memory database for API tests, in no shipped binary",
}

// gormConfigSkippedDirs are not ours to check (vendored code, the separate
// plugin and enterprise repositories, which have no gorm connection of their
// own in non-test code) or hold no Go that opens a database.
var gormConfigSkippedDirs = map[string]bool{
	".git": true, "third_party": true, "node_modules": true, "ui": true, "docs": true,
	"enterprise": true, "community": true, "tyk-internal": true,
}

// Every database connection in AI Studio and the Microgateway logs SQL
// through a logger that drops statement values (DB_LOG_SQL_PARAMS opts them
// back in). This test keeps it that way: outside the files listed above, no
// non-test Go file may write a gorm.Config literal or call gorm.Open without
// a configuration. The PostgreSQL chat queue, the pgvector datasource
// connection and the start-up checks did, with an empty gorm.Config, and so
// logged failed and slow statements with message and document content.
func TestEveryGormConnectionUsesTheFilteredLogger(t *testing.T) {
	root := ".." // the repository root, from this package's directory
	seenAllowed := map[string]bool{}
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if gormConfigSkippedDirs[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil // not this test's business; the build reports it
		}
		gormName := ""
		for _, imp := range file.Imports {
			importPath, _ := strconv.Unquote(imp.Path.Value)
			// The vendored copy, or upstream should it ever be imported
			// (spelled in two parts: make gorm-verify greps for the
			// upstream import path as a string).
			if importPath == "gorm"+".io/gorm" || strings.HasSuffix(importPath, "/third_party/gorm.io/gorm") {
				gormName = "gorm"
				if imp.Name != nil {
					gormName = imp.Name.Name
				}
			}
		}
		if gormName == "" {
			return nil
		}
		checked++

		isGorm := func(expr ast.Expr, name string) bool {
			sel, ok := expr.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != name {
				return false
			}
			pkg, ok := sel.X.(*ast.Ident)
			return ok && pkg.Name == gormName
		}
		ast.Inspect(file, func(n ast.Node) bool {
			var problem string
			switch node := n.(type) {
			case *ast.CompositeLit:
				if isGorm(node.Type, "Config") {
					problem = "builds a gorm.Config itself; use logger.GetGormConfig() (and set further fields on what it returns)"
				}
			case *ast.CallExpr:
				if isGorm(node.Fun, "Open") && len(node.Args) < 2 {
					problem = "calls gorm.Open without a configuration; pass logger.GetGormConfig()"
				}
			}
			if problem == "" {
				return true
			}
			if _, ok := gormConfigAllowed[rel]; ok {
				seenAllowed[rel] = true
				return true
			}
			t.Errorf("%s:%d %s. Otherwise the connection logs slow and failed statements with their values (see gormConfigAllowed for the exceptions and their reasons)",
				rel, fset.Position(n.Pos()).Line, problem)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("only %d files import gorm under %s: the walk is not seeing the repository", checked, root)
	}
	for rel := range gormConfigAllowed {
		if !seenAllowed[rel] {
			t.Errorf("%s is allowed to build a gorm.Config but no longer does; remove it from gormConfigAllowed", rel)
		}
	}
}
