// plugins/sdk/grpc_servers.go
package sdk

import (
	"context"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
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