package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
)

// A handler's 500 becomes 503 database_unavailable with Retry-After when the
// database was found unavailable during the request, and only then; other
// statuses are left alone.
func TestDatabaseUnavailableMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(databaseUnavailableMiddleware())
	r.GET("/fails-while-unavailable", func(c *gin.Context) {
		dbhold.NoteUnavailable()
		c.JSON(http.StatusInternalServerError, gin.H{"errors": []gin.H{{"title": "Internal Server Error", "detail": "dial tcp: connection refused"}}})
	})
	r.GET("/fails", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "a bug"})
	})
	r.GET("/not-found-while-unavailable", func(c *gin.Context) {
		dbhold.NoteUnavailable()
		c.JSON(http.StatusNotFound, gin.H{"error": "nope"})
	})
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get("/fails-while-unavailable")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	var body struct {
		Errors []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	require.Len(t, body.Errors, 1)
	assert.Equal(t, "database_unavailable", body.Errors[0].Code)
	assert.NotContains(t, w.Body.String(), "connection refused", "the handler's body is replaced")

	// Unavailability seen before this request started does not count.
	w = get("/fails")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "a bug")

	w = get("/not-found-while-unavailable")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
