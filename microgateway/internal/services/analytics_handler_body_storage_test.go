package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	probeRequestBody  = `{"messages":[{"role":"user","content":"a confidential prompt"}]}`
	probeResponseBody = `{"choices":[{"message":{"content":"a confidential answer"}}]}`
)

// recordAndLoad sends one proxy log through the live RecordProxyLog path and
// returns the analytics event it left in the edge database. The pulse reads
// the bodies it forwards to the hub back from this row, so what is stored
// here is also what leaves the gateway.
func recordAndLoad(t *testing.T, cfg *config.AnalyticsConfig) database.AnalyticsEvent {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))

	handler := NewMicrogatewaAnalyticsHandler(db, cfg, nil, nil)
	handler.RecordProxyLog(context.Background(), &models.ProxyLog{
		AppID:        1,
		LLMID:        2,
		Vendor:       "openai",
		ResponseCode: 200,
		TimeStamp:    time.Now(),
		RequestBody:  probeRequestBody,
		ResponseBody: probeResponseBody,
	})

	var event database.AnalyticsEvent
	require.NoError(t, db.First(&event).Error)
	return event
}

// ANALYTICS_STORE_REQUESTS / ANALYTICS_STORE_RESPONSES default to false. The
// live path used to check only ANALYTICS_MAX_BODY_SIZE, so every edge stored
// (and, with include_request_response_data, pulsed) up to 4 KB of each body.
func TestRecordProxyLog_BodyStorageFlags(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.AnalyticsConfig
		wantRequest  string
		wantResponse string
	}{
		{
			name:         "both off (default)",
			cfg:          &config.AnalyticsConfig{MaxBodySize: 4096},
			wantRequest:  "",
			wantResponse: "",
		},
		{
			name:         "requests only",
			cfg:          &config.AnalyticsConfig{StoreRequestBodies: true, MaxBodySize: 4096},
			wantRequest:  probeRequestBody,
			wantResponse: "",
		},
		{
			name:         "responses only",
			cfg:          &config.AnalyticsConfig{StoreResponseBodies: true, MaxBodySize: 4096},
			wantRequest:  "",
			wantResponse: probeResponseBody,
		},
		{
			name:         "both on",
			cfg:          &config.AnalyticsConfig{StoreRequestBodies: true, StoreResponseBodies: true, MaxBodySize: 4096},
			wantRequest:  probeRequestBody,
			wantResponse: probeResponseBody,
		},
		{
			name:         "both on but max body size zero",
			cfg:          &config.AnalyticsConfig{StoreRequestBodies: true, StoreResponseBodies: true, MaxBodySize: 0},
			wantRequest:  "",
			wantResponse: "",
		},
		{
			name:         "no config stores nothing",
			cfg:          nil,
			wantRequest:  "",
			wantResponse: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := recordAndLoad(t, tt.cfg)
			assert.Equal(t, tt.wantRequest, event.RequestBody)
			assert.Equal(t, tt.wantResponse, event.ResponseBody)
		})
	}
}

func TestRecordProxyLog_StoredBodiesAreTruncated(t *testing.T) {
	event := recordAndLoad(t, &config.AnalyticsConfig{
		StoreRequestBodies:  true,
		StoreResponseBodies: true,
		MaxBodySize:         10,
	})

	assert.Equal(t, probeRequestBody[:10]+"... [truncated]", event.RequestBody)
	assert.Equal(t, probeResponseBody[:10]+"... [truncated]", event.ResponseBody)
	assert.False(t, strings.Contains(event.RequestBody, "confidential"))
}
