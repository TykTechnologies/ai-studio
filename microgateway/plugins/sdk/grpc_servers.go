// plugins/sdk/grpc_servers.go
package sdk

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
)

// Base gRPC server implementation
type BaseGRPCServer struct {
	pb.UnimplementedPluginServiceServer
	BaseImpl interfaces.BasePlugin
}

func (s *BaseGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	config := make(map[string]interface{})
	for k, v := range req.Config {
		config[k] = v
	}
	
	err := s.BaseImpl.Initialize(config)
	if err != nil {
		return &pb.InitResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return &pb.InitResponse{Success: true}, nil
}

func (s *BaseGRPCServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

func (s *BaseGRPCServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	err := s.BaseImpl.Shutdown()
	if err != nil {
		return &pb.ShutdownResponse{Success: false}, nil
	}
	return &pb.ShutdownResponse{Success: true}, nil
}

func (s *BaseGRPCServer) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	// Check if the plugin implements ConfigSchemaProvider
	if schemaProvider, ok := s.BaseImpl.(interfaces.ConfigSchemaProvider); ok {
		schemaBytes, err := schemaProvider.GetConfigSchema()
		if err != nil {
			return &pb.GetConfigSchemaResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}

		return &pb.GetConfigSchemaResponse{
			Success:    true,
			SchemaJson: string(schemaBytes),
		}, nil
	}

	// Default implementation returns a basic schema that accepts any configuration
	defaultSchema := `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Plugin Configuration",
  "description": "Configuration schema for this plugin (default - plugin does not provide custom schema)",
  "properties": {},
  "additionalProperties": true
}`

	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: defaultSchema,
	}, nil
}

// Pre-auth plugin gRPC server
type PreAuthGRPCServer struct {
	BaseGRPCServer
	Impl interfaces.PreAuthPlugin
}

func (s *PreAuthGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.BaseImpl = s.Impl
	return s.BaseGRPCServer.Initialize(ctx, req)
}

func (s *PreAuthGRPCServer) ProcessPreAuth(ctx context.Context, req *pb.PluginRequest) (*pb.PluginResponse, error) {
	pluginReq := convertPBPluginRequest(req)
	pluginCtx := convertPBPluginContext(req.Context)
	
	result, err := s.Impl.ProcessRequest(ctx, pluginReq, pluginCtx)
	if err != nil {
		return &pb.PluginResponse{
			Modified:     false,
			Block:        true,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return convertInterfacePluginResponse(result), nil
}

// Auth plugin gRPC server
type AuthGRPCServer struct {
	BaseGRPCServer
	Impl interfaces.AuthPlugin
}

func (s *AuthGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.BaseImpl = s.Impl
	return s.BaseGRPCServer.Initialize(ctx, req)
}

func (s *AuthGRPCServer) Authenticate(ctx context.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	authReq := convertPBAuthRequest(req)
	pluginCtx := convertPBPluginContext(req.Context)
	
	resp, err := s.Impl.Authenticate(ctx, authReq, pluginCtx)
	if err != nil {
		return &pb.AuthResponse{
			Authenticated: false,
			ErrorMessage:  err.Error(),
		}, nil
	}
	
	return convertInterfaceAuthResponse(resp), nil
}

// Post-auth plugin gRPC server
type PostAuthGRPCServer struct {
	BaseGRPCServer
	Impl interfaces.PostAuthPlugin
}

func (s *PostAuthGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.BaseImpl = s.Impl
	return s.BaseGRPCServer.Initialize(ctx, req)
}

func (s *PostAuthGRPCServer) ProcessPostAuth(ctx context.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	enrichedReq := convertPBEnrichedRequest(req)
	pluginCtx := convertPBPluginContext(req.Request.Context)
	
	result, err := s.Impl.ProcessRequest(ctx, enrichedReq, pluginCtx)
	if err != nil {
		return &pb.PluginResponse{
			Modified:     false,
			Block:        true,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	return convertInterfacePluginResponse(result), nil
}

// Response plugin gRPC server (new clean interface)
type ResponseGRPCServer struct {
	BaseGRPCServer
	Impl interfaces.ResponsePlugin
}

func (s *ResponseGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.BaseImpl = s.Impl
	return s.BaseGRPCServer.Initialize(ctx, req)
}

func (s *ResponseGRPCServer) OnBeforeWriteHeaders(ctx context.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	headersReq := &interfaces.HeadersRequest{
		Headers: req.Headers,
		Context: convertPBPluginContext(req.Context),
	}
	
	result, err := s.Impl.OnBeforeWriteHeaders(ctx, headersReq, headersReq.Context)
	if err != nil {
		return &pb.HeadersResponse{Modified: false, Headers: req.Headers}, nil
	}
	
	return &pb.HeadersResponse{
		Modified: result.Modified,
		Headers:  result.Headers,
	}, nil
}

func (s *ResponseGRPCServer) OnBeforeWrite(ctx context.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	writeReq := &interfaces.ResponseWriteRequest{
		Body:         req.Body,
		Headers:      req.Headers,
		IsStreamChunk: req.IsStreamChunk,
		Context:      convertPBPluginContext(req.Context),
	}
	
	result, err := s.Impl.OnBeforeWrite(ctx, writeReq, writeReq.Context)
	if err != nil {
		return &pb.ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}
	
	return &pb.ResponseWriteResponse{
		Modified: result.Modified,
		Body:     result.Body,
		Headers:  result.Headers,
	}, nil
}

// Data collection plugin gRPC server
type DataCollectionGRPCServer struct {
	BaseGRPCServer
	Impl interfaces.DataCollectionPlugin
}

func (s *DataCollectionGRPCServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.BaseImpl = s.Impl
	return s.BaseGRPCServer.Initialize(ctx, req)
}

func (s *DataCollectionGRPCServer) HandleProxyLog(ctx context.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	proxyLogData := &interfaces.ProxyLogData{
		AppID:        uint(req.AppId),
		UserID:       uint(req.UserId),
		Vendor:       req.Vendor,
		RequestBody:  req.RequestBody,
		ResponseBody: req.ResponseBody,
		ResponseCode: int(req.ResponseCode),
		Timestamp:    timeFromUnix(req.Timestamp),
		RequestID:    req.RequestId,
	}
	
	pluginCtx := convertPBPluginContext(req.Context)
	
	result, err := s.Impl.HandleProxyLog(ctx, proxyLogData, pluginCtx)
	if err != nil {
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	// Convert metadata to protobuf map
	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}
	
	return &pb.DataCollectionResponse{
		Success:      result.Success,
		Handled:      result.Handled,
		ErrorMessage: result.ErrorMessage,
		Metadata:     metadata,
	}, nil
}

func (s *DataCollectionGRPCServer) HandleAnalytics(ctx context.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	analyticsData := &interfaces.AnalyticsData{
		LLMID:                   uint(req.LlmId),
		ModelName:              req.ModelName,
		Vendor:                 req.Vendor,
		PromptTokens:           int(req.PromptTokens),
		ResponseTokens:         int(req.ResponseTokens),
		CacheWritePromptTokens: int(req.CacheWritePromptTokens),
		CacheReadPromptTokens:  int(req.CacheReadPromptTokens),
		TotalTokens:            int(req.TotalTokens),
		Cost:                   req.Cost,
		Currency:               req.Currency,
		AppID:                  uint(req.AppId),
		UserID:                 uint(req.UserId),
		Timestamp:              timeFromUnix(req.Timestamp),
		ToolCalls:              int(req.ToolCalls),
		Choices:                int(req.Choices),
		RequestID:              req.RequestId,
	}
	
	pluginCtx := convertPBPluginContext(req.Context)
	
	result, err := s.Impl.HandleAnalytics(ctx, analyticsData, pluginCtx)
	if err != nil {
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	// Convert metadata to protobuf map
	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}
	
	return &pb.DataCollectionResponse{
		Success:      result.Success,
		Handled:      result.Handled,
		ErrorMessage: result.ErrorMessage,
		Metadata:     metadata,
	}, nil
}

func (s *DataCollectionGRPCServer) HandleBudgetUsage(ctx context.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	budgetData := &interfaces.BudgetUsageData{
		AppID:            uint(req.AppId),
		LLMID:            uint(req.LlmId),
		TokensUsed:       req.TokensUsed,
		Cost:             req.Cost,
		RequestsCount:    int(req.RequestsCount),
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		PeriodStart:      timeFromUnix(req.PeriodStart),
		PeriodEnd:        timeFromUnix(req.PeriodEnd),
		Timestamp:        timeFromUnix(req.Timestamp),
		RequestID:        req.RequestId,
	}
	
	pluginCtx := convertPBPluginContext(req.Context)
	
	result, err := s.Impl.HandleBudgetUsage(ctx, budgetData, pluginCtx)
	if err != nil {
		return &pb.DataCollectionResponse{
			Success:      false,
			Handled:      false,
			ErrorMessage: err.Error(),
		}, nil
	}
	
	// Convert metadata to protobuf map
	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}
	
	return &pb.DataCollectionResponse{
		Success:      result.Success,
		Handled:      result.Handled,
		ErrorMessage: result.ErrorMessage,
		Metadata:     metadata,
	}, nil
}
// Helper function to convert Unix timestamp to time.Time
func timeFromUnix(ts int64) time.Time {
	return time.Unix(ts, 0)
}

// ConfigProviderGRPC server - Isolated service for configuration schema extraction
type ConfigProviderGRPC struct {
	configpb.UnimplementedConfigProviderServiceServer
	Impl interfaces.BasePlugin
}

func (s *ConfigProviderGRPC) GetConfigSchema(ctx context.Context, req *configpb.ConfigSchemaRequest) (*configpb.ConfigSchemaResponse, error) {
	// Check if the plugin implements ConfigSchemaProvider
	if schemaProvider, ok := s.Impl.(interfaces.ConfigSchemaProvider); ok {
		schemaBytes, err := schemaProvider.GetConfigSchema()
		if err != nil {
			return &configpb.ConfigSchemaResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}

		return &configpb.ConfigSchemaResponse{
			Success:    true,
			SchemaJson: string(schemaBytes),
		}, nil
	}

	// Default implementation returns a basic schema that accepts any configuration
	defaultSchema := `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Plugin Configuration",
  "description": "Configuration schema for this plugin (default - plugin does not provide custom schema)",
  "properties": {},
  "additionalProperties": true
}`

	return &configpb.ConfigSchemaResponse{
		Success:    true,
		SchemaJson: defaultSchema,
	}, nil
}

func (s *ConfigProviderGRPC) Ping(ctx context.Context, req *configpb.ConfigPingRequest) (*configpb.ConfigPingResponse, error) {
	return &configpb.ConfigPingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

func (s *ConfigProviderGRPC) GetManifest(ctx context.Context, req *configpb.GetManifestRequest) (*configpb.GetManifestResponse, error) {
	// Check if the plugin implements ManifestProvider
	type ManifestProvider interface {
		GetManifest() ([]byte, error)
	}

	if manifestProvider, ok := s.Impl.(ManifestProvider); ok {
		manifestBytes, err := manifestProvider.GetManifest()
		if err != nil {
			return &configpb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}

		return &configpb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Plugin doesn't provide manifest
	return &configpb.GetManifestResponse{
		Success:      false,
		ErrorMessage: "plugin does not implement GetManifest",
	}, nil
}
