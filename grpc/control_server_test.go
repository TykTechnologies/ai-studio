package grpc

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	testEncryptionKey = "12345678901234567890123456789012" // 32 characters
	testAuthToken     = "test-auth-token"
	testNextToken     = "test-next-token"
)

// setupTestDB creates an in-memory database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Configure SQLite connection for concurrency
	sqlDB, err := db.DB()
	require.NoError(t, err)

	// Set connection pool parameters for concurrent access
	sqlDB.SetMaxOpenConns(1)    // SQLite works best with a single connection
	sqlDB.SetMaxIdleConns(1)    // Keep the connection alive
	sqlDB.SetConnMaxLifetime(0) // Never close the connection

	// Initialize models
	err = models.InitModels(db)
	require.NoError(t, err)

	return db
}

// setupTestServer creates a control server for testing with proper encryption key setup
func setupTestServer(t *testing.T, config *Config) (*ControlServer, *gorm.DB) {
	// Set required environment variable
	os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", testEncryptionKey)
	t.Cleanup(func() {
		os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY")
	})

	db := setupTestDB(t)

	if config == nil {
		config = &Config{
			GRPCPort:             0, // Use random port
			GRPCHost:             "localhost",
			TLSEnabled:           false,
			AuthToken:            testAuthToken,
			MaxConcurrentStreams: 100,
		}
	}

	server := NewControlServer(config, db)
	return server, db
}

// createTestLLMs creates test LLM data
func createTestLLMs(db *gorm.DB, namespace string) []models.LLM {
	llms := []models.LLM{
		{
			Name:         "Test OpenAI",
			Vendor:       models.OPENAI,
			APIKey:       "test-key-1",
			APIEndpoint:  "https://api.openai.com/v1",
			DefaultModel: "gpt-4",
			Active:       true,
			Namespace:    namespace,
		},
		{
			Name:         "Test Anthropic",
			Vendor:       models.ANTHROPIC,
			APIKey:       "test-key-2",
			APIEndpoint:  "https://api.anthropic.com/v1",
			DefaultModel: "claude-3-sonnet",
			Active:       true,
			Namespace:    namespace,
		},
	}

	for i := range llms {
		db.Create(&llms[i])
	}

	return llms
}

// createTestApps creates test app data
func createTestApps(db *gorm.DB, namespace string, llms []models.LLM) []models.App {
	apps := []models.App{
		{
			Name:        "Test App 1",
			Description: "Test application 1",
			IsActive:    true,
			Namespace:   namespace,
		},
		{
			Name:        "Test App 2",
			Description: "Test application 2",
			IsActive:    true,
			Namespace:   namespace,
		},
	}

	for i := range apps {
		db.Create(&apps[i])
		// Associate with LLMs
		for _, llm := range llms {
			apps[i].LLMs = append(apps[i].LLMs, llm)
		}
		db.Save(&apps[i])
	}

	return apps
}

// createTestCredentialAndApp creates a test credential with associated app
func createTestCredentialAndApp(db *gorm.DB, token string) (*models.Credential, *models.App) {
	credential := &models.Credential{
		KeyID:  "test-valid-key-id",
		Secret: token,
		Active: true,
	}
	db.Create(credential)

	app := &models.App{
		Name:         "Test Token App",
		Description:  "App for token testing",
		IsActive:     true,
		CredentialID: credential.ID,
		Namespace:    "",
	}
	db.Create(app)

	return credential, app
}

// TestNewControlServer tests the control server creation and configuration validation
func TestNewControlServer(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func()
		config      *Config
		expectPanic bool
		panicMsg    string
	}{
		{
			name: "valid configuration",
			setupEnv: func() {
				os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", testEncryptionKey)
			},
			config: &Config{
				GRPCPort:             8080,
				GRPCHost:             "0.0.0.0",
				AuthToken:            testAuthToken,
				MaxConcurrentStreams: 1000,
			},
			expectPanic: false,
		},
		{
			name: "missing encryption key",
			setupEnv: func() {
				os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY")
			},
			config: &Config{
				AuthToken: testAuthToken,
			},
			expectPanic: true,
			panicMsg:    "MICROGATEWAY_ENCRYPTION_KEY environment variable is required",
		},
		{
			name: "invalid encryption key length",
			setupEnv: func() {
				os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "short-key")
			},
			config: &Config{
				AuthToken: testAuthToken,
			},
			expectPanic: true,
			panicMsg:    "must be exactly 32 characters long",
		},
		{
			name: "default insecure key",
			setupEnv: func() {
				os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", DEFAULT_ENCRYPTION_KEY)
			},
			config: &Config{
				AuthToken: testAuthToken,
			},
			expectPanic: true,
			panicMsg:    "cannot use the default insecure key",
		},
		{
			name: "zero max concurrent streams - should use default",
			setupEnv: func() {
				os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", testEncryptionKey)
			},
			config: &Config{
				AuthToken:            testAuthToken,
				MaxConcurrentStreams: 0,
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			tt.setupEnv()
			defer func() {
				os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY")
			}()

			db := setupTestDB(t)

			if tt.expectPanic {
				// Note: We can't easily test log.Fatal() panics in unit tests
				// since log.Fatal() calls os.Exit(1) which terminates the test process
				t.Skip("Skipping panic test for log.Fatal() - would terminate test process")
			} else {
				server := NewControlServer(tt.config, db)
				assert.NotNil(t, server)
				assert.Equal(t, tt.config, server.config)
				assert.Equal(t, db, server.db)
				assert.NotNil(t, server.edgeConnections)

				// Check default max concurrent streams
				expectedMax := tt.config.MaxConcurrentStreams
				if expectedMax <= 0 {
					expectedMax = 1000
				}
				assert.Equal(t, expectedMax, server.maxConcurrentStreams)

				// Verify cleanup ticker is started
				assert.NotNil(t, server.cleanupTicker)

				// Stop cleanup ticker to prevent resource leak
				server.cleanupTicker.Stop()
			}
		})
	}
}

// TestControlServer_Stop tests graceful server shutdown
func TestControlServer_Stop(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	// Add some mock connections
	server.edgeMutex.Lock()
	server.edgeConnections["edge1"] = &EdgeInstanceConnection{
		EdgeID:    "edge1",
		Namespace: "test",
		Status:    "connected",
		Stream:    nil, // Mock stream would go here
	}
	server.edgeMutex.Unlock()

	// Stop should not panic
	assert.NotPanics(t, func() {
		server.Stop()
	})

	// Note: Cleanup ticker is not easily testable since it's an internal timer
	// The main thing is that Stop() doesn't panic
}

// TestControlServer_authenticate tests the authentication logic
func TestControlServer_authenticate(t *testing.T) {
	tests := []struct {
		name       string
		authToken  string
		nextToken  string
		metadata   map[string]string
		expectErr  bool
		expectCode codes.Code
	}{
		{
			name:      "valid auth token",
			authToken: testAuthToken,
			metadata: map[string]string{
				"authorization": "Bearer " + testAuthToken,
			},
			expectErr: false,
		},
		{
			name:      "valid next token",
			authToken: testAuthToken,
			nextToken: testNextToken,
			metadata: map[string]string{
				"authorization": "Bearer " + testNextToken,
			},
			expectErr: false,
		},
		{
			name:      "missing metadata",
			authToken: testAuthToken,
			metadata:  nil,
			expectErr: true,
			expectCode: codes.Unauthenticated,
		},
		{
			name:      "missing authorization header",
			authToken: testAuthToken,
			metadata:  map[string]string{},
			expectErr: true,
			expectCode: codes.Unauthenticated,
		},
		{
			name:      "invalid token",
			authToken: testAuthToken,
			metadata: map[string]string{
				"authorization": "Bearer invalid-token",
			},
			expectErr:  true,
			expectCode: codes.Unauthenticated,
		},
		{
			name:      "no auth tokens configured",
			authToken: "",
			nextToken: "",
			metadata: map[string]string{
				"authorization": "Bearer some-token",
			},
			expectErr:  true,
			expectCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				AuthToken:     tt.authToken,
				NextAuthToken: tt.nextToken,
			}
			server, _ := setupTestServer(t, config)

			// Create context with metadata
			ctx := context.Background()
			if tt.metadata != nil {
				md := metadata.New(tt.metadata)
				ctx = metadata.NewIncomingContext(ctx, md)
			}

			err := server.authenticate(ctx)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectCode != codes.OK {
					grpcErr, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectCode, grpcErr.Code())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestControlServer_encryptForMicrogateway tests the encryption functionality
func TestControlServer_encryptForMicrogateway(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	tests := []struct {
		name      string
		plaintext string
		expectErr bool
	}{
		{
			name:      "encrypt valid string",
			plaintext: "test-api-key-12345",
			expectErr: false,
		},
		{
			name:      "encrypt empty string",
			plaintext: "",
			expectErr: false,
		},
		{
			name:      "encrypt long string",
			plaintext: "very-long-api-key-with-lots-of-characters-that-should-still-work-fine",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := server.encryptForMicrogateway(tt.plaintext)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.plaintext == "" {
					assert.Empty(t, encrypted)
				} else {
					assert.NotEmpty(t, encrypted)
					assert.NotEqual(t, tt.plaintext, encrypted)
				}
			}
		})
	}
}

// TestControlServer_encryptForMicrogateway_NoKey tests encryption without environment key
func TestControlServer_encryptForMicrogateway_NoKey(t *testing.T) {
	// Don't use setupTestServer since it requires the encryption key
	// Test the encryptForMicrogateway function directly
	server := &ControlServer{}

	// Temporarily unset the environment variable
	originalKey := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", originalKey)
		}
	}()

	_, err := server.encryptForMicrogateway("test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MICROGATEWAY_ENCRYPTION_KEY environment variable is required")
}

// TestControlServer_RegisterEdge tests edge registration functionality
func TestControlServer_RegisterEdge(t *testing.T) {
	server, db := setupTestServer(t, nil)

	tests := []struct {
		name        string
		request     *pb.EdgeRegistrationRequest
		expectErr   bool
		expectCode  codes.Code
		setupDB     func()
		validateDB  func(t *testing.T)
	}{
		{
			name: "new edge registration",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "edge-001",
				EdgeNamespace: "test-namespace",
				Version:       "1.0.0",
				BuildHash:     "abc123",
				Metadata: map[string]string{
					"region": "us-west-1",
					"env":    "production",
				},
			},
			expectErr: false,
			validateDB: func(t *testing.T) {
				var edge models.EdgeInstance
				err := db.Where("edge_id = ?", "edge-001").First(&edge).Error
				assert.NoError(t, err)
				assert.Equal(t, "edge-001", edge.EdgeID)
				assert.Equal(t, "test-namespace", edge.Namespace)
				assert.Equal(t, models.EdgeStatusRegistered, edge.Status)
				assert.NotEmpty(t, edge.SessionID)
			},
		},
		{
			name: "existing edge re-registration",
			request: &pb.EdgeRegistrationRequest{
				EdgeId:        "edge-002",
				EdgeNamespace: "test-namespace",
				Version:       "1.1.0",
				BuildHash:     "def456",
			},
			expectErr: false,
			setupDB: func() {
				// Create existing edge
				edge := models.EdgeInstance{
					EdgeID:    "edge-002",
					Namespace: "test-namespace",
					Version:   "1.0.0",
					Status:    models.EdgeStatusDisconnected,
				}
				db.Create(&edge)
			},
			validateDB: func(t *testing.T) {
				var edge models.EdgeInstance
				err := db.Where("edge_id = ?", "edge-002").First(&edge).Error
				assert.NoError(t, err)
				assert.Equal(t, "1.1.0", edge.Version) // Version should be updated
				assert.Equal(t, models.EdgeStatusRegistered, edge.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupDB != nil {
				tt.setupDB()
			}

			ctx := context.Background()
			response, err := server.RegisterEdge(ctx, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectCode != codes.OK {
					grpcErr, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectCode, grpcErr.Code())
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.True(t, response.Success)
				assert.NotEmpty(t, response.SessionId)
				assert.NotNil(t, response.InitialConfig)

				if tt.validateDB != nil {
					tt.validateDB(t)
				}
			}
		})
	}
}

// TestControlServer_UnregisterEdge tests edge unregistration
func TestControlServer_UnregisterEdge(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Setup an edge connection
	server.edgeMutex.Lock()
	server.edgeConnections["edge-test"] = &EdgeInstanceConnection{
		EdgeID:    "edge-test",
		Namespace: "test",
		Status:    "connected",
	}
	server.edgeMutex.Unlock()

	// Create edge in database
	edge := models.EdgeInstance{
		EdgeID:    "edge-test",
		Namespace: "test",
		Status:    models.EdgeStatusConnected,
	}
	db.Create(&edge)

	ctx := context.Background()
	req := &pb.EdgeUnregistrationRequest{
		EdgeId: "edge-test",
		Reason: "shutdown",
	}

	_, err := server.UnregisterEdge(ctx, req)
	assert.NoError(t, err)

	// Verify connection is removed
	server.edgeMutex.RLock()
	_, exists := server.edgeConnections["edge-test"]
	server.edgeMutex.RUnlock()
	assert.False(t, exists)

	// Verify database status
	var dbEdge models.EdgeInstance
	db.Where("edge_id = ?", "edge-test").First(&dbEdge)
	assert.Equal(t, "unregistered", dbEdge.Status)
}

// TestControlServer_SendHeartbeat tests heartbeat handling
func TestControlServer_SendHeartbeat(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Setup an edge connection
	edge := &EdgeInstanceConnection{
		EdgeID:    "edge-heartbeat",
		Namespace: "test",
		Status:    "connected",
	}
	server.edgeMutex.Lock()
	server.edgeConnections["edge-heartbeat"] = edge
	server.edgeMutex.Unlock()

	// Create edge in database
	dbEdge := models.EdgeInstance{
		EdgeID:    "edge-heartbeat",
		Namespace: "test",
		Status:    models.EdgeStatusConnected,
	}
	db.Create(&dbEdge)

	tests := []struct {
		name       string
		request    *pb.HeartbeatRequest
		expectErr  bool
		expectCode codes.Code
	}{
		{
			name: "valid heartbeat",
			request: &pb.HeartbeatRequest{
				EdgeId:    "edge-heartbeat",
				SessionId: "test-session",
			},
			expectErr: false,
		},
		{
			name: "edge not found",
			request: &pb.HeartbeatRequest{
				EdgeId:    "nonexistent-edge",
				SessionId: "test-session",
			},
			expectErr:  true,
			expectCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			response, err := server.SendHeartbeat(ctx, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectCode != codes.OK {
					grpcErr, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectCode, grpcErr.Code())
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.True(t, response.Acknowledged)
				assert.NotEmpty(t, response.Message)
			}
		})
	}
}

// TestControlServer_ValidateToken tests token validation functionality
func TestControlServer_ValidateToken(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Create test credential and app
	testToken := "test-valid-token-123"
	credential, app := createTestCredentialAndApp(db, testToken)

	tests := []struct {
		name       string
		request    *pb.TokenValidationRequest
		expectErr  bool
		expectCode codes.Code
		setupDB    func()
		validate   func(t *testing.T, response *pb.TokenValidationResponse)
	}{
		{
			name: "valid token",
			request: &pb.TokenValidationRequest{
				Token:         testToken,
				EdgeId:        "edge-001",
				EdgeNamespace: "test",
			},
			expectErr: false,
			validate: func(t *testing.T, response *pb.TokenValidationResponse) {
				assert.True(t, response.Valid)
				assert.Equal(t, uint32(app.ID), response.AppId)
				assert.Equal(t, app.Name, response.AppName)
				assert.Empty(t, response.Scopes) // AI Studio doesn't use scopes
				assert.Nil(t, response.ExpiresAt) // AI Studio credentials don't expire
			},
		},
		{
			name: "invalid token",
			request: &pb.TokenValidationRequest{
				Token:         "invalid-token",
				EdgeId:        "edge-001",
				EdgeNamespace: "test",
			},
			expectErr: false,
			validate: func(t *testing.T, response *pb.TokenValidationResponse) {
				assert.False(t, response.Valid)
				assert.Equal(t, "Invalid token", response.ErrorMessage)
			},
		},
		{
			name: "inactive credential",
			request: &pb.TokenValidationRequest{
				Token:         "inactive-token",
				EdgeId:        "edge-001",
				EdgeNamespace: "test",
			},
			setupDB: func() {
				// Create inactive credential
				inactiveCredential := &models.Credential{
					KeyID:  "inactive-key-id",
					Secret: "inactive-token",
					Active: false,
				}
				db.Create(inactiveCredential)
			},
			expectErr: false,
			validate: func(t *testing.T, response *pb.TokenValidationResponse) {
				assert.False(t, response.Valid)
				assert.Equal(t, "Invalid token", response.ErrorMessage)
			},
		},
		{
			name: "inactive app",
			request: &pb.TokenValidationRequest{
				Token:         "inactive-app-token",
				EdgeId:        "edge-001",
				EdgeNamespace: "test",
			},
			setupDB: func() {
				// Create credential with inactive app
				inactiveAppCredential := &models.Credential{
					KeyID:  "inactive-app-key-id",
					Secret: "inactive-app-token",
					Active: true,
				}
				db.Create(inactiveAppCredential)

				inactiveApp := &models.App{
					Name:         "Inactive App",
					IsActive:     false, // Inactive app
					CredentialID: inactiveAppCredential.ID,
				}
				result := db.Create(inactiveApp)
				if result.Error != nil {
					t.Fatalf("Failed to create inactive app: %v", result.Error)
				}

				// Force update IsActive to false (due to GORM default behavior)
				db.Model(inactiveApp).Update("is_active", false)
			},
			expectErr: false,
			validate: func(t *testing.T, response *pb.TokenValidationResponse) {
				assert.False(t, response.Valid)
				assert.Equal(t, "Associated app not found or inactive", response.ErrorMessage)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupDB != nil {
				tt.setupDB()
			}

			ctx := context.Background()
			response, err := server.ValidateToken(ctx, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectCode != codes.OK {
					grpcErr, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectCode, grpcErr.Code())
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validate != nil {
					tt.validate(t, response)
				}
			}

			// Cleanup for independent tests
			_ = credential
		})
	}
}

// TestControlServer_getConfigurationSnapshot tests configuration snapshot generation
func TestControlServer_getConfigurationSnapshot(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Create test data
	namespace := "test-namespace"
	llms := createTestLLMs(db, namespace)
	apps := createTestApps(db, namespace, llms)

	// Create test filters
	filter := models.Filter{
		Name:        "Test Filter",
		Description: "Test filter description",
		Script:      []byte("console.log('test filter');"),
		Namespace:   namespace,
	}
	db.Create(&filter)

	// Create test model price
	modelPrice := models.ModelPrice{
		Vendor:      "openai",
		ModelName:   "gpt-4",
		CPT:         0.03,
		CPIT:        0.06,
		Currency:    "USD",
	}
	db.Create(&modelPrice)

	tests := []struct {
		name      string
		namespace string
		validate  func(t *testing.T, snapshot *pb.ConfigurationSnapshot)
		expectErr bool
	}{
		{
			name:      "valid namespace snapshot",
			namespace: namespace,
			validate: func(t *testing.T, snapshot *pb.ConfigurationSnapshot) {
				assert.NotEmpty(t, snapshot.Version)
				assert.Len(t, snapshot.Llms, len(llms))
				assert.Len(t, snapshot.Apps, len(apps))
				assert.Len(t, snapshot.Filters, 1)
				assert.Len(t, snapshot.ModelPrices, 1)

				// Verify LLM data
				for _, llmConfig := range snapshot.Llms {
					assert.NotEmpty(t, llmConfig.Name)
					assert.NotEmpty(t, llmConfig.Vendor)
					assert.NotEmpty(t, llmConfig.Endpoint)
					assert.True(t, llmConfig.IsActive)
					assert.Equal(t, namespace, llmConfig.Namespace)
				}

				// Verify App data
				for _, appConfig := range snapshot.Apps {
					assert.NotEmpty(t, appConfig.Name)
					assert.True(t, appConfig.IsActive)
					assert.Equal(t, namespace, appConfig.Namespace)
					assert.NotEmpty(t, appConfig.LlmIds)
				}
			},
			expectErr: false,
		},
		{
			name:      "empty namespace (global)",
			namespace: "",
			validate: func(t *testing.T, snapshot *pb.ConfigurationSnapshot) {
				// Should only include global items
				assert.NotEmpty(t, snapshot.Version)
				assert.NotNil(t, snapshot.Llms)
				assert.NotNil(t, snapshot.Apps)
				assert.NotNil(t, snapshot.Filters)
				assert.NotNil(t, snapshot.ModelPrices)
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := server.getConfigurationSnapshot(tt.namespace)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, snapshot)
				if tt.validate != nil {
					tt.validate(t, snapshot)
				}
			}
		})
	}
}

// TestControlServer_GetFullConfiguration tests the gRPC configuration endpoint
func TestControlServer_GetFullConfiguration(t *testing.T) {
	server, db := setupTestServer(t, nil)

	namespace := "test"
	createTestLLMs(db, namespace)

	ctx := context.Background()
	req := &pb.ConfigurationRequest{
		EdgeNamespace: namespace,
	}

	response, err := server.GetFullConfiguration(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotEmpty(t, response.Version)
	assert.NotNil(t, response.Llms)
	assert.NotNil(t, response.Apps)
}

// TestControlServer_extractVendorFromEvent tests vendor extraction from analytics events
func TestControlServer_extractVendorFromEvent(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Create test LLM
	llm := models.LLM{
		Name:   "Test LLM",
		Vendor: models.ANTHROPIC,
	}
	db.Create(&llm)

	tests := []struct {
		name     string
		event    *pb.AnalyticsEvent
		expected string
	}{
		{
			name: "extract from database by LLM ID",
			event: &pb.AnalyticsEvent{
				LlmId: uint32(llm.ID),
			},
			expected: "anthropic",
		},
		{
			name: "extract from OpenAI endpoint",
			event: &pb.AnalyticsEvent{
				Endpoint: "https://api.openai.com/v1/chat/completions",
			},
			expected: "openai",
		},
		{
			name: "extract from Anthropic endpoint",
			event: &pb.AnalyticsEvent{
				Endpoint: "https://api.anthropic.com/v1/messages",
			},
			expected: "anthropic",
		},
		{
			name: "extract from vertex endpoint",
			event: &pb.AnalyticsEvent{
				Endpoint: "https://vertex-ai.googleapis.com/v1/projects",
			},
			expected: "vertex",
		},
		{
			name: "unknown endpoint",
			event: &pb.AnalyticsEvent{
				Endpoint: "https://unknown-provider.com/api",
			},
			expected: "unknown",
		},
		{
			name:     "no information available",
			event:    &pb.AnalyticsEvent{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.extractVendorFromEvent(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestControlServer_isEdgeStreamActive tests edge stream activity checking
func TestControlServer_isEdgeStreamActive(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	tests := []struct {
		name     string
		edge     *EdgeInstanceConnection
		expected bool
	}{
		{
			name:     "nil edge",
			edge:     nil,
			expected: false,
		},
		{
			name: "edge without stream",
			edge: &EdgeInstanceConnection{
				EdgeID: "test-edge",
				Stream: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.isEdgeStreamActive(tt.edge)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestControlServer_cleanupStaleConnections tests the cleanup routine
func TestControlServer_cleanupStaleConnections(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Create edge in database
	edge := models.EdgeInstance{
		EdgeID:    "stale-edge",
		Namespace: "test",
		Status:    models.EdgeStatusConnected,
	}
	db.Create(&edge)

	// Add stale connection
	server.edgeMutex.Lock()
	server.edgeConnections["stale-edge"] = &EdgeInstanceConnection{
		EdgeID:    "stale-edge",
		Namespace: "test",
		Status:    "connected",
		Stream:    nil, // No active stream
	}
	server.edgeMutex.Unlock()

	// Run cleanup
	server.cleanupStaleConnections()

	// Verify connection is removed
	server.edgeMutex.RLock()
	_, exists := server.edgeConnections["stale-edge"]
	server.edgeMutex.RUnlock()
	assert.False(t, exists, "Stale connection should be removed")

	// Verify database status is updated
	var dbEdge models.EdgeInstance
	db.Where("edge_id = ?", "stale-edge").First(&dbEdge)
	assert.Equal(t, models.EdgeStatusDisconnected, dbEdge.Status)
}

// TestControlServer_SendAnalyticsPulse tests analytics pulse processing
func TestControlServer_SendAnalyticsPulse(t *testing.T) {
	server, db := setupTestServer(t, nil)

	// Create test LLM for vendor extraction
	llm := models.LLM{
		Name:   "Test LLM",
		Vendor: models.OPENAI,
	}
	db.Create(&llm)

	tests := []struct {
		name     string
		request  *pb.AnalyticsPulse
		validate func(t *testing.T, response *pb.AnalyticsPulseResponse)
	}{
		{
			name: "valid analytics pulse",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-001",
				EdgeNamespace:  "test",
				SequenceNumber: 1,
				TotalRecords:   2,
				AnalyticsEvents: []*pb.AnalyticsEvent{
					{
						RequestId:      "req-001",
						AppId:          1,
						LlmId:          uint32(llm.ID),
						ModelName:      "gpt-4",
						Vendor:         "openai",
						Endpoint:       "https://api.openai.com/v1/chat/completions",
						StatusCode:     200,
						TotalTokens:    100,
						RequestTokens:  80,
						ResponseTokens: 20,
						Cost:           0.002,
						Timestamp:      timestamppb.Now(),
						RequestBody:    "test request",
						ResponseBody:   "test response",
					},
				},
				BudgetEvents: []*pb.BudgetUsageEvent{
					{
						AppId:      1,
						LlmId:      uint32(llm.ID),
						TokensUsed: 100,
						Cost:       0.002,
						Timestamp:  timestamppb.Now(),
					},
				},
				ProxySummaries: []*pb.ProxyLogSummary{
					{
						AppId:        1,
						Vendor:       "openai",
						RequestCount: 1,
						TotalCost:    0.002,
					},
				},
			},
			validate: func(t *testing.T, response *pb.AnalyticsPulseResponse) {
				assert.True(t, response.Success)
				assert.Equal(t, "Analytics pulse processed successfully", response.Message)
				assert.Equal(t, uint64(3), response.ProcessedRecords) // 1 analytics + 1 budget + 1 summary
				assert.Equal(t, uint64(1), response.SequenceNumber)
				assert.NotNil(t, response.ProcessedAt)
			},
		},
		{
			name: "empty analytics pulse",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-002",
				EdgeNamespace:  "test",
				SequenceNumber: 2,
				TotalRecords:   0,
			},
			validate: func(t *testing.T, response *pb.AnalyticsPulseResponse) {
				assert.True(t, response.Success)
				assert.Equal(t, uint64(0), response.ProcessedRecords)
				assert.Equal(t, uint64(2), response.SequenceNumber)
			},
		},
		{
			name: "analytics with vendor extraction",
			request: &pb.AnalyticsPulse{
				EdgeId:         "edge-003",
				EdgeNamespace:  "test",
				SequenceNumber: 3,
				TotalRecords:   1,
				AnalyticsEvents: []*pb.AnalyticsEvent{
					{
						RequestId:      "req-002",
						AppId:          1,
						LlmId:          999, // Non-existent LLM
						Endpoint:       "https://api.anthropic.com/v1/messages",
						StatusCode:     200,
						TotalTokens:    50,
						RequestTokens:  40,
						ResponseTokens: 10,
						Cost:           0.001,
						Timestamp:      timestamppb.Now(),
					},
				},
			},
			validate: func(t *testing.T, response *pb.AnalyticsPulseResponse) {
				assert.True(t, response.Success)
				assert.Equal(t, uint64(1), response.ProcessedRecords)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			response, err := server.SendAnalyticsPulse(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, response)
			if tt.validate != nil {
				tt.validate(t, response)
			}
		})
	}
}

// TestControlServer_ConcurrentConnections tests concurrent connection handling
func TestControlServer_ConcurrentConnections(t *testing.T) {
	config := &Config{
		GRPCPort:             0,
		GRPCHost:             "localhost",
		TLSEnabled:           false,
		AuthToken:            testAuthToken,
		MaxConcurrentStreams: 5, // Low limit for testing
	}
	server, _ := setupTestServer(t, config)

	// Test concurrent edge registrations
	numConcurrent := 10
	var wg sync.WaitGroup
	results := make([]error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			ctx := context.Background()
			req := &pb.EdgeRegistrationRequest{
				EdgeId:        "edge-" + string(rune(index+'A')),
				EdgeNamespace: "test",
				Version:       "1.0.0",
			}

			_, err := server.RegisterEdge(ctx, req)
			results[index] = err
		}(i)
	}

	wg.Wait()

	// All registrations should succeed (registration doesn't count against stream limit)
	for i, err := range results {
		assert.NoError(t, err, "Registration %d should succeed", i)
	}

	// Verify all edges were registered
	server.edgeMutex.RLock()
	// Note: EdgeInstanceConnection objects aren't created during RegisterEdge,
	// only during streaming connections
	server.edgeMutex.RUnlock()
}

// TestControlServer_GetConnectedEdges tests getting connected edges
func TestControlServer_GetConnectedEdges(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	// Add some mock connections
	now := time.Now()
	server.edgeMutex.Lock()
	server.edgeConnections["edge1"] = &EdgeInstanceConnection{
		EdgeID:        "edge1",
		Namespace:     "test1",
		Status:        "connected",
		Version:       "1.0.0",
		SessionID:     "session1",
		LastHeartbeat: now,
	}
	server.edgeConnections["edge2"] = &EdgeInstanceConnection{
		EdgeID:        "edge2",
		Namespace:     "test2",
		Status:        "connected",
		Version:       "1.1.0",
		SessionID:     "session2",
		LastHeartbeat: now,
	}
	// Add stale connection (no stream)
	server.edgeConnections["stale-edge"] = &EdgeInstanceConnection{
		EdgeID:        "stale-edge",
		Namespace:     "test",
		Status:        "connected",
		Stream:        nil, // No active stream
		LastHeartbeat: now.Add(-15 * time.Minute), // Stale heartbeat
	}
	server.edgeMutex.Unlock()

	connected := server.GetConnectedEdges()

	// Should only include edges with active streams (none in this test)
	// Since isEdgeStreamActive returns false for nil streams
	assert.Len(t, connected, 0, "No edges should be considered connected without active streams")
}

// TestControlServer_SendReloadRequest tests sending reload requests to edges
func TestControlServer_SendReloadRequest(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	tests := []struct {
		name        string
		edgeID      string
		setupEdge   func()
		expectErr   bool
		expectErrMsg string
	}{
		{
			name:         "edge not found",
			edgeID:       "nonexistent-edge",
			setupEdge:    func() {}, // No setup
			expectErr:    true,
			expectErrMsg: "edge instance not found",
		},
		{
			name:   "edge without stream",
			edgeID: "edge-no-stream",
			setupEdge: func() {
				server.edgeMutex.Lock()
				server.edgeConnections["edge-no-stream"] = &EdgeInstanceConnection{
					EdgeID:    "edge-no-stream",
					Namespace: "test",
					Status:    "connected",
					Stream:    nil, // No stream
				}
				server.edgeMutex.Unlock()
			},
			expectErr:    true,
			expectErrMsg: "edge instance not available for reload",
		},
		{
			name:   "edge with disconnected status",
			edgeID: "edge-disconnected",
			setupEdge: func() {
				server.edgeMutex.Lock()
				server.edgeConnections["edge-disconnected"] = &EdgeInstanceConnection{
					EdgeID:    "edge-disconnected",
					Namespace: "test",
					Status:    "disconnected", // Disconnected status
					Stream:    nil,
				}
				server.edgeMutex.Unlock()
			},
			expectErr:    true,
			expectErrMsg: "edge instance not available for reload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEdge()

			reloadReq := &pb.ConfigurationReloadRequest{
				OperationId:      "test-reload-001",
				TargetNamespace:  "test",
				InitiatedBy:      "test-user",
				TimeoutSeconds:   300,
			}

			err := server.SendReloadRequest(tt.edgeID, reloadReq)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestControlServer_SetReloadCoordinator tests setting the reload coordinator
func TestControlServer_SetReloadCoordinator(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	// Mock reload coordinator
	mockCoordinator := &struct {
		ProcessReloadResponse func(*pb.ConfigurationReloadResponse)
	}{
		ProcessReloadResponse: func(*pb.ConfigurationReloadResponse) {
			// Mock implementation
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		server.SetReloadCoordinator(mockCoordinator)
	})

	// Verify coordinator is set
	assert.NotNil(t, server.reloadCoordinator)
	assert.Equal(t, mockCoordinator, server.reloadCoordinator)
}

// TestControlServer_ThreadSafety tests thread safety of edge connection management
func TestControlServer_ThreadSafety(t *testing.T) {
	server, _ := setupTestServer(t, nil)

	numGoroutines := 50
	numOperations := 10

	var wg sync.WaitGroup

	// Concurrent edge connection operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			edgeID := "edge-" + string(rune(id%26+'A'))

			for j := 0; j < numOperations; j++ {
				// Add connection
				server.edgeMutex.Lock()
				server.edgeConnections[edgeID] = &EdgeInstanceConnection{
					EdgeID:        edgeID,
					Namespace:     "test",
					Status:        "connected",
					LastHeartbeat: time.Now(),
				}
				server.edgeMutex.Unlock()

				// Read connections
				server.edgeMutex.RLock()
				_, exists := server.edgeConnections[edgeID]
				server.edgeMutex.RUnlock()
				assert.True(t, exists)

				// Remove connection
				server.edgeMutex.Lock()
				delete(server.edgeConnections, edgeID)
				server.edgeMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify no connections remain
	server.edgeMutex.RLock()
	assert.Empty(t, server.edgeConnections)
	server.edgeMutex.RUnlock()
}