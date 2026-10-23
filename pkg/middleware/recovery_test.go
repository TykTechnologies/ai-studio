package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// A panic is answered 500 and logged with the method, route, request id and
// panic value, never the request headers. Debug mode is where gin.Recovery
// dumps the headers, so the test runs in it.
func TestRecovery_LogsNoHeaders(t *testing.T) {
	var buf bytes.Buffer
	prevLog, prevMode := log.Logger, gin.Mode()
	prevOut, prevErr := gin.DefaultWriter, gin.DefaultErrorWriter
	log.Logger = zerolog.New(&buf)
	gin.SetMode(gin.DebugMode)
	gin.DefaultWriter, gin.DefaultErrorWriter = &buf, &buf
	t.Cleanup(func() {
		log.Logger = prevLog
		gin.SetMode(prevMode)
		gin.DefaultWriter, gin.DefaultErrorWriter = prevOut, prevErr
	})

	router := gin.New()
	router.Use(Recovery())
	router.GET("/items/:id", func(c *gin.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/items/7", nil)
	req = req.WithContext(requestid.WithID(req.Context(), "tyai_req1", ""))
	req.Header.Set("X-Tyk-User-Id", "secret-user-42")
	req.Header.Set("Baggage", "user.id=secret-user-42")
	req.Header.Set("Cookie", "session=secret-cookie")
	req.Header.Set("X-Api-Key", "secret-api-key")
	req.Header.Set("Authorization", "Bearer secret-bearer")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	out := buf.String()
	assert.Contains(t, out, `"method":"GET"`)
	assert.Contains(t, out, `"route":"/items/:id"`)
	assert.Contains(t, out, `"request_id":"tyai_req1"`)
	assert.Contains(t, out, `"panic":"boom"`)
	for _, secret := range []string{"secret-user-42", "secret-cookie", "secret-api-key", "secret-bearer", "Cookie", "X-Api-Key"} {
		assert.NotContains(t, out, secret)
	}
}
