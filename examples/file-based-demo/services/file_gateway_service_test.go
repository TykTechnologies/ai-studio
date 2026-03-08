package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// setupTestConfig creates a temporary directory with test configuration files
func setupTestConfig(t *testing.T) string {
	tempDir := t.TempDir()

	// Create test LLM config
	llmConfig := `{
		"llms": [
			{
				"id": 1,
				"name": "Test GPT",
				"slug": "test-gpt",
				"vendor": "openai",
				"endpoint": "https://api.openai.com/v1/chat/completions",
				"api_key": "test-key-123",
				"model": "gpt-3.5-turbo",
				"active": true,
				"max_tokens": 4096,
				"monthly_budget": 100.0
			},
			{
				"id": 2,
				"name": "Inactive LLM",
				"slug": "inactive",
				"vendor": "openai",
				"endpoint": "https://api.openai.com/v1/chat/completions",
				"api_key": "inactive-key",
				"model": "gpt-4",
				"active": false,
				"max_tokens": 8192,
				"monthly_budget": 50.0
			}
		]
	}`

	// Create test credentials config
	credConfig := `{
		"credentials": [
			{
				"id": 1,
				"name": "Test Credential",
				"secret": "test-secret-123",
				"active": true,
				"description": "Test credential for testing"
			},
			{
				"id": 2,
				"name": "Inactive Credential",
				"secret": "inactive-secret",
				"active": false,
				"description": "Inactive test credential"
			}
		]
	}`

	// Create test apps config
	appConfig := `{
		"apps": [
			{
				"id": 1,
				"name": "Test App",
				"description": "Test application",
				"user_id": 1,
				"credential_id": 1,
				"llm_ids": [1],
				"datasource_ids": [],
				"tool_ids": [],
				"monthly_budget": 50.0,
				"budget_start_date": "2025-01-01T00:00:00Z"
			}
		]
	}`

	// Create test pricing config
	pricingConfig := `{
		"model_prices": [
			{
				"id": 1,
				"model": "gpt-3.5-turbo",
				"vendor": "openai",
				"prompt_price": 0.0015,
				"response_price": 0.002,
				"currency": "USD",
				"per_tokens": 1000
			}
		]
	}`

	// Write config files
	if err := os.WriteFile(filepath.Join(tempDir, "llms.json"), []byte(llmConfig), 0644); err != nil {
		t.Fatalf("Failed to write llms.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "credentials.json"), []byte(credConfig), 0644); err != nil {
		t.Fatalf("Failed to write credentials.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "apps.json"), []byte(appConfig), 0644); err != nil {
		t.Fatalf("Failed to write apps.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pricing.json"), []byte(pricingConfig), 0644); err != nil {
		t.Fatalf("Failed to write pricing.json: %v", err)
	}

	return tempDir
}

func TestNewFileGatewayService(t *testing.T) {
	configDir := setupTestConfig(t)

	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create FileGatewayService: %v", err)
	}

	if service == nil {
		t.Fatal("Expected service to be non-nil")
	}

	if service.configDir != configDir {
		t.Errorf("Expected configDir %s, got %s", configDir, service.configDir)
	}
}

func TestNewFileGatewayService_InvalidDir(t *testing.T) {
	_, err := NewFileGatewayService("/nonexistent/directory")
	if err == nil {
		t.Fatal("Expected error for non-existent directory")
	}
}

func TestGetActiveLLMs(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	llms, err := service.GetActiveLLMs(context.Background())
	if err != nil {
		t.Fatalf("Failed to get active LLMs: %v", err)
	}

	if len(llms) != 1 {
		t.Errorf("Expected 1 active LLM, got %d", len(llms))
	}

	if llms[0].Name != "Test GPT" {
		t.Errorf("Expected LLM name 'Test GPT', got '%s'", llms[0].Name)
	}

	if llms[0].Vendor != models.OPENAI {
		t.Errorf("Expected vendor 'openai', got '%s'", llms[0].Vendor)
	}

	if llms[0].APIKey != "test-key-123" {
		t.Errorf("Expected API key 'test-key-123', got '%s'", llms[0].APIKey)
	}
}

func TestGetActiveDatasources(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	datasources, err := service.GetActiveDatasources()
	if err != nil {
		t.Fatalf("Failed to get active datasources: %v", err)
	}

	if len(datasources) != 0 {
		t.Errorf("Expected 0 datasources, got %d", len(datasources))
	}
}

func TestGetCredentialBySecret(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test valid active credential
	cred, err := service.GetCredentialBySecret("test-secret-123")
	if err != nil {
		t.Fatalf("Failed to get credential: %v", err)
	}

	if cred.KeyID != "Test Credential" {
		t.Errorf("Expected credential name 'Test Credential', got '%s'", cred.KeyID)
	}

	// Test inactive credential should fail
	_, err = service.GetCredentialBySecret("inactive-secret")
	if err == nil {
		t.Fatal("Expected error for inactive credential")
	}

	// Test non-existent credential
	_, err = service.GetCredentialBySecret("non-existent")
	if err == nil {
		t.Fatal("Expected error for non-existent credential")
	}
}

func TestGetAppByCredentialID(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test valid credential ID
	app, err := service.GetAppByCredentialID(1)
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if app.Name != "Test App" {
		t.Errorf("Expected app name 'Test App', got '%s'", app.Name)
	}

	if app.CredentialID != 1 {
		t.Errorf("Expected credential ID 1, got %d", app.CredentialID)
	}

	// Test non-existent credential ID
	_, err = service.GetAppByCredentialID(999)
	if err == nil {
		t.Fatal("Expected error for non-existent credential ID")
	}
}

func TestGetModelPriceByModelNameAndVendor(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test valid model and vendor
	price, err := service.GetModelPriceByModelNameAndVendor("gpt-3.5-turbo", "openai")
	if err != nil {
		t.Fatalf("Failed to get model price: %v", err)
	}

	if price.ModelName != "gpt-3.5-turbo" {
		t.Errorf("Expected model name 'gpt-3.5-turbo', got '%s'", price.ModelName)
	}

	if price.CPT != 0.0015 {
		t.Errorf("Expected CPT 0.0015, got %f", price.CPT)
	}

	// Test non-existent model
	_, err = service.GetModelPriceByModelNameAndVendor("non-existent", "openai")
	if err == nil {
		t.Fatal("Expected error for non-existent model")
	}
}

func TestGetToolBySlug(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Tools are not implemented in file-based demo
	_, err = service.GetToolBySlug(context.Background(),"any-slug")
	if err == nil {
		t.Fatal("Expected error for tool operations")
	}
}

func TestCallToolOperation(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Tool operations are not supported in file-based demo
	_, err = service.CallToolOperation(1, "test", nil, nil, nil)
	if err == nil {
		t.Fatal("Expected error for tool operations")
	}
}

func TestGetDB(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	db := service.GetDB()
	if db != nil {
		t.Error("Expected GetDB to return nil for file-based service")
	}
}

func TestGetUserByID(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	user, err := service.GetUserByID(123)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if user.ID != 123 {
		t.Errorf("Expected user ID 123, got %d", user.ID)
	}

	expectedEmail := "user123@example.com"
	if user.Email != expectedEmail {
		t.Errorf("Expected email '%s', got '%s'", expectedEmail, user.Email)
	}
}

func TestReload(t *testing.T) {
	configDir := setupTestConfig(t)
	service, err := NewFileGatewayService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test reload
	err = service.Reload()
	if err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	// Verify data is still loaded correctly
	llms, err := service.GetActiveLLMs(context.Background())
	if err != nil {
		t.Fatalf("Failed to get LLMs after reload: %v", err)
	}

	if len(llms) != 1 {
		t.Errorf("Expected 1 LLM after reload, got %d", len(llms))
	}
}

func TestEnvironmentVariableResolution(t *testing.T) {
	tempDir := t.TempDir()

	// Set an environment variable for testing
	os.Setenv("TEST_API_KEY", "resolved-key-value")
	defer os.Unsetenv("TEST_API_KEY")

	// Create LLM config with environment variable
	llmConfig := `{
		"llms": [
			{
				"id": 1,
				"name": "Env Test LLM",
				"slug": "env-test",
				"vendor": "openai",
				"endpoint": "https://api.openai.com/v1/chat/completions",
				"api_key": "$TEST_API_KEY",
				"model": "gpt-3.5-turbo",
				"active": true,
				"max_tokens": 4096,
				"monthly_budget": 100.0
			}
		]
	}`

	// Write minimal config files
	if err := os.WriteFile(filepath.Join(tempDir, "llms.json"), []byte(llmConfig), 0644); err != nil {
		t.Fatalf("Failed to write llms.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "credentials.json"), []byte(`{"credentials": []}`), 0644); err != nil {
		t.Fatalf("Failed to write credentials.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "apps.json"), []byte(`{"apps": []}`), 0644); err != nil {
		t.Fatalf("Failed to write apps.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pricing.json"), []byte(`{"model_prices": []}`), 0644); err != nil {
		t.Fatalf("Failed to write pricing.json: %v", err)
	}

	service, err := NewFileGatewayService(tempDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	llms, err := service.GetActiveLLMs(context.Background())
	if err != nil {
		t.Fatalf("Failed to get LLMs: %v", err)
	}

	if len(llms) != 1 {
		t.Fatalf("Expected 1 LLM, got %d", len(llms))
	}

	if llms[0].APIKey != "resolved-key-value" {
		t.Errorf("Expected API key 'resolved-key-value', got '%s'", llms[0].APIKey)
	}
}

func TestLoadAppsWithBudgetDate(t *testing.T) {
	tempDir := t.TempDir()

	testTime := "2025-06-15T10:30:00Z"

	// Create app config with proper budget start date
	appConfig := `{
		"apps": [
			{
				"id": 1,
				"name": "Date Test App",
				"description": "App with budget date",
				"user_id": 1,
				"credential_id": 1,
				"llm_ids": [],
				"datasource_ids": [],
				"tool_ids": [],
				"monthly_budget": 100.0,
				"budget_start_date": "` + testTime + `"
			}
		]
	}`

	// Write minimal config files
	if err := os.WriteFile(filepath.Join(tempDir, "llms.json"), []byte(`{"llms": []}`), 0644); err != nil {
		t.Fatalf("Failed to write llms.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "credentials.json"), []byte(`{"credentials": []}`), 0644); err != nil {
		t.Fatalf("Failed to write credentials.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "apps.json"), []byte(appConfig), 0644); err != nil {
		t.Fatalf("Failed to write apps.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pricing.json"), []byte(`{"model_prices": []}`), 0644); err != nil {
		t.Fatalf("Failed to write pricing.json: %v", err)
	}

	service, err := NewFileGatewayService(tempDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Access the apps directly to test date parsing
	if len(service.apps) != 1 {
		t.Fatalf("Expected 1 app, got %d", len(service.apps))
	}

	app := service.apps[0]
	if app.BudgetStartDate == nil {
		t.Fatal("Expected budget start date to be parsed")
	}

	expectedTime, _ := time.Parse(time.RFC3339, testTime)
	if !app.BudgetStartDate.Equal(expectedTime) {
		t.Errorf("Expected budget start date %v, got %v", expectedTime, *app.BudgetStartDate)
	}
}
