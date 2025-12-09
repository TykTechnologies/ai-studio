package api

import (
	"context"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
)

// GRPCResponsePluginAdapter bridges gRPC response plugins to the AI Gateway response hook system
// It implements proxy.ResponseHook and manages per-LLM plugin routing with in-memory caching
type GRPCResponsePluginAdapter struct {
	pluginManager   *plugins.PluginManager
	serviceContainer *services.ServiceContainer
	mu              sync.RWMutex
	cacheValid      bool
}

// NewGRPCResponsePluginAdapter creates a new adapter for bridging gRPC plugins to response hooks
func NewGRPCResponsePluginAdapter(serviceContainer *services.ServiceContainer, pluginManager *plugins.PluginManager) *GRPCResponsePluginAdapter {
	adapter := &GRPCResponsePluginAdapter{
		pluginManager:   pluginManager,
		serviceContainer: serviceContainer,
		cacheValid:      true, // Use plugin manager's caching, not our own
	}
	
	log.Debug().Msg("gRPC response plugin adapter initialized - will use plugin manager for caching")
	return adapter
}

// GetName implements proxy.ResponseHook
func (a *GRPCResponsePluginAdapter) GetName() string {
	return "grpc-response-plugin-adapter"
}

// OnBeforeWriteHeaders implements proxy.ResponseHook
func (a *GRPCResponsePluginAdapter) OnBeforeWriteHeaders(ctx context.Context, req *proxy.HeadersRequest) (*proxy.HeadersResponse, error) {
	log.Debug().Uint("llm_id", req.Context.LLMID).Msg("OnBeforeWriteHeaders called via adapter")
	
	// Get plugins for this LLM using the plugin manager directly
	plugins, err := a.pluginManager.GetPluginsForLLM(req.Context.LLMID, interfaces.HookTypeOnResponse)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", req.Context.LLMID).Msg("Failed to get response plugins for LLM")
		return &proxy.HeadersResponse{Modified: false, Headers: req.Headers}, nil
	}
	
	if len(plugins) == 0 {
		log.Debug().Uint("llm_id", req.Context.LLMID).Msg("ℹ️ No response plugins configured for LLM")
		return &proxy.HeadersResponse{
			Modified: false,
			Headers:  req.Headers,
		}, nil
	}

	log.Debug().Uint("llm_id", req.Context.LLMID).Int("plugin_count", len(plugins)).Msg("📝 Executing response plugins (OnBeforeWriteHeaders)")
	
	// Convert to protobuf and execute plugins
	result := &proxy.HeadersResponse{
		Modified: false,
		Headers:  make(map[string]string),
	}
	
	// Copy original headers
	for k, v := range req.Headers {
		result.Headers[k] = v
	}
	
	// Execute each plugin in sequence
	for _, plugin := range plugins {
		// Convert to protobuf
		pbReq := &pb.HeadersRequest{
			Headers: result.Headers,
			Context: convertPluginContextToPB(req.Context),
		}
		
		// Call gRPC plugin
		pbResp, err := plugin.GRPCClient.OnBeforeWriteHeaders(ctx, pbReq)
		if err != nil {
			log.Error().Err(err).Str("plugin_name", plugin.Name).Msg("gRPC OnBeforeWriteHeaders failed")
			continue // Continue with other plugins
		}
		
		if pbResp.Modified {
			result.Modified = true
			result.Headers = pbResp.Headers
		}
		
		log.Debug().Str("plugin_name", plugin.Name).Bool("modified", pbResp.Modified).Msg("OnBeforeWriteHeaders executed")
	}
	
	return result, nil
}

// OnBeforeWrite implements proxy.ResponseHook
func (a *GRPCResponsePluginAdapter) OnBeforeWrite(ctx context.Context, req *proxy.ResponseWriteRequest) (*proxy.ResponseWriteResponse, error) {
	log.Debug().Uint("llm_id", req.Context.LLMID).Msg("OnBeforeWrite called via adapter")
	
	// Get plugins for this LLM using the plugin manager directly
	plugins, err := a.pluginManager.GetPluginsForLLM(req.Context.LLMID, interfaces.HookTypeOnResponse)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", req.Context.LLMID).Msg("Failed to get response plugins for LLM")
		return &proxy.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
	}
	
	if len(plugins) == 0 {
		log.Debug().Uint("llm_id", req.Context.LLMID).Msg("ℹ️ No response plugins configured for LLM (OnBeforeWrite)")
		return &proxy.ResponseWriteResponse{
			Modified: false,
			Body:     req.Body,
			Headers:  req.Headers,
		}, nil
	}

	log.Debug().Uint("llm_id", req.Context.LLMID).Int("plugin_count", len(plugins)).Msg("📝 Executing response plugins (OnBeforeWrite)")
	
	// Convert to protobuf and execute plugins
	result := &proxy.ResponseWriteResponse{
		Modified: false,
		Body:     make([]byte, len(req.Body)),
		Headers:  make(map[string]string),
	}
	
	// Copy original data
	copy(result.Body, req.Body)
	for k, v := range req.Headers {
		result.Headers[k] = v
	}
	
	// Execute each plugin in sequence
	for _, plugin := range plugins {
		// Convert to protobuf
		pbReq := &pb.ResponseWriteRequest{
			Body:         result.Body,
			Headers:      result.Headers,
			IsStreamChunk: false, // REST-only, always false
			Context:      convertPluginContextToPB(req.Context),
		}
		
		// Call gRPC plugin
		pbResp, err := plugin.GRPCClient.OnBeforeWrite(ctx, pbReq)
		if err != nil {
			log.Error().Err(err).Str("plugin_name", plugin.Name).Msg("gRPC OnBeforeWrite failed")
			continue // Continue with other plugins
		}
		
		if pbResp.Modified {
			result.Modified = true
			result.Body = pbResp.Body
			result.Headers = pbResp.Headers
		}
		
		log.Debug().Str("plugin_name", plugin.Name).Bool("modified", pbResp.Modified).Int("body_len", len(pbResp.Body)).Msg("OnBeforeWrite executed")
	}

	return result, nil
}

// OnStreamComplete implements proxy.ResponseHook
// This is called after a streaming response has finished, providing the accumulated response.
func (a *GRPCResponsePluginAdapter) OnStreamComplete(ctx context.Context, req *proxy.StreamCompleteRequest) (*proxy.StreamCompleteResponse, error) {
	log.Debug().Uint("llm_id", req.Context.LLMID).Int("chunk_count", req.ChunkCount).Msg("OnStreamComplete called via adapter")

	// Get plugins for this LLM using the plugin manager directly
	plugins, err := a.pluginManager.GetPluginsForLLM(req.Context.LLMID, interfaces.HookTypeOnResponse)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", req.Context.LLMID).Msg("Failed to get response plugins for LLM")
		return &proxy.StreamCompleteResponse{Handled: false}, nil
	}

	if len(plugins) == 0 {
		log.Debug().Uint("llm_id", req.Context.LLMID).Msg("ℹ️ No response plugins configured for LLM (OnStreamComplete)")
		return &proxy.StreamCompleteResponse{Handled: false}, nil
	}

	log.Debug().Uint("llm_id", req.Context.LLMID).Int("plugin_count", len(plugins)).Int("response_size", len(req.AccumulatedResponse)).Msg("📝 Executing response plugins (OnStreamComplete)")

	result := &proxy.StreamCompleteResponse{
		Handled: false,
	}

	// Execute each plugin in sequence
	for _, plugin := range plugins {
		// Convert to protobuf
		pbReq := &pb.StreamCompleteRequest{
			AccumulatedResponse: req.AccumulatedResponse,
			Headers:             req.Headers,
			StatusCode:          int32(req.StatusCode),
			Context:             convertPluginContextToPB(req.Context),
			ChunkCount:          int32(req.ChunkCount),
			RequestBody:         req.RequestBody,
		}

		// Call gRPC plugin
		pbResp, err := plugin.GRPCClient.OnStreamComplete(ctx, pbReq)
		if err != nil {
			log.Error().Err(err).Str("plugin_name", plugin.Name).Msg("gRPC OnStreamComplete failed")
			continue // Continue with other plugins
		}

		if pbResp.Handled {
			result.Handled = true
		}
		if pbResp.Cached {
			result.Cached = true
		}
		if pbResp.ErrorMessage != "" && result.ErrorMessage == "" {
			result.ErrorMessage = pbResp.ErrorMessage
		}

		log.Debug().Str("plugin_name", plugin.Name).Bool("handled", pbResp.Handled).Bool("cached", pbResp.Cached).Msg("OnStreamComplete executed")
	}

	return result, nil
}

// Reload is called when gateway configuration is reloaded (no-op since plugin manager handles caching)
func (a *GRPCResponsePluginAdapter) Reload() error {
	log.Debug().Msg("gRPC response plugin adapter reload requested - plugin manager will handle caching")
	return nil
}

// convertPluginContextToPB converts proxy.PluginContext to protobuf PluginContext
func convertPluginContextToPB(ctx *proxy.PluginContext) *pb.PluginContext {
	if ctx == nil {
		return &pb.PluginContext{}
	}
	
	return &pb.PluginContext{
		RequestId: ctx.RequestID,
		Vendor:    "", // Will be set from LLM context if needed
		LlmId:     uint32(ctx.LLMID),
		LlmSlug:   ctx.LLMSlug,
		AppId:     uint32(ctx.AppID),
		UserId:    uint32(ctx.UserID),
		Metadata:  ctx.Metadata,
		TraceContext: make(map[string]string), // Not implemented yet
	}
}

// GetStats returns adapter statistics for debugging
func (a *GRPCResponsePluginAdapter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"adapter_active": a.cacheValid,
		"plugin_manager_available": a.pluginManager != nil,
	}
}