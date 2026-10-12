package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServer_ManagementAPIByMode runs the real server with real token auth.
// POST /api/v1/auth/token issued an admin token to anyone; it is gone in every
// mode. An edge serves no /api/v1 at all, even to a valid admin token.
func TestServer_ManagementAPIByMode(t *testing.T) {
	mint := func(t *testing.T, router http.Handler) int {
		body := `{"app_id":1,"name":"x","scopes":["admin"]}`
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", strings.NewReader(body)))
		return w.Code
	}

	for _, tc := range []struct {
		mode       string
		wantListOK int
	}{
		{"standalone", http.StatusOK},
		{"edge", http.StatusNotFound},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			srv, sc := setupIntegrationTestInMode(t, tc.mode)
			router := srv.GetRouter()

			app := database.App{Name: "admin app", IsActive: true}
			require.NoError(t, sc.DB.Create(&app).Error)
			adminToken, err := sc.AuthProvider.GenerateToken(app.ID, "admin", []string{"admin"}, time.Hour)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, mint(t, router), "unauthenticated token issuance must not be routed")

			var tokens int64
			require.NoError(t, sc.DB.Model(&database.APIToken{}).Count(&tokens).Error)
			assert.EqualValues(t, 1, tokens, "only the test's own token may exist")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/llms", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tc.wantListOK, w.Code, w.Body.String())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}
