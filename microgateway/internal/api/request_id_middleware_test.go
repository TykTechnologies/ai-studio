package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The edge answers its own request id in X-Tyk-AI-Request-ID, keeps the
// caller's X-Request-ID as theirs (recorded, never overwritten), and does not
// let a client choose the id.
func TestRequestIDMiddleware_OwnHeaderAndTheCallersIDKept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotID, gotClient string
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.POST("/llm/rest/x/v1/chat/completions", func(c *gin.Context) {
		gotID = requestid.FromContext(c.Request.Context())
		gotClient = requestid.ClientFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)
	req.Header.Set(requestid.ClientHeader, "alb-1-5f84c7a6")
	req.Header.Set(requestid.Header, "tyai_00000000000000000000000000000000")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.True(t, requestid.Valid(gotID), gotID)
	assert.NotEqual(t, "tyai_00000000000000000000000000000000", gotID, "the client chose the id")
	assert.Equal(t, gotID, rec.Header().Get(requestid.Header))
	assert.Empty(t, rec.Header().Get(requestid.ClientHeader), "the edge wrote X-Request-ID")
	assert.Equal(t, "alb-1-5f84c7a6", gotClient)
	// Plugins and hooks read the plain context key.
	assert.NotEmpty(t, gotID)
}
