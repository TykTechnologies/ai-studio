package plugin_sdk

import (
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/grpc"
)

// Capability interfaces define optional functionality that plugins can implement.
// Plugins only need to implement the capabilities they require.

// PreAuthHandler processes requests before authentication.
// Use this when you need to modify or block requests before they reach the auth layer.
type PreAuthHandler interface {
	Plugin
	HandlePreAuth(ctx Context, req *pb.PluginRequest) (*pb.PluginResponse, error)
}

// AuthHandler performs custom authentication.
// Use this when you need to implement a custom authentication scheme.
type AuthHandler interface {
	Plugin
	HandleAuth(ctx Context, req *pb.AuthRequest) (*pb.AuthResponse, error)

	// GetAppByCredential retrieves app information during authentication.
	// This is called by the auth system to validate app credentials.
	GetAppByCredential(ctx Context, credential string) (*pb.App, error)

	// GetUserByCredential retrieves user information during authentication.
	// This is called by the auth system to validate user credentials.
	GetUserByCredential(ctx Context, credential string) (*pb.User, error)
}

// PostAuthHandler processes requests after authentication.
// This is the most common hook for implementing request policies like rate limiting,
// content filtering, or request enrichment.
type PostAuthHandler interface {
	Plugin
	HandlePostAuth(ctx Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error)
}

// ResponseHandler modifies response headers and body before sending to client.
// Use this when you need to transform responses or add custom headers.
type ResponseHandler interface {
	Plugin

	// OnBeforeWriteHeaders is called before response headers are sent to the client.
	// Use this to modify or add response headers.
	OnBeforeWriteHeaders(ctx Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error)

	// OnBeforeWrite is called before response body is sent to the client.
	// This is called for both regular responses and streaming chunks.
	OnBeforeWrite(ctx Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error)
}

// StreamCompleteHandler processes completed streaming responses.
// Use this when you need to cache, analyze, or process the full response after
// all streaming chunks have been received and sent to the client.
// This is called asynchronously after the streaming response completes.
type StreamCompleteHandler interface {
	Plugin

	// OnStreamComplete is called after a streaming response has finished.
	// The accumulated_response contains all SSE chunks concatenated together.
	// This can be used to cache the complete response for future requests.
	OnStreamComplete(ctx Context, req *pb.StreamCompleteRequest) (*pb.StreamCompleteResponse, error)
}

// DataCollector collects telemetry data for analytics, monitoring, or billing.
// Use this when you need to export data to external systems.
type DataCollector interface {
	Plugin

	// HandleProxyLog processes raw proxy request/response logs.
	HandleProxyLog(ctx Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error)

	// HandleAnalytics processes token usage and cost analytics.
	HandleAnalytics(ctx Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error)

	// HandleBudgetUsage processes budget consumption data.
	HandleBudgetUsage(ctx Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error)
}

// UIProvider serves web UI assets for the AI Studio admin interface.
// This is only relevant for Studio plugins that have a management UI.
type UIProvider interface {
	Plugin

	// GetAsset serves a static asset file (JS, CSS, images, etc.).
	// assetPath is relative to the plugin's asset root (e.g., "ui/webc/dashboard.js").
	// Returns: (content []byte, mimeType string, error)
	GetAsset(assetPath string) ([]byte, string, error)

	// ListAssets returns a list of all available assets.
	// pathPrefix can be used to filter assets (e.g., "ui/").
	ListAssets(pathPrefix string) ([]*pb.AssetInfo, error)

	// GetManifest returns the plugin manifest as JSON bytes.
	// The manifest declares UI components, permissions, and metadata.
	GetManifest() ([]byte, error)

	// HandleRPC processes custom RPC method calls from the UI.
	// method: The RPC method name (e.g., "get_statistics", "update_settings")
	// payload: JSON payload as bytes from the frontend
	// Returns: JSON response as bytes
	HandleRPC(method string, payload []byte) ([]byte, error)
}

// PortalUserContext provides information about the authenticated portal user.
// This is passed to plugins on portal RPC calls so they can make authorization decisions.
type PortalUserContext struct {
	UserID   uint32
	Email    string
	Name     string
	IsAdmin  bool
	Groups   []string
	Metadata map[string]string
}

// PortalUIProvider serves portal-facing UI pages and handles portal RPC calls.
// This is SEPARATE from UIProvider to enforce security boundaries between admin and portal contexts.
// Plugins can implement both UIProvider (for admin UI) and PortalUIProvider (for portal UI).
//
// Note: PortalUIProvider does not include GetAsset/GetManifest - those come from UIProvider.
// A plugin that wants portal UI must implement both UIProvider (for assets/manifest) and
// PortalUIProvider (for portal RPC).
type PortalUIProvider interface {
	Plugin

	// HandlePortalRPC processes RPC calls from the portal (end-user) UI.
	// This is SEPARATE from HandleRPC which only handles admin UI calls.
	// method: The RPC method name (e.g., "submit_feedback", "get_user_data")
	// payload: JSON payload as bytes from the portal frontend
	// userCtx: Authenticated portal user context (never nil)
	// Returns: JSON response as bytes
	HandlePortalRPC(method string, payload []byte, userCtx *PortalUserContext) ([]byte, error)
}

// ConfigProvider provides configuration schema for the plugin.
// This is used by the admin UI to generate configuration forms.
type ConfigProvider interface {
	Plugin

	// GetConfigSchema returns the JSON Schema for plugin configuration.
	// The schema should follow jsonschema.org format.
	GetConfigSchema() ([]byte, error)
}

// ManifestProvider provides a plugin manifest without requiring full UI capabilities.
// This is useful for gateway-only plugins that need to be installed via Studio
// but don't have a UI component.
type ManifestProvider interface {
	Plugin

	// GetManifest returns the plugin manifest as JSON bytes.
	// The manifest declares hooks, permissions, and metadata.
	GetManifest() ([]byte, error)
}

// AgentPlugin handles conversational AI agent interactions.
// This is for plugins that implement custom agent behavior.
type AgentPlugin interface {
	Plugin

	// HandleAgentMessage processes a user message and streams responses back.
	// req contains: user message, available LLMs/tools/datasources, conversation history
	// stream is used to send back content chunks, tool calls, thinking, errors, etc.
	// The agent should call stream.Send() for each chunk and end with a DONE chunk.
	HandleAgentMessage(req *pb.AgentMessageRequest, stream grpc.ServerStreamingServer[pb.AgentMessageChunk]) error

	// GetManifest returns the plugin manifest as JSON bytes.
	// The manifest declares permissions, scopes, and metadata.
	GetManifest() ([]byte, error)
}

// ObjectHookHandler intercepts CRUD operations on AI Studio objects (LLMs, Datasources, Tools, Users).
// Use this to implement custom validation, enrichment, or integration with external systems.
// Hooks are executed in priority order, and "before_*" hooks can reject operations.
type ObjectHookHandler interface {
	Plugin

	// GetObjectHookRegistrations declares which object operations this plugin wants to handle.
	// Returns a list of registrations specifying object types, hook types, and execution priority.
	GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error)

	// HandleObjectHook processes a single object hook invocation.
	// req contains: hook type, object type, object data (JSON), user ID, operation ID
	// Returns: allow/reject decision, optional modified object, plugin metadata
	HandleObjectHook(ctx Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error)
}

// SchedulerPlugin allows plugins to execute tasks on a cron-based schedule.
// Use this when you need periodic background tasks like data synchronization,
// cleanup operations, or scheduled processing.
type SchedulerPlugin interface {
	Plugin

	// ExecuteScheduledTask is called when a scheduled task needs to run.
	// ctx provides access to services (KV, logging) but has no request context.
	// schedule contains the schedule definition including ID, name, cron expression, and custom config.
	// Returns error if execution failed (will be recorded in execution history).
	ExecuteScheduledTask(ctx Context, schedule *Schedule) error
}

// Schedule represents a cron-based task definition.
type Schedule struct {
	ID             string                 // Unique identifier from manifest
	Name           string                 // Human-readable name
	Cron           string                 // Cron expression (e.g., "0 * * * *")
	Timezone       string                 // Timezone for cron evaluation (e.g., "America/New_York", "UTC")
	Enabled        bool                   // Whether schedule is currently enabled
	TimeoutSeconds int                    // Maximum execution time in seconds
	Config         map[string]interface{} // Schedule-specific configuration from manifest
}

// CustomEndpointHandler allows plugins to register and serve custom HTTP endpoints.
// Endpoints are mounted under /plugins/{slug}/ on the microgateway.
// Each endpoint registration must explicitly declare whether authentication is required.
//
// Plugins can serve both unary responses (HandleEndpointRequest) and streaming
// responses (HandleEndpointRequestStream) — the latter enables SSE and MCP
// Streamable HTTP support. Set stream_response=true on an EndpointRegistration
// to indicate the gateway should use the streaming path.
//
// Most plugins should register a single "/*" catch-all and handle routing internally
// using the pre-split path_segments field on EndpointRequest.
//
// This capability is supported in Gateway runtime. AI Studio support may be added later.
type CustomEndpointHandler interface {
	Plugin

	// GetEndpointRegistrations declares which HTTP endpoints this plugin serves.
	// Called after Initialize() and again on plugin reload.
	GetEndpointRegistrations() ([]*pb.EndpointRegistration, error)

	// HandleEndpointRequest handles a non-streaming HTTP request.
	// Returns a complete HTTP response (status, headers, body).
	HandleEndpointRequest(ctx Context, req *pb.EndpointRequest) (*pb.EndpointResponse, error)

	// HandleEndpointRequestStream handles a streaming HTTP request (SSE, MCP Streamable HTTP).
	// Plugin sends EndpointResponseChunks: first HEADERS, then BODY chunks, finally DONE.
	// The gateway flushes each BODY chunk to the HTTP client immediately.
	// Plugins that only serve non-streaming endpoints can return an Unimplemented error.
	HandleEndpointRequestStream(ctx Context, req *pb.EndpointRequest, stream grpc.ServerStreamingServer[pb.EndpointResponseChunk]) error
}

// EdgePayloadReceiver handles payloads sent from edge (microgateway) instances.
// Use this when you need to receive data from plugins running on edge instances
// that are connected to AI Studio via the hub-and-spoke architecture.
//
// Example use case: An llm-cache plugin running on edge instances sends cache
// statistics or shared cache data back to a central plugin on AI Studio.
type EdgePayloadReceiver interface {
	Plugin

	// AcceptEdgePayload is called when a payload arrives from an edge instance.
	// payload contains the raw data sent by the edge plugin via SendToControl().
	// edgeID identifies which edge instance sent the payload.
	// correlationID can be used for request/response matching or tracking.
	// metadata contains any additional key-value data from the edge plugin.
	//
	// Returns:
	//   - handled: true if this plugin processed the payload
	//   - error: non-nil if processing failed
	AcceptEdgePayload(ctx Context, payload *EdgePayload) (handled bool, err error)
}

// EdgePayload represents data sent from an edge plugin to a control plane plugin
type EdgePayload struct {
	Payload           []byte            // Arbitrary payload data from edge plugin
	EdgeID            string            // Edge instance that sent the payload
	EdgeNamespace     string            // Namespace of the edge instance
	CorrelationID     string            // Optional correlation ID for tracking
	Metadata          map[string]string // Optional key-value metadata
	EdgeTimestamp     int64             // Unix timestamp when generated at edge
	ReceivedTimestamp int64             // Unix timestamp when received at control
}

// SessionAware is an optional interface for plugins that need session lifecycle callbacks.
// Implement this when your plugin needs to set up or tear down resources tied to the
// session-based broker connection (e.g., event subscriptions, background tasks).
//
// The session pattern keeps go-plugin broker connections alive for plugins that need
// persistent background services like event pub/sub.
//
// Example:
//
//	func (p *MyPlugin) OnSessionReady(ctx Context) {
//	    // Subscribe to events, start background tasks
//	    p.eventSubID, _ = ctx.Services.Events().Subscribe("my-topic", p.handleEvent)
//	}
//
//	func (p *MyPlugin) OnSessionClosing(ctx Context) {
//	    // Cleanup subscriptions before session ends
//	    ctx.Services.Events().Unsubscribe(p.eventSubID)
//	}
type SessionAware interface {
	// OnSessionReady is called when a session is first established.
	// This is the place to set up event subscriptions, start background goroutines,
	// or initialize resources that require a live broker connection.
	// This is only called once per plugin lifetime (on the first OpenSession),
	// not on session renewals (subsequent OpenSession calls after timeout).
	OnSessionReady(ctx Context)

	// OnSessionClosing is called before a session is explicitly closed.
	// This is NOT called on session timeout (timeouts expect the host to re-open).
	// Use this to clean up resources before the plugin shuts down.
	OnSessionClosing(ctx Context)
}

// HookType represents the type of plugin hook (for gateway compatibility)
type HookType string

const (
	HookTypePreAuth        HookType = "pre_auth"
	HookTypeAuth           HookType = "auth"
	HookTypePostAuth       HookType = "post_auth"
	HookTypeResponse       HookType = "response"
	HookTypeDataCollection HookType = "data_collection"
)

// GetHookType returns the primary hook type for a plugin based on its capabilities.
// This is used for gateway compatibility to determine plugin placement in the chain.
func GetHookType(p Plugin) HookType {
	// Check capabilities in order of precedence
	if _, ok := p.(PreAuthHandler); ok {
		return HookTypePreAuth
	}
	if _, ok := p.(AuthHandler); ok {
		return HookTypeAuth
	}
	if _, ok := p.(PostAuthHandler); ok {
		return HookTypePostAuth
	}
	if _, ok := p.(ResponseHandler); ok {
		return HookTypeResponse
	}
	if _, ok := p.(DataCollector); ok {
		return HookTypeDataCollection
	}

	// Default to post-auth if no capability detected
	return HookTypePostAuth
}
