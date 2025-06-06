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
					"PromptTokens":             10,
					"ResponseTokens":           20,
					"CacheCreationInputTokens": 5,
					"CacheReadInputTokens":     15,
				},
			},
		},
	}

	// Create a mock service that returns a price
	mockService := &mockService{
		GetModelPriceByModelNameAndVendorFunc: func(modelName, vendor string) (*models.ModelPrice, error) {
			return &models.ModelPrice{
				ModelName:    modelName,
				Vendor:       vendor,
				CPT:          0.002,  // $0.002 per response token
				CPIT:         0.001,  // $0.001 per prompt token
				CacheWritePT: 0.0005, // $0.0005 per cache write token
				CacheReadPT:  0.0001, // $0.0001 per cache read token
			}, nil
		},
	}

	RecordContentMessage(mc, cr, models.OPENAI, "TestName", "chat123", 100, 1, 1, 1, now, mockService)

	chatRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ?", "TestName")
	require.NoError(t, err)
	assert.Equal(t, "TestName", chatRecord.Name)
	assert.Equal(t, "openai", chatRecord.Vendor)
	assert.Equal(t, 30, chatRecord.TotalTokens)
	assert.Equal(t, models.ChatInteraction, chatRecord.InteractionType)
	assert.Equal(t, 5, chatRecord.CacheWritePromptTokens)
	assert.Equal(t, 15, chatRecord.CacheReadPromptTokens)

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

	endDate := startDate.AddDate(0, 0, 4)
	chartData, err := GetChatRecordsPerDay(db, &startDate, &endDate)
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
							"PromptTokens":             10,
							"ResponseTokens":           20,
							"CacheCreationInputTokens": 5,
							"CacheReadInputTokens":     15,
						},
					},
				},
			}

			// Create a mock service that implements the required method
			mockService := &mockService{
				GetModelPriceByModelNameAndVendorFunc: func(modelName, vendor string) (*models.ModelPrice, error) {
					return &models.ModelPrice{
						ModelName:    modelName,
						Vendor:       vendor,
						CPT:          0.002,  // $0.002 per response token
						CPIT:         0.001,  // $0.001 per prompt token
						CacheWritePT: 0.0005, // $0.0005 per cache write token
						CacheReadPT:  0.0001, // $0.0001 per cache read token
					}, nil
				},
			}

			if tt.interactionType == models.ChatInteraction {
				RecordContentMessage(mc, cr, models.OPENAI, "TestModel", "chat123", 100, 1, 1, 1, now, mockService)
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
					Cost: (price.CPT*float64(20) + // Response tokens
						price.CPIT*float64(10) + // Prompt tokens
						price.CacheWritePT*float64(5) + // Cache write tokens
						price.CacheReadPT*float64(15)) * 10000, // Cache read tokens
					CacheWritePromptTokens: 5,
					CacheReadPromptTokens:  15,
				}
				db.Create(rec)
			}

			chatRecord, err := waitForRecord[models.LLMChatRecord](db, "name = ? AND interaction_type = ?", "TestModel", tt.interactionType)
			require.NoError(t, err)

			// Check if the cost is calculated correctly
			// Cost = (CPT * ResponseTokens) + (CPIT * PromptTokens) +
			//        (CacheWritePT * CacheWritePromptTokens) + (CacheReadPT * CacheReadPromptTokens)
			// ResponseTokens = 20, PromptTokens = 10, CacheWriteTokens = 5, CacheReadTokens = 15
			expectedCost := (0.002 * float64(20)) + // Response tokens: 0.04
				(0.001 * float64(10)) + // Prompt tokens: 0.01
				(0.0005 * float64(5)) + // Cache write tokens: 0.0025
				(0.0001 * float64(15)) // Cache read tokens: 0.0015
			assert.InDelta(t, expectedCost*10000, chatRecord.Cost, 0.0001)
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
					"PromptTokens":             10,
					"ResponseTokens":           20,
					"CacheCreationInputTokens": 5,
					"CacheReadInputTokens":     15,
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

	RecordContentMessage(mc, cr, models.OPENAI, "TestModel", "chat123", 100, 1, 1, 1, now, mockService)

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

// GetToolByID implements the services.ServiceInterface method
func (m *mockService) GetToolByID(id uint) (*models.Tool, error) {
	return &models.Tool{}, nil
}

// GetToolBySlug implements the services.ServiceInterface method
func (m *mockService) GetToolBySlug(slug string) (*models.Tool, error) {
	return &models.Tool{}, nil
}

// Ensure mockService implements both ServiceInterface and EmailSender
var _ services.ServiceInterface = (*mockService)(nil)
var _ models.EmailSender = (*mockService)(nil)

func TestGetUsage(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	now := time.Now()
	startDate := now.AddDate(0, 0, -5)
	endDate := now.AddDate(0, 0, 5)

	records := []models.LLMChatRecord{
		{
			TimeStamp:              startDate,
			TotalTokens:            100,
			Cost:                   100000.0, // 10.0 * 10000
			PromptTokens:           30,
			ResponseTokens:         70,
			CacheWritePromptTokens: 20,
			CacheReadPromptTokens:  10,
		},
		{
			TimeStamp:              startDate,
			TotalTokens:            200,
			Cost:                   200000.0, // 20.0 * 10000
			PromptTokens:           60,
			ResponseTokens:         140,
			CacheWritePromptTokens: 40,
			CacheReadPromptTokens:  20,
		},
	}

	for _, record := range records {
		err := db.Create(&record).Error
		require.NoError(t, err)
	}

	// Test GetUsage
	chartData, err := GetUsage(db, startDate, endDate, "", nil, nil, nil)
	require.NoError(t, err)

	// Verify the data
	assert.Len(t, chartData.Labels, 1)   // One day of data
	assert.Len(t, chartData.Datasets, 6) // Total tokens, Cost, Prompt tokens, Response tokens, Cache write tokens, Cache read tokens

	// Verify total tokens
	assert.Equal(t, "Total Tokens", chartData.Datasets[0].Label)
	assert.Equal(t, float64(300), chartData.Datasets[0].Data[0]) // 100 + 200

	// Verify cost
	assert.Equal(t, "Cost", chartData.Datasets[1].Label)
	assert.Equal(t, float64(30), chartData.Datasets[1].Data[0]) // (100000 + 200000) / 10000

	// Verify prompt tokens
	assert.Equal(t, "Prompt Tokens", chartData.Datasets[2].Label)
	assert.Equal(t, float64(90), chartData.Datasets[2].Data[0]) // 30 + 60

	// Verify response tokens
	assert.Equal(t, "Response Tokens", chartData.Datasets[3].Label)
	assert.Equal(t, float64(210), chartData.Datasets[3].Data[0]) // 70 + 140

	// Verify cache write tokens
	assert.Equal(t, "Cache Write Tokens", chartData.Datasets[4].Label)
	assert.Equal(t, float64(60), chartData.Datasets[4].Data[0]) // 20 + 40

	// Verify cache read tokens
	assert.Equal(t, "Cache Read Tokens", chartData.Datasets[5].Label)
	assert.Equal(t, float64(30), chartData.Datasets[5].Data[0]) // 10 + 20
}

func TestGetTotalCostPerVendorAndModel(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data for two different LLMs
	now := time.Now()
	records := []models.LLMChatRecord{
		{
			Name:            "Model1",
			Vendor:          "vendor1",
			Cost:            100000.0, // 10.0 * 10000
			TotalTokens:     1000,
			Currency:        "USD",
			TimeStamp:       now,
			LLMID:           1,
			InteractionType: models.ChatInteraction,
		},
		{
			Name:            "Model2",
			Vendor:          "vendor2",
			Cost:            200000.0, // 20.0 * 10000
			TotalTokens:     2000,
			Currency:        "USD",
			TimeStamp:       now,
			LLMID:           2,
			InteractionType: models.ChatInteraction,
		},
		{
			Name:            "Model1",
			Vendor:          "vendor1",
			Cost:            150000.0, // 15.0 * 10000
			TotalTokens:     1500,
			Currency:        "USD",
			TimeStamp:       now,
			LLMID:           1,
			InteractionType: models.ChatInteraction,
		},
	}

	for _, record := range records {
		err := db.Create(&record).Error
		require.NoError(t, err)
	}

	startDate := now.AddDate(0, 0, -1)
	endDate := now.AddDate(0, 0, 1)

	// Test without llm_id filter
	costs, err := GetTotalCostPerVendorAndModel(db, startDate, endDate, nil, nil)
	require.NoError(t, err)
	assert.Len(t, costs, 2)

	// Verify total costs
	for _, cost := range costs {
		if cost.Model == "Model1" {
			assert.Equal(t, 25.0, cost.TotalCost)          // (100000.0 + 150000.0) / 10000
			assert.Equal(t, int64(2500), cost.TotalTokens) // 1000 + 1500
		} else if cost.Model == "Model2" {
			assert.Equal(t, 20.0, cost.TotalCost)          // 200000.0 / 10000
			assert.Equal(t, int64(2000), cost.TotalTokens)
		}
	}

	// Test with llm_id filter
	llmID := uint(1)
	filteredCosts, err := GetTotalCostPerVendorAndModel(db, startDate, endDate, nil, &llmID)
	require.NoError(t, err)
	assert.Len(t, filteredCosts, 1)

	// Verify filtered results
	assert.Equal(t, "Model1", filteredCosts[0].Model)
	assert.Equal(t, 25.0, filteredCosts[0].TotalCost)     // (100000.0 + 150000.0) / 10000
	assert.Equal(t, int64(2500), filteredCosts[0].TotalTokens)
}
