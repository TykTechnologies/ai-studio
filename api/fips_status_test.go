package api

import (
	"crypto/fips140"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /health reports the process's FIPS 140-3 status: enabled=false in a
// standard build, and the module's values in a FIPS one.
func TestHealthReportsFIPSStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	(&API{}).handleHealth(c)

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

// The edge API carries each edge's FIPS status, null when unknown.
func TestEdgeResponseCarriesFIPS(t *testing.T) {
	st := fipsinfo.Status{Enabled: true, Version: "v1.0.0-c2097c7c"}
	for _, tc := range []struct {
		fips *fipsinfo.Status
		want string
	}{
		{nil, `null`},
		{&st, `{"enabled":true,"version":"v1.0.0-c2097c7c","enforced":false}`},
	} {
		edge := &models.EdgeInstance{EdgeID: "e1", FIPS: tc.fips}
		for _, resp := range []EdgeResponse{
			serializeEdge(edge),
			serializeEdgeWithHealth(&services.EdgeInstanceWithHealth{EdgeInstance: edge}),
		} {
			b, err := json.Marshal(resp)
			require.NoError(t, err)
			var raw struct {
				Attributes map[string]json.RawMessage `json:"attributes"`
			}
			require.NoError(t, json.Unmarshal(b, &raw))
			assert.JSONEq(t, tc.want, string(raw.Attributes["fips"]))
		}
	}
}
