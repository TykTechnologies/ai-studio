package grpc

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AnalyticsServer implements the AIStudioManagementService for analytics operations
// Note: Analytics functionality is not available to plugins in the current version
// Analytics logic remains in the REST API layer for architectural consistency
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

// Analytics methods are not available to plugins
// Analytics logic remains in the REST API layer for architectural consistency

func (s *AnalyticsServer) GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetVendorUsage(ctx context.Context, req *pb.GetVendorUsageRequest) (*pb.GetVendorUsageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetTokenUsagePerApp(ctx context.Context, req *pb.GetTokenUsagePerAppRequest) (*pb.GetTokenUsagePerAppResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}

func (s *AnalyticsServer) GetToolUsageStatistics(ctx context.Context, req *pb.GetToolUsageStatisticsRequest) (*pb.GetToolUsageStatisticsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
}