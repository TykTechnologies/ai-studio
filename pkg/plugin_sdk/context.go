package plugin_sdk

import (
	"context"
	"os"

	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
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
// The actual implementation differs between Studio and Gateway,
// but the interface remains the same for plugin developers.
type ServiceBroker interface {
	// KV returns the key-value storage service
	KV() KVService

	// Logger returns the logging service
	Logger() LogService

	// AppManager returns the app management service (may be nil in some contexts)
	AppManager() AppManagerService
}

// KVService provides key-value storage for plugin data.
// In Studio: Uses PostgreSQL-backed storage shared across all hosts.
// In Gateway: Uses local database storage per gateway instance.
type KVService interface {
	// Read retrieves a value by key
	// Returns empty bytes if key doesn't exist
	Read(ctx context.Context, key string) ([]byte, error)

	// Write stores a value with the given key
	// Returns whether a new key was created (true) or existing updated (false)
	Write(ctx context.Context, key string, value []byte) (bool, error)

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

// AppManagerService provides access to application management.
// This is primarily available in Studio context. In Gateway, it may make
// remote calls back to Studio.
type AppManagerService interface {
	// GetApp retrieves app details by ID
	GetApp(ctx context.Context, appID uint32) (*mgmt.GetAppResponse, error)

	// ListApps lists all apps with pagination
	ListApps(ctx context.Context, page, limit int32) (*mgmt.ListAppsResponse, error)

	// UpdateApp updates app configuration
	UpdateApp(ctx context.Context, req *mgmt.UpdateAppRequest) (*mgmt.UpdateAppResponse, error)

	// ListLLMs lists available LLMs
	ListLLMs(ctx context.Context, page, limit int32) (*mgmt.ListLLMsResponse, error)
}

// detectRuntime determines the runtime environment from environment variables
func detectRuntime() RuntimeType {
	// Check for explicit runtime setting
	if runtime := os.Getenv("PLUGIN_RUNTIME"); runtime != "" {
		if runtime == "studio" {
			return RuntimeStudio
		}
		if runtime == "gateway" {
			return RuntimeGateway
		}
	}

	// Try to detect based on other env vars
	if os.Getenv("GATEWAY_MODE") != "" || os.Getenv("MICROGATEWAY_MODE") != "" {
		return RuntimeGateway
	}

	// Default to studio if ambiguous
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
