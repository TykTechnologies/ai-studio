package analytics

import (
	"context"
	"fmt"
	"testing"
	"time"

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

	err = db.AutoMigrate(&LLMChatRecord{}, &LLMChatLogEntry{}, &ToolCallRecord{})
	require.NoError(t, err)

	return db
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

	// Wait for goroutine to process
	time.Sleep(100 * time.Millisecond)

	var chatRecord LLMChatRecord
	result := db.First(&chatRecord)
	assert.NoError(t, result.Error)
	assert.Equal(t, "TestName", chatRecord.Name)
	assert.Equal(t, "openai", chatRecord.Vendor)
	assert.Equal(t, 30, chatRecord.TotalTokens)

	var chatLog LLMChatLogEntry
	result = db.First(&chatLog)
	assert.NoError(t, result.Error)
	assert.Equal(t, "TestName", chatLog.Name)
	assert.Equal(t, "Test prompt", chatLog.Prompt)
	assert.Equal(t, "Test content", chatLog.Response)
}

func TestRecordToolCall(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRecording(ctx, db)

	now := time.Now()
	RecordToolCall("TestTool", now, 50, 1)

	// Wait for goroutine to process
	time.Sleep(100 * time.Millisecond)

	var toolCall ToolCallRecord
	result := db.First(&toolCall)
	assert.NoError(t, result.Error)
	assert.Equal(t, "TestTool", toolCall.Name)
	assert.Equal(t, 50, toolCall.ExecTime)
	assert.Equal(t, uint(1), toolCall.ToolID)
}

func TestGetChatRecordsPerDay(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	startDate := time.Now().AddDate(0, 0, -5)
	for i := 0; i < 5; i++ {
		db.Create(&LLMChatRecord{
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
		db.Create(&ToolCallRecord{
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
		db.Create(&LLMChatRecord{
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
				CPT:       0.002, // $0.002 per token
			}, nil
		},
	}

	RecordContentMessage(mc, cr, models.OPENAI, "TestModel", "chat123", 100, 1, 1, now, mockService)

	// Wait for goroutine to process
	time.Sleep(100 * time.Millisecond)

	var chatRecord LLMChatRecord
	result := db.First(&chatRecord)
	assert.NoError(t, result.Error)

	// Check if the cost is calculated correctly
	expectedCost := 0.002 * float64(30) // CPT * TotalTokens
	assert.InDelta(t, expectedCost, chatRecord.Cost, 0.0001)
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

	// Wait for goroutine to process
	time.Sleep(100 * time.Millisecond)

	var chatRecord LLMChatRecord
	result := db.First(&chatRecord)
	assert.NoError(t, result.Error)

	// Check if the cost is zero when price is not available
	assert.Equal(t, float64(0), chatRecord.Cost)
}

// Mock service for testing
type mockService struct {
	GetModelPriceByModelNameAndVendorFunc func(modelName, vendor string) (*models.ModelPrice, error)
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
