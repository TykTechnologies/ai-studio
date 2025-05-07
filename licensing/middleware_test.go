package licensing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTelemetryMiddleware(t *testing.T) {
	// Set up test environment
	gin.SetMode(gin.TestMode)

	// Create a test licenser with telemetry enabled
	l := NewLicenser(LicenseConfig{
		DisableTelemetry: false,
	})
	l.InitializeForTests(map[string]interface{}{
		TrackLicenseUsage: true,
	})

	// Test cases
	tests := []struct {
		name       string
		path       string
		method     string
		setAction  bool
		action     string
		authHeader string
	}{
		{
			name:       "With explicit action and auth",
			path:       "/api/v1/users",
			method:     "GET",
			setAction:  true,
			action:     "List Users",
			authHeader: "Bearer token",
		},
		{
			name:       "With explicit action without auth",
			path:       "/api/v1/users",
			method:     "GET",
			setAction:  true,
			action:     "List Users",
			authHeader: "",
		},
		{
			name:       "Without explicit action",
			path:       "/api/v1/users",
			method:     "GET",
			setAction:  false,
			action:     "",
			authHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test router with middleware
			r := gin.New()
			r.Use(l.TelemetryMiddleware())

			// Add a test handler
			r.GET("/api/v1/users", func(c *gin.Context) {
				if tt.setAction {
					SetAction(c, tt.action)
				}
				c.Status(http.StatusOK)
			})

			// Execute request and verify
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}
		})
	}
}
