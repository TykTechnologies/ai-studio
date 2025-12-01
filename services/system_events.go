package services

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
)

// System event topics for CRUD operations
const (
	// LLM events
	TopicLLMCreated = "system.llm.created"
	TopicLLMUpdated = "system.llm.updated"
	TopicLLMDeleted = "system.llm.deleted"

	// App events
	TopicAppCreated  = "system.app.created"
	TopicAppUpdated  = "system.app.updated"
	TopicAppDeleted  = "system.app.deleted"
	TopicAppApproved = "system.app.approved"

	// Datasource events
	TopicDatasourceCreated = "system.datasource.created"
	TopicDatasourceUpdated = "system.datasource.updated"
	TopicDatasourceDeleted = "system.datasource.deleted"

	// User events
	TopicUserCreated = "system.user.created"
	TopicUserUpdated = "system.user.updated"
	TopicUserDeleted = "system.user.deleted"

	// Group events
	TopicGroupCreated = "system.group.created"
	TopicGroupUpdated = "system.group.updated"
	TopicGroupDeleted = "system.group.deleted"

	// Tool events
	TopicToolCreated = "system.tool.created"
	TopicToolUpdated = "system.tool.updated"
	TopicToolDeleted = "system.tool.deleted"
)

// ObjectEventPayload is the standard payload for all system CRUD events
type ObjectEventPayload struct {
	ObjectType string      `json:"object_type"` // "llm", "app", "datasource", "user", "group", "tool"
	Action     string      `json:"action"`      // "created", "updated", "deleted", "approved"
	ObjectID   uint        `json:"object_id"`   // ID of the affected object
	UserID     uint        `json:"user_id"`     // User who performed the action (0 if system/unknown)
	Timestamp  time.Time   `json:"timestamp"`   // When the event occurred
	Object     interface{} `json:"object"`      // The full object (for create/update, nil for delete)
}

// SystemEventEmitter provides a clean interface for emitting system events
type SystemEventEmitter struct {
	bus    eventbridge.Bus
	nodeID string
}

// NewSystemEventEmitter creates a new SystemEventEmitter
func NewSystemEventEmitter(bus eventbridge.Bus, nodeID string) *SystemEventEmitter {
	if nodeID == "" {
		nodeID = "control"
	}
	return &SystemEventEmitter{
		bus:    bus,
		nodeID: nodeID,
	}
}

// EmitObjectEvent emits a system event for an object CRUD operation
func (e *SystemEventEmitter) EmitObjectEvent(topic string, objectType string, action string, objectID uint, userID uint, object interface{}) {
	if e == nil || e.bus == nil {
		return // Silently skip if not configured
	}

	payload := ObjectEventPayload{
		ObjectType: objectType,
		Action:     action,
		ObjectID:   objectID,
		UserID:     userID,
		Timestamp:  time.Now().UTC(),
		Object:     object,
	}

	if err := eventbridge.PublishLocal(e.bus, e.nodeID, topic, payload); err != nil {
		logger.Warnf("Failed to emit system event %s: %v", topic, err)
	} else {
		logger.Debugf("Emitted system event: topic=%s object_type=%s action=%s object_id=%d", topic, objectType, action, objectID)
	}
}

// Convenience methods for each object type

// EmitLLMCreated emits an event when an LLM is created
func (e *SystemEventEmitter) EmitLLMCreated(llm interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicLLMCreated, "llm", "created", objectID, userID, llm)
}

// EmitLLMUpdated emits an event when an LLM is updated
func (e *SystemEventEmitter) EmitLLMUpdated(llm interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicLLMUpdated, "llm", "updated", objectID, userID, llm)
}

// EmitLLMDeleted emits an event when an LLM is deleted
func (e *SystemEventEmitter) EmitLLMDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicLLMDeleted, "llm", "deleted", objectID, userID, nil)
}

// EmitAppCreated emits an event when an App is created
func (e *SystemEventEmitter) EmitAppCreated(app interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicAppCreated, "app", "created", objectID, userID, app)
}

// EmitAppUpdated emits an event when an App is updated
func (e *SystemEventEmitter) EmitAppUpdated(app interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicAppUpdated, "app", "updated", objectID, userID, app)
}

// EmitAppDeleted emits an event when an App is deleted
func (e *SystemEventEmitter) EmitAppDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicAppDeleted, "app", "deleted", objectID, userID, nil)
}

// EmitAppApproved emits an event when an App is approved (credential activated)
func (e *SystemEventEmitter) EmitAppApproved(app interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicAppApproved, "app", "approved", objectID, userID, app)
}

// EmitDatasourceCreated emits an event when a Datasource is created
func (e *SystemEventEmitter) EmitDatasourceCreated(datasource interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicDatasourceCreated, "datasource", "created", objectID, userID, datasource)
}

// EmitDatasourceUpdated emits an event when a Datasource is updated
func (e *SystemEventEmitter) EmitDatasourceUpdated(datasource interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicDatasourceUpdated, "datasource", "updated", objectID, userID, datasource)
}

// EmitDatasourceDeleted emits an event when a Datasource is deleted
func (e *SystemEventEmitter) EmitDatasourceDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicDatasourceDeleted, "datasource", "deleted", objectID, userID, nil)
}

// EmitUserCreated emits an event when a User is created
func (e *SystemEventEmitter) EmitUserCreated(user interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicUserCreated, "user", "created", objectID, userID, user)
}

// EmitUserUpdated emits an event when a User is updated
func (e *SystemEventEmitter) EmitUserUpdated(user interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicUserUpdated, "user", "updated", objectID, userID, user)
}

// EmitUserDeleted emits an event when a User is deleted
func (e *SystemEventEmitter) EmitUserDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicUserDeleted, "user", "deleted", objectID, userID, nil)
}

// EmitGroupCreated emits an event when a Group is created
func (e *SystemEventEmitter) EmitGroupCreated(group interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicGroupCreated, "group", "created", objectID, userID, group)
}

// EmitGroupUpdated emits an event when a Group is updated
func (e *SystemEventEmitter) EmitGroupUpdated(group interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicGroupUpdated, "group", "updated", objectID, userID, group)
}

// EmitGroupDeleted emits an event when a Group is deleted
func (e *SystemEventEmitter) EmitGroupDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicGroupDeleted, "group", "deleted", objectID, userID, nil)
}

// EmitToolCreated emits an event when a Tool is created
func (e *SystemEventEmitter) EmitToolCreated(tool interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicToolCreated, "tool", "created", objectID, userID, tool)
}

// EmitToolUpdated emits an event when a Tool is updated
func (e *SystemEventEmitter) EmitToolUpdated(tool interface{}, objectID uint, userID uint) {
	e.EmitObjectEvent(TopicToolUpdated, "tool", "updated", objectID, userID, tool)
}

// EmitToolDeleted emits an event when a Tool is deleted
func (e *SystemEventEmitter) EmitToolDeleted(objectID uint, userID uint) {
	e.EmitObjectEvent(TopicToolDeleted, "tool", "deleted", objectID, userID, nil)
}
