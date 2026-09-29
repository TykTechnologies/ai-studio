// Package ui holds the built admin frontend. The build directory is produced
// by `npm run build` in admin-frontend and is not committed, so it must exist
// before this package compiles.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed admin-frontend/build
var build embed.FS

// FS is the built admin frontend rooted at its build directory: index.html,
// static/, logos/ and the root-level assets.
var FS fs.FS = mustSub(build, "admin-frontend/build")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err) // dir is a constant valid path, so this cannot happen
	}
	return sub
}
