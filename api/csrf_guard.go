package api

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"

	appconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
)

// devCSRFTrustedOrigins builds the DEVMODE-only CSRF trusted-origin list.
//
// gorilla/csrf compares the browser's Origin (or Referer) host:port against
// the request Host. In development the two never match: the React dev server
// proxies API calls with changeOrigin, and in Docker the frontend is reached on
// a host-mapped port. The frontend origin is whatever SITE_URL says, so its
// host is trusted automatically; extra is CSRF_TRUSTED_ORIGINS, a
// comma-separated list of host[:port] values for anything else (a locally
// served production build, a second frontend). localhost:3000 stays in the
// list so a stack that never sets SITE_URL keeps working.
func devCSRFTrustedOrigins(siteURL, extra string) []string {
	trusted := []string{"localhost:3000"}
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		for _, existing := range trusted {
			if existing == host {
				return
			}
		}
		trusted = append(trusted, host)
	}

	if siteURL = strings.TrimSpace(siteURL); siteURL != "" {
		if parsed, err := url.Parse(siteURL); err == nil && parsed.Host != "" {
			add(parsed.Host)
		}
	}
	for _, origin := range strings.Split(extra, ",") {
		add(origin)
	}
	return trusted
}

// csrfGuard adapts a net/http CSRF middleware (gorilla/csrf) into gin's chain.
//
// The subtlety this exists to make explicit: in gin, a middleware that returns
// without calling c.Next() does NOT stop the chain. gin's Next() is a loop over
// the handler slice, so control returns to that loop and the next handler --
// including the route handler that performs the mutation -- runs anyway. Only
// c.Abort() stops it.
//
// So the abort on rejection is load-bearing: without it, a request answered
// with 403 would still be written. The previous implementation inferred
// rejection from c.Writer.Status() == 403, which works today but is decided by
// something outside this function: change gorilla's error handler, or have any
// other middleware touch the status, and the abort silently stops happening --
// producing exactly the failure it is there to prevent, with no test to catch
// it (CSRF is disabled entirely under TestMode).
//
// Rely instead on the one fact this function owns: gorilla only invokes the
// handler it wraps when the request passes.
func csrfGuard(csrfMiddleware func(http.Handler) http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Token-authenticated calls are not browser-driven, so they are not
		// subject to CSRF. A cross-site attacker cannot set an Authorization
		// header without a CORS preflight the server must approve.
		if c.GetHeader("Authorization") != "" {
			c.Next()
			return
		}

		// OAuth endpoints run their own state/PKCE checks.
		if strings.HasPrefix(c.Request.URL.Path, "/oauth/") {
			c.Next()
			return
		}

		// gorilla assumes HTTPS unless told otherwise: with no Origin header it
		// then demands a Referer, and it compares an Origin against an https
		// URL, so a same-host "http://" Origin fails. Both are wrong on plain
		// HTTP (dev, tests, a stack with no TLS in front), where browsers may
		// legitimately send neither header. Mark the request plaintext when
		// nothing says it was HTTPS to the browser: no TLS on the socket, no
		// X-Forwarded-Proto: https from a TLS-terminating proxy, and no https
		// Origin (a proxy that terminates TLS without setting the header).
		if c.Request.TLS == nil && !forwardedHTTPS(c.Request) && !strings.HasPrefix(strings.ToLower(c.GetHeader("Origin")), "https://") {
			c.Request = csrf.PlaintextHTTPRequest(c.Request)
		}

		passed := false
		csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			passed = true
			c.Request = r
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)

		if !passed {
			// Rejected: gorilla has already written the 403. Stop the chain so
			// nothing downstream mutates.
			c.Abort()
		}
	}
}

// forwardedHTTPS reports whether a proxy in front of us terminated TLS: the
// first X-Forwarded-Proto value (the header may list one hop per entry) is
// "https", case-insensitively.
func forwardedHTTPS(r *http.Request) bool {
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// studioCSRF builds Studio's own CSRF protection: gorilla/csrf double-submit
// tokens, with the cookie scoped to the base path.
func (a *API) studioCSRF() (func(http.Handler) http.Handler, error) {
	appConf := appconfig.Get("")
	key, err := csrfKey(appConf.CSRFKey)
	if err != nil {
		return nil, err
	}
	opts := []csrf.Option{
		// Only unsets the cookie's Secure flag. What lets HTTP dev/test
		// setups through is csrfGuard marking plain-HTTP requests
		// plaintext, so gorilla does not demand an Origin/Referer.
		csrf.Secure(false),
		csrf.Path(a.cookiePath()),
	}
	if appConf.CSRFCookieName != "" {
		opts = append(opts, csrf.CookieName(appConf.CSRFCookieName))
	}
	if appConf.DevMode {
		// The dev frontend proxies to the API from another origin (its own
		// port, or a host-mapped port in Docker), so the browser's Origin never
		// matches the request Host. Trust the SITE_URL host plus any extra
		// CSRF_TRUSTED_ORIGINS (comma-separated host[:port] values).
		trusted := devCSRFTrustedOrigins(appConf.SiteURL, appConf.CSRFTrustedOrigins)
		logger.Infof("DEVMODE: CSRF trusted origins: %s", strings.Join(trusted, ", "))
		opts = append(opts, csrf.TrustedOrigins(trusted))
	}
	return csrf.Protect(key, opts...), nil
}

// csrfKey derives the 32-byte token key from secret, or makes a random one
// when secret is empty (tokens then last only as long as the process).
func csrfKey(secret string) ([]byte, error) {
	if secret != "" {
		sum := sha256.Sum256([]byte("tyk-ai-studio-csrf:" + secret))
		return sum[:], nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate CSRF key: %w", err)
	}
	return key, nil
}
