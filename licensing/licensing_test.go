package licensing

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func OneYear() time.Duration {
	day := 24 * time.Hour
	year := 365 * day

	return year
}

func TestLicenseCreate(t *testing.T) {
	t.Skip()
	privateKey, err := os.ReadFile("./keys/license_key.key")
	if err != nil {
		t.Fatal(err)
	}

	allLic, _ := Create(OneYear(),
		"feature_portal,feature_chat,feature_gateway",
		[]byte(privateKey))
	portal_gateway, _ := Create(OneYear(), "feature_portal,feature_gateway", []byte(privateKey))
	chat_only, _ := Create(OneYear(), "feature_chat", []byte(privateKey))
	os.WriteFile("keys/all.lic", []byte(allLic), 0644)
	os.WriteFile("keys/portal_gateway.lic", []byte(portal_gateway), 0644)
	os.WriteFile("keys/chat_only.lic", []byte(chat_only), 0644)
}

func TestLIcenseValidateFeatures(t *testing.T) {
	t.Skip()
	licenseData := map[string]interface{}{
		"feature_portal":  true,
		"feature_gateway": true,
		"num_seats":       5,
	}

	// Create a new licenser with minimal config for testing
	licenser := NewLicenser(LicenseConfig{
		DisableTelemetry: true,
	})

	// Initialize for tests with our test features
	licenser.InitializeForTests(licenseData)

	// test some features
	portal, ok := licenser.Entitlement("feature_portal")
	if !ok {
		t.Fatal("feature_portal not found")
	}

	if !portal.Bool() {
		t.Fatal("feature_portal should be true")
	}

	gateway, ok := licenser.Entitlement("feature_gateway")
	if !ok {
		t.Fatal("feature_gateway not found")
	}

	if !gateway.Bool() {
		t.Fatal("feature_gateway should be true")
	}

	seats, ok := licenser.Entitlement("num_seats")
	if !ok {
		t.Fatal("num_seats not found")
	}

	if seats.Int() != 5 {
		t.Fatal("num_seats should be 5")
	}
}

func LicenseData(email, org, id, licID string) map[string]interface{} {
	return map[string]interface{}{
		"email":           email,
		"org":             org,
		"customer_id":     id,
		"license_id":      licID,
		"feature_portal":  true,
		"feature_gateway": true,
		"num_seats":       5,
	}
}

func TestLicenserWithRealLicense(t *testing.T) {
	t.Skip()

	privateKey, err := os.ReadFile("./keys/signing.key")
	if err != nil {
		t.Fatal(err)
	}

	licenseData := map[string]interface{}{
		"feature_portal":  true,
		"feature_gateway": true,
		"num_seats":       5,
	}

	lic, err := Create(OneYear(), licenseData, []byte(privateKey))
	if err != nil {
		t.Fatal(err)
	}

	// Create a licenser with the test license
	licenser := NewLicenser(LicenseConfig{
		LicenseKey:       string(lic),
		DisableTelemetry: true,
	})

	// Start the licenser, which will validate the license
	licenser.Start()
	defer licenser.Stop()

	// Verify the license was loaded properly
	if licenser.License() == nil || !licenser.License().IsValid {
		t.Fatal("License should be valid")
	}

	// Verify features
	portal, ok := licenser.Entitlement("feature_portal")
	if !ok {
		t.Fatal("feature_portal not found")
	}

	if !portal.Bool() {
		t.Fatal("feature_portal should be true")
	}
}

func HundredYears() time.Duration {
	day := 24 * time.Hour
	year := 365 * day

	return 100 * year
}

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "Failed to open database")

	err = models.InitModels(db)
	require.NoError(t, err, "Failed to init models")

	return db
}

// Test for NewLicenser() to verify proper initialization with various config inputs
func TestNewLicenser(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		// Check default values
		assert.Equal(t, 1*time.Hour, licenser.config.TelemetryPeriod)
		assert.Equal(t, 10*time.Minute, licenser.config.ValidityCheckPeriod)
		assert.Equal(t, telemetryAPIURL, licenser.config.TelemetryURL)
		assert.NotNil(t, licenser.telemetryClient)
		assert.NotNil(t, licenser.done)
		assert.NotNil(t, licenser.featuresInit)
		assert.False(t, licenser.initialized)
	})

	t.Run("custom values", func(t *testing.T) {
		customTelemetryPeriod := 2 * time.Hour
		customValidityPeriod := 20 * time.Minute
		customTelemetryURL := "https://custom-telemetry.example.com"

		licenser := NewLicenser(LicenseConfig{
			TelemetryPeriod:     customTelemetryPeriod,
			ValidityCheckPeriod: customValidityPeriod,
			TelemetryURL:        customTelemetryURL,
			DisableTelemetry:    true,
		})

		// Check custom values
		assert.Equal(t, customTelemetryPeriod, licenser.config.TelemetryPeriod)
		assert.Equal(t, customValidityPeriod, licenser.config.ValidityCheckPeriod)
		assert.Equal(t, customTelemetryURL, licenser.config.TelemetryURL)
		assert.True(t, licenser.config.DisableTelemetry)
		assert.NotNil(t, licenser.telemetryClient)
		assert.Equal(t, customTelemetryURL, licenser.telemetryClient.URL)
	})
}

// Test for FeatureSet() to verify it returns the correct set of features
func TestFeatureSet(t *testing.T) {
	t.Run("uninitialized licenser", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		// Close the channel in a goroutine after a short delay to simulate initialization
		go func() {
			time.Sleep(10 * time.Millisecond)
			// Use proper synchronization when modifying initialized
			licenser.lock.Lock()
			licenser.initialized = true
			licenser.lock.Unlock()
			close(licenser.featuresInit)
		}()

		// FeatureSet should wait for the featuresInit channel to be closed
		features := licenser.FeatureSet()
		assert.Nil(t, features)
	})

	t.Run("nil license", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		features := licenser.FeatureSet()
		assert.Nil(t, features)
	})

	t.Run("with features", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		// Create test features
		testFeatures := map[string]interface{}{
			"feature_portal":  true,
			"feature_gateway": true,
			"num_seats":       5,
		}

		licenser.InitializeForTests(testFeatures)

		features := licenser.FeatureSet()
		assert.NotNil(t, features)
		assert.Len(t, features, 3)

		// Verify feature values
		assert.True(t, features["feature_portal"].Bool())
		assert.True(t, features["feature_gateway"].Bool())
		assert.Equal(t, 5, features["num_seats"].Int())
	})
}

// Test for the validate() method to verify proper token validation
func TestValidate(t *testing.T) {
	t.Run("invalid public key", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		invalidPubKey := []byte("invalid-key")

		_, err := licenser.validate("dummy-token", invalidPubKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate: parse key")
	})

	t.Run("invalid token format", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		_, err := licenser.validate("invalid-token", []byte(pubKey))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate:")
	})
}

// Test for License() method to verify it returns the current license
func TestLicense(t *testing.T) {
	t.Run("nil license", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		license := licenser.License()
		assert.Nil(t, license)
	})

	t.Run("with license", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		// Create test features
		testFeatures := map[string]interface{}{
			"feature_portal":  true,
			"feature_gateway": true,
		}

		licenser.InitializeForTests(testFeatures)

		license := licenser.License()
		assert.NotNil(t, license)
		assert.Equal(t, "test-license", license.Key)
		assert.True(t, license.IsValid)
		assert.Len(t, license.Features, 2)
	})
}

// Test to verify telemetry functionality using an in-memory database
func TestTelemetry(t *testing.T) {
	// Set up an in-memory SQLite database
	db := setupTestDB(t)

	// Seed the database with test data
	// Add LLM models
	for i := 0; i < 10; i++ {
		llm := &models.LLM{
			Name:             fmt.Sprintf("test-llm-%d", i),
			Vendor:           models.Vendor("test-vendor"),
			ShortDescription: "Test LLM Model",
		}
		require.NoError(t, db.Create(llm).Error)
	}

	// Add some chat records with tokens for total tokens calculation
	require.NoError(t, db.Create(&models.LLMChatRecord{
		Name:            "test-model",
		Vendor:          "test-vendor",
		TotalTokens:     5000,
		InteractionType: models.ChatInteraction,
	}).Error)
	require.NoError(t, db.Create(&models.LLMChatRecord{
		Name:            "test-model",
		Vendor:          "test-vendor",
		TotalTokens:     3000,
		InteractionType: models.ProxyInteraction,
	}).Error)

	// Add some apps
	for i := 0; i < 5; i++ {
		app := &models.App{
			Name: fmt.Sprintf("test-app-%d", i),
		}
		require.NoError(t, db.Create(app).Error)
	}

	// Add some users
	for i := 0; i < 20; i++ {
		// Determine user type based on index
		isAdmin := false
		showPortal := false
		showChat := true

		if i < 2 {
			// Admin users
			isAdmin = true
			showPortal = true
			showChat = true
		} else if i < 7 {
			// Developer users (portal access)
			showPortal = true
		}
		// Else: regular chat users

		user := &models.User{
			Email:      fmt.Sprintf("user%d@example.com", i),
			Password:   "password",
			IsAdmin:    isAdmin,
			ShowPortal: showPortal,
			ShowChat:   showChat,
		}
		require.NoError(t, db.Create(user).Error)
	}

	// Add some user groups
	for i := 0; i < 3; i++ {
		group := &models.Group{
			Name: fmt.Sprintf("test-group-%d", i),
		}
		require.NoError(t, db.Create(group).Error)
	}

	// Add some chats
	for i := 0; i < 30; i++ {
		chat := &models.Chat{
			Name: fmt.Sprintf("test-chat-%d", i),
		}
		require.NoError(t, db.Create(chat).Error)
	}

	// Create a TelemetryService with our test database
	telemetryService := services.NewTelemetryService(db)

	// Create a test HTTP server to capture telemetry requests
	var trackCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trackCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a licenser with our test components
	licenser := NewLicenser(LicenseConfig{
		TelemetryService: telemetryService,
		TelemetryURL:     server.URL,
	})

	// Initialize with test features
	testFeatures := map[string]interface{}{
		"feature_portal":  true,
		"feature_gateway": true,
		"track":           true, // Enable telemetry tracking
	}
	licenser.InitializeForTests(testFeatures)

	// Test individual collection methods
	t.Run("collect LLM stats", func(t *testing.T) {
		trackCallsBefore := trackCalls
		licenser.collectLLMStats()

		// Verify the request was sent
		assert.Equal(t, trackCallsBefore+1, trackCalls, "Track should have been called once")
	})

	t.Run("collect App stats", func(t *testing.T) {
		trackCallsBefore := trackCalls
		licenser.collectAppStats()

		// Verify the request was sent
		assert.Equal(t, trackCallsBefore+1, trackCalls, "Track should have been called once")
	})

	t.Run("collect User stats", func(t *testing.T) {
		trackCallsBefore := trackCalls
		licenser.collectUserStats()

		// Verify the request was sent
		assert.Equal(t, trackCallsBefore+1, trackCalls, "Track should have been called once")
	})

	t.Run("collect Chat stats", func(t *testing.T) {
		trackCallsBefore := trackCalls
		licenser.collectChatStats()

		// Verify the request was sent
		assert.Equal(t, trackCallsBefore+1, trackCalls, "Track should have been called once")
	})

	t.Run("send all telemetry", func(t *testing.T) {
		// Reset trackCalls to ensure we're starting fresh
		trackCalls = 0
		licenser.SendTelemetry()

		// SendTelemetry should call all 4 collect methods
		assert.Equal(t, 4, trackCalls, "SendTelemetry should make 4 track calls")
	})

	t.Run("telemetry disabled", func(t *testing.T) {
		licenser.config.DisableTelemetry = true
		trackCallsBefore := trackCalls
		licenser.SendTelemetry()

		// No telemetry should be sent when disabled
		assert.Equal(t, trackCallsBefore, trackCalls, "No telemetry should be sent when disabled")
	})

	t.Run("telemetry service nil", func(t *testing.T) {
		licenser.config.DisableTelemetry = false
		licenser.config.TelemetryService = nil
		trackCallsBefore := trackCalls
		licenser.SendTelemetry()

		// No telemetry should be sent when service is nil
		assert.Equal(t, trackCallsBefore, trackCalls, "No telemetry should be sent when service is nil")
	})

	// Test error handling in track sending
	t.Run("handle track errors", func(t *testing.T) {
		// Restore telemetry service
		licenser.config.TelemetryService = telemetryService

		// Create a server that returns errors
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer errorServer.Close()

		// Create a new licenser with error server
		errorLicenser := NewLicenser(LicenseConfig{
			TelemetryService: telemetryService,
			TelemetryURL:     errorServer.URL,
		})
		errorLicenser.InitializeForTests(testFeatures)

		// This should not panic despite tracking errors
		errorLicenser.collectLLMStats()
		// We can't assert much here other than that it doesn't panic
	})

	// Test error handling in database queries
	t.Run("handle database errors", func(t *testing.T) {
		// Create a new database with no tables to simulate errors
		emptyDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Create a telemetry service with the empty database
		emptyService := services.NewTelemetryService(emptyDB)

		// Update the licenser to use the empty service
		licenser.config.TelemetryService = emptyService

		// The calls should not panic despite database errors
		licenser.SendTelemetry()
		// We can't assert much here other than that it doesn't panic
	})
}

// generateTestKey generates a private key for testing
func generateTestKey(t *testing.T) []byte {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)

	return privateKeyPEM
}

// createTestToken creates a test JWT token with the given claims
func createTestToken(t *testing.T, privateKeyPEM []byte, claims jwt.MapClaims) string {
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	require.NoError(t, err)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)
	require.NoError(t, err)

	return signedToken
}

// generatePublicKey extracts the public key from a private key
func generatePublicKey(t *testing.T, privateKeyPEM []byte) []byte {
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	require.NoError(t, err)

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	publicKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: publicKeyBytes,
		},
	)

	return publicKeyPEM
}

// TestStartAndStop tests the Start and Stop functions
func TestStartAndStop(t *testing.T) {
	// Generate a key pair for testing
	privateKeyPEM := generateTestKey(t)
	publicKeyPEM := generatePublicKey(t, privateKeyPEM)

	// Create claims for the token
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	claims := jwt.MapClaims{
		"exp":   float64(expiry.Unix()),
		"iat":   float64(now.Unix()),
		"nbf":   float64(now.Unix()),
		"scope": "feature_portal,feature_chat",
		"v":     "1.0",
	}

	// Create a token
	token := createTestToken(t, privateKeyPEM, claims)

	t.Run("Start with valid license", func(t *testing.T) {
		// Create a new licenser with our test token
		licenser := NewLicenser(LicenseConfig{
			LicenseKey:       token,
			DisableTelemetry: true, // Disable telemetry for testing
		})

		// Replace the public key with our test public key
		originalPubKey := pubKey
		pubKey = string(publicKeyPEM)
		defer func() { pubKey = originalPubKey }() // Restore original key after test

		// We need to use a recovery function because Start can call log.Fatalf
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Start() panicked: %v", r)
			}
		}()

		// Test the Start method
		licenser.Start()
		defer licenser.Stop() // Ensure cleanup

		// Verify that the licenser is properly initialized
		assert.True(t, licenser.initialized)

		// Verify that the license is set
		license := licenser.License()
		require.NotNil(t, license)
		assert.True(t, license.IsValid)
		assert.Equal(t, token, license.Key)
		assert.Equal(t, "1.0", license.Version)
		assert.Equal(t, expiry.Unix(), license.ExpiresAt.Unix())

		// Verify features
		featureSet := licenser.FeatureSet()
		assert.NotNil(t, featureSet)
		assert.Contains(t, featureSet, "feature_portal")
		assert.Contains(t, featureSet, "feature_chat")
	})

	t.Run("Start and Stop", func(t *testing.T) {
		// Replace the public key with our test public key
		originalPubKey := pubKey
		pubKey = string(publicKeyPEM)
		defer func() { pubKey = originalPubKey }() // Restore original key after test

		// Create a new licenser with telemetry enabled
		licenser := NewLicenser(LicenseConfig{
			LicenseKey:          token,
			DisableTelemetry:    false,
			ValidityCheckPeriod: 100 * time.Millisecond,
			TelemetryPeriod:     100 * time.Millisecond,
		})

		// We need to use a recovery function because Start can call log.Fatalf
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Start() panicked: %v", r)
			}
		}()

		// Test the Start method
		licenser.Start()

		// Allow some time for goroutines to start
		time.Sleep(10 * time.Millisecond)

		// Test the Stop method
		licenser.Stop()

		// Verify that the licenser is properly stopped
		// This is a bit tricky to test directly, but we can at least verify
		// that Stop doesn't panic
	})
}

// TestEntitlement tests the Entitlement function
func TestEntitlement(t *testing.T) {
	t.Run("with nil license", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		feature, ok := licenser.Entitlement("any-feature")
		assert.False(t, ok)
		assert.Nil(t, feature)
	})

	t.Run("with nil features", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		licenser.lock.Lock()
		licenser.license = &LicenseInfo{
			Key:      "test-license",
			IsValid:  true,
			Features: nil,
		}
		licenser.lock.Unlock()

		feature, ok := licenser.Entitlement("any-feature")
		assert.False(t, ok)
		assert.Nil(t, feature)
	})

	t.Run("with non-existent feature", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		testFeatures := map[string]interface{}{
			"feature_portal": true,
			"feature_chat":   true,
		}

		licenser.InitializeForTests(testFeatures)

		feature, ok := licenser.Entitlement("non-existent-feature")
		assert.False(t, ok)
		assert.Nil(t, feature)
	})

	t.Run("with existing feature", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})
		licenser.initialized = true
		close(licenser.featuresInit)

		testFeatures := map[string]interface{}{
			"feature_portal": true,
			"feature_chat":   false,
			"num_seats":      5,
		}

		licenser.InitializeForTests(testFeatures)

		// Test boolean feature (true)
		feature, ok := licenser.Entitlement("feature_portal")
		assert.True(t, ok)
		assert.NotNil(t, feature)
		assert.True(t, feature.Bool())

		// Test boolean feature (false)
		feature, ok = licenser.Entitlement("feature_chat")
		assert.True(t, ok)
		assert.NotNil(t, feature)
		assert.False(t, feature.Bool())

		// Test integer feature
		feature, ok = licenser.Entitlement("num_seats")
		assert.True(t, ok)
		assert.NotNil(t, feature)
		assert.Equal(t, 5, feature.Int())
	})

	t.Run("wait for initialization", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{})

		// Create a channel to signal when initialization is complete
		initDone := make(chan struct{})

		// Initialize features in a goroutine after a delay
		go func() {
			time.Sleep(50 * time.Millisecond)

			testFeatures := map[string]interface{}{
				"feature_portal": true,
			}

			licenser.InitializeForTests(testFeatures)
			close(initDone)
		}()

		// Wait for initialization to complete
		<-initDone

		// Now check the entitlement
		feature, ok := licenser.Entitlement("feature_portal")
		assert.True(t, ok)
		assert.NotNil(t, feature)
		assert.True(t, feature.Bool())
	})
}

// TestIsLicensed tests the isLicensed function
func TestIsLicensed(t *testing.T) {
	// Generate a key pair for testing
	privateKeyPEM := generateTestKey(t)
	publicKeyPEM := generatePublicKey(t, privateKeyPEM)

	// Create claims for the token
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	claims := jwt.MapClaims{
		"exp":   float64(expiry.Unix()),
		"iat":   float64(now.Unix()),
		"nbf":   float64(now.Unix()),
		"scope": "feature_portal,feature_chat",
		"v":     "1.0",
	}

	// Create a token
	token := createTestToken(t, privateKeyPEM, claims)

	t.Run("with empty license key", func(t *testing.T) {
		licenser := NewLicenser(LicenseConfig{
			LicenseKey: "",
		})

		err := licenser.isLicensed()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no TYK_AI_LICENSE env var found")
	})

	t.Run("with invalid license key", func(t *testing.T) {
		// Replace the public key with our test public key
		originalPubKey := pubKey
		pubKey = string(publicKeyPEM)
		defer func() { pubKey = originalPubKey }() // Restore original key after test

		licenser := NewLicenser(LicenseConfig{
			LicenseKey: "invalid-token",
		})

		err := licenser.isLicensed()
		assert.Error(t, err)
	})

	t.Run("with valid license key", func(t *testing.T) {
		// Replace the public key with our test public key
		originalPubKey := pubKey
		pubKey = string(publicKeyPEM)
		defer func() { pubKey = originalPubKey }() // Restore original key after test

		licenser := NewLicenser(LicenseConfig{
			LicenseKey: token,
		})

		err := licenser.isLicensed()
		assert.NoError(t, err)

		// Verify that the license is properly set up
		license := licenser.License()
		require.NotNil(t, license)
		assert.True(t, license.IsValid)
		assert.Equal(t, token, license.Key)
		assert.Equal(t, "1.0", license.Version)
		assert.Equal(t, expiry.Unix(), license.ExpiresAt.Unix())

		// Verify features
		assert.Len(t, license.Features, 2)
		assert.Contains(t, license.Features, "feature_portal")
		assert.Contains(t, license.Features, "feature_chat")
	})
}

// TestLicenseValidateExtended tests the validate function more thoroughly
func TestLicenseValidateExtended(t *testing.T) {
	// Generate a key pair for testing
	privateKeyPEM := generateTestKey(t)
	publicKeyPEM := generatePublicKey(t, privateKeyPEM)

	// Create a different key pair for testing invalid signatures
	otherPrivateKeyPEM := generateTestKey(t)

	// Create claims for the token
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	claims := jwt.MapClaims{
		"exp":   float64(expiry.Unix()),
		"iat":   float64(now.Unix()),
		"nbf":   float64(now.Unix()),
		"scope": "feature_portal,feature_chat",
		"v":     "1.0",
	}

	licenser := NewLicenser(LicenseConfig{})

	t.Run("with valid token and key", func(t *testing.T) {
		token := createTestToken(t, privateKeyPEM, claims)

		// Validate the token with the correct public key
		resultClaims, err := licenser.validate(token, publicKeyPEM)
		assert.NoError(t, err)
		assert.NotNil(t, resultClaims)

		// Verify claims
		assert.Equal(t, claims["v"], resultClaims["v"])
		assert.Equal(t, claims["scope"], resultClaims["scope"])
	})

	t.Run("with malformed token", func(t *testing.T) {
		malformedToken := "not-a-jwt-token"

		_, err := licenser.validate(malformedToken, publicKeyPEM)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate:")
	})

	t.Run("with invalid signature", func(t *testing.T) {
		// Create a token with a different private key
		tokenWithDifferentKey := createTestToken(t, otherPrivateKeyPEM, claims)

		// Try to validate with the original public key
		_, err := licenser.validate(tokenWithDifferentKey, publicKeyPEM)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate:")
	})

	t.Run("with expired token", func(t *testing.T) {
		// Create claims for an expired token
		expiredClaims := jwt.MapClaims{
			"exp":   float64(now.Add(-24 * time.Hour).Unix()), // expired
			"iat":   float64(now.Add(-48 * time.Hour).Unix()),
			"nbf":   float64(now.Add(-48 * time.Hour).Unix()),
			"scope": "feature_portal,feature_chat",
			"v":     "1.0",
		}

		expiredToken := createTestToken(t, privateKeyPEM, expiredClaims)

		_, err := licenser.validate(expiredToken, publicKeyPEM)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate:")
	})
}

// TestLicenseInfoSetup tests the setup function and its related methods
func TestLicenseInfoSetup(t *testing.T) {
	t.Run("setup with all claims", func(t *testing.T) {
		// Create license info with all claims
		claims := jwt.MapClaims{
			"v":     "2.0",
			"exp":   float64(time.Now().Add(24 * time.Hour).Unix()),
			"scope": "feature_portal,feature_chat,feature_gateway",
		}

		licenseInfo := &LicenseInfo{
			Key:      "test-license",
			IsValid:  true,
			Features: make(map[string]*Feature),
			claims:   claims,
		}

		// Call setup
		licenseInfo.setup()

		// Verify setup
		assert.Equal(t, "2.0", licenseInfo.Version)
		assert.NotZero(t, licenseInfo.ExpiresAt)
		assert.Len(t, licenseInfo.Features, 3)
		assert.Contains(t, licenseInfo.Features, "feature_portal")
		assert.Contains(t, licenseInfo.Features, "feature_chat")
		assert.Contains(t, licenseInfo.Features, "feature_gateway")
	})

	t.Run("setup with no claims", func(t *testing.T) {
		// Create license info with no claims
		licenseInfo := &LicenseInfo{
			Key:      "test-license",
			IsValid:  true,
			Features: make(map[string]*Feature),
			claims:   make(jwt.MapClaims),
		}

		// Call setup
		licenseInfo.setup()

		// Verify setup
		assert.Empty(t, licenseInfo.Version)
		assert.Zero(t, licenseInfo.ExpiresAt)
		assert.Empty(t, licenseInfo.Features)
	})

	t.Run("setup when not valid", func(t *testing.T) {
		// Create license info that is not valid
		licenseInfo := &LicenseInfo{
			Key:      "test-license",
			IsValid:  false, // Not valid
			Features: make(map[string]*Feature),
			claims:   make(jwt.MapClaims),
		}

		// Call setup
		licenseInfo.setup()

		// Verify setup does nothing when not valid
		assert.Empty(t, licenseInfo.Version)
		assert.Zero(t, licenseInfo.ExpiresAt)
		assert.Empty(t, licenseInfo.Features)
	})
}

// TestGetClaim tests the getClaim function
func TestGetClaim(t *testing.T) {
	claims := jwt.MapClaims{
		"string_claim": "value",
		"int_claim":    42,
		"bool_claim":   true,
	}

	licenseInfo := &LicenseInfo{
		claims: claims,
	}

	t.Run("existing string claim", func(t *testing.T) {
		claim, found := licenseInfo.getClaim("string_claim")
		assert.True(t, found)
		assert.Equal(t, "value", claim)
	})

	t.Run("existing int claim", func(t *testing.T) {
		claim, found := licenseInfo.getClaim("int_claim")
		assert.True(t, found)
		assert.Equal(t, 42, claim)
	})

	t.Run("existing bool claim", func(t *testing.T) {
		claim, found := licenseInfo.getClaim("bool_claim")
		assert.True(t, found)
		assert.Equal(t, true, claim)
	})

	t.Run("non-existent claim", func(t *testing.T) {
		claim, found := licenseInfo.getClaim("non_existent_claim")
		assert.False(t, found)
		assert.Nil(t, claim)
	})

	t.Run("with nil claims", func(t *testing.T) {
		nilClaimsLicense := &LicenseInfo{
			claims: nil,
		}

		claim, found := nilClaimsLicense.getClaim("any_claim")
		assert.False(t, found)
		assert.Nil(t, claim)
	})
}

// TestSetVersion tests the setVersion function
func TestSetVersion(t *testing.T) {
	t.Run("with version claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"v": "3.0",
		}

		licenseInfo := &LicenseInfo{
			claims: claims,
		}

		licenseInfo.setVersion()
		assert.Equal(t, "3.0", licenseInfo.Version)
	})

	t.Run("without version claim", func(t *testing.T) {
		licenseInfo := &LicenseInfo{
			claims: make(jwt.MapClaims),
		}

		licenseInfo.setVersion()
		assert.Empty(t, licenseInfo.Version)
	})

	t.Run("with non-string version claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"v": 123, // Not a string
		}

		licenseInfo := &LicenseInfo{
			claims: claims,
		}

		// This should not panic
		licenseInfo.setVersion()

		// The Version field won't be set because the type assertion fails
		assert.Empty(t, licenseInfo.Version)
	})
}

// TestSetLicenseExpire tests the setLicenseExpire function
func TestSetLicenseExpire(t *testing.T) {
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	t.Run("with expiry claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"exp": float64(expiry.Unix()),
		}

		licenseInfo := &LicenseInfo{
			claims: claims,
		}

		licenseInfo.setLicenseExpire()
		assert.Equal(t, expiry.Unix(), licenseInfo.ExpiresAt.Unix())
	})

	t.Run("without expiry claim", func(t *testing.T) {
		licenseInfo := &LicenseInfo{
			claims: make(jwt.MapClaims),
		}

		licenseInfo.setLicenseExpire()
		assert.Zero(t, licenseInfo.ExpiresAt)
	})

	t.Run("with non-numeric expiry claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"exp": "not-a-number",
		}

		licenseInfo := &LicenseInfo{
			claims: claims,
		}

		// This should not panic
		licenseInfo.setLicenseExpire()

		// The ExpiresAt field won't be set because the type assertion fails
		assert.Zero(t, licenseInfo.ExpiresAt)
	})
}

// TestSetFeatures tests the setFeatures function
func TestSetFeatures(t *testing.T) {
	t.Run("with scope claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"scope": "feature_portal,feature_chat,custom_feature",
		}

		licenseInfo := &LicenseInfo{
			claims:   claims,
			Features: make(map[string]*Feature),
		}

		licenseInfo.setFeatures()
		assert.Len(t, licenseInfo.Features, 3)
		assert.Contains(t, licenseInfo.Features, "feature_portal")
		assert.Contains(t, licenseInfo.Features, "feature_chat")
		assert.Contains(t, licenseInfo.Features, "custom_feature")
	})

	t.Run("without scope claim", func(t *testing.T) {
		licenseInfo := &LicenseInfo{
			claims:   make(jwt.MapClaims),
			Features: make(map[string]*Feature),
		}

		licenseInfo.setFeatures()
		assert.Empty(t, licenseInfo.Features)
	})

	t.Run("with non-string scope claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"scope": 123, // Not a string
		}

		licenseInfo := &LicenseInfo{
			claims:   claims,
			Features: make(map[string]*Feature),
		}

		// This should not panic
		licenseInfo.setFeatures()

		// The Features field won't be set because the type assertion fails
		assert.Empty(t, licenseInfo.Features)
	})

	// Note: We don't test the error case where NewFeature returns an error
	// because it's hard to create that scenario without modifying the code
}
