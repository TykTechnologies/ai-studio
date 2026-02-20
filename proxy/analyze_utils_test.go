package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{})
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

type MockService struct {
	mock.Mock
	services.ServiceInterface
}

func (m *MockService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	args := m.Called(modelName, vendor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModelPrice), args.Error(1)
}

type MockTokenResponse struct {
	models.ITokenResponse
	model            string
	promptTokens     int
	responseTokens   int
	choiceCount      int
	toolCount        int
	cacheWriteTokens int
	cacheReadTokens  int
}

func (m *MockTokenResponse) GetModel() string {
	return m.model
}

func (m *MockTokenResponse) GetPromptTokens() int {
	return m.promptTokens
}

func (m *MockTokenResponse) GetResponseTokens() int {
	return m.responseTokens
}

func (m *MockTokenResponse) GetChoiceCount() int {
	return m.choiceCount
}

func (m *MockTokenResponse) GetToolCount() int {
	return m.toolCount
}

func (m *MockTokenResponse) GetCacheWritePromptTokens() int {
	return m.cacheWriteTokens
}

func (m *MockTokenResponse) GetCacheReadPromptTokens() int {
	return m.cacheReadTokens
}

func TestAnalyzeCompletionResponse_Currency(t *testing.T) {
	tests := []struct {
		name             string
		price            *models.ModelPrice
		expectedCurrency string
	}{
		{
			name: "with price model",
			price: &models.ModelPrice{
				ModelName: "test-model",
				Vendor:    "test-vendor",
				CPT:       0.001,
				CPIT:      0.002,
				Currency:  "EUR",
			},
			expectedCurrency: "EUR",
		},
		{
			name:             "without price model",
			price:            nil,
			expectedCurrency: "USD", // Default currency
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global analytics handler state before subtest
			analytics.ResetHandler()

			mockService := new(MockService)
			mockService.On("GetModelPriceByModelNameAndVendor", "test-model", "test-vendor").Return(tt.price, nil)

			response := &MockTokenResponse{
				model:            "test-model",
				promptTokens:     100,
				responseTokens:   50,
				choiceCount:      1,
				toolCount:        0,
				cacheWriteTokens: 0,
				cacheReadTokens:  0,
			}

			llm := &models.LLM{
				ID:     1,
				Vendor: "test-vendor",
			}

			app := &models.App{
				ID:     1,
				UserID: 1,
			}

			// Setup test DB and start recording
			db := setupTestDB(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			analytics.StartRecording(ctx, db)

			// Create request with model in context
			req, _ := http.NewRequest("POST", "/test", nil)
			ctx = context.WithValue(req.Context(), "model_name", "test-model")
			req = req.WithContext(ctx)

			// Run the test
			AnalyzeCompletionResponse(mockService, llm, app, response, req, time.Now())

			// Wait for and verify the record
			record, err := waitForRecord[models.LLMChatRecord](db, "currency = ?", tt.expectedCurrency)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCurrency, record.Currency)
			mockService.AssertExpectations(t)
		})
	}
}

func Test_decompressResponseBody(t *testing.T) {
	originalData := []byte("test data for compression.")
	emptyData := []byte{}

	gzipData := compressWithGzip(t, originalData)
	brotliData := compressWithBrotli(t, originalData)
	invalidGzipData := []byte("invalid gzip data")
	invalidBrotliData := []byte("invalid brotli data")

	tests := []struct {
		name            string
		data            []byte
		contentEncoding string
		want            []byte
		wantErr         bool
		errorContains   string
	}{
		{
			name:            "successful gzip decompression",
			data:            gzipData,
			contentEncoding: "gzip",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "successful brotli decompression with 'br' encoding",
			data:            brotliData,
			contentEncoding: "br",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "successful brotli decompression with 'brotli' encoding",
			data:            brotliData,
			contentEncoding: "brotli",
			want:            originalData,
			wantErr:         false,
		},

		{
			name:            "returns original data when content encoding is empty",
			data:            originalData,
			contentEncoding: "",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "returns empty data when data is empty",
			data:            emptyData,
			contentEncoding: "gzip",
			want:            emptyData,
			wantErr:         false,
		},
		{
			name:            "returns original data for unsupported encoding",
			data:            originalData,
			contentEncoding: "zstd",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "handles uppercase GZIP encoding",
			data:            gzipData,
			contentEncoding: "GZIP",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "handles mixed case GZip encoding",
			data:            gzipData,
			contentEncoding: "GZip",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "handles uppercase BROTLI encoding",
			data:            brotliData,
			contentEncoding: "BROTLI",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "handles uppercase BR encoding",
			data:            brotliData,
			contentEncoding: "BR",
			want:            originalData,
			wantErr:         false,
		},
		{
			name:            "returns error for invalid gzip data",
			data:            invalidGzipData,
			contentEncoding: "gzip",
			want:            nil,
			wantErr:         true,
			errorContains:   "failed to create gzip reader",
		},
		{
			name:            "returns error for invalid brotli data",
			data:            invalidBrotliData,
			contentEncoding: "br",
			want:            nil,
			wantErr:         true,
			errorContains:   "failed to decompress brotli data",
		},
		{
			name:            "returns error for invalid brotli data with 'brotli' encoding",
			data:            invalidBrotliData,
			contentEncoding: "brotli",
			want:            nil,
			wantErr:         true,
			errorContains:   "failed to decompress brotli data",
		},
		{
			name:            "handles nil data slice",
			data:            nil,
			contentEncoding: "gzip",
			want:            nil,
			wantErr:         false,
		},
		{
			name:            "handles large gzip data",
			data:            compressWithGzip(t, generateLargeData(10000)),
			contentEncoding: "gzip",
			want:            generateLargeData(10000),
			wantErr:         false,
		},
		{
			name:            "handles empty gzip compressed data",
			data:            compressWithGzip(t, []byte{}),
			contentEncoding: "gzip",
			want:            []byte{},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decompressResponseBody(tt.data, tt.contentEncoding)

			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "error message should contain expected text")
				}
				return
			}

			require.NoError(t, err, "unexpected error")
			assert.Equal(t, tt.want, got, "decompressed data should match expected")
		})
	}
}

func generateLargeData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}
