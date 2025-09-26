package grpc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ToolsServer implements the AIStudioManagementService for tools management operations
type ToolsServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewToolsServer creates a new tools management gRPC server
func NewToolsServer(service *services.Service) *ToolsServer {
	return &ToolsServer{
		service: service,
	}
}

// ListTools returns a list of tools with filtering and pagination
func (s *ToolsServer) ListTools(ctx context.Context, req *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Call existing service method - using GetAllTools with pagination
	tools, totalCount, _, err := s.service.GetAllTools(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list tools via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list tools: %v", err)
	}

	// TODO: Apply tool_type, is_active, and namespace filtering in future versions
	// For MVP, return all tools

	// Convert service response to gRPC protobuf
	pbTools := make([]*pb.ToolInfo, len(tools))
	for i, tool := range tools {
		pbTools[i] = convertToolToPB(&tool)
	}

	log.Debug().
		Int("tool_count", len(tools)).
		Int64("total_count", totalCount).
		Msg("Listed tools via gRPC")

	return &pb.ListToolsResponse{
		Tools:      pbTools,
		TotalCount: totalCount,
	}, nil
}

// GetTool returns details for a specific tool
func (s *ToolsServer) GetTool(ctx context.Context, req *pb.GetToolRequest) (*pb.GetToolResponse, error) {
	toolID := req.GetToolId()
	if toolID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tool_id is required")
	}

	// Call existing service method
	tool, err := s.service.GetToolByID(uint(toolID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tool not found: %d", toolID)
		}
		log.Error().Err(err).Uint32("tool_id", toolID).Msg("Failed to get tool via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get tool: %v", err)
	}

	log.Debug().
		Uint32("tool_id", toolID).
		Str("tool_name", tool.Name).
		Msg("Retrieved tool via gRPC")

	return &pb.GetToolResponse{
		Tool: convertToolToPB(tool),
	}, nil
}

// GetToolOperations returns operations available for a specific tool
func (s *ToolsServer) GetToolOperations(ctx context.Context, req *pb.GetToolOperationsRequest) (*pb.GetToolOperationsResponse, error) {
	toolID := req.GetToolId()
	if toolID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tool_id is required")
	}

	// Get tool first to verify it exists
	tool, err := s.service.GetToolByID(uint(toolID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tool not found: %d", toolID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get tool: %v", err)
	}

	// Get tool operations from service
	operations, err := s.service.GetToolOperations(uint(toolID))
	if err != nil {
		log.Error().Err(err).Uint32("tool_id", toolID).Msg("Failed to get tool operations via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get tool operations: %v", err)
	}

	// Convert to protobuf (simplified for MVP)
	pbOperations := make([]*pb.ToolOperation, len(operations))
	for i, op := range operations {
		pbOperations[i] = &pb.ToolOperation{
			OperationId: op,
			Method:      "GET", // Simplified - would need actual operation details
			Path:        "",    // Would need to parse from OpenAPI spec
			Summary:     "",    // Would need to extract from spec
			Description: "",    // Would need to extract from spec
		}
	}

	log.Debug().
		Uint32("tool_id", toolID).
		Str("tool_name", tool.Name).
		Int("operation_count", len(operations)).
		Msg("Retrieved tool operations via gRPC")

	return &pb.GetToolOperationsResponse{
		Operations: pbOperations,
	}, nil
}

// CallToolOperation executes a tool operation
func (s *ToolsServer) CallToolOperation(ctx context.Context, req *pb.CallToolOperationRequest) (*pb.CallToolOperationResponse, error) {
	toolID := req.GetToolId()
	if toolID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tool_id is required")
	}

	operationID := req.GetOperationId()
	if operationID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "operation_id is required")
	}

	// Parse JSON parameters
	var params map[string][]string
	if req.GetParamsJson() != "" {
		if err := json.Unmarshal([]byte(req.GetParamsJson()), &params); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid params_json: %v", err)
		}
	}

	var payload map[string]interface{}
	if req.GetPayloadJson() != "" {
		if err := json.Unmarshal([]byte(req.GetPayloadJson()), &payload); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid payload_json: %v", err)
		}
	}

	var headers map[string][]string
	if req.GetHeadersJson() != "" {
		if err := json.Unmarshal([]byte(req.GetHeadersJson()), &headers); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid headers_json: %v", err)
		}
	}

	// Call existing service method
	result, err := s.service.CallToolOperation(uint(toolID), operationID, params, payload, headers)
	if err != nil {
		log.Error().Err(err).
			Uint32("tool_id", toolID).
			Str("operation_id", operationID).
			Msg("Failed to call tool operation via gRPC")
		return &pb.CallToolOperationResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Convert result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &pb.CallToolOperationResponse{
			Success:      false,
			ErrorMessage: "failed to serialize operation result",
		}, nil
	}

	log.Info().
		Uint32("tool_id", toolID).
		Str("operation_id", operationID).
		Msg("Successfully called tool operation via gRPC")

	return &pb.CallToolOperationResponse{
		Success:    true,
		ResultJson: string(resultJSON),
	}, nil
}

// convertToolToPB converts a models.Tool to protobuf ToolInfo
func convertToolToPB(tool *models.Tool) *pb.ToolInfo {
	// For security, don't expose the full OAS spec - just basic info
	var truncatedSpec string
	if len(tool.OASSpec) > 1000 {
		truncatedSpec = tool.OASSpec[:1000] + "... [truncated for security]"
	} else {
		truncatedSpec = tool.OASSpec
	}

	// Generate slug from name if not present in model
	slug := strings.ToLower(strings.ReplaceAll(tool.Name, " ", "-"))

	return &pb.ToolInfo{
		Id:           uint32(tool.ID),
		Name:         tool.Name,
		Slug:         slug, // Generated from name
		Description:  tool.Description,
		ToolType:     tool.ToolType,
		OasSpec:      truncatedSpec,
		Operations:   tool.GetOperations(), // Get whitelisted operations
		IsActive:     true,                 // Tool model doesn't have IsActive - assume true
		Namespace:    "",                   // Tool model doesn't have Namespace - assume global
		PrivacyScore: int32(tool.PrivacyScore),
		CreatedAt:    timestamppb.New(tool.CreatedAt),
		UpdatedAt:    timestamppb.New(tool.UpdatedAt),
	}
}