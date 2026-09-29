package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Unexpected errors (a database failure, say) are logged, not returned:
// their text can carry SQL, schema or host details.
func TestSendPushError_HidesInternalErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/edges/reload-all", nil)
	sendPushError(c, errors.New(`pq: relation "edge_push_commands" does not exist at 10.0.3.7:5432`))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "edge_push_commands")
	assert.NotContains(t, w.Body.String(), "10.0.3.7")
	assert.Contains(t, w.Body.String(), "see the server log")
}
