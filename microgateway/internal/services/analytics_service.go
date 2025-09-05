// internal/services/analytics_service.go
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// DatabaseAnalyticsService implements AnalyticsServiceInterface using database storage
type DatabaseAnalyticsService struct {
	db     *gorm.DB
	repo   *database.Repository
	config config.AnalyticsConfig
	buffer []database.AnalyticsEvent
	mu     sync.Mutex
}

// NewDatabaseAnalyticsService creates a new database-backed analytics service
func NewDatabaseAnalyticsService(db *gorm.DB, repo *database.Repository, cfg config.AnalyticsConfig) AnalyticsServiceInterface {
	return &DatabaseAnalyticsService{
		db:     db,
		repo:   repo,
		config: cfg,
		buffer: make([]database.AnalyticsEvent, 0, cfg.BufferSize),
	}
}

// RecordRequest records an analytics event (implements analytics.Handler interface)
func (s *DatabaseAnalyticsService) RecordRequest(ctx context.Context, record interface{}) error {
	if !s.config.Enabled {
		return nil
	}

	// Convert the record to our analytics event
	event, err := s.convertToAnalyticsEvent(record)
	if err != nil {
		return fmt.Errorf("failed to convert analytics record: %w", err)
	}

	// Add to buffer
	s.mu.Lock()
	s.buffer = append(s.buffer, *event)
	shouldFlush := len(s.buffer) >= s.config.BufferSize
	s.mu.Unlock()

	// Flush if buffer is full
	if shouldFlush {
		go s.Flush()
	}

	return nil
}

// GetEvents returns analytics events with pagination
func (s *DatabaseAnalyticsService) GetEvents(appID uint, page, limit int) ([]AnalyticsEvent, int64, error) {
	events, total, err := s.repo.GetAnalyticsEvents(appID, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get analytics events: %w", err)
	}

	// Convert to service model
	result := make([]AnalyticsEvent, len(events))
	for i, event := range events {
		result[i] = AnalyticsEvent{
			ID:             event.ID,
			RequestID:      event.RequestID,
			AppID:          event.AppID,
			LLMID:          event.LLMID,
			CredentialID:   event.CredentialID,
			Endpoint:       event.Endpoint,
			Method:         event.Method,
			StatusCode:     event.StatusCode,
			RequestTokens:  event.RequestTokens,
			ResponseTokens: event.ResponseTokens,
			TotalTokens:    event.TotalTokens,
			Cost:           event.Cost,
			LatencyMs:      event.LatencyMs,
			ErrorMessage:   event.ErrorMessage,
			CreatedAt:      event.CreatedAt,
		}
	}

	return result, total, nil
}

// GetSummary returns analytics summary for a time period
func (s *DatabaseAnalyticsService) GetSummary(appID uint, startTime, endTime time.Time) (*AnalyticsSummary, error) {
	var summary AnalyticsSummary

	// Get total requests
	err := s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ?", appID, startTime, endTime).
		Count(&summary.TotalRequests).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests: %w", err)
	}

	// Get successful requests
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ? AND status_code >= 200 AND status_code < 300", 
			appID, startTime, endTime).
		Count(&summary.SuccessfulRequests).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get successful requests: %w", err)
	}

	summary.FailedRequests = summary.TotalRequests - summary.SuccessfulRequests

	// Get total tokens and cost
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ?", appID, startTime, endTime).
		Select("COALESCE(SUM(total_tokens), 0), COALESCE(SUM(cost), 0)").
		Row().Scan(&summary.TotalTokens, &summary.TotalCost)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokens and cost: %w", err)
	}

	// Get average latency
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ?", appID, startTime, endTime).
		Select("COALESCE(AVG(latency_ms), 0)").
		Scan(&summary.AverageLatency).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get average latency: %w", err)
	}

	// Calculate requests per hour
	duration := endTime.Sub(startTime).Hours()
	if duration > 0 {
		summary.RequestsPerHour = float64(summary.TotalRequests) / duration
	}

	return &summary, nil
}

// GetCostAnalysis returns cost analysis data
func (s *DatabaseAnalyticsService) GetCostAnalysis(appID uint, startTime, endTime time.Time) (*CostAnalysis, error) {
	var analysis CostAnalysis

	// Get total cost
	err := s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ?", appID, startTime, endTime).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&analysis.TotalCost).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}

	// Get cost by LLM
	var costByLLM []struct {
		LLMName string  `json:"llm_name"`
		Cost    float64 `json:"cost"`
	}
	
	err = s.db.Raw(`
		SELECT l.name as llm_name, COALESCE(SUM(ae.cost), 0) as cost
		FROM analytics_events ae
		JOIN llms l ON ae.llm_id = l.id
		WHERE ae.app_id = ? AND ae.created_at BETWEEN ? AND ?
		GROUP BY l.id, l.name
		ORDER BY cost DESC
	`, appID, startTime, endTime).Scan(&costByLLM).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get cost by LLM: %w", err)
	}

	analysis.CostByLLM = make(map[string]float64)
	for _, item := range costByLLM {
		analysis.CostByLLM[item.LLMName] = item.Cost
	}

	// Get average cost per request
	var requestCount int64
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ? AND created_at BETWEEN ? AND ?", appID, startTime, endTime).
		Count(&requestCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get request count: %w", err)
	}

	if requestCount > 0 {
		analysis.AverageCostPerRequest = analysis.TotalCost / float64(requestCount)
	}

	return &analysis, nil
}

// Flush flushes buffered analytics data to database
func (s *DatabaseAnalyticsService) Flush() error {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return nil
	}

	// Copy buffer and reset
	events := make([]database.AnalyticsEvent, len(s.buffer))
	copy(events, s.buffer)
	s.buffer = s.buffer[:0]
	s.mu.Unlock()

	// Batch insert to database
	if err := s.repo.CreateAnalyticsEventsBatch(events); err != nil {
		log.Error().Err(err).Int("count", len(events)).Msg("Failed to flush analytics buffer")
		return err
	}

	log.Debug().Int("count", len(events)).Msg("Flushed analytics events to database")
	return nil
}

// StartBufferFlush starts the background buffer flushing routine
func (s *DatabaseAnalyticsService) StartBufferFlush(ctx context.Context) {
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit
			s.Flush()
			return
		case <-ticker.C:
			s.Flush()
		}
	}
}

// GetStats returns analytics service statistics
func (s *DatabaseAnalyticsService) GetStats() map[string]interface{} {
	s.mu.Lock()
	bufferSize := len(s.buffer)
	s.mu.Unlock()

	return map[string]interface{}{
		"buffer_size":     bufferSize,
		"max_buffer_size": s.config.BufferSize,
		"flush_interval":  s.config.FlushInterval,
		"enabled":         s.config.Enabled,
		"realtime":        s.config.EnableRealtime,
	}
}

// convertToAnalyticsEvent converts a generic record to our analytics event
func (s *DatabaseAnalyticsService) convertToAnalyticsEvent(record interface{}) (*database.AnalyticsEvent, error) {
	// This would convert from the midsommar analytics record format
	// For now, return a basic event
	event := &database.AnalyticsEvent{
		RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		AppID:     1, // Default app for now
		Endpoint:  "/api/unknown",
		Method:    "POST",
		StatusCode: 200,
		CreatedAt: time.Now(),
	}

	return event, nil
}

// CleanupOldEvents removes analytics events older than retention period
func (s *DatabaseAnalyticsService) CleanupOldEvents() error {
	cutoffDate := time.Now().AddDate(0, 0, -s.config.RetentionDays)
	
	result := s.db.Where("created_at < ?", cutoffDate).Delete(&database.AnalyticsEvent{})
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old events: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().Int64("deleted", result.RowsAffected).Msg("Cleaned up old analytics events")
	}

	return nil
}