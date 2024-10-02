package api

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var emptyFile embed.FS

func setupTestAPI(t *testing.T) (*API, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := services.NewService(db)

	config := &auth.Config{
		DB:                  db,
		Service:             service,
		CookieName:          "session",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteStrictMode,
		ResetTokenExpiry:    time.Hour,
		FrontendURL:         "http://example.com",
		RegistrationAllowed: true,
		AdminEmail:          "admin@example.com",
		TestMode:            true,
	}

	api := NewAPI(service, true, auth.NewAuthService(config, newMockMailer()), config, nil, emptyFile)

	return api, db
}

func performRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}
