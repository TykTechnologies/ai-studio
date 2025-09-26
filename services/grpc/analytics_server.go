package grpc

import (
	"context"
	"time"

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

// GetAnalyticsSummary returns high-level analytics summary
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

	// Get analytics data from service layer
	// Note: This is simplified - real implementation would call appropriate service methods
	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// For MVP, return basic analytics structure
	// Real implementation would call s.analyticsService methods
	summary := &pb.GetAnalyticsSummaryResponse{
		TotalRequests:      0,
		SuccessfulRequests: 0,
		FailedRequests:     0,
		TotalCost:          0.0,
		Currency:           "USD",
		TotalTokens:        0,
		TopEndpoints:       []*pb.TopEndpoint{},
		ModelUsage:         []*pb.ModelUsage{},
	}

	log.Debug().
		Str("time_range", timeRange).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Retrieved analytics summary via gRPC")

	return summary, nil
}

// GetUsageStatistics returns detailed usage statistics
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

	// For MVP, return empty statistics
	// Real implementation would call analytics service methods based on groupBy
	statistics := []*pb.UsageStatistic{}

	log.Debug().
		Str("time_range", timeRange).
		Str("group_by", groupBy).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Retrieved usage statistics via gRPC")

	return &pb.GetUsageStatisticsResponse{
		Statistics: statistics,
	}, nil
}

// GetCostAnalysis returns cost breakdown analysis
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

	// For MVP, return basic cost analysis structure
	// Real implementation would call analytics service methods
	analysis := &pb.GetCostAnalysisResponse{
		TotalCost: 0.0,
		Currency:  "USD",
		Breakdown: []*pb.CostBreakdown{},
	}

	log.Debug().
		Str("time_range", timeRange).
		Uint32("app_id", appID).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Retrieved cost analysis via gRPC")

	return analysis, nil
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