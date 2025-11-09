# Object Interaction Hooks for AI Studio Plugins

## Overview

The Object Hooks system enables plugins to intercept and modify CRUD operations on core AI Studio objects:
- **LLMs**: Language model configurations
- **Datasources**: Data source connections
- **Tools**: External tool integrations
- **Users**: User accounts

This allows plugins to implement:
- Custom validation rules
- Data enrichment and transformation
- External system synchronization
- Audit trails and compliance
- Business logic enforcement

## Architecture

### Components

1. **Proto Definitions** (`proto/plugin.proto`, `proto/object_hooks.proto`)
   - Defines gRPC message structures for hook communication
   - Includes metadata field in all object proto messages

2. **Model Updates** (`models/*.go`)
   - All core models have `Metadata JSONMap` field for plugin data storage
   - GORM auto-migration handles schema updates

3. **Hook Registry** (`services/hook_registry.go`)
   - Manages plugin hook registrations
   - Maintains priority-ordered lists of hooks per object/operation type
   - Supports enable/disable at runtime

4. **Hook Manager** (`services/hook_manager.go`)
   - Executes hooks in priority order
   - Handles timeouts and errors gracefully
   - Merges plugin metadata into objects
   - Sanitizes sensitive fields for security

5. **Plugin SDK** (`pkg/plugin_sdk/capabilities.go`)
   - Provides `ObjectHookHandler` interface
   - Automatic wrapper for gRPC communication
   - Simple, type-safe API for plugin developers

## Hook Types

### Lifecycle Hooks

- **before_create**: Called before object creation
  - Can reject operation
  - Can modify object
  - Use for: validation, defaults, enrichment

- **after_create**: Called after object creation
  - Cannot reject (already created)
  - Use for: notifications, logging, sync

- **before_update**: Called before object update
  - Can reject operation
  - Can modify object
  - Use for: validation, transformation

- **after_update**: Called after object update
  - Cannot reject (already updated)
  - Use for: audit trail, external sync

- **before_delete**: Called before object deletion
  - Can reject operation
  - Use for: prevent deletion, cleanup checks

- **after_delete**: Called after object deletion
  - Cannot reject (already deleted)
  - Use for: cleanup, notifications

## Plugin Development

### 1. Implement ObjectHookHandler Interface

```go
package main

import (
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
    pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type MyHookPlugin struct {
    plugin_sdk.BasePlugin
}

// Declare which hooks to handle
func (p *MyHookPlugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
    return []*pb.ObjectHookRegistration{
        {
            ObjectType: "llm",
            HookTypes:  []string{"before_create", "before_update"},
            Priority:   10, // Lower = earlier execution
        },
    }, nil
}

// Process hook invocations
func (p *MyHookPlugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
    // Parse object JSON
    var llm MyLLMStruct
    json.Unmarshal([]byte(req.ObjectJson), &llm)

    // Validate/transform
    if err := validateLLM(&llm); err != nil {
        return &pb.ObjectHookResponse{
            AllowOperation:  false,
            RejectionReason: err.Error(),
        }, nil
    }

    // Return success with metadata
    return &pb.ObjectHookResponse{
        AllowOperation: true,
        Modified:       false,
        PluginMetadata: map[string]string{
            "validated": "true",
            "validator": "my-plugin",
        },
    }, nil
}

func main() {
    plugin_sdk.Serve(&MyHookPlugin{})
}
```

### 2. Object Structures

Plugins receive objects as JSON. Define structs matching the fields you need:

```go
// LLM object subset
type LLM struct {
    ID               uint                   `json:"id"`
    Name             string                 `json:"name"`
    APIEndpoint      string                 `json:"api_endpoint"`
    Vendor           string                 `json:"vendor"`
    PrivacyScore     int                    `json:"privacy_score"`
    Metadata         map[string]interface{} `json:"metadata"`
}

// Datasource object subset
type Datasource struct {
    ID               uint                   `json:"id"`
    Name             string                 `json:"name"`
    PrivacyScore     int                    `json:"privacy_score"`
    DBSourceType     string                 `json:"db_source_type"`
    Metadata         map[string]interface{} `json:"metadata"`
}

// Tool object subset
type Tool struct {
    ID           uint                   `json:"id"`
    Name         string                 `json:"name"`
    ToolType     string                 `json:"tool_type"`
    PrivacyScore int                    `json:"privacy_score"`
    Metadata     map[string]interface{} `json:"metadata"`
}

// User object subset
type User struct {
    ID       uint                   `json:"id"`
    Email    string                 `json:"email"`
    Name     string                 `json:"name"`
    IsAdmin  bool                   `json:"is_admin"`
    Metadata map[string]interface{} `json:"metadata"`
}
```

### 3. Modifying Objects

To modify an object, return the modified JSON:

```go
func (p *MyHookPlugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
    var llm LLM
    json.Unmarshal([]byte(req.ObjectJson), &llm)

    // Modify object
    llm.Name = strings.ToUpper(llm.Name)
    llm.PrivacyScore = 10

    // Serialize back to JSON
    modifiedJSON, _ := json.Marshal(llm)

    return &pb.ObjectHookResponse{
        AllowOperation:     true,
        Modified:           true,
        ModifiedObjectJson: string(modifiedJSON),
    }, nil
}
```

### 4. Storing Plugin Metadata

Plugins can store custom data in the object's Metadata field:

```go
return &pb.ObjectHookResponse{
    AllowOperation: true,
    PluginMetadata: map[string]string{
        "external_id":     "ext-12345",
        "validation_time": time.Now().String(),
        "validator":       "my-plugin",
    },
}
```

The hook manager automatically prefixes keys with `plugin_{id}_` to avoid conflicts.

## Service Integration

See [OBJECT_HOOKS_INTEGRATION.md](OBJECT_HOOKS_INTEGRATION.md) for detailed integration guide.

### Quick Integration Pattern

```go
// Before creating/updating/deleting an object:
hookResult, err := service.hookManager.ExecuteHooks(
    ctx,
    ObjectTypeLLM,
    HookBeforeCreate,
    objectData,
    userID,
)

if !hookResult.Allowed {
    return fmt.Errorf("rejected: %s", hookResult.RejectionReason)
}

// Use modified object if changed
if hookResult.ModifiedObject != nil {
    objectData = hookResult.ModifiedObject.(*models.LLM)
}

// Merge metadata
service.hookManager.MergeMetadata(objectData, hookResult.Metadata)

// Perform database operation
objectData.Create(db)

// After operation
service.hookManager.ExecuteHooks(ctx, ObjectTypeLLM, HookAfterCreate, objectData, userID)
```

## Security

### Sensitive Field Protection

The Hook Manager automatically sanitizes sensitive fields for `after_*` hooks:

- **LLM**: API keys redacted
- **Datasource**: Connection strings and API keys redacted
- **Tool**: Auth keys redacted
- **User**: Passwords, API keys, tokens redacted

`before_*` hooks receive full objects (plugins need to validate credentials).

### Permission Model

Plugins must be:
1. Installed by admin
2. Enabled explicitly
3. Can be disabled without uninstalling

## Performance

### Optimization Strategies

1. **Connection Pooling**: gRPC connections reused
2. **Timeouts**: Default 5 seconds per hook
3. **Circuit Breaker**: Failing plugins auto-disabled temporarily
4. **Priority Ordering**: Critical hooks run first
5. **Early Termination**: Chain stops on rejection

### Performance Targets

- Single hook: <50ms overhead
- Hook chain (5 plugins): <200ms total
- Metadata query: <10ms (indexed)

## Error Handling

### Plugin Failure Modes

1. **Timeout**: Hook canceled, operation continues (logged)
2. **Crash**: Circuit breaker prevents retries
3. **Rejection**: Operation blocked, error returned to user
4. **Invalid Response**: Validation fails, operation continues (logged)

### Rollback Strategy

All hook executions wrapped in database transaction:
- On `before_*` rejection → Full rollback
- On plugin error → Rollback, return error
- On `after_*` failure → Logged but don't rollback

## Example Plugins

### 1. LLM Validator
Location: `examples/plugins/studio/llm-validator/`

Validates:
- HTTPS endpoints required
- Blocked vendor list
- Minimum privacy scores
- Required descriptions

### 2. External Sync (Future)
Synchronizes object creation to external CMDB/inventory system.

### 3. Audit Logger (Future)
Logs all object changes to external audit system.

## Testing

### Unit Tests

```go
// Test hook registration
func TestHookRegistration(t *testing.T) {
    registry := NewHookRegistry()
    regs := []*pb.ObjectHookRegistration{
        {ObjectType: "llm", HookTypes: []string{"before_create"}, Priority: 10},
    }
    err := registry.RegisterHooks(1, "test-plugin", regs)
    assert.NoError(t, err)
}

// Test hook execution
func TestHookExecution(t *testing.T) {
    // Setup mock plugin
    // Execute hooks
    // Assert results
}
```

### Integration Tests

```bash
# Build example plugin
cd examples/plugins/studio/llm-validator
go build

# Start AI Studio with plugin enabled
# Create LLM with invalid config → Should fail
# Create LLM with valid config → Should succeed
```

## Troubleshooting

### Plugin Not Registering Hooks

1. Check plugin implements `ObjectHookHandler`
2. Verify `GetObjectHookRegistrations()` returns non-empty list
3. Check plugin loaded successfully (logs)
4. Verify plugin enabled in database

### Hooks Not Executing

1. Check hook registry contains registrations
2. Verify plugin still loaded/healthy
3. Check hook enabled (not circuit-broken)
4. Review logs for errors

### Performance Issues

1. Check hook timeout settings
2. Review plugin execution times (logs)
3. Consider adjusting priorities
4. Implement caching in plugin

## Migration Guide

### Existing Systems

1. Deploy code with hooks disabled
2. Test without hooks active
3. Enable hooks per object type
4. Monitor performance
5. Enable remaining hooks

### For Plugin Developers

1. Update SDK to latest version
2. Implement `ObjectHookHandler` interface
3. Test with local AI Studio instance
4. Deploy to production

## Future Enhancements

- Async hook execution for `after_*` hooks
- Hook execution metrics/dashboards
- Conditional hook execution (filters)
- Hook dependency declarations
- Bulk operation hooks

## References

- **Implementation**: `services/hook_*.go`
- **SDK**: `pkg/plugin_sdk/capabilities.go`
- **Proto**: `proto/plugin.proto`
- **Example**: `examples/plugins/studio/llm-validator/`
- **Integration Guide**: `OBJECT_HOOKS_INTEGRATION.md`
