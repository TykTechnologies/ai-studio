package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On an edge /ready names the control endpoint in use and stays 200 whether
// or not the stream to it is open: an edge serves through a control-plane
// outage.
func TestReadinessReportsTheControlEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ready := func(sc *services.ServiceContainer) (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
		ReadinessCheck(sc)(c)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w.Code, body
	}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sc := &services.ServiceContainer{DB: db}
	code, body := ready(sc)
	require.Equal(t, http.StatusOK, code, "%v", body)
	assert.NotContains(t, body, "control_endpoint", "a standalone gateway has no control endpoint")

	sc.ControlLink = func() (string, bool) { return "cp-b.example:50051", true }
	code, body = ready(sc)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "cp-b.example:50051", body["control_endpoint"])
	assert.Equal(t, true, body["control_connected"])

	sc.ControlLink = func() (string, bool) { return "cp-a.example:50051", false }
	code, body = ready(sc)
	require.Equal(t, http.StatusOK, code, "an edge without a control plane is still ready")
	assert.Equal(t, false, body["control_connected"])
}
