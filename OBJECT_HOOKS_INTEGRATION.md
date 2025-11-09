# Object Hooks Integration Guide

## Overview

The Object Hooks system allows plugins to intercept and modify CRUD operations on core AI Studio objects (LLMs, Datasources, Tools, Users). This guide shows how to integrate hooks into services.

## Architecture

1. **Hook Registry** (`services/hook_registry.go`): Manages plugin hook registrations
2. **Hook Manager** (`services/hook_manager.go`): Executes hooks in priority order
3. **Plugin SDK** (`pkg/plugin_sdk/capabilities.go`): Provides `ObjectHookHandler` interface
4. **Proto Definitions** (`proto/plugin.proto`): Defines gRPC messages

## Service Integration Pattern

### Step 1: Initialize Hook System in Service

```go
// In services/service.go
type Service struct {
    // ... existing fields ...
    hookRegistry *HookRegistry
    hookManager  *HookManager
}

func NewService(db *gorm.DB, ...) *Service {
    // ... existing initialization ...

    hookRegistry := NewHookRegistry()
    pluginManager := NewAIStudioPluginManager(db, ociClient)
    hookManager := NewHookManager(hookRegistry, pluginManager)

    service := &Service{
        // ... existing fields ...
        hookRegistry: hookRegistry,
        hookManager:  hookManager,
    }

    return service
}
```

### Step 2: Register Plugin Hooks on Load

```go
// In services/ai_studio_plugin_manager.go LoadPlugin method
// After plugin is loaded and client is created:

// Check if plugin implements object hooks
regs, err := grpcClient.GetObjectHookRegistrations(ctx, &pb.GetObjectHookRegistrationsRequest{})
if err == nil && len(regs.Registrations) > 0 {
    // Register hooks
    err = m.service.hookRegistry.RegisterHooks(uint32(pluginID), plugin.Name, regs.Registrations)
    if err != nil {
        log.Warn().Err(err).Msg("Failed to register object hooks")
    } else {
        log.Info().
            Uint("plugin_id", pluginID).
            Int("hook_count", len(regs.Registrations)).
            Msg("Registered object hooks")
    }
}
```

### Step 3: Integrate Hooks into Service Methods

```go
// Example: LLM Service CreateLLM method
func (s *Service) CreateLLM(userID uint, llmData *models.LLM) (*models.LLM, error) {
    // Step 1: Execute "before_create" hooks
    hookResult, err := s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookBeforeCreate,
        llmData,
        uint32(userID),
    )
    if err != nil {
        return nil, fmt.Errorf("hook execution failed: %w", err)
    }

    // Step 2: Check if operation was rejected
    if !hookResult.Allowed {
        return nil, fmt.Errorf("operation rejected: %s", hookResult.RejectionReason)
    }

    // Step 3: Use modified object if hooks modified it
    if hookResult.ModifiedObject != nil {
        if modified, ok := hookResult.ModifiedObject.(*models.LLM); ok {
            llmData = modified
        }
    }

    // Step 4: Merge plugin metadata
    if err := s.hookManager.MergeMetadata(llmData, hookResult.Metadata); err != nil {
        logger.Warn("Failed to merge hook metadata", "error", err)
    }

    // Step 5: Perform the actual database operation
    if err := llmData.Create(s.db); err != nil {
        return nil, err
    }

    // Step 6: Execute "after_create" hooks (for notifications, etc.)
    _, err = s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookAfterCreate,
        llmData,
        uint32(userID),
    )
    if err != nil {
        // Log but don't fail the operation
        logger.Warn("After-create hooks failed", "error", err)
    }

    return llmData, nil
}
```

### Step 4: Similar Pattern for Update

```go
func (s *Service) UpdateLLM(userID uint, llmID uint, updates *models.LLM) (*models.LLM, error) {
    // Get existing object
    existingLLM := &models.LLM{}
    if err := existingLLM.Get(s.db, llmID); err != nil {
        return nil, err
    }

    // Apply updates to existing object
    existingLLM.Name = updates.Name
    // ... other fields ...

    // Execute "before_update" hooks
    hookResult, err := s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookBeforeUpdate,
        existingLLM,
        uint32(userID),
    )
    if err != nil {
        return nil, fmt.Errorf("hook execution failed: %w", err)
    }

    if !hookResult.Allowed {
        return nil, fmt.Errorf("operation rejected: %s", hookResult.RejectionReason)
    }

    // Use modified object if hooks modified it
    if hookResult.ModifiedObject != nil {
        if modified, ok := hookResult.ModifiedObject.(*models.LLM); ok {
            existingLLM = modified
        }
    }

    // Merge plugin metadata
    if err := s.hookManager.MergeMetadata(existingLLM, hookResult.Metadata); err != nil {
        logger.Warn("Failed to merge hook metadata", "error", err)
    }

    // Perform database update
    if err := existingLLM.Update(s.db); err != nil {
        return nil, err
    }

    // Execute "after_update" hooks
    _, err = s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookAfterUpdate,
        existingLLM,
        uint32(userID),
    )
    if err != nil {
        logger.Warn("After-update hooks failed", "error", err)
    }

    return existingLLM, nil
}
```

### Step 5: Similar Pattern for Delete

```go
func (s *Service) DeleteLLM(userID uint, llmID uint) error {
    // Get existing object
    llm := &models.LLM{}
    if err := llm.Get(s.db, llmID); err != nil {
        return err
    }

    // Execute "before_delete" hooks
    hookResult, err := s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookBeforeDelete,
        llm,
        uint32(userID),
    )
    if err != nil {
        return fmt.Errorf("hook execution failed: %w", err)
    }

    if !hookResult.Allowed {
        return fmt.Errorf("operation rejected: %s", hookResult.RejectionReason)
    }

    // Perform database delete
    if err := llm.Delete(s.db); err != nil {
        return err
    }

    // Execute "after_delete" hooks
    _, err = s.hookManager.ExecuteHooks(
        context.Background(),
        ObjectTypeLLM,
        HookAfterDelete,
        llm,
        uint32(userID),
    )
    if err != nil {
        logger.Warn("After-delete hooks failed", "error", err)
    }

    return nil
}
```

## Hook Types

- **before_create**: Validate/modify object before creation, can reject
- **after_create**: React to creation (logging, notifications)
- **before_update**: Validate/modify object before update, can reject
- **after_update**: React to update (audit trail, sync)
- **before_delete**: Prevent deletion based on custom rules
- **after_delete**: Cleanup after deletion (external systems)

## Security Considerations

The `HookManager.sanitizeObject()` method automatically removes sensitive fields (API keys, passwords) from objects before passing to "after_*" hooks.

## Plugin Metadata

Plugins can store custom data in the object's `Metadata` field:

```go
// In plugin
return &pb.ObjectHookResponse{
    AllowOperation: true,
    Modified: false,
    PluginMetadata: map[string]string{
        "validation_status": "approved",
        "external_id": "ext-12345",
    },
}
```

This metadata is stored as: `plugin_{plugin_id}_{key}` in the object's Metadata JSONB field.

## Error Handling

- **Hook timeouts**: Default 5 seconds per hook, configurable
- **Plugin failures**: Logged but don't block operations (except before_* rejections)
- **Circuit breaker**: Prevents repeated calls to failing plugins
- **Transaction rollback**: If before_* hook rejects, no database changes occur

## Example Integration Locations

1. `services/llm_service.go` - LLM CRUD operations
2. `services/datasource_service.go` - Datasource CRUD operations
3. `services/tool_service.go` - Tool CRUD operations
4. `services/user_service.go` - User CRUD operations

## Testing

See `services/hook_manager_test.go` for comprehensive unit tests covering:
- Hook registration
- Hook execution order (priority)
- Timeout handling
- Error handling
- Metadata merging
- Object modification
- Operation rejection
