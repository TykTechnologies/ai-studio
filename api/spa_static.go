package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// serveBuildRootFile answers a request for a file at the root of the
// frontend build (manifest.json, robots.txt, logo192.png) with that file.
// It reports false, having written nothing, for anything else, which the SPA
// fallback then answers with index.html.
func serveBuildRootFile(c *gin.Context, fsys fs.FS) bool {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}
	name := strings.TrimPrefix(c.Request.URL.Path, "/")
	if name == "" || name == "index.html" || strings.Contains(name, "/") || !fs.ValidPath(name) {
		return false
	}
	data, err := fs.ReadFile(fsys, "ui/admin-frontend/build/"+name)
	if err != nil {
		return false
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	c.Data(http.StatusOK, contentType, data)
	return true
}
