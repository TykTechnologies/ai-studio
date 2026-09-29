// Package templates holds the default email and notification templates.
package templates

import (
	"embed"
	"os"
	"path/filepath"
)

// FS holds the default templates, embedded so they render wherever the
// process runs.
//
//go:embed *.tmpl
var FS embed.FS

// Read returns the named template from templates/ under the working
// directory when that file exists, which lets a deployment customise a
// template by providing its own, and the embedded default otherwise.
func Read(name string) ([]byte, error) {
	if b, err := os.ReadFile(filepath.Join("templates", name)); err == nil {
		return b, nil
	}
	return FS.ReadFile(name)
}
