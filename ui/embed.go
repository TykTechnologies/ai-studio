//go:build !studio_noui

package ui

import "embed"

//go:embed admin-frontend/build
var files embed.FS

const (
	root     = "admin-frontend/build"
	embedded = true
)
