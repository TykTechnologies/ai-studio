package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/google/uuid"
)

// HookManager executes hooks for object CRUD operations
type HookManager struct {
	registry      *HookRegistry
	pluginManager *AIStudioPluginManager
	timeout       time.Duration // Default timeout per hook
}

// NewHookManager creates a new hook manager
func NewHookManager(registry *HookRegistry, pluginManager *AIStudioPluginManager) *HookManager {
	return &HookManager{
		registry:      registry,
		pluginManager: pluginManager,
		timeout:       5 * time.Second, // Default 5 second timeout
	}
}

// SetTimeout sets the timeout for hook execution
func (m *HookManager) SetTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// HookExecutionResult contains the result of executing all hooks for an operation
type HookExecutionResult struct {
	Allowed        bool                      // Whether operation should proceed
	RejectionReason string                   // Why operation was rejected (if any)
	ModifiedObject  interface{}              // Modified object (if any plugin modified it)
	Metadata       map[string]string         // Merged metadata from all plugins
	Executed       []string                   // List of plugins that executed
	Errors         []error                    // Non-fatal errors during execution
}

// ExecuteHooks executes all registered hooks for a specific operation
func (m *HookManager) ExecuteHooks(
	ctx context.Context,
	objectType ObjectType,
	hookType HookType,
	object interface{},
	userID uint32,
) (*HookExecutionResult, error) {
	// Get registered hooks
	hooks := m.registry.GetHooks(objectType, hookType)
	if len(hooks) == 0 {
		// No hooks registered - allow operation with no modifications
		return &HookExecutionResult{
			Allowed:  true,
			Metadata: make(map[string]string),
		}, nil
	}

	logger.Info(fmt.Sprintf("Executing hooks: object_type=%s hook_type=%s hook_count=%d", objectType, hookType, len(hooks)))

	result := &HookExecutionResult{
		Allowed:        true,
		ModifiedObject: object,
		Metadata:       make(map[string]string),
	}

	operationID := uuid.New().String()
	currentObject := object

	// Execute hooks in priority order
	for _, hookReg := range hooks {
		if !hookReg.Enabled {
			continue
		}

		// Get plugin client
		plugin, err := m.pluginManager.GetPlugin(uint(hookReg.PluginID))
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to get plugin for hook: plugin_id=%d error=%v", hookReg.PluginID, err))
			result.Errors = append(result.Errors, fmt.Errorf("plugin %d not available: %w", hookReg.PluginID, err))
			continue
		}

		// Execute hook with timeout
		hookCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		hookResult, err := m.executeHook(hookCtx, plugin, hookReg, currentObject, userID, operationID, hookType)
		if err != nil {
			logger.Error(fmt.Sprintf("Hook execution failed: plugin=%s error=%v", hookReg.PluginName, err))
			result.Errors = append(result.Errors, fmt.Errorf("hook %s failed: %w", hookReg.PluginName, err))
			continue
		}

		result.Executed = append(result.Executed, hookReg.PluginName)

		// Check if plugin rejected the operation (for "before_*" hooks)
		if !hookResult.AllowOperation {
			result.Allowed = false
			result.RejectionReason = hookResult.RejectionReason
			logger.Info(fmt.Sprintf("Hook rejected operation: plugin=%s reason=%s", hookReg.PluginName, hookResult.RejectionReason))
			return result, nil // Early termination
		}

		// If plugin modified the object, use the modified version
		if hookResult.Modified && hookResult.ModifiedObjectJson != "" {
			modifiedObj, err := m.unmarshalObject(objectType, hookResult.ModifiedObjectJson)
			if err != nil {
				logger.Error(fmt.Sprintf("Failed to unmarshal modified object: plugin=%s error=%v", hookReg.PluginName, err))
				result.Errors = append(result.Errors, fmt.Errorf("failed to unmarshal modified object: %w", err))
			} else {
				currentObject = modifiedObj
				result.ModifiedObject = modifiedObj
			}
		}

		// Merge plugin metadata
		if hookResult.PluginMetadata != nil {
			for k, v := range hookResult.PluginMetadata {
				// Prefix with plugin ID to avoid conflicts
				key := fmt.Sprintf("plugin_%d_%s", hookReg.PluginID, k)
				result.Metadata[key] = v
			}
		}

		// Log any messages from the plugin
		if hookResult.Message != "" {
			logger.Info(fmt.Sprintf("Hook message: plugin=%s message=%s", hookReg.PluginName, hookResult.Message))
		}
	}

	logger.Info(fmt.Sprintf("Hook execution completed: executed=%d errors=%d", len(result.Executed), len(result.Errors)))
	return result, nil
}

// executeHook executes a single hook
func (m *HookManager) executeHook(
	ctx context.Context,
	plugin *LoadedAIStudioPlugin,
	hookReg *HookRegistration,
	object interface{},
	userID uint32,
	operationID string,
	hookType HookType,
) (*pb.ObjectHookResponse, error) {
	// Serialize object to JSON
	objectJSON, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}

	// Create hook request
	req := &pb.ObjectHookRequest{
		HookType:    string(hookType),
		ObjectType:  string(hookReg.ObjectType),
		OperationId: operationID,
		UserId:      userID,
		PluginId:    hookReg.PluginID,
		ObjectJson:  string(objectJSON),
		Metadata:    make(map[string]string),
		Timestamp:   time.Now().Unix(),
	}

	// Call plugin
	resp, err := plugin.GRPCClient.HandleObjectHook(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC call failed: %w", err)
	}

	return resp, nil
}

// unmarshalObject unmarshals a JSON string into the appropriate object type
func (m *HookManager) unmarshalObject(objectType ObjectType, jsonStr string) (interface{}, error) {
	switch objectType {
	case ObjectTypeLLM:
		var llm models.LLM
		if err := json.Unmarshal([]byte(jsonStr), &llm); err != nil {
			return nil, err
		}
		return &llm, nil

	case ObjectTypeDatasource:
		var ds models.Datasource
		if err := json.Unmarshal([]byte(jsonStr), &ds); err != nil {
			return nil, err
		}
		return &ds, nil

	case ObjectTypeTool:
		var tool models.Tool
		if err := json.Unmarshal([]byte(jsonStr), &tool); err != nil {
			return nil, err
		}
		return &tool, nil

	case ObjectTypeUser:
		var user models.User
		if err := json.Unmarshal([]byte(jsonStr), &user); err != nil {
			return nil, err
		}
		return &user, nil

	default:
		return nil, fmt.Errorf("unknown object type: %s", objectType)
	}
}

// Helper function to sanitize sensitive fields before passing to hooks
func (m *HookManager) sanitizeObject(objectType ObjectType, object interface{}, hookType HookType) interface{} {
	// For "after_*" hooks, remove sensitive fields
	if hookType == HookAfterCreate || hookType == HookAfterUpdate || hookType == HookAfterDelete {
		switch objectType {
		case ObjectTypeLLM:
			if llm, ok := object.(*models.LLM); ok {
				sanitized := *llm
				sanitized.APIKey = "[REDACTED]"
				return &sanitized
			}

		case ObjectTypeDatasource:
			if ds, ok := object.(*models.Datasource); ok {
				sanitized := *ds
				sanitized.DBConnAPIKey = "[REDACTED]"
				sanitized.EmbedAPIKey = "[REDACTED]"
				sanitized.DBConnString = "[REDACTED]"
				return &sanitized
			}

		case ObjectTypeTool:
			if tool, ok := object.(*models.Tool); ok {
				sanitized := *tool
				sanitized.AuthKey = "[REDACTED]"
				return &sanitized
			}

		case ObjectTypeUser:
			if user, ok := object.(*models.User); ok {
				sanitized := *user
				sanitized.Password = "[REDACTED]"
				sanitized.APIKey = "[REDACTED]"
				sanitized.SessionToken = "[REDACTED]"
				sanitized.ResetToken = "[REDACTED]"
				sanitized.VerificationToken = "[REDACTED]"
				return &sanitized
			}
		}
	}

	return object
}

// MergeMetadata merges plugin metadata into the object's metadata field
func (m *HookManager) MergeMetadata(object interface{}, hookMetadata map[string]string) error {
	if len(hookMetadata) == 0 {
		return nil
	}

	switch obj := object.(type) {
	case *models.LLM:
		if obj.Metadata == nil {
			obj.Metadata = make(models.JSONMap)
		}
		for k, v := range hookMetadata {
			obj.Metadata[k] = v
		}

	case *models.Datasource:
		if obj.Metadata == nil {
			obj.Metadata = make(models.JSONMap)
		}
		for k, v := range hookMetadata {
			obj.Metadata[k] = v
		}

	case *models.Tool:
		if obj.Metadata == nil {
			obj.Metadata = make(models.JSONMap)
		}
		for k, v := range hookMetadata {
			obj.Metadata[k] = v
		}

	case *models.User:
		if obj.Metadata == nil {
			obj.Metadata = make(models.JSONMap)
		}
		for k, v := range hookMetadata {
			obj.Metadata[k] = v
		}

	default:
		return fmt.Errorf("unknown object type for metadata merge")
	}

	return nil
}
