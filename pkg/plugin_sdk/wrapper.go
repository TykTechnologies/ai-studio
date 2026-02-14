package plugin_sdk

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// pluginServerWrapper implements pb.PluginServiceServer
// It wraps a user's plugin and adapts it to the proto gRPC interface.
// This is the bridge between the go-plugin gRPC layer and the simplified SDK interface.
type pluginServerWrapper struct {
	pb.UnimplementedPluginServiceServer
	plugin   Plugin        // The user's plugin implementation
	runtime  RuntimeType   // Detected runtime (studio/gateway)
	services ServiceBroker // Runtime-specific services
}

// newPluginServerWrapper creates a new wrapper around a user plugin
func newPluginServerWrapper(plugin Plugin, runtime RuntimeType, services ServiceBroker) *pluginServerWrapper {
	return &pluginServerWrapper{
		plugin:   plugin,
		runtime:  runtime,
		services: services,
	}
}

// createPluginContext creates a Context from proto request context
func (w *pluginServerWrapper) createPluginContext(baseCtx context.Context, pbCtx *pb.PluginContext) Context {
	if pbCtx == nil {
		pbCtx = &pb.PluginContext{}
	}

	ctx := Context{
		Runtime:      w.runtime,
		RequestID:    pbCtx.RequestId,
		AppID:        pbCtx.AppId,
		UserID:       pbCtx.UserId,
		LLMID:        pbCtx.LlmId,
		LLMSlug:      pbCtx.LlmSlug,
		Vendor:       pbCtx.Vendor,
		Metadata:     pbCtx.Metadata,
		TraceContext: pbCtx.TraceContext,
		Services:     w.services,
		Context:      baseCtx,
	}

	// Populate edge info from metadata if available (set by microgateway)
	if pbCtx.Metadata != nil {
		if edgeID, ok := pbCtx.Metadata["edge_id"]; ok {
			ctx.EdgeID = edgeID
		}
		if edgeNamespace, ok := pbCtx.Metadata["edge_namespace"]; ok {
			ctx.EdgeNamespace = edgeNamespace
		}
	}

	return ctx
}

// Initialize is implemented in serve.go to handle service broker setup

// Ping implements pb.PluginServiceServer
func (w *pluginServerWrapper) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

// Shutdown implements pb.PluginServiceServer
func (w *pluginServerWrapper) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	pluginCtx := w.createPluginContext(ctx, nil)

	// Close any active session
	CloseSession("shutdown")

	// Cleanup service broker resources (event subscriptions, etc.)
	if broker, ok := w.services.(*defaultServiceBroker); ok {
		broker.Cleanup()
	}

	err := w.plugin.Shutdown(pluginCtx)
	if err != nil {
		return &pb.ShutdownResponse{Success: false}, err
	}

	return &pb.ShutdownResponse{Success: true}, nil
}

// ProcessPreAuth implements pb.PluginServiceServer
func (w *pluginServerWrapper) ProcessPreAuth(ctx context.Context, req *pb.PluginRequest) (*pb.PluginResponse, error) {
	// Check if plugin implements PreAuthHandler
	handler, ok := w.plugin.(PreAuthHandler)
	if !ok {
		// Plugin doesn't handle pre-auth, return unmodified
		return &pb.PluginResponse{Modified: false}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandlePreAuth(pluginCtx, req)
}

// Authenticate implements pb.PluginServiceServer
func (w *pluginServerWrapper) Authenticate(ctx context.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	// Check if plugin implements AuthHandler
	handler, ok := w.plugin.(AuthHandler)
	if !ok {
		// Plugin doesn't handle auth
		return &pb.AuthResponse{
			Authenticated: false,
			ErrorMessage:  "plugin does not implement authentication",
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleAuth(pluginCtx, req)
}

// GetAppByCredential implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetAppByCredential(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error) {
	// Check if plugin implements AuthHandler
	handler, ok := w.plugin.(AuthHandler)
	if !ok {
		return &pb.GetAppResponse{
			Success:      false,
			ErrorMessage: "plugin does not implement authentication",
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	app, err := handler.GetAppByCredential(pluginCtx, req.Credential)
	if err != nil {
		return &pb.GetAppResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetAppResponse{
		Success: true,
		App:     app,
	}, nil
}

// GetUserByCredential implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetUserByCredential(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	// Check if plugin implements AuthHandler
	handler, ok := w.plugin.(AuthHandler)
	if !ok {
		return &pb.GetUserResponse{
			Success:      false,
			ErrorMessage: "plugin does not implement authentication",
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	user, err := handler.GetUserByCredential(pluginCtx, req.Credential)
	if err != nil {
		return &pb.GetUserResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetUserResponse{
		Success: true,
		User:    user,
	}, nil
}

// ProcessPostAuth implements pb.PluginServiceServer
// This is the KEY method for gateway compatibility
func (w *pluginServerWrapper) ProcessPostAuth(ctx context.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	// Check if plugin implements PostAuthHandler
	handler, ok := w.plugin.(PostAuthHandler)
	if !ok {
		// Plugin doesn't handle post-auth, return unmodified
		return &pb.PluginResponse{Modified: false}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Request.Context)
	return handler.HandlePostAuth(pluginCtx, req)
}

// OnBeforeWriteHeaders implements pb.PluginServiceServer
func (w *pluginServerWrapper) OnBeforeWriteHeaders(ctx context.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	// Check if plugin implements ResponseHandler
	handler, ok := w.plugin.(ResponseHandler)
	if !ok {
		// Plugin doesn't handle responses, return unmodified
		return &pb.HeadersResponse{Modified: false}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.OnBeforeWriteHeaders(pluginCtx, req)
}

// OnBeforeWrite implements pb.PluginServiceServer
func (w *pluginServerWrapper) OnBeforeWrite(ctx context.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	// Check if plugin implements ResponseHandler
	handler, ok := w.plugin.(ResponseHandler)
	if !ok {
		// Plugin doesn't handle responses, return unmodified
		return &pb.ResponseWriteResponse{Modified: false}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.OnBeforeWrite(pluginCtx, req)
}

// OnStreamComplete implements pb.PluginServiceServer
// This is called after a streaming response has finished, providing the accumulated response.
func (w *pluginServerWrapper) OnStreamComplete(ctx context.Context, req *pb.StreamCompleteRequest) (*pb.StreamCompleteResponse, error) {
	// Check if plugin implements StreamCompleteHandler
	handler, ok := w.plugin.(StreamCompleteHandler)
	if !ok {
		// Plugin doesn't handle stream complete, return unhandled
		return &pb.StreamCompleteResponse{Handled: false}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.OnStreamComplete(pluginCtx, req)
}

// HandleProxyLog implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleProxyLog(ctx context.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	// Check if plugin implements DataCollector
	handler, ok := w.plugin.(DataCollector)
	if !ok {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleProxyLog(pluginCtx, req)
}

// HandleAnalytics implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleAnalytics(ctx context.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	// Check if plugin implements DataCollector
	handler, ok := w.plugin.(DataCollector)
	if !ok {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleAnalytics(pluginCtx, req)
}

// HandleBudgetUsage implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleBudgetUsage(ctx context.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	// Check if plugin implements DataCollector
	handler, ok := w.plugin.(DataCollector)
	if !ok {
		return &pb.DataCollectionResponse{
			Success: true,
			Handled: false,
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleBudgetUsage(pluginCtx, req)
}

// GetAsset implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	// Check if plugin provides UI
	provider, ok := w.plugin.(UIProvider)
	if !ok {
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: "plugin does not provide UI assets",
		}, nil
	}

	// Serve the asset
	data, mimeType, err := provider.GetAsset(req.AssetPath)
	if err != nil {
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetAssetResponse{
		Success:       true,
		Content:       data,
		MimeType:      mimeType,
		ContentLength: int64(len(data)),
	}, nil
}

// ListAssets implements pb.PluginServiceServer
func (w *pluginServerWrapper) ListAssets(ctx context.Context, req *pb.ListAssetsRequest) (*pb.ListAssetsResponse, error) {
	// Check if plugin provides UI
	provider, ok := w.plugin.(UIProvider)
	if !ok {
		return &pb.ListAssetsResponse{
			Success:      false,
			ErrorMessage: "plugin does not provide UI assets",
		}, nil
	}

	assets, err := provider.ListAssets(req.PathPrefix)
	if err != nil {
		return &pb.ListAssetsResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.ListAssetsResponse{
		Success: true,
		Assets:  assets,
	}, nil
}

// GetManifest implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	// Try UIProvider first
	if provider, ok := w.plugin.(UIProvider); ok {
		manifestBytes, err := provider.GetManifest()
		if err != nil {
			return &pb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &pb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Try AgentPlugin
	if agent, ok := w.plugin.(AgentPlugin); ok {
		manifestBytes, err := agent.GetManifest()
		if err != nil {
			return &pb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &pb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Try ManifestProvider (for non-UI plugins that still need installation metadata)
	if provider, ok := w.plugin.(ManifestProvider); ok {
		manifestBytes, err := provider.GetManifest()
		if err != nil {
			return &pb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &pb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	return &pb.GetManifestResponse{
		Success:      false,
		ErrorMessage: "plugin does not provide a manifest",
	}, nil
}

// Call implements pb.PluginServiceServer
func (w *pluginServerWrapper) Call(ctx context.Context, req *pb.CallRequest) (*pb.CallResponse, error) {
	// Check if plugin provides UI with RPC support
	provider, ok := w.plugin.(UIProvider)
	if !ok {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: "plugin does not support RPC calls",
		}, nil
	}

	// Set service broker ID if provided (for service API access)
	// In AI Studio, each RPC call may get a per-request broker ID that should
	// be used for service API calls during this RPC.
	if req.ServiceBrokerId != 0 && w.runtime == RuntimeStudio {
		stdlog.Printf("[Call] Setting ai_studio_sdk broker ID to %d for RPC method %s", req.ServiceBrokerId, req.Method)
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	}

	payload := []byte(req.Payload)

	// Call the plugin's RPC handler
	response, err := provider.HandleRPC(req.Method, payload)
	if err != nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.CallResponse{
		Success: true,
		Data:    string(response),
	}, nil
}

// PortalCall implements pb.PluginServiceServer
// This handles portal-scoped RPC calls (separate from admin Call) with user context.
func (w *pluginServerWrapper) PortalCall(ctx context.Context, req *pb.PortalCallRequest) (*pb.PortalCallResponse, error) {
	// Check if plugin provides portal RPC support
	provider, ok := w.plugin.(PortalUIProvider)
	if !ok {
		return &pb.PortalCallResponse{
			Success:      false,
			ErrorMessage: "plugin does not support portal RPC calls",
		}, nil
	}

	// Set service broker ID if provided (for service API access)
	if req.ServiceBrokerId != 0 && w.runtime == RuntimeStudio {
		stdlog.Printf("[PortalCall] Setting ai_studio_sdk broker ID to %d for portal RPC method %s", req.ServiceBrokerId, req.Method)
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	}

	// Build portal user context from proto
	var userCtx *PortalUserContext
	if req.UserContext != nil {
		userCtx = &PortalUserContext{
			UserID:   req.UserContext.UserId,
			Email:    req.UserContext.Email,
			Name:     req.UserContext.Name,
			IsAdmin:  req.UserContext.IsAdmin,
			Groups:   req.UserContext.Groups,
			Metadata: req.UserContext.Metadata,
		}
	} else {
		// Provide empty context if none passed (should not happen in practice)
		userCtx = &PortalUserContext{
			Metadata: make(map[string]string),
		}
	}

	payload := []byte(req.Payload)

	// Call the plugin's portal RPC handler
	response, err := provider.HandlePortalRPC(req.Method, payload, userCtx)
	if err != nil {
		return &pb.PortalCallResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.PortalCallResponse{
		Success: true,
		Data:    string(response),
	}, nil
}

// GetConfigSchema implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	// Check if plugin provides config schema
	provider, ok := w.plugin.(ConfigProvider)
	if !ok {
		// Return empty schema - plugin doesn't need configuration
		return &pb.GetConfigSchemaResponse{
			Success:    true,
			SchemaJson: "{}",
		}, nil
	}

	schemaBytes, err := provider.GetConfigSchema()
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

// HandleAgentMessage implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error {
	// Check if plugin implements AgentPlugin
	agent, ok := w.plugin.(AgentPlugin)
	if !ok {
		// Send error chunk
		return fmt.Errorf("plugin does not implement agent functionality")
	}

	return agent.HandleAgentMessage(req, stream)
}

// GetObjectHookRegistrations implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetObjectHookRegistrations(ctx context.Context, req *pb.GetObjectHookRegistrationsRequest) (*pb.GetObjectHookRegistrationsResponse, error) {
	// Check if plugin implements ObjectHookHandler
	handler, ok := w.plugin.(ObjectHookHandler)
	if !ok {
		// Plugin doesn't handle object hooks - return empty registrations
		return &pb.GetObjectHookRegistrationsResponse{
			Registrations: nil,
		}, nil
	}

	regs, err := handler.GetObjectHookRegistrations()
	if err != nil {
		return nil, fmt.Errorf("failed to get object hook registrations: %w", err)
	}

	return &pb.GetObjectHookRegistrationsResponse{
		Registrations: regs,
	}, nil
}

// HandleObjectHook implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleObjectHook(ctx context.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
	// Check if plugin implements ObjectHookHandler
	handler, ok := w.plugin.(ObjectHookHandler)
	if !ok {
		// Plugin doesn't handle object hooks - allow operation without modification
		return &pb.ObjectHookResponse{
			AllowOperation: true,
			Modified:       false,
		}, nil
	}

	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleObjectHook(pluginCtx, req)
}

// ExecuteScheduledTask implements pb.PluginServiceServer
func (w *pluginServerWrapper) ExecuteScheduledTask(ctx context.Context, req *pb.ExecuteScheduledTaskRequest) (*pb.ExecuteScheduledTaskResponse, error) {
	// Check if plugin implements SchedulerPlugin
	scheduler, ok := w.plugin.(SchedulerPlugin)
	if !ok {
		return &pb.ExecuteScheduledTaskResponse{
			Success:      false,
			ErrorMessage: "plugin does not implement SchedulerPlugin",
		}, fmt.Errorf("plugin does not implement SchedulerPlugin")
	}

	// Set service broker ID if provided (for service API access)
	if req.ServiceBrokerId != 0 && w.runtime == RuntimeStudio {
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	}

	// Build plugin context
	pluginCtx := w.createPluginContext(ctx, req.Context)

	// Convert protobuf schedule to SDK schedule
	var configMap map[string]interface{}
	if req.Schedule.ConfigJson != "" {
		// Parse JSON config
		if err := json.Unmarshal([]byte(req.Schedule.ConfigJson), &configMap); err != nil {
			return &pb.ExecuteScheduledTaskResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to parse schedule config: %v", err),
			}, fmt.Errorf("failed to parse schedule config: %w", err)
		}
	}

	schedule := &Schedule{
		ID:             req.Schedule.Id,
		Name:           req.Schedule.Name,
		Cron:           req.Schedule.Cron,
		Timezone:       req.Schedule.Timezone,
		Enabled:        req.Schedule.Enabled,
		TimeoutSeconds: int(req.Schedule.TimeoutSeconds),
		Config:         configMap,
	}

	// Execute scheduled task
	err := scheduler.ExecuteScheduledTask(pluginCtx, schedule)

	if err != nil {
		return &pb.ExecuteScheduledTaskResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil // Don't return gRPC error - just mark as failed
	}

	return &pb.ExecuteScheduledTaskResponse{
		Success: true,
	}, nil
}

// AcceptEdgePayload implements pb.PluginServiceServer
// This is called when a payload arrives from an edge (microgateway) instance
func (w *pluginServerWrapper) AcceptEdgePayload(ctx context.Context, req *pb.EdgePayloadRequest) (*pb.EdgePayloadResponse, error) {
	// Check if plugin implements EdgePayloadReceiver
	receiver, ok := w.plugin.(EdgePayloadReceiver)
	if !ok {
		// Plugin doesn't handle edge payloads
		return &pb.EdgePayloadResponse{
			Success: false,
			Handled: false,
			ErrorMessage: "plugin does not implement EdgePayloadReceiver",
		}, nil
	}

	// Set service broker ID if provided (for service API access)
	if req.ServiceBrokerId != 0 && w.runtime == RuntimeStudio {
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	}

	// Build plugin context
	pluginCtx := w.createPluginContext(ctx, req.Context)

	// Convert proto to SDK EdgePayload
	edgePayload := &EdgePayload{
		Payload:           req.Payload,
		EdgeID:            req.EdgeId,
		EdgeNamespace:     req.EdgeNamespace,
		CorrelationID:     req.CorrelationId,
		Metadata:          req.Metadata,
		EdgeTimestamp:     req.EdgeTimestamp,
		ReceivedTimestamp: req.ReceivedTimestamp,
	}

	// Call the plugin's AcceptEdgePayload handler
	handled, err := receiver.AcceptEdgePayload(pluginCtx, edgePayload)
	if err != nil {
		return &pb.EdgePayloadResponse{
			Success:      false,
			Handled:      handled,
			ErrorMessage: err.Error(),
		}, nil // Don't return gRPC error - mark as failed
	}

	return &pb.EdgePayloadResponse{
		Success: true,
		Handled: handled,
	}, nil
}

// GetEndpointRegistrations implements pb.PluginServiceServer
func (w *pluginServerWrapper) GetEndpointRegistrations(ctx context.Context, req *pb.GetEndpointRegistrationsRequest) (*pb.GetEndpointRegistrationsResponse, error) {
	handler, ok := w.plugin.(CustomEndpointHandler)
	if !ok {
		return &pb.GetEndpointRegistrationsResponse{}, nil
	}
	regs, err := handler.GetEndpointRegistrations()
	if err != nil {
		return nil, fmt.Errorf("get endpoint registrations: %w", err)
	}
	return &pb.GetEndpointRegistrationsResponse{Registrations: regs}, nil
}

// HandleEndpointRequest implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleEndpointRequest(ctx context.Context, req *pb.EndpointRequest) (*pb.EndpointResponse, error) {
	handler, ok := w.plugin.(CustomEndpointHandler)
	if !ok {
		return &pb.EndpointResponse{StatusCode: 501, ErrorMessage: "plugin does not implement custom endpoints"}, nil
	}
	pluginCtx := w.createPluginContext(ctx, req.Context)
	return handler.HandleEndpointRequest(pluginCtx, req)
}

// HandleEndpointRequestStream implements pb.PluginServiceServer
func (w *pluginServerWrapper) HandleEndpointRequestStream(req *pb.EndpointRequest, stream pb.PluginService_HandleEndpointRequestStreamServer) error {
	handler, ok := w.plugin.(CustomEndpointHandler)
	if !ok {
		return fmt.Errorf("plugin does not implement streaming custom endpoints")
	}
	pluginCtx := w.createPluginContext(stream.Context(), req.Context)
	return handler.HandleEndpointRequestStream(pluginCtx, req, stream)
}

// OpenSession implements pb.PluginServiceServer
// This is the key method for session-based broker management.
// It blocks until timeout or CloseSession is called, keeping the broker alive.
//
// The SDK automatically handles all broker setup - plugins don't need to do anything.
// Service APIs (ai_studio_sdk, microgateway SDK) will "just work" after this is called.
func (w *pluginServerWrapper) OpenSession(ctx context.Context, req *pb.OpenSessionRequest) (*pb.OpenSessionResponse, error) {
	// Generate a session ID
	sessionID := fmt.Sprintf("session-%d-%d", req.PluginId, req.ServiceBrokerId)

	stdlog.Printf("[OpenSession] Starting: sessionID=%s, pluginID=%d, brokerID=%d", sessionID, req.PluginId, req.ServiceBrokerId)

	// Get the stored broker and verify it's valid
	broker := GetStoredBroker()

	// Initialize session state
	firstSession, err := InitSession(sessionID, req.ServiceBrokerId, broker)
	if err != nil {
		stdlog.Printf("[OpenSession] InitSession failed: %v", err)
		return &pb.OpenSessionResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// ========================================================================
	// CRITICAL: Set broker IDs for ALL SDKs so service APIs work automatically
	// This is the key to good DX - plugins don't need to know about broker IDs
	// ========================================================================

	// Set event service broker ID
	SetEventServiceBrokerID(req.ServiceBrokerId)

	// Set AI Studio SDK broker ID (for studio runtime)
	ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	ai_studio_sdk.SetPluginID(req.PluginId)

	// Set Microgateway SDK broker ID (for gateway runtime)
	// This is done via the build-tag-controlled functions
	setBrokerIDForMicrogatewaySDK(req.ServiceBrokerId)
	setPluginIDForMicrogatewaySDK(req.PluginId)

	stdlog.Printf("[OpenSession] SDK broker IDs configured: brokerID=%d, pluginID=%d", req.ServiceBrokerId, req.PluginId)

	// If this is the first session, notify plugin if it implements SessionAware (optional)
	// IMPORTANT: Run this in a goroutine because:
	// 1. OnSessionReady may call broker.Dial() for event subscriptions
	// 2. broker.Dial() blocks until the broker server accepts the connection
	// 3. The broker server can't accept new connections while we're blocking in this RPC handler
	// By running async, we allow this RPC to proceed to WaitForClose, which lets the broker serve.
	stdlog.Printf("[OpenSession] firstSession=%v, checking if plugin implements SessionAware", firstSession)
	stdlog.Printf("[OpenSession] Plugin type: %T", w.plugin)
	if firstSession {
		if aware, ok := w.plugin.(SessionAware); ok {
			stdlog.Printf("[OpenSession] Plugin implements SessionAware (type assertion succeeded), will call OnSessionReady in 500ms")
			pluginCtx := w.createPluginContext(ctx, nil)
			go func() {
				// Delay to ensure:
				// 1. This RPC handler returns and WaitForClose can proceed
				// 2. The host's AcceptAndServe has time to start listening
				// The host starts AcceptAndServe before calling OpenSession, but the
				// listener setup is async. 500ms should be sufficient.
				time.Sleep(500 * time.Millisecond)
				stdlog.Printf("[OpenSession] Calling OnSessionReady now")
				aware.OnSessionReady(pluginCtx)
				stdlog.Printf("[OpenSession] OnSessionReady returned")
			}()
		} else {
			stdlog.Printf("[OpenSession] Plugin does NOT implement SessionAware interface")
		}
	}

	// Block until timeout or CloseSession
	// This is where the broker becomes available for dial operations
	reason := WaitForClose(req.TimeoutMs)

	// Map reason to proto enum
	closeReason := pb.OpenSessionResponse_UNKNOWN
	switch reason {
	case "timeout":
		closeReason = pb.OpenSessionResponse_TIMEOUT
	case "explicit_close", "shutdown", "unload":
		closeReason = pb.OpenSessionResponse_EXPLICIT_CLOSE
	default:
		if reason != "" {
			closeReason = pb.OpenSessionResponse_PLUGIN_ERROR
		}
	}

	return &pb.OpenSessionResponse{
		Success:     true,
		SessionId:   sessionID,
		CloseReason: closeReason,
	}, nil
}

// CloseSession implements pb.PluginServiceServer
// This explicitly closes an active session before timeout.
func (w *pluginServerWrapper) CloseSession(ctx context.Context, req *pb.CloseSessionRequest) (*pb.CloseSessionResponse, error) {
	// Notify plugin if it implements SessionAware
	if aware, ok := w.plugin.(SessionAware); ok {
		pluginCtx := w.createPluginContext(ctx, nil)
		aware.OnSessionClosing(pluginCtx)
	}

	// Close the session
	reason := req.Reason
	if reason == "" {
		reason = "explicit_close"
	}
	CloseSession(reason)

	return &pb.CloseSessionResponse{
		Success: true,
	}, nil
}
