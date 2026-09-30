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

// frontendBootstrap is what the admin frontend reads before its first
// request (ui/admin-frontend/src/runtimeConfig.js); the server injects it
// into index.html as window.__TYK_AI_STUDIO__. /auth/config repeats it.
type frontendBootstrap struct {
	BasePath        string `json:"basePath"`
	AuthMode        string `json:"authMode"`
	LoginURL        string `json:"loginURL,omitempty"`
	LogoutURL       string `json:"logoutURL,omitempty"`
	CSRFTokenHeader string `json:"csrfTokenHeader"`
	CSRFTokenURL    string `json:"csrfTokenURL"`
	Chrome          string `json:"chrome"`
}

func (a *API) frontendBootstrap() frontendBootstrap {
	b := frontendBootstrap{
		BasePath:        a.basePath,
		AuthMode:        "local",
		CSRFTokenHeader: "X-CSRF-Token",
		CSRFTokenURL:    a.publicPath("/csrf-token"),
		Chrome:          "full",
	}
	if a.config == nil {
		return b
	}
	if a.config.Chromeless {
		b.Chrome = "none"
	}
	if !a.config.LocalAccountsEnabled() {
		b.AuthMode = "host"
		b.LoginURL = a.config.HostLoginURL
		b.LogoutURL = a.config.HostLogoutURL
	}
	if a.config.CSRFTokenHeader != "" {
		b.CSRFTokenHeader = a.config.CSRFTokenHeader
	}
	if a.config.CSRFTokenURL != "" {
		b.CSRFTokenURL = a.config.CSRFTokenURL
	}
	return b
}

// injectBootstrap prepares index.html: a <base> element at the base path,
// which is what the frontend's relative asset URLs (it is built with
// PUBLIC_URL ".") resolve against on any route, and the bootstrap settings.
func injectBootstrap(index []byte, b frontendBootstrap) []byte {
	// json.Marshal escapes <, > and &, so no value can close the script.
	settings, _ := json.Marshal(b)
	head := `<base href="` + html.EscapeString(b.BasePath+"/") + `">` +
		`<script>window.__TYK_AI_STUDIO__=` + string(settings) + `;</script>`
	if i := bytes.Index(index, []byte("<head>")); i >= 0 {
		i += len("<head>")
		return append(index[:i:i], append([]byte(head), index[i:]...)...)
	}
	return append([]byte(head), index...)
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
