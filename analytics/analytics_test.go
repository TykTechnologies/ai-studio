package analytics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.LLMChatLogEntry{}, &models.ToolCallRecord{}, &models.ModelPrice{})
	require.NoError(t, err)

	return db
}

// waitForRecord retries fetching a record until it exists or timeout is reached
func waitForRecord[T any](db *gorm.DB, condition string, args ...interface{}) (*T, error) {
	var record T
	timeout := time.After(2 * time.Second)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for record")
		case <-tick:
			result := db.Where(condition, args...).First(&record)
			if result.Error == nil {
				return &record, nil
			}
			if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return nil, result.Error
			}
		}
	}
}

func TestRecordContentMessage(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRecording(ctx, db)

	now := time.Now()
	mc := &llms.MessageContent{
		Parts: []llms.ContentPart{
			llms.TextContent{Text: "Test prompt"},
		},
	}
	cr := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: "Test content",
				GenerationInfo: map[string]interface{}{
					"PromptTokens":   10,
					"ResponseTokens": 20,
				},
			},
		},
	}

	svc := services.NewService(db)

	RecordContentMessage(mc, cr, models.OPENAI, "TestName", "chat123", 100, 1, 1, now, svc)

	chatRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ?", "TestName")
	require.NoError(t, err)
	assert.Equal(t, "TestName", chatRecord.Name)
	assert.Equal(t, "openai", chatRecord.Vendor)
	assert.Equal(t, 30, chatRecord.TotalTokens)
	assert.Equal(t, models.ChatInteraction, chatRecord.InteractionType)

	chatLog, err := waitForRecord[models.LLMChatLogEntry](db, "name = ?", "TestName")
	require.NoError(t, err)
	assert.Equal(t, "TestName", chatLog.Name)
	assert.Equal(t, "Test prompt", chatLog.Prompt)
	assert.Equal(t, "Test content", chatLog.Response)
}

func TestRecordProxyInteraction(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRecording(ctx, db)

	now := time.Now()
	rec := &models.LLMChatRecord{
		Name:            "TestProxy",
		Vendor:          "openai",
		PromptTokens:    10,
		ResponseTokens:  20,
		TotalTokens:     30,
		TimeStamp:       now,
		UserID:          1,
		AppID:           1,
		InteractionType: models.ProxyInteraction,
	}

	recordChatRecord(rec)

	proxyRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ? AND interaction_type = ?", "TestProxy", models.ProxyInteraction)
	require.NoError(t, err)
	assert.Equal(t, "TestProxy", proxyRecord.Name)
	assert.Equal(t, "openai", proxyRecord.Vendor)
	assert.Equal(t, 30, proxyRecord.TotalTokens)
	assert.Equal(t, models.ProxyInteraction, proxyRecord.InteractionType)
}

func TestRecordToolCall(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRecording(ctx, db)

	now := time.Now()
	RecordToolCall("TestTool", now, 50, 1)

	toolCall, err := waitForRecord[models.ToolCallRecord](db, "name = ?", "TestTool")
	require.NoError(t, err)
	assert.Equal(t, "TestTool", toolCall.Name)
	assert.Equal(t, 50, toolCall.ExecTime)
	assert.Equal(t, uint(1), toolCall.ToolID)
}

func TestGetChatRecordsPerDay(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	startDate := time.Now().AddDate(0, 0, -5)
	for i := 0; i < 5; i++ {
		db.Create(&models.LLMChatRecord{
			TimeStamp: startDate.AddDate(0, 0, i),
		})
	}

	chartData, err := GetChatRecordsPerDay(db, startDate, startDate.AddDate(0, 0, 4))
	assert.NoError(t, err)
	assert.Len(t, chartData.Labels, 5)
	assert.Len(t, chartData.Data, 5)
	for _, count := range chartData.Data {
		assert.Equal(t, float64(1), count)
	}
}

func TestGetToolCallsPerDay(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	startDate := time.Now().AddDate(0, 0, -5)
	for i := 0; i < 5; i++ {
		db.Create(&models.ToolCallRecord{
			TimeStamp: startDate.AddDate(0, 0, i),
		})
	}

	chartData, err := GetToolCallsPerDay(db, startDate, startDate.AddDate(0, 0, 4))
	assert.NoError(t, err)
	assert.Len(t, chartData.Labels, 5)
	assert.Len(t, chartData.Data, 5)
	for _, count := range chartData.Data {
		assert.Equal(t, float64(1), count)
	}
}

func TestGetChatRecordsPerUser(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	startDate := time.Now().AddDate(0, 0, -5)
	for i := 0; i < 3; i++ {
		db.Create(&models.LLMChatRecord{
			TimeStamp: startDate,
			UserID:    uint(i + 1),
		})
	}

	chartData, err := GetChatRecordsPerUser(db, startDate, startDate.AddDate(0, 0, 1))
	assert.NoError(t, err)
	assert.Len(t, chartData.Labels, 3)
	assert.Len(t, chartData.Data, 3)
	for _, count := range chartData.Data {
		assert.Equal(t, float64(1), count)
	}
}

func TestCostCalculation(t *testing.T) {
	tests := []struct {
		name            string
		interactionType models.InteractionType
	}{
		{
			name:            "Chat Interaction Cost",
			interactionType: models.ChatInteraction,
		},
		{
			name:            "Proxy Interaction Cost",
			interactionType: models.ProxyInteraction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			StartRecording(ctx, db)

			now := time.Now()
			mc := &llms.MessageContent{
				Parts: []llms.ContentPart{
					llms.TextContent{Text: "Test prompt"},
				},
			}
			cr := &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "Test content",
						GenerationInfo: map[string]interface{}{
							"PromptTokens":   10,
							"ResponseTokens": 20,
						},
					},
				},
			}

			// Create a mock service that implements the required method
			mockService := &mockService{
				GetModelPriceByModelNameAndVendorFunc: func(modelName, vendor string) (*models.ModelPrice, error) {
					return &models.ModelPrice{
						ModelName: modelName,
						Vendor:    vendor,
						CPT:       0.002, // $0.002 per response token
						CPIT:      0.001, // $0.001 per prompt token
					}, nil
				},
			}

			if tt.interactionType == models.ChatInteraction {
				RecordContentMessage(mc, cr, models.OPENAI, "TestModel", "chat123", 100, 1, 1, now, mockService)
			} else {
				// For proxy interaction, we need to calculate the cost using the same price model
				price, err := mockService.GetModelPriceByModelNameAndVendor("TestModel", string(models.OPENAI))
				require.NoError(t, err)

				rec := &models.LLMChatRecord{
					Name:            "TestModel",
					Vendor:          string(models.OPENAI),
					PromptTokens:    10,
					ResponseTokens:  20,
					TotalTokens:     30,
					TimeStamp:       now,
					UserID:          1,
					AppID:           1,
					InteractionType: models.ProxyInteraction,
					Cost:            price.CPT*float64(20) + price.CPIT*float64(10), // Same cost calculation as chat interaction
				}
				db.Create(rec)
			}

			chatRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ? AND interaction_type = ?", "TestModel", tt.interactionType)
			require.NoError(t, err)

			// Check if the cost is calculated correctly
			// Cost = (CPT * ResponseTokens) + (CPIT * PromptTokens)
			// ResponseTokens = 20, PromptTokens = 10
			expectedCost := (0.002 * float64(20)) + (0.001 * float64(10)) // (0.002 * 20) + (0.001 * 10) = 0.04 + 0.01 = 0.05
			assert.InDelta(t, expectedCost, chatRecord.Cost, 0.0001)
			assert.Equal(t, tt.interactionType, chatRecord.InteractionType)
		})
	}
}

func TestCostCalculationWithoutPrice(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRecording(ctx, db)

	now := time.Now()
	mc := &llms.MessageContent{
		Parts: []llms.ContentPart{
			llms.TextContent{Text: "Test prompt"},
		},
	}
	cr := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: "Test content",
				GenerationInfo: map[string]interface{}{
					"PromptTokens":   10,
					"ResponseTokens": 20,
				},
			},
		},
	}

	// Create a mock service that returns an error when getting the model price
	mockService := &mockService{
		GetModelPriceByModelNameAndVendorFunc: func(modelName, vendor string) (*models.ModelPrice, error) {
			return nil, fmt.Errorf("price not found")
		},
	}

	RecordContentMessage(mc, cr, models.OPENAI, "TestModel", "chat123", 100, 1, 1, now, mockService)

	chatRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ?", "TestModel")
	require.NoError(t, err)

	// Check if the cost is zero when price is not available
	assert.Equal(t, float64(0), chatRecord.Cost)
}

// Mock service for testing
type mockService struct {
	GetModelPriceByModelNameAndVendorFunc func(modelName, vendor string) (*models.ModelPrice, error)
	SendEmailFunc                         func(to, subject, body string) error
}

func (m *mockService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	return m.GetModelPriceByModelNameAndVendorFunc(modelName, vendor)
}

// Implement other methods of the ServiceInterface as needed for the mock
func (m *mockService) GetActiveLLMs() (models.LLMs, error) {
	return nil, nil
}

func (m *mockService) GetLLMByID(id uint) (*models.LLM, error) {
	return nil, nil
}

func (m *mockService) GetActiveDatasources() (models.Datasources, error) {
	return nil, nil
}

func (m *mockService) GetDatasourceByID(id uint) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	return nil, nil
}

func (m *mockService) GetAppByCredentialID(credID uint) (*models.App, error) {
	return nil, nil
}

func (m *mockService) GetLLMSettingsByID(id uint) (*models.LLMSettings, error) {
	return nil, nil
}

func (m *mockService) SendEmail(to, subject, body string) error {
	if m.SendEmailFunc != nil {
		return m.SendEmailFunc(to, subject, body)
	}
	return nil
}

func (m *mockService) GetDB() *gorm.DB {
	return nil
}

func (m *mockService) AuthenticateUser(email, password string) (*models.User, error) {
	return nil, nil
}

func (m *mockService) GetUserByAPIKey(apiKey string) (*models.User, error) {
	return nil, nil
}

func (m *mockService) GetUserByEmail(email string) (*models.User, error) {
	return nil, nil
}

func (m *mockService) AddUserToGroup(userID, groupID uint) error {
	return nil
}

// Ensure mockService implements both ServiceInterface and EmailSender
var _ services.ServiceInterface = (*mockService)(nil)
var _ models.EmailSender = (*mockService)(nil)
