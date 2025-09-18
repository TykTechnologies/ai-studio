package testing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func SetupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err, "Failed to open database")

	err = models.InitModels(db)
	assert.NoError(t, err, "Failed to init models")

	return db
}

func SetupTestService(db *gorm.DB) *services.Service {
	notificationService := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationService)
	
	// Initialize hub-and-spoke services
	edgeService := services.NewEdgeService(db)
	namespaceService := services.NewNamespaceService(db, edgeService)
	pluginService := services.NewPluginService(db)
	
	return &services.Service{
		DB:                  db,
		NotificationService: notificationService,
		Budget:              budgetService,
		EdgeService:         edgeService,
		NamespaceService:    namespaceService,
		PluginService:       pluginService,
	}
}

func SetupTestNotificationService(db *gorm.DB) *services.NotificationService {
	return services.NewTestNotificationService(db)
}

func SetupTestAuthService(db *gorm.DB, service *services.Service) *auth.AuthService {
	config := SetupTestAuthConfig(db, service)
	notificationService := SetupTestNotificationService(db)
	return auth.NewAuthService(config, nil, service, notificationService)
}

func SetupTestAuthConfig(db *gorm.DB, service *services.Service) *auth.Config {
	return &auth.Config{
		DB:                  db,
		Service:             service,
		CookieName:          "session",
		CookieSecure:        false, // Allow cookies over HTTP in tests
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteLaxMode, // Less restrictive for tests
		ResetTokenExpiry:    3600,
		FrontendURL:         "http://example.com",
		RegistrationAllowed: true,
		AdminEmail:          "admin@example.com",
		TestMode:            true,
	}
}

// SetupTestLicenser is deprecated - licensing has been removed
// This function is kept for backward compatibility but does nothing
func SetupTestLicenser() interface{} {
	return nil
}

func PerformRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// PerformAuthRequest performs an HTTP request with authentication token
func PerformAuthRequest(r http.Handler, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ParseID converts a string ID to an integer. In tests, we often get string IDs
// from API responses but need to convert them to integers for database operations.
func ParseID(id string) int {
	i, err := strconv.Atoi(id)
	if err != nil {
		panic(err)
	}
	return i
}
