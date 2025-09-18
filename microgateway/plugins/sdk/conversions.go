// plugins/sdk/conversions.go
package sdk

import (
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
)

// Convert from protobuf types to interfaces types

func convertPBPluginContext(ctx *pb.PluginContext) *interfaces.PluginContext {
	if ctx == nil {
		return &interfaces.PluginContext{}
	}
	
	metadata := make(map[string]interface{})
	for k, v := range ctx.Metadata {
		metadata[k] = v
	}
	
	return &interfaces.PluginContext{
		RequestID:    ctx.RequestId,
		LLMID:        uint(ctx.LlmId),
		LLMSlug:      ctx.LlmSlug,
		Vendor:       ctx.Vendor,
		AppID:        uint(ctx.AppId),
		UserID:       uint(ctx.UserId),
		Metadata:     metadata,
		TraceContext: ctx.TraceContext,
	}
}

func convertPBPluginRequest(req *pb.PluginRequest) *interfaces.PluginRequest {
	if req == nil {
		return &interfaces.PluginRequest{}
	}
	
	ctx := convertPBPluginContext(req.Context)
	
	return &interfaces.PluginRequest{
		Method:     req.Method,
		Path:       req.Path,
		Headers:    req.Headers,
		Body:       req.Body,
		RemoteAddr: req.RemoteAddr,
		Context:    ctx,
	}
}

func convertPBAuthRequest(req *pb.AuthRequest) *interfaces.AuthRequest {
	if req == nil {
		return &interfaces.AuthRequest{}
	}
	
	var pluginReq *interfaces.PluginRequest
	if req.Request != nil {
		pluginReq = convertPBPluginRequest(req.Request)
	}
	
	return &interfaces.AuthRequest{
		Credential: req.Credential,
		AuthType:   req.AuthType,
		Request:    pluginReq,
	}
}

func convertPBEnrichedRequest(req *pb.EnrichedRequest) *interfaces.EnrichedRequest {
	if req == nil {
		return &interfaces.EnrichedRequest{}
	}
	
	var pluginReq *interfaces.PluginRequest
	if req.Request != nil {
		pluginReq = convertPBPluginRequest(req.Request)
	}
	
	return &interfaces.EnrichedRequest{
		PluginRequest: pluginReq,
		UserID:        req.UserId,
		AppID:         req.AppId,
		AuthClaims:    req.AuthClaims,
		Authenticated: req.Authenticated,
	}
}

// Convert from interfaces types to protobuf types

func convertPluginContext(ctx *interfaces.PluginContext) *pb.PluginContext {
	if ctx == nil {
		return &pb.PluginContext{}
	}
	
	metadata := make(map[string]string)
	for k, v := range ctx.Metadata {
		metadata[k] = interfaceToString(v)
	}
	
	return &pb.PluginContext{
		RequestId:    ctx.RequestID,
		Vendor:       ctx.Vendor,
		LlmId:        uint32(ctx.LLMID),
		LlmSlug:      ctx.LLMSlug,
		AppId:        uint32(ctx.AppID),
		UserId:       uint32(ctx.UserID),
		Metadata:     metadata,
		TraceContext: ctx.TraceContext,
	}
}

func convertInterfacePluginResponse(resp *interfaces.PluginResponse) *pb.PluginResponse {
	if resp == nil {
		return &pb.PluginResponse{}
	}
	
	return &pb.PluginResponse{
		Modified:     resp.Modified,
		StatusCode:   int32(resp.StatusCode),
		Headers:      resp.Headers,
		Body:         resp.Body,
		Block:        resp.Block,
		ErrorMessage: resp.ErrorMessage,
	}
}

func convertInterfaceAuthResponse(resp *interfaces.AuthResponse) *pb.AuthResponse {
	if resp == nil {
		return &pb.AuthResponse{}
	}
	
	return &pb.AuthResponse{
		Authenticated: resp.Authenticated,
		UserId:       resp.UserID,
		AppId:        resp.AppID,
		Claims:       resp.Claims,
		ErrorMessage: resp.ErrorMessage,
	}
}

// Helper function to convert interface{} to string
func interfaceToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}