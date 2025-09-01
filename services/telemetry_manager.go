package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
)

const (
	// TelemetryURL is the hardcoded endpoint for sending telemetry data
	TelemetryURL = "https://telemetry.tyk.technologies"
	// TelemetryPeriod is the hardcoded interval for collecting telemetry
	TelemetryPeriod = time.Hour
)

// TelemetryManager handles the periodic collection and transmission of telemetry data
type TelemetryManager struct {
	db               *gorm.DB
	telemetryService *TelemetryService
	enabled          bool
	version          string
	ctx              context.Context
	cancel           context.CancelFunc
}

// TelemetryPayload represents the structure of data sent to the telemetry service
type TelemetryPayload struct {
	Timestamp  time.Time              `json:"timestamp"`
	InstanceID string                 `json:"instance_id"`
	Version    string                 `json:"version"`
	LLMStats   map[string]interface{} `json:"llm_stats"`
	AppStats   map[string]interface{} `json:"app_stats"`
	UserStats  map[string]interface{} `json:"user_stats"`
	ChatStats  map[string]interface{} `json:"chat_stats"`
}

// NewTelemetryManager creates a new telemetry manager
func NewTelemetryManager(db *gorm.DB, enabled bool, version string) *TelemetryManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TelemetryManager{
		db:               db,
		telemetryService: NewTelemetryService(db),
		enabled:          enabled,
		version:          version,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins the telemetry collection process
func (tm *TelemetryManager) Start() {
	if !tm.enabled {
		log.Println("Telemetry is disabled")
		return
	}

	log.Printf("Telemetry collection started - collecting usage statistics every %v", TelemetryPeriod)
	log.Printf("Telemetry data will be sent to: %s", TelemetryURL)
	log.Println("To disable telemetry, set environment variable: TELEMETRY_ENABLED=false")

	// Send initial telemetry data
	go tm.collectAndSend()

	// Start periodic collection
	ticker := time.NewTicker(TelemetryPeriod)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-tm.ctx.Done():
				log.Println("Telemetry collection stopped")
				return
			case <-ticker.C:
				tm.collectAndSend()
			}
		}
	}()
}

// Stop halts the telemetry collection process
func (tm *TelemetryManager) Stop() {
	if tm.cancel != nil {
		tm.cancel()
	}
}

// collectAndSend gathers telemetry data and sends it to the telemetry service
func (tm *TelemetryManager) collectAndSend() {
	if !tm.enabled {
		return
	}

	log.Println("Collecting telemetry data...")

	payload := TelemetryPayload{
		Timestamp:  time.Now(),
		InstanceID: tm.generateInstanceID(),
		Version:    tm.version,
	}

	// Collect LLM statistics
	llmStats, err := tm.telemetryService.GetLLMStats()
	if err != nil {
		log.Printf("Warning: Failed to collect LLM statistics: %v", err)
		llmStats = map[string]interface{}{}
	}
	payload.LLMStats = llmStats

	// Collect App statistics
	appStats, err := tm.telemetryService.GetAppStats()
	if err != nil {
		log.Printf("Warning: Failed to collect App statistics: %v", err)
		appStats = map[string]interface{}{}
	}
	payload.AppStats = appStats

	// Collect User statistics
	userStats, err := tm.telemetryService.GetUserStats()
	if err != nil {
		log.Printf("Warning: Failed to collect User statistics: %v", err)
		userStats = map[string]interface{}{}
	}
	payload.UserStats = userStats

	// Collect Chat statistics
	chatStats, err := tm.telemetryService.GetChatStats()
	if err != nil {
		log.Printf("Warning: Failed to collect Chat statistics: %v", err)
		chatStats = map[string]interface{}{}
	}
	payload.ChatStats = chatStats

	// Send telemetry data
	err = tm.sendTelemetry(payload)
	if err != nil {
		log.Printf("Warning: Failed to send telemetry data: %v", err)
	} else {
		log.Println("Telemetry data sent successfully")
	}
}

// sendTelemetry sends the collected telemetry data to the telemetry service
func (tm *TelemetryManager) sendTelemetry(payload TelemetryPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry payload: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(tm.ctx, "POST", TelemetryURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telemetry request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("Tyk-AI-Portal/%s", tm.version))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telemetry request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry service returned status code: %d", resp.StatusCode)
	}

	return nil
}

// generateInstanceID creates a consistent but anonymized instance identifier
func (tm *TelemetryManager) generateInstanceID() string {
	// Create a hash based on database connection info to ensure consistent but anonymous ID
	hasher := sha256.New()

	// Get database connection info for hashing
	sqlDB, err := tm.db.DB()
	if err == nil {
		if stats := sqlDB.Stats(); stats.OpenConnections > 0 {
			hasher.Write([]byte(fmt.Sprintf("midsommar_%d", time.Now().Unix()/86400))) // Daily rotation
		}
	}

	// Fallback to a simple daily hash
	hasher.Write([]byte(fmt.Sprintf("midsommar_instance_%s", time.Now().Format("2006-01-02"))))

	return hex.EncodeToString(hasher.Sum(nil))[:16] // Use first 16 characters
}

// IsEnabled returns whether telemetry is enabled
func (tm *TelemetryManager) IsEnabled() bool {
	return tm.enabled
}
