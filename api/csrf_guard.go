package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
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
