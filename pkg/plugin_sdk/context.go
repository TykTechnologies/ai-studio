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

	// StoreApp writes an App from the control plane into the local gateway database.
	// Used by auth plugins that need to provision apps dynamically without waiting
	// for periodic config sync. Uses upsert semantics (creates if not exists, updates if exists).
	// The request should contain the full app configuration including resource associations.
	// Requires "apps.write" scope in the plugin manifest.
	StoreApp(ctx context.Context, appID uint32, name, description string, isActive bool,
		userID uint32, metadata string, namespace string,
		llmIDs, toolIDs, datasourceIDs []uint32) error

	// SendToControl queues a payload to be sent to the AI Studio control plane
	// This is used by plugins running on edge (microgateway) instances to send
	// data back to the control plane for aggregation or processing.
	// Returns the number of payloads pending in the queue.
	SendToControl(ctx context.Context, payload []byte, correlationID string, metadata map[string]string) (int64, error)

	// SendToControlJSON is a convenience method that JSON-encodes a value and sends it to control
	SendToControlJSON(ctx context.Context, value interface{}, correlationID string, metadata map[string]string) (int64, error)
}

// ListAppsOptions provides optional filters for listing apps
type ListAppsOptions struct {
	IsActive  *bool
	Namespace string
	UserID    *uint32
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

	// PatchAppMetadata atomically updates a single metadata key on an app.
	// To set a key: PatchAppMetadata(ctx, appID, "mykey", `"myvalue"`, false)
	// To delete a key: PatchAppMetadata(ctx, appID, "mykey", "", true)
	// Returns the full metadata JSON string after the operation.
	PatchAppMetadata(ctx context.Context, appID uint32, key, value string, deleteKey bool) (string, error)

	// ListAppsWithFilters lists apps with optional filtering by user_id, namespace, and is_active
	ListAppsWithFilters(ctx context.Context, page, limit int32, opts *ListAppsOptions) (interface{}, error)

	// GetLLM retrieves LLM details from AI Studio database
	GetLLM(ctx context.Context, llmID uint32) (interface{}, error)

	// ListLLMs lists LLMs from AI Studio database
	ListLLMs(ctx context.Context, page, limit int32) (interface{}, error)

	// ListTools lists tools (Studio-only feature)
	ListTools(ctx context.Context, page, limit int32) (interface{}, error)

	// CallLLM proxies LLM requests (for agent plugins, Studio-only)
	CallLLM(ctx context.Context, llmID uint32, model string, messages interface{}, temperature float64, maxTokens int32) (interface{}, error)

	// UpdatePluginConfig updates the plugin's own configuration in the database
	// configJSON should be a valid JSON string containing the full configuration
	// Returns success status and any error message
	UpdatePluginConfig(ctx context.Context, pluginID uint32, configJSON string) (bool, string, error)

	// UpdateLLMPlugins updates plugin associations for an LLM
	// If append is true, the plugin is added to existing associations; otherwise replaces all
	// Returns success status and the final list of plugin IDs
	UpdateLLMPlugins(ctx context.Context, llmID uint32, pluginIDs []uint32, append bool) (bool, string, []uint32, error)

	// ===== Governed Metadata (Enterprise) =====
	// objectType is "llm", "tool", "datasource" or "plugin_resource:<plugin_id>:<slug>".
	// A plugin may write "plugin_resource:self:<slug>" (see SelfResourceObjectType) for
	// its own resource types; Studio resolves it to the calling plugin.
	// objectID is the numeric ID as a string, or the plugin resource instance ID.
	//
	// Typical resource-provider lifecycle:
	//   create/update → SetObjectMetadata (honour ok=false when the schema enforces)
	//   render (admin) → GetResolvedMetadataSchema + GetObjectMetadata
	//   render (portal) → GetObjectMetadataForAudience(..., MetadataVisibilityPortal)
	//   delete → DeleteObjectMetadata

	// GetObjectMetadata returns the stored governed metadata values as JSON and whether a record exists.
	// Requires the metadata.read scope.
	GetObjectMetadata(ctx context.Context, objectType, objectID string) (valuesJSON string, found bool, err error)

	// GetObjectMetadataForAudience narrows the stored values to one audience
	// (MetadataVisibilityPortal or MetadataVisibilityGateway). For the portal it also
	// returns displayJSON: [{key,label,type,value}] with vocabulary labels and user
	// names resolved, i.e. exactly what the built-in portal shows end users.
	// Requires the metadata.read scope.
	GetObjectMetadataForAudience(ctx context.Context, objectType, objectID, visibility string) (valuesJSON string, displayJSON string, found bool, err error)

	// SetObjectMetadata validates and stores governed metadata. merge=true keeps existing keys.
	// Returns the stored values and the validation result as JSON. When an enforcing schema
	// rejects the values, err is nil and ok is false; inspect validationResultJSON for details.
	// Requires the metadata.write scope.
	SetObjectMetadata(ctx context.Context, objectType, objectID, valuesJSON string, merge bool) (ok bool, storedValuesJSON string, validationResultJSON string, err error)

	// DeleteObjectMetadata removes the governed metadata of an object. Call it when the
	// plugin deletes the resource instance so no orphaned record remains. Absent records succeed.
	// Requires the metadata.write scope.
	DeleteObjectMetadata(ctx context.Context, objectType, objectID string) error

	// GetResolvedMetadataSchema returns the merged schema for an object type: field
	// definitions, the draft-07 JSON Schema, the vocabularies those fields use and the
	// enforcement level. Enough to render the form in the plugin's own UI.
	// Requires the metadata.read scope.
	GetResolvedMetadataSchema(ctx context.Context, objectType string) (*ResolvedMetadataSchema, error)

	// ValidateObjectMetadata validates values without storing them.
	// Requires the metadata.read scope.
	ValidateObjectMetadata(ctx context.Context, objectType, valuesJSON string) (valid bool, enforced bool, resultJSON string, err error)
}

// Governed metadata audiences for StudioServices.GetObjectMetadataForAudience.
const (
	MetadataVisibilityAdmin   = "admin"   // every field
	MetadataVisibilityPortal  = "portal"  // fields flagged portal_visible
	MetadataVisibilityGateway = "gateway" // fields flagged gateway_visible
)

// Governed metadata enforcement levels reported by ResolvedMetadataSchema.
const (
	MetadataEnforcementAdvisory = "advisory"
	MetadataEnforcementEnforce  = "enforce"
)

// ResolvedMetadataSchema is the merged governed metadata schema for one object type.
// All payloads are JSON strings so plugins can hand them to their own UI unchanged.
type ResolvedMetadataSchema struct {
	// FieldsJSON is a JSON array of field definitions:
	// {key,label,description,type,required,severity,vocabulary_slug,pattern,min,max,
	//  max_length,warn_if_past,portal_visible,gateway_visible,order}.
	FieldsJSON string
	// JSONSchema is a draft-07 JSON Schema for the values object (vocabularies as enums).
	JSONSchema string
	// VocabulariesJSON maps vocabulary slug → [{value,label,description,deprecated}]
	// for every vocabulary referenced by FieldsJSON.
	VocabulariesJSON string
	// Enforcement is MetadataEnforcementAdvisory or MetadataEnforcementEnforce.
	Enforcement string
	// SchemaSlugs lists the schemas that contributed fields.
	SchemaSlugs []string
}

// HasFields reports whether any governed field applies to the object type.
func (r *ResolvedMetadataSchema) HasFields() bool {
	return r != nil && r.FieldsJSON != "" && r.FieldsJSON != "[]" && r.FieldsJSON != "null"
}

// IsEnforced reports whether an enforcing schema applies (hard errors block saves).
func (r *ResolvedMetadataSchema) IsEnforced() bool {
	return r != nil && r.Enforcement == MetadataEnforcementEnforce
}

// SelfResourceObjectType returns the governed metadata object type for one of the
// calling plugin's own resource types without needing the numeric plugin ID:
// "plugin_resource:self:<slug>". Studio resolves it on every management API call.
func SelfResourceObjectType(resourceTypeSlug string) string {
	return "plugin_resource:self:" + resourceTypeSlug
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
