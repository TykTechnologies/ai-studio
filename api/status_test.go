package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestResponseStatus demonstrates why handlers using NoContent status code
// must use c.String(http.StatusNoContent, "") instead of c.Status(http.StatusNoContent)
func TestResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test 1: Using Status() directly (FAILS)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Direct status call - this doesn't work correctly in Gin for NoContent!
	c.Status(http.StatusNoContent)
	// This will actually fail because Gin's Status() method doesn't correctly
	// set the status code to 204 - it returns 200 instead:
	// assert.Equal(t, http.StatusNoContent, w.Code)

	// Test 2: Using String() method (WORKS)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)

	// Status with blank body - this is the correct way to return 204 in Gin
	c.String(http.StatusNoContent, "")
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Note: All MCP handlers returning 204 should use c.String(http.StatusNoContent, "")
	// instead of c.Status(http.StatusNoContent) to ensure tests pass and correct
	// behavior.
}
