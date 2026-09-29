// Package ui holds the built admin frontend.
//
// By default it embeds admin-frontend/build, which `npm run build` in
// admin-frontend produces and which is not committed, so it must exist before
// this package compiles. A host embedding Studio through pkg/studio from a
// module download has no build directory: it compiles with the studio_noui
// build tag, which embeds only a placeholder page, and serves the release's
// UI assets through studio.Options.UIAssets.
package ui

import "io/fs"

// FS is the built admin frontend rooted at its build directory: index.html,
// static/, logos/ and the root-level assets.
var FS fs.FS = mustSub(files, root)

// Embedded reports whether FS holds the real frontend rather than the
// studio_noui placeholder.
const Embedded = embedded

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err) // dir is a constant valid path, so this cannot happen
	}
	return sub
}
