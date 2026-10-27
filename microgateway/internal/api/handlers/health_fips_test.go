package handlers

import (
	"crypto/fips140"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /health reports the process's FIPS 140-3 status: enabled=false in a
// standard build, and the module's values in a FIPS one.
func TestHealthCheckReportsFIPSStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	HealthCheck(nil)(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Status string          `json:"status"`
		FIPS   fipsinfo.Status `json:"fips"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body.Status)
	assert.Equal(t, fipsinfo.Current(), body.FIPS)
	if !fips140.Enabled() {
		assert.False(t, body.FIPS.Enabled, "a standard build reports FIPS mode off")
	}
}
