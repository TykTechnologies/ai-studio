package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
)

// databaseUnavailableMiddleware answers a request's 500 as 503
// database_unavailable, with Retry-After, when a database statement found
// the database unavailable (a failover: pkg/dbhold) while the request was
// running. Handlers write their own 500s; this is the one place their
// failover errors become a retryable answer. Nothing else is changed.
func databaseUnavailableMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &unavailableWriter{ResponseWriter: c.Writer, started: time.Now()}
		c.Next()
	}
}

// unavailableWriter replaces a 500 with the 503 when the database was
// unavailable since the request started, and drops the handler's body.
type unavailableWriter struct {
	gin.ResponseWriter
	started  time.Time
	replaced bool
}

func (w *unavailableWriter) WriteHeader(code int) {
	if code == http.StatusInternalServerError && !w.Written() && dbhold.UnavailableSince(w.started) {
		w.replaced = true
		h := w.ResponseWriter.Header()
		h.Del("Content-Length")
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("Retry-After", helpers.DatabaseUnavailableRetryAfter)
		w.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.ResponseWriter.Write(helpers.DatabaseUnavailableBody())
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *unavailableWriter) Write(b []byte) (int, error) {
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func (w *unavailableWriter) WriteString(s string) (int, error) {
	if w.replaced {
		return len(s), nil
	}
	return w.ResponseWriter.WriteString(s)
}
