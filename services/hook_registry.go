package services

import (
	"fmt"
	"sort"
	"sync"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// HookType represents the stage at which a hook is invoked
type HookType string

const (
	HookBeforeCreate HookType = "before_create"
	HookAfterCreate  HookType = "after_create"
	HookBeforeUpdate HookType = "before_update"
	HookAfterUpdate  HookType = "after_update"
	HookBeforeDelete HookType = "before_delete"
	HookAfterDelete  HookType = "after_delete"
)

// ObjectType represents the type of object being hooked
type ObjectType string

const (
	ObjectTypeLLM        ObjectType = "llm"
	ObjectTypeDatasource ObjectType = "datasource"
	ObjectTypeTool       ObjectType = "tool"
	ObjectTypeUser       ObjectType = "user"
)

// HookRegistration represents a plugin's registration for a specific hook
type HookRegistration struct {
	PluginID   uint32     `json:"plugin_id"`
	PluginName string     `json:"plugin_name"`
	ObjectType ObjectType `json:"object_type"`
	HookType   HookType   `json:"hook_type"`
	Priority   int32      `json:"priority"`
	Enabled    bool       `json:"enabled"`
}

// HookRegistry manages hook registrations from plugins
type HookRegistry struct {
	mu            sync.RWMutex
	registrations map[string][]*HookRegistration // key: "objectType:hookType"
	pluginHooks   map[uint32][]*HookRegistration // key: pluginID
}

// NewHookRegistry creates a new hook registry
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		registrations: make(map[string][]*HookRegistration),
		pluginHooks:   make(map[uint32][]*HookRegistration),
	}
}

// RegisterHooks registers hooks for a plugin
func (r *HookRegistry) RegisterHooks(pluginID uint32, pluginName string, protoRegs []*pb.ObjectHookRegistration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing registrations for this plugin
	if existing, ok := r.pluginHooks[pluginID]; ok {
		for _, reg := range existing {
			key := makeKey(reg.ObjectType, reg.HookType)
			r.removeFromList(key, pluginID)
		}
	}

	// Register new hooks
	var newRegistrations []*HookRegistration
	for _, protoReg := range protoRegs {
		objType := ObjectType(protoReg.ObjectType)
		if !r.isValidObjectType(objType) {
			return fmt.Errorf("invalid object type: %s", protoReg.ObjectType)
		}

		for _, hookTypeStr := range protoReg.HookTypes {
			hookType := HookType(hookTypeStr)
			if !r.isValidHookType(hookType) {
				return fmt.Errorf("invalid hook type: %s", hookTypeStr)
			}

			reg := &HookRegistration{
				PluginID:   pluginID,
				PluginName: pluginName,
				ObjectType: objType,
				HookType:   hookType,
				Priority:   protoReg.Priority,
				Enabled:    true,
			}

			newRegistrations = append(newRegistrations, reg)

			// Add to registrations map
			key := makeKey(objType, hookType)
			r.registrations[key] = append(r.registrations[key], reg)

			// Sort by priority (lower = earlier)
			sort.Slice(r.registrations[key], func(i, j int) bool {
				return r.registrations[key][i].Priority < r.registrations[key][j].Priority
			})
		}
	}

	// Store plugin's registrations
	r.pluginHooks[pluginID] = newRegistrations

	return nil
}

// UnregisterPlugin removes all hooks for a plugin
func (r *HookRegistry) UnregisterPlugin(pluginID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if registrations, ok := r.pluginHooks[pluginID]; ok {
		for _, reg := range registrations {
			key := makeKey(reg.ObjectType, reg.HookType)
			r.removeFromList(key, pluginID)
		}
		delete(r.pluginHooks, pluginID)
	}
}

// GetHooks returns all registered hooks for a specific object type and hook type
func (r *HookRegistry) GetHooks(objectType ObjectType, hookType HookType) []*HookRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeKey(objectType, hookType)
	hooks, ok := r.registrations[key]
	if !ok {
		return nil
	}

	// Return only enabled hooks
	var enabledHooks []*HookRegistration
	for _, hook := range hooks {
		if hook.Enabled {
			enabledHooks = append(enabledHooks, hook)
		}
	}

	return enabledHooks
}

// GetPluginHooks returns all hooks registered by a specific plugin
func (r *HookRegistry) GetPluginHooks(pluginID uint32) []*HookRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.pluginHooks[pluginID]
}

// EnableHook enables a specific hook registration
func (r *HookRegistry) EnableHook(pluginID uint32, objectType ObjectType, hookType HookType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	registrations, ok := r.pluginHooks[pluginID]
	if !ok {
		return fmt.Errorf("no hooks registered for plugin %d", pluginID)
	}

	found := false
	for _, reg := range registrations {
		if reg.ObjectType == objectType && reg.HookType == hookType {
			reg.Enabled = true
			found = true
		}
	}

	if !found {
		return fmt.Errorf("hook not found: plugin=%d, object=%s, hook=%s", pluginID, objectType, hookType)
	}

	return nil
}

// DisableHook disables a specific hook registration
func (r *HookRegistry) DisableHook(pluginID uint32, objectType ObjectType, hookType HookType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	registrations, ok := r.pluginHooks[pluginID]
	if !ok {
		return fmt.Errorf("no hooks registered for plugin %d", pluginID)
	}

	found := false
	for _, reg := range registrations {
		if reg.ObjectType == objectType && reg.HookType == hookType {
			reg.Enabled = false
			found = true
		}
	}

	if !found {
		return fmt.Errorf("hook not found: plugin=%d, object=%s, hook=%s", pluginID, objectType, hookType)
	}

	return nil
}

// GetAllRegistrations returns all hook registrations
func (r *HookRegistry) GetAllRegistrations() map[string][]*HookRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string][]*HookRegistration)
	for key, regs := range r.registrations {
		result[key] = make([]*HookRegistration, len(regs))
		copy(result[key], regs)
	}

	return result
}

// Helper methods

func makeKey(objectType ObjectType, hookType HookType) string {
	return fmt.Sprintf("%s:%s", objectType, hookType)
}

func (r *HookRegistry) removeFromList(key string, pluginID uint32) {
	if list, ok := r.registrations[key]; ok {
		var newList []*HookRegistration
		for _, reg := range list {
			if reg.PluginID != pluginID {
				newList = append(newList, reg)
			}
		}
		if len(newList) == 0 {
			delete(r.registrations, key)
		} else {
			r.registrations[key] = newList
		}
	}
}

func (r *HookRegistry) isValidObjectType(objType ObjectType) bool {
	switch objType {
	case ObjectTypeLLM, ObjectTypeDatasource, ObjectTypeTool, ObjectTypeUser:
		return true
	default:
		return false
	}
}

func (r *HookRegistry) isValidHookType(hookType HookType) bool {
	switch hookType {
	case HookBeforeCreate, HookAfterCreate, HookBeforeUpdate, HookAfterUpdate, HookBeforeDelete, HookAfterDelete:
		return true
	default:
		return false
	}
}
