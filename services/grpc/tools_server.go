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

	// Apply filtering based on request parameters
	var tools []models.Tool
	var totalCount int64
	var err error

	// Check if filtering is requested
	if req.GetToolType() != "" {
		// Use tool type filtering
		tools, err = s.service.GetToolsByType(req.GetToolType())
		if err != nil {
			log.Error().Err(err).Str("tool_type", req.GetToolType()).Msg("Failed to get tools by type via gRPC")
			return nil, status.Errorf(codes.Internal, "failed to get tools by type: %v", err)
		}
		totalCount = int64(len(tools))

		// Apply manual pagination since service method doesn't support it
		start := (page - 1) * limit
		end := start + limit
		if start < len(tools) {
			if end > len(tools) {
				end = len(tools)
			}
			tools = tools[start:end]
		} else {
			tools = []models.Tool{}
		}
	} else {
		// Use standard pagination without filtering
		tools, totalCount, _, err = s.service.GetAllTools(limit, page, false)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list tools via gRPC")
			return nil, status.Errorf(codes.Internal, "failed to list tools: %v", err)
		}
	}

	// Note: is_active and namespace filtering not implemented as Tool model doesn't have these fields
	// Tools are considered active if they exist and namespace is always global

	// Convert service response to gRPC protobuf
	pbTools := make([]*pb.ToolInfo, len(tools))
	for i, tool := range tools {
		pbTools[i] = convertToolToPB(&tool)
	}

	log.Debug().
		Int("tool_count", len(tools)).
		Int64("total_count", totalCount).
		Str("tool_type_filter", req.GetToolType()).
		Msg("Listed tools with filtering via gRPC")

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

	// Get detailed tool operation information from service
	operationDetails, err := s.service.GetToolOperationDetails(uint(toolID))
	if err != nil {
		log.Error().Err(err).Uint32("tool_id", toolID).Msg("Failed to get tool operation details via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get tool operation details: %v", err)
	}

	// Convert to protobuf with full operation details
	pbOperations := make([]*pb.ToolOperation, len(operationDetails))
	for i, detail := range operationDetails {
		pbOperations[i] = &pb.ToolOperation{
			OperationId: detail.OperationID,
			Method:      detail.Method,
			Path:        detail.Path,
			Summary:     detail.Summary,
			Description: detail.Description,
		}
	}

	log.Debug().
		Uint32("tool_id", toolID).
		Str("tool_name", tool.Name).
		Int("operation_count", len(operationDetails)).
		Msg("Retrieved detailed tool operations via gRPC")

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

// CreateTool creates a new tool
func (s *ToolsServer) CreateTool(ctx context.Context, req *pb.CreateToolRequest) (*pb.CreateToolResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.GetDescription() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "description is required")
	}
	if req.GetToolType() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tool_type is required")
	}

	// Call existing service method
	tool, err := s.service.CreateTool(
		req.GetName(),
		req.GetDescription(),
		req.GetToolType(),
		req.GetOasSpec(),
		int(req.GetPrivacyScore()),
		req.GetAuthSchemaName(),
		req.GetAuthKey(),
	)
	if err != nil {
		log.Error().Err(err).
			Str("name", req.GetName()).
			Str("tool_type", req.GetToolType()).
			Msg("Failed to create tool via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create tool: %v", err)
	}

	log.Info().
		Uint("tool_id", tool.ID).
		Str("tool_name", tool.Name).
		Str("tool_type", tool.ToolType).
		Msg("Created tool via gRPC")

	return &pb.CreateToolResponse{
		Tool: convertToolToPB(tool),
	}, nil
}

// UpdateTool updates an existing tool
func (s *ToolsServer) UpdateTool(ctx context.Context, req *pb.UpdateToolRequest) (*pb.UpdateToolResponse, error) {
	toolID := req.GetToolId()
	if toolID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tool_id is required")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.GetDescription() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "description is required")
	}
	if req.GetToolType() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tool_type is required")
	}

	// Call existing service method
	tool, err := s.service.UpdateTool(
		uint(toolID),
		req.GetName(),
		req.GetDescription(),
		req.GetToolType(),
		req.GetOasSpec(),
		int(req.GetPrivacyScore()),
		req.GetAuthSchemaName(),
		req.GetAuthKey(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tool not found: %d", toolID)
		}
		log.Error().Err(err).
			Uint32("tool_id", toolID).
			Str("name", req.GetName()).
			Msg("Failed to update tool via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update tool: %v", err)
	}

	log.Info().
		Uint32("tool_id", toolID).
		Str("tool_name", tool.Name).
		Msg("Updated tool via gRPC")

	return &pb.UpdateToolResponse{
		Tool: convertToolToPB(tool),
	}, nil
}

// DeleteTool deletes a tool
func (s *ToolsServer) DeleteTool(ctx context.Context, req *pb.DeleteToolRequest) (*pb.DeleteToolResponse, error) {
	toolID := req.GetToolId()
	if toolID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tool_id is required")
	}

	// Call existing service method
	err := s.service.DeleteTool(uint(toolID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tool not found: %d", toolID)
		}
		log.Error().Err(err).
			Uint32("tool_id", toolID).
			Msg("Failed to delete tool via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete tool: %v", err)
	}

	log.Info().
		Uint32("tool_id", toolID).
		Msg("Deleted tool via gRPC")

	return &pb.DeleteToolResponse{
		Success: true,
		Message: "Tool deleted successfully",
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
		IsActive:     true,                 // Tool model doesn't have IsActive field - default to true
		Namespace:    "",                   // Tool model doesn't have Namespace field - default to global
		PrivacyScore: int32(tool.PrivacyScore),
		CreatedAt:    timestamppb.New(tool.CreatedAt),
		UpdatedAt:    timestamppb.New(tool.UpdatedAt),
	}
}