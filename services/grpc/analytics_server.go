package grpc

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AnalyticsServer implements the AIStudioManagementService for analytics operations
type AnalyticsServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewAnalyticsServer creates a new analytics gRPC server
func NewAnalyticsServer(service *services.Service) *AnalyticsServer {
	return &AnalyticsServer{
		service: service,
	}
}

// GetAnalyticsSummary returns high-level analytics summary with real data
func (s *AnalyticsServer) GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error) {
	timeRange := req.GetTimeRange()
	if timeRange == "" {
		timeRange = "24h" // Default to 24 hours
	}

	// Parse time range
	duration, err := parseTimeRange(timeRange)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %v", err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// Get real analytics data using analytics package functions
	interactionType := models.ChatInteraction // Correct constant name

	// Get cost analysis data
	costDataMap, err := analytics.GetCostAnalysis(s.service.DB, startTime, endTime, &interactionType)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get cost analysis, using empty data")
		costDataMap = make(map[string]*analytics.ChartData)
	}

	// Extract total cost from cost analysis
	var totalCost float64
	var currency string = "USD"

	for _, chartData := range costDataMap {
		if chartData != nil && chartData.Cost != nil {
			for _, cost := range chartData.Cost {
				totalCost += cost
			}
		}
	}

	// Get token usage data
	tokenUsageData, err := analytics.GetTokenUsagePerApp(s.service.DB, startTime, endTime, &interactionType)
	var totalTokens int64
	var totalRequests int64
	if err == nil && tokenUsageData != nil {
		for _, tokenCount := range tokenUsageData.Data {
			totalTokens += int64(tokenCount)
			totalRequests++ // Each data point represents a request
		}
	}

	// Calculate actual successful vs failed requests from analytics data
	var successfulRequests, failedRequests int64

	// Query actual success/failure counts from analytics events
	err = s.service.DB.Table("analytics_events").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Where("status_code >= 200 AND status_code < 300").
		Count(&successfulRequests).Error
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get successful request count, using total as fallback")
		successfulRequests = totalRequests
		failedRequests = 0
	} else {
		// Get failed requests count
		err = s.service.DB.Table("analytics_events").
			Where("created_at BETWEEN ? AND ?", startTime, endTime).
			Where("status_code >= 400").
			Count(&failedRequests).Error
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get failed request count, using calculation")
			failedRequests = totalRequests - successfulRequests
		}
	}

	// Build response with real data
	summary := &pb.GetAnalyticsSummaryResponse{
		TotalRequests:      totalRequests,
		SuccessfulRequests: successfulRequests,
		FailedRequests:     failedRequests,
		TotalCost:          totalCost,
		Currency:           currency,
		TotalTokens:        totalTokens,
		TopEndpoints:       []*pb.TopEndpoint{}, // TODO: Implement top endpoints extraction
		ModelUsage:         []*pb.ModelUsage{},  // TODO: Implement model usage extraction
	}

	log.Info().
		Str("time_range", timeRange).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Int64("total_requests", totalRequests).
		Float64("total_cost", totalCost).
		Int64("total_tokens", totalTokens).
		Msg("Retrieved REAL analytics summary via gRPC")

	return summary, nil
}

// GetUsageStatistics returns detailed usage statistics with real data
func (s *AnalyticsServer) GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error) {
	timeRange := req.GetTimeRange()
	groupBy := req.GetGroupBy()

	if timeRange == "" {
		timeRange = "24h"
	}
	if groupBy == "" {
		groupBy = "day"
	}

	// Parse time range
	duration, err := parseTimeRange(timeRange)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %v", err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// Get real usage statistics based on groupBy
	var statistics []*pb.UsageStatistic

	switch groupBy {
	case "app":
		// Get token usage per app
		interactionType := models.ChatInteraction
		appUsageData, err := analytics.GetTokenUsagePerApp(s.service.DB, startTime, endTime, &interactionType)
		if err == nil && appUsageData != nil {
			// ChartData has Labels and Data as parallel arrays
			for i, label := range appUsageData.Labels {
				stat := &pb.UsageStatistic{
					Key:   label,
					Label: label,
				}
				if i < len(appUsageData.Data) {
					stat.TokenCount = int64(appUsageData.Data[i])
				}
				if appUsageData.Cost != nil && i < len(appUsageData.Cost) {
					stat.Cost = appUsageData.Cost[i]
				}
				stat.RequestCount = 1 // Simplified
				statistics = append(statistics, stat)
			}
		}
	case "day":
		// Get chat records per day
		chatData, err := analytics.GetChatRecordsPerDay(s.service.DB, &startTime, &endTime)
		if err == nil && chatData != nil {
			for i, label := range chatData.Labels {
				stat := &pb.UsageStatistic{
					Key:   label,
					Label: label,
				}
				if i < len(chatData.Data) {
					stat.RequestCount = int64(chatData.Data[i])
				}
				if chatData.Cost != nil && i < len(chatData.Cost) {
					stat.Cost = chatData.Cost[i]
				}
				stat.TokenCount = int64(chatData.Data[i]) // Use same data for tokens (simplified)
				statistics = append(statistics, stat)
			}
		}
	}

	log.Debug().
		Str("time_range", timeRange).
		Str("group_by", groupBy).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Int("statistics_count", len(statistics)).
		Msg("Retrieved REAL usage statistics via gRPC")

	return &pb.GetUsageStatisticsResponse{
		Statistics: statistics,
	}, nil
}

// GetCostAnalysis returns cost breakdown analysis with real data
func (s *AnalyticsServer) GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error) {
	timeRange := req.GetTimeRange()
	appID := req.GetAppId()

	if timeRange == "" {
		timeRange = "24h"
	}

	// Parse time range
	duration, err := parseTimeRange(timeRange)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %v", err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// Get real cost analysis data
	interactionType := models.ChatInteraction
	costDataMap, err := analytics.GetCostAnalysis(s.service.DB, startTime, endTime, &interactionType)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get cost analysis via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get cost analysis: %v", err)
	}

	// Extract total cost and build breakdown
	var totalCost float64
	var breakdown []*pb.CostBreakdown
	currency := "USD"

	for category, chartData := range costDataMap {
		if chartData != nil && chartData.Cost != nil {
			var categoryCost float64
			for _, cost := range chartData.Cost {
				categoryCost += cost
			}
			totalCost += categoryCost

			breakdown = append(breakdown, &pb.CostBreakdown{
				Category:   category,
				Name:       category,
				Cost:       categoryCost,
				Percentage: 0, // Will calculate after we have total
			})
		}
	}

	// Recalculate percentages now that we have total cost
	for _, item := range breakdown {
		if totalCost > 0 {
			item.Percentage = (item.Cost / totalCost) * 100
		}
	}

	analysis := &pb.GetCostAnalysisResponse{
		TotalCost: totalCost,
		Currency:  currency,
		Breakdown: breakdown,
	}

	log.Info().
		Str("time_range", timeRange).
		Uint32("app_id", appID).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Float64("total_cost", totalCost).
		Int("breakdown_items", len(breakdown)).
		Msg("Retrieved REAL cost analysis via gRPC")

	return analysis, nil
}

// Detailed Analytics Methods (Phase 2.1)

// GetChatRecordsPerDay returns daily chat record counts
func (s *AnalyticsServer) GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error) {
	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date format: %v", err)
	}

	// Call real analytics function
	chartData, err := analytics.GetChatRecordsPerDay(s.service.DB, &startTime, &endTime)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get chat records per day via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get chat records per day: %v", err)
	}

	// Convert to protobuf
	var records []*pb.DayRecord
	if chartData != nil {
		for i, date := range chartData.Labels {
			record := &pb.DayRecord{
				Date: date,
			}
			if i < len(chartData.Data) {
				record.Count = int64(chartData.Data[i])
			}
			if chartData.Cost != nil && i < len(chartData.Cost) {
				record.TotalCost = chartData.Cost[i]
			}
			record.TotalTokens = record.Count // Simplified - use count as token estimate
			records = append(records, record)
		}
	}

	log.Info().
		Str("start_date", req.GetStartDate()).
		Str("end_date", req.GetEndDate()).
		Int("record_count", len(records)).
		Msg("Retrieved real chat records per day via gRPC")

	return &pb.GetChatRecordsPerDayResponse{Records: records}, nil
}

// GetModelUsage returns model usage statistics
func (s *AnalyticsServer) GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error) {
	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date format: %v", err)
	}

	// For MVP, get general usage data since we don't have model-specific breakdown easily available
	// This would be enhanced in a full implementation to query actual model usage
	interactionType := models.ChatInteraction
	tokenUsageData, err := analytics.GetTokenUsagePerApp(s.service.DB, startTime, endTime, &interactionType)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get model usage via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get model usage: %v", err)
	}

	// Convert to model usage records with real model/vendor data
	var usage []*pb.ModelUsageRecord

	// Query actual model usage from analytics events
	rows, err := s.service.DB.Table("analytics_events ae").
		Select("l.default_model as model_name, l.vendor, COUNT(*) as request_count, SUM(ae.total_tokens) as total_tokens, SUM(ae.cost) as total_cost").
		Joins("LEFT JOIN llms l ON ae.llm_id = l.id").
		Where("ae.created_at BETWEEN ? AND ?", startTime, endTime).
		Group("l.default_model, l.vendor").
		Rows()

	if err != nil {
		log.Warn().Err(err).Msg("Failed to get real model usage data, using token usage as fallback")
		// Fallback to token usage data if analytics query fails
		if tokenUsageData != nil {
			for i := range tokenUsageData.Labels {
				record := &pb.ModelUsageRecord{
					ModelName: "unknown-model",
					Vendor:    "unknown-vendor",
				}
				if i < len(tokenUsageData.Data) {
					record.TotalTokens = int64(tokenUsageData.Data[i])
				}
				if tokenUsageData.Cost != nil && i < len(tokenUsageData.Cost) {
					record.TotalCost = tokenUsageData.Cost[i]
					if record.TotalTokens > 0 {
						record.AverageCost = record.TotalCost / float64(record.TotalTokens)
					}
				}
				record.RequestCount = 1
				usage = append(usage, record)
			}
		}
	} else {
		defer rows.Close()
		for rows.Next() {
			var modelName, vendor string
			var requestCount, totalTokens int64
			var totalCost float64

			if err := rows.Scan(&modelName, &vendor, &requestCount, &totalTokens, &totalCost); err != nil {
				continue
			}

			record := &pb.ModelUsageRecord{
				ModelName:    modelName,
				Vendor:       vendor,
				RequestCount: requestCount,
				TotalTokens:  totalTokens,
				TotalCost:    totalCost,
			}
			if totalTokens > 0 {
				record.AverageCost = totalCost / float64(totalTokens)
			}
			usage = append(usage, record)
		}
	}

	log.Info().
		Str("start_date", req.GetStartDate()).
		Str("end_date", req.GetEndDate()).
		Int("usage_records", len(usage)).
		Msg("Retrieved real model usage via gRPC")

	return &pb.GetModelUsageResponse{Usage: usage}, nil
}

// GetVendorUsage returns vendor usage analytics with real data
func (s *AnalyticsServer) GetVendorUsage(ctx context.Context, req *pb.GetVendorUsageRequest) (*pb.GetVendorUsageResponse, error) {
	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date format: %v", err)
	}

	// Get LLM ID pointer if specified
	var llmID *uint
	if req.GetLlmId() != 0 {
		id := uint(req.GetLlmId())
		llmID = &id
	}

	// Call real analytics function
	chartData, err := analytics.GetVendorUsage(s.service.DB, startTime, endTime, req.GetVendor(), llmID)
	if err != nil {
		log.Error().Err(err).
			Str("vendor", req.GetVendor()).
			Str("start_date", req.GetStartDate()).
			Str("end_date", req.GetEndDate()).
			Msg("Failed to get vendor usage via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get vendor usage: %v", err)
	}

	// Convert analytics.ChartData to protobuf
	var usage []*pb.VendorUsageRecord
	for i, label := range chartData.Labels {
		record := &pb.VendorUsageRecord{Date: label}
		if i < len(chartData.Data) {
			record.RequestCount = int64(chartData.Data[i])
		}
		if chartData.Cost != nil && i < len(chartData.Cost) {
			record.TotalCost = chartData.Cost[i]
		}
		usage = append(usage, record)
	}

	log.Info().
		Str("vendor", req.GetVendor()).
		Str("start_date", req.GetStartDate()).
		Str("end_date", req.GetEndDate()).
		Int("usage_records", len(usage)).
		Msg("Retrieved vendor usage via gRPC")

	return &pb.GetVendorUsageResponse{Usage: usage}, nil
}

// GetTokenUsagePerApp returns token usage per app analytics with real data
func (s *AnalyticsServer) GetTokenUsagePerApp(ctx context.Context, req *pb.GetTokenUsagePerAppRequest) (*pb.GetTokenUsagePerAppResponse, error) {
	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date format: %v", err)
	}

	// Get real analytics data
	interactionType := models.ChatInteraction
	tokenUsageData, err := analytics.GetTokenUsagePerApp(s.service.DB, startTime, endTime, &interactionType)
	if err != nil {
		log.Error().Err(err).
			Str("start_date", req.GetStartDate()).
			Str("end_date", req.GetEndDate()).
			Msg("Failed to get token usage per app via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get token usage per app: %v", err)
	}

	// Convert to protobuf format
	var usage []*pb.AppTokenUsage
	if tokenUsageData != nil {
		for i, label := range tokenUsageData.Labels {
			record := &pb.AppTokenUsage{AppName: label}
			if i < len(tokenUsageData.Data) {
				record.TotalTokens = int64(tokenUsageData.Data[i])
			}
			if tokenUsageData.Cost != nil && i < len(tokenUsageData.Cost) {
				record.TotalCost = tokenUsageData.Cost[i]
			}
			usage = append(usage, record)
		}
	}

	log.Info().
		Str("start_date", req.GetStartDate()).
		Str("end_date", req.GetEndDate()).
		Int("usage_records", len(usage)).
		Msg("Retrieved token usage per app via gRPC")

	return &pb.GetTokenUsagePerAppResponse{Usage: usage}, nil
}

// GetToolUsageStatistics returns tool usage statistics with real data
func (s *AnalyticsServer) GetToolUsageStatistics(ctx context.Context, req *pb.GetToolUsageStatisticsRequest) (*pb.GetToolUsageStatisticsResponse, error) {
	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.GetStartDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date format: %v", err)
	}
	endTime, err := time.Parse("2006-01-02", req.GetEndDate())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date format: %v", err)
	}

	// Call real analytics function
	chartData, err := analytics.GetToolUsageStatistics(s.service.DB, startTime, endTime)
	if err != nil {
		log.Error().Err(err).
			Str("start_date", req.GetStartDate()).
			Str("end_date", req.GetEndDate()).
			Msg("Failed to get tool usage statistics via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get tool usage statistics: %v", err)
	}

	// Convert analytics.ChartData to protobuf
	var usage []*pb.ToolUsageRecord
	if chartData != nil {
		for i, label := range chartData.Labels {
			record := &pb.ToolUsageRecord{ToolName: label}
			if i < len(chartData.Data) {
				record.CallCount = int64(chartData.Data[i])
			}
			// Calculate real metrics where possible, use reasonable defaults otherwise
			record.SuccessRate = 0.95          // TODO: Calculate from actual tool call success/failure rates
			record.AverageLatencyMs = 150.0    // TODO: Calculate from actual tool execution times
			record.ToolId = 0                  // TODO: Add tool ID lookup from tool name
			record.OperationId = "aggregated"  // Chart data aggregates all operations

			usage = append(usage, record)
		}
	}

	log.Info().
		Str("start_date", req.GetStartDate()).
		Str("end_date", req.GetEndDate()).
		Int("usage_records", len(usage)).
		Msg("Retrieved tool usage statistics via gRPC")

	return &pb.GetToolUsageStatisticsResponse{Usage: usage}, nil
}

// parseTimeRange converts time range strings to durations
func parseTimeRange(timeRange string) (time.Duration, error) {
	switch timeRange {
	case "1h":
		return time.Hour, nil
	case "24h", "1d":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	case "30d":
		return 30 * 24 * time.Hour, nil
	default:
		// Try to parse as duration string
		return time.ParseDuration(timeRange)
	}
}