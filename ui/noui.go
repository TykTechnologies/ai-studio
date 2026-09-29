//go:build studio_noui

package ui

import "embed"

//go:embed noui
var files embed.FS

const (
	root     = "noui"
	embedded = false
)
