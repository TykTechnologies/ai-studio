package api

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// withBasePath serves h under prefix (a normalised base path such as
// "/ai-studio"). Routes stay registered at the root: a request under the
// prefix has it stripped, and one without it passes through unchanged,
// because a reverse proxy in front of Studio may already have stripped it.
// The bare prefix redirects to prefix + "/" so relative URLs resolve.
func withBasePath(prefix string, h http.Handler) http.Handler {
	if prefix == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == prefix:
			target := prefix + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		case strings.HasPrefix(p, prefix+"/"):
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = strings.TrimPrefix(p, prefix)
			r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
			h.ServeHTTP(w, r2)
		default:
			h.ServeHTTP(w, r)
		}
	})
}

// basePathPlaceholder is the PUBLIC_URL the admin frontend is built with, so
// one build can be served under any base path.
const basePathPlaceholder = "/__TYK_AI_BASE__"

// injectBasePath prepares index.html for serving under basePath: it replaces
// the build's base path placeholder and, when basePath is set, adds a <base>
// element and window.__TYK_AI_BASE__ for the frontend. At the root a build
// without the placeholder is served unchanged.
func injectBasePath(index []byte, basePath string) []byte {
	out := bytes.ReplaceAll(index, []byte(basePathPlaceholder), []byte(basePath))
	if basePath == "" {
		return out
	}
	// json.Marshal escapes <, > and &, so the value cannot close the script.
	quoted, _ := json.Marshal(basePath)
	head := `<base href="` + html.EscapeString(basePath+"/") + `">` +
		`<script>window.__TYK_AI_BASE__=` + string(quoted) + `;</script>`
	if i := bytes.Index(out, []byte("<head>")); i >= 0 {
		i += len("<head>")
		return append(out[:i:i], append([]byte(head), out[i:]...)...)
	}
	return append([]byte(head), out...)
}

// publicPath returns path (which starts with "/") as the browser sees it,
// under the base path.
func (a *API) publicPath(path string) string {
	return a.basePath + path
}

// cookiePath is the path Studio's cookies are scoped to.
func (a *API) cookiePath() string {
	if a.basePath == "" {
		return "/"
	}
	return a.basePath
}

// OAuthMetadataHandler serves the OAuth authorization server metadata. With
// a base path the issuer carries a path, and RFC 8414 clients look the
// metadata up at the host root with the path appended
// (/.well-known/oauth-authorization-server/ai-studio); a host mounts this
// handler there, since that URL lies outside Studio's base path.
func (a *API) OAuthMetadataHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/.well-known/oauth-authorization-server"
		r2.URL.RawPath = ""
		a.router.ServeHTTP(w, r2)
	})
}

// localAccountsOnly answers 404 for Studio's own sign-in routes (password,
// registration, SSO) when a host application authenticates users instead.
func (a *API) localAccountsOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.config != nil && !a.config.LocalAccountsEnabled() {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Next()
	}
}
