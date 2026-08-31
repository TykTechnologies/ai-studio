package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

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
