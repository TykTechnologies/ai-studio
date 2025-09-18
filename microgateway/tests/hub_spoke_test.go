// tests/hub_spoke_test.go
package tests

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/providers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHubSpokeServiceContainer(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	tests := []struct {
		name        string
		gatewayMode string
		expectError bool
	}{
		{
			name:        "Standalone Mode",
			gatewayMode: "standalone",
			expectError: false,
		},
		{
			name:        "Control Mode",
			gatewayMode: "control",
			expectError: false,
		},
		{
			name:        "Edge Mode",
			gatewayMode: "edge",
			expectError: false,
		},
		{
			name:        "Invalid Mode",
			gatewayMode: "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test configuration
			cfg := &config.Config{
				HubSpoke: config.HubSpokeConfig{
					Mode: tt.gatewayMode,
					ControlEndpoint: "localhost:9090",
					EdgeID: "test-edge",
					EdgeNamespace: "test",
				},
				Database: config.DatabaseConfig{
					Type: "sqlite",
					DSN:  ":memory:",
				},
				Security: config.SecurityConfig{
					EncryptionKey: "12345678901234567890123456789012",
				},
			}

			// Create service container
			container, err := services.NewHubSpokeServiceContainer(db, cfg)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, container)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, container)
				assert.NotNil(t, container.ConfigProvider)
				
				// Verify provider type
				if tt.gatewayMode == "edge" {
					assert.Equal(t, providers.ProviderTypeGRPC, container.ConfigProvider.GetProviderType())
				} else {
					assert.Equal(t, providers.ProviderTypeDatabase, container.ConfigProvider.GetProviderType())
				}
			}
		})
	}
}

func TestNamespaceFiltering(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Create test LLMs with different namespaces
	globalLLM := &database.LLM{
		Name:      "Global LLM",
		Slug:      "global-llm",
		Vendor:    "openai",
		Namespace: "", // Global
		IsActive:  true,
	}
	err = db.Create(globalLLM).Error
	require.NoError(t, err)

	tenantLLM := &database.LLM{
		Name:      "Tenant A LLM",
		Slug:      "tenant-a-llm",
		Vendor:    "openai",
		Namespace: "tenant-a",
		IsActive:  true,
	}
	err = db.Create(tenantLLM).Error
	require.NoError(t, err)

	tests := []struct {
		name              string
		providerNamespace string
		expectedLLMCount  int
		expectedLLMs      []string
	}{
		{
			name:              "Global Namespace Provider",
			providerNamespace: "",
			expectedLLMCount:  1, // Only global LLM
			expectedLLMs:      []string{"global-llm"},
		},
		{
			name:              "Tenant A Namespace Provider",
			providerNamespace: "tenant-a",
			expectedLLMCount:  2, // Global + tenant A LLMs
			expectedLLMs:      []string{"global-llm", "tenant-a-llm"},
		},
		{
			name:              "Tenant B Namespace Provider",
			providerNamespace: "tenant-b",
			expectedLLMCount:  1, // Only global LLM
			expectedLLMs:      []string{"global-llm"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create database provider with specific namespace
			provider := providers.NewDatabaseProvider(db, tt.providerNamespace)

			// List LLMs
			llms, err := provider.ListLLMs("", true)
			assert.NoError(t, err)
			assert.Len(t, llms, tt.expectedLLMCount)

			// Verify specific LLMs are present
			slugs := make([]string, len(llms))
			for i, llm := range llms {
				slugs[i] = llm.Slug
			}
			
			for _, expectedSlug := range tt.expectedLLMs {
				assert.Contains(t, slugs, expectedSlug)
			}
		})
	}
}

func TestNamespaceFilterLogic(t *testing.T) {
	filter := &providers.DefaultNamespaceFilter{}

	tests := []struct {
		name              string
		objectNamespace   string
		requestNamespace  string
		expectedVisible   bool
	}{
		{
			name:             "Global object, global request",
			objectNamespace:  "",
			requestNamespace: "",
			expectedVisible:  true,
		},
		{
			name:             "Global object, tenant request", 
			objectNamespace:  "",
			requestNamespace: "tenant-a",
			expectedVisible:  true,
		},
		{
			name:             "Tenant object, global request",
			objectNamespace:  "tenant-a",
			requestNamespace: "",
			expectedVisible:  false,
		},
		{
			name:             "Tenant object, matching tenant request",
			objectNamespace:  "tenant-a",
			requestNamespace: "tenant-a",
			expectedVisible:  true,
		},
		{
			name:             "Tenant object, different tenant request",
			objectNamespace:  "tenant-a",
			requestNamespace: "tenant-b",
			expectedVisible:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.MatchesNamespace(tt.objectNamespace, tt.requestNamespace)
			assert.Equal(t, tt.expectedVisible, result)
		})
	}
}

func TestConfigurationProviderSwitching(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	// Test provider factory
	tests := []struct {
		name               string
		gatewayMode        string
		expectedProvider   providers.ProviderType
	}{
		{
			name:             "Standalone creates database provider",
			gatewayMode:      "standalone",
			expectedProvider: providers.ProviderTypeDatabase,
		},
		{
			name:             "Control creates database provider",
			gatewayMode:      "control",
			expectedProvider: providers.ProviderTypeDatabase,
		},
		{
			name:             "Edge creates gRPC provider",
			gatewayMode:      "edge",
			expectedProvider: providers.ProviderTypeGRPC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HubSpoke: config.HubSpokeConfig{
					Mode: tt.gatewayMode,
					ControlEndpoint: "localhost:9090",
					EdgeNamespace: "test",
				},
			}

			factory := providers.NewProviderFactory(cfg, db)
			provider, err := factory.CreateProvider()
			
			require.NoError(t, err)
			assert.Equal(t, tt.expectedProvider, provider.GetProviderType())
		})
	}
}