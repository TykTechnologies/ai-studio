package plugin_sdk

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

// RuntimeType indicates where the plugin is running
type RuntimeType string

const (
	RuntimeStudio  RuntimeType = "studio"  // Running in AI Studio
	RuntimeGateway RuntimeType = "gateway" // Running in Microgateway
)

// Context provides runtime information and services to the plugin.
// This is passed to most plugin methods to give access to the host environment.
type Context struct {
	// Runtime indicates whether this is Studio or Gateway
	Runtime RuntimeType

	// RequestID uniquely identifies this request (if applicable)
	RequestID string

	// AppID is the application making the request (if known)
	AppID uint32

	// UserID is the user making the request (if known)
	UserID uint32

	// LLMID is the LLM being accessed (if applicable)
	LLMID uint32

	// LLMSlug is the LLM slug/identifier (if applicable)
	LLMSlug string

	// Vendor is the LLM vendor (e.g., "anthropic", "openai")
	Vendor string

	// EdgeID is the edge instance identifier (only set in RuntimeGateway)
	// Empty string in RuntimeStudio context
	EdgeID string

	// EdgeNamespace is the edge namespace (only set in RuntimeGateway)
	// Empty string in RuntimeStudio context
	EdgeNamespace string

	// Metadata provides additional context as key-value pairs
	Metadata map[string]string

	// TraceContext provides distributed tracing context
	TraceContext map[string]string

	// Services provides access to host services (KV storage, logging, app management)
	Services ServiceBroker

	// Internal context for cancellation and timeouts
	context.Context
}

// ServiceBroker provides access to host services.
// Services are accessed via runtime-specific methods for clarity and type safety.
type ServiceBroker interface {
	// KV returns the key-value storage service (works in both contexts)
	KV() KVService

	// Logger returns the logging service (works in both contexts)
	Logger() LogService

	// Gateway returns Gateway-specific services (only available in RuntimeGateway)
	// Returns nil if called in Studio context
	Gateway() GatewayServices

	// Studio returns Studio-specific services (only available in RuntimeStudio)
	// Returns nil if called in Gateway context
	Studio() StudioServices

	// Events returns the event pub/sub service (works in both contexts)
	// Allows plugins to publish and subscribe to events that flow across
	// edge/control boundaries via the event bridge system.
	Events() EventService
}

// KVService provides key-value storage for plugin data.
// In Studio: Uses PostgreSQL-backed storage shared across all hosts.
// In Gateway: Uses local database storage per gateway instance.
type KVService interface {
	// Read retrieves a value by key
	// Returns error if key doesn't exist or has expired
	Read(ctx context.Context, key string) ([]byte, error)

	// Write stores a value with the given key and optional expiration
	// expireAt is optional - pass nil for no expiration
	// Returns whether a new key was created (true) or existing updated (false)
	Write(ctx context.Context, key string, value []byte, expireAt *time.Time) (bool, error)

	// WriteWithTTL stores a value with a TTL (time-to-live)
	// Expiration is calculated as time.Now().Add(ttl)
	// Returns whether a new key was created (true) or existing updated (false)
	WriteWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Delete removes a key
	// Returns whether the key existed and was deleted
	Delete(ctx context.Context, key string) (bool, error)

	// List returns all keys with the given prefix
	// Returns empty slice if no keys match
	List(ctx context.Context, prefix string) ([]string, error)
}

// LogService provides structured logging
type LogService interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// GatewayServices provides access to Microgateway-specific services
// Only available when Runtime == RuntimeGateway
type GatewayServices interface {
	// GetApp retrieves app details from local Gateway database
	// Returns microgateway_management.GetAppResponse proto type
	GetApp(ctx context.Context, appID uint32) (interface{}, error)

	// ListApps lists apps from local Gateway database
	ListApps(ctx context.Context, page, limit int32, isActive *bool) (interface{}, error)

	// GetLLM retrieves LLM details from local Gateway database
	GetLLM(ctx context.Context, llmID uint32) (interface{}, error)

	// ListLLMs lists LLMs from local Gateway database
	ListLLMs(ctx context.Context, page, limit int32, vendor string, isActive *bool) (interface{}, error)

	// GetBudgetStatus retrieves budget status (Gateway-only feature)
	GetBudgetStatus(ctx context.Context, appID uint32, llmID *uint32) (interface{}, error)

	// GetModelPrice retrieves model pricing from local Gateway database
	GetModelPrice(ctx context.Context, modelName, vendor string) (interface{}, error)

	// ListModelPrices lists model prices from local Gateway database
	ListModelPrices(ctx context.Context, vendor string) (interface{}, error)

	// ValidateCredential validates an API credential (Gateway-only feature)
	ValidateCredential(ctx context.Context, secret string) (interface{}, error)

	// SendToControl queues a payload to be sent to the AI Studio control plane
	// This is used by plugins running on edge (microgateway) instances to send
	// data back to the control plane for aggregation or processing.
	// Returns the number of payloads pending in the queue.
	SendToControl(ctx context.Context, payload []byte, correlationID string, metadata map[string]string) (int64, error)

	// SendToControlJSON is a convenience method that JSON-encodes a value and sends it to control
	SendToControlJSON(ctx context.Context, value interface{}, correlationID string, metadata map[string]string) (int64, error)
}

// StudioServices provides access to AI Studio-specific services
// Only available when Runtime == RuntimeStudio
type StudioServices interface {
	// GetApp retrieves app details from AI Studio database
	// Returns ai_studio_management.GetAppResponse proto type
	GetApp(ctx context.Context, appID uint32) (interface{}, error)

	// ListApps lists apps from AI Studio database
	ListApps(ctx context.Context, page, limit int32) (interface{}, error)

	// UpdateAppWithMetadata updates app configuration including metadata
	UpdateAppWithMetadata(ctx context.Context, appID uint32, name, description string, isActive bool, llmIDs, toolIDs, datasourceIDs []uint32, monthlyBudget *float64, metadata string) (interface{}, error)

	// GetLLM retrieves LLM details from AI Studio database
	GetLLM(ctx context.Context, llmID uint32) (interface{}, error)

	// ListLLMs lists LLMs from AI Studio database
	ListLLMs(ctx context.Context, page, limit int32) (interface{}, error)

	// ListTools lists tools (Studio-only feature)
	ListTools(ctx context.Context, page, limit int32) (interface{}, error)

	// CallLLM proxies LLM requests (for agent plugins, Studio-only)
	CallLLM(ctx context.Context, llmID uint32, model string, messages interface{}, temperature float64, maxTokens int32) (interface{}, error)
}

// detectRuntime determines the runtime environment from environment variables
func detectRuntime() RuntimeType {
	// Check for explicit runtime setting (takes precedence)
	if runtime := os.Getenv("PLUGIN_RUNTIME"); runtime != "" {
		if runtime == "studio" {
			return RuntimeStudio
		}
		if runtime == "gateway" {
			return RuntimeGateway
		}
	}

	// IMPORTANT: GATEWAY_MODE=control means AI Studio is running in control/hub mode
	// This does NOT mean plugins should use Gateway runtime
	// Only detect Gateway runtime if we're actually IN the microgateway process
	gatewayMode := os.Getenv("GATEWAY_MODE")
	if gatewayMode == "edge" || gatewayMode == "standalone_gateway" {
		// Only these modes indicate we're in actual microgateway process
		return RuntimeGateway
	}

	// Legacy check for MICROGATEWAY_MODE (used by old standalone gateway)
	if os.Getenv("MICROGATEWAY_MODE") == "standalone" {
		return RuntimeGateway
	}

	// Default to studio (including when GATEWAY_MODE=control)
	return RuntimeStudio
}

// NewContext creates a new plugin context with the given parameters
func NewContext(baseCtx context.Context, services ServiceBroker, requestID string, appID, userID, llmID uint32, llmSlug, vendor string, metadata, traceContext map[string]string) Context {
	return Context{
		Runtime:      detectRuntime(),
		RequestID:    requestID,
		AppID:        appID,
		UserID:       userID,
		LLMID:        llmID,
		LLMSlug:      llmSlug,
		Vendor:       vendor,
		Metadata:     metadata,
		TraceContext: traceContext,
		Services:     services,
		Context:      baseCtx,
	}
}

// ============================================================================
// Event Service Types
// ============================================================================

// Direction controls event routing and prevents feedback loops.
// Events are routed based on their direction:
// - DirLocal: Events stay on the local bus, never forwarded
// - DirUp: Events flow from edge to control
// - DirDown: Events flow from control to edge(s)
type Direction int

const (
	// DirLocal indicates events that should only be processed locally
	DirLocal Direction = iota
	// DirUp indicates events flowing from edge to control
	DirUp
	// DirDown indicates events flowing from control to edge(s)
	DirDown
)

// String returns a human-readable representation of the Direction.
func (d Direction) String() string {
	switch d {
	case DirLocal:
		return "local"
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	default:
		return "unknown"
	}
}

// DirectionFromInt32 converts an int32 to Direction.
func DirectionFromInt32(d int32) Direction {
	switch d {
	case 0:
		return DirLocal
	case 1:
		return DirUp
	case 2:
		return DirDown
	default:
		return DirLocal
	}
}

// Event represents an event received from the event bus.
type Event struct {
	// ID is a UUID for deduplication and tracing
	ID string

	// Topic is the logical topic name (e.g., "config.update", "metrics.report")
	Topic string

	// Origin is the node/plugin ID that created the event
	Origin string

	// Dir is the direction (informational - when received, always treated as local)
	Dir Direction

	// Payload is the application-specific data as JSON
	Payload json.RawMessage
}

// EventHandler is a callback function invoked when an event is received.
type EventHandler func(event Event)

// EventService provides pub/sub capabilities for plugins.
// This service allows plugins to publish events to the event bus and
// subscribe to receive events. Events can flow across node boundaries
// (edge ↔ control) based on their direction.
type EventService interface {
	// Publish sends an event to the event bus.
	// topic: logical topic name (e.g., "cache.hit", "config.reload")
	// payload: arbitrary data (will be JSON-encoded)
	// dir: routing direction:
	//   - DirLocal: stays on local bus only
	//   - DirUp: forwarded from edge to control
	//   - DirDown: forwarded from control to edge(s)
	Publish(ctx context.Context, topic string, payload interface{}, dir Direction) error

	// PublishRaw sends an event with a pre-encoded JSON payload.
	PublishRaw(ctx context.Context, topic string, payload []byte, dir Direction) error

	// Subscribe registers a handler for events on a specific topic.
	// Returns a subscription ID that can be used to unsubscribe.
	// The handler is called asynchronously for each matching event.
	Subscribe(topic string, handler EventHandler) (string, error)

	// SubscribeAll registers a handler for all events regardless of topic.
	// Returns a subscription ID that can be used to unsubscribe.
	SubscribeAll(handler EventHandler) (string, error)

	// Unsubscribe removes a subscription by ID.
	// After this call, the handler will no longer receive events.
	Unsubscribe(subscriptionID string) error
}
