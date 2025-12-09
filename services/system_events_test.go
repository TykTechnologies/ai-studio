package services

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventCollector helps capture events for testing
type TestEventCollector struct {
	mu     sync.Mutex
	events []eventbridge.Event
	wg     sync.WaitGroup
}

func NewTestEventCollector() *TestEventCollector {
	return &TestEventCollector{
		events: make([]eventbridge.Event, 0),
	}
}

func (c *TestEventCollector) Expect(count int) {
	c.wg.Add(count)
}

func (c *TestEventCollector) Handler() func(ev eventbridge.Event) {
	return func(ev eventbridge.Event) {
		c.mu.Lock()
		c.events = append(c.events, ev)
		c.mu.Unlock()
		c.wg.Done()
	}
}

func (c *TestEventCollector) WaitWithTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (c *TestEventCollector) GetEvents() []eventbridge.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]eventbridge.Event, len(c.events))
	copy(result, c.events)
	return result
}

func (c *TestEventCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = make([]eventbridge.Event, 0)
}

// Helper to setup service with event bus
func setupServiceWithEventBus(t *testing.T) (*Service, eventbridge.Bus) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create and wire event bus
	bus := eventbridge.NewBus()
	service.SetEventBus(bus)

	return service, bus
}

// Helper to parse event payload
func parseEventPayload(t *testing.T, ev eventbridge.Event) ObjectEventPayload {
	var payload ObjectEventPayload
	err := json.Unmarshal(ev.Payload, &payload)
	require.NoError(t, err)
	return payload
}

// ========== SystemEventEmitter Unit Tests ==========

func TestSystemEventEmitter_NilBus(t *testing.T) {
	// Test that nil bus doesn't cause panics
	emitter := NewSystemEventEmitter(nil, "test")
	// Should not panic
	emitter.EmitObjectEvent(TopicLLMCreated, "llm", "created", 1, 0, nil)
}

func TestSystemEventEmitter_NilEmitter(t *testing.T) {
	// Test that calling methods on nil emitter doesn't panic
	var emitter *SystemEventEmitter
	// Should not panic
	emitter.EmitObjectEvent(TopicLLMCreated, "llm", "created", 1, 0, nil)
}

func TestSystemEventEmitter_EmitObjectEvent(t *testing.T) {
	bus := eventbridge.NewBus()
	emitter := NewSystemEventEmitter(bus, "test-node")

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	testObject := map[string]string{"name": "test-llm"}
	emitter.EmitObjectEvent(TopicLLMCreated, "llm", "created", 123, 456, testObject)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, TopicLLMCreated, ev.Topic)
	assert.Equal(t, "test-node", ev.Origin)
	assert.Equal(t, eventbridge.DirLocal, ev.Dir)

	payload := parseEventPayload(t, ev)
	assert.Equal(t, "llm", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, uint(123), payload.ObjectID)
	assert.Equal(t, uint(456), payload.UserID)
	assert.NotZero(t, payload.Timestamp)
}

func TestSystemEventEmitter_DefaultNodeID(t *testing.T) {
	bus := eventbridge.NewBus()
	emitter := NewSystemEventEmitter(bus, "") // Empty nodeID

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	emitter.EmitLLMCreated(nil, 1, 0)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "control", events[0].Origin) // Should default to "control"
}

// ========== LLM Event Tests ==========

func TestSystemEvents_LLM_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	llm, err := service.CreateLLM("Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "llm", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, llm.ID, payload.ObjectID)
}

func TestSystemEvents_LLM_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	// Create LLM first (without collecting this event)
	llm, err := service.CreateLLM("Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	require.NoError(t, err)

	// Now subscribe for update event
	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateLLM(llm.ID, "Updated LLM", "new-api-key", "https://new-api.test.com", 80,
		"Updated short", "Updated long", "https://new-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil, "")
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "llm", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, llm.ID, payload.ObjectID)
}

func TestSystemEvents_LLM_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	llm, err := service.CreateLLM("Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteLLM(llm.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "llm", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, llm.ID, payload.ObjectID)
	assert.Nil(t, payload.Object) // Deleted events should have nil object
}

// ========== App Event Tests ==========

func TestSystemEvents_App_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	// Need a user for app creation
	user, err := service.CreateUser(UserDTO{
		Email:                "appowner@example.com",
		Name:                 "App Owner",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicAppCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	app, err := service.CreateApp("Test App", "App description", user.ID, []uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "app", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, app.ID, payload.ObjectID)
}

func TestSystemEvents_App_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "appowner2@example.com",
		Name:                 "App Owner 2",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	app, err := service.CreateApp("Test App 2", "App description", user.ID, []uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicAppUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateApp(app.ID, "Updated App", "Updated description", user.ID, []uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "app", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, app.ID, payload.ObjectID)
}

func TestSystemEvents_App_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "appowner3@example.com",
		Name:                 "App Owner 3",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	app, err := service.CreateApp("Test App 3", "App description", user.ID, []uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicAppDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteApp(app.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "app", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, app.ID, payload.ObjectID)
}

func TestSystemEvents_App_Approved(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "appowner4@example.com",
		Name:                 "App Owner 4",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	// Create app with credential
	app, err := service.CreateApp("Test App 4", "App description", user.ID, []uint{}, []uint{}, []uint{}, nil, nil, nil)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicAppApproved, collector.Handler())
	defer bus.Unsubscribe(sub)

	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "app", payload.ObjectType)
	assert.Equal(t, "approved", payload.Action)
	assert.Equal(t, app.ID, payload.ObjectID)
}

// ========== User Event Tests ==========

func TestSystemEvents_User_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicUserCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	user, err := service.CreateUser(UserDTO{
		Email:                "eventtest@example.com",
		Name:                 "Event Test User",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "user", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, user.ID, payload.ObjectID)
}

func TestSystemEvents_User_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "eventtest2@example.com",
		Name:                 "Event Test User 2",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicUserUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateUser(user, UserDTO{
		Email:                "eventtest2_updated@example.com",
		Name:                 "Updated Event Test User 2",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "user", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, user.ID, payload.ObjectID)
}

func TestSystemEvents_User_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "eventtest3@example.com",
		Name:                 "Event Test User 3",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicUserDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteUser(user)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "user", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, user.ID, payload.ObjectID)
}

// ========== Group Event Tests ==========

func TestSystemEvents_Group_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicGroupCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	group, err := service.CreateGroup("Event Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "group", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, group.ID, payload.ObjectID)
}

func TestSystemEvents_Group_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	group, err := service.CreateGroup("Event Test Group 2", []uint{}, []uint{}, []uint{}, []uint{})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicGroupUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateGroup(group.ID, "Updated Event Test Group 2", []uint{}, []uint{}, []uint{}, []uint{})
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "group", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, group.ID, payload.ObjectID)
}

func TestSystemEvents_Group_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	group, err := service.CreateGroup("Event Test Group 3", []uint{}, []uint{}, []uint{}, []uint{})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicGroupDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteGroup(group.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "group", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, group.ID, payload.ObjectID)
}

// ========== Datasource Event Tests ==========

func TestSystemEvents_Datasource_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	// Need a user for datasource creation
	user, err := service.CreateUser(UserDTO{
		Email:                "dsowner@example.com",
		Name:                 "DS Owner",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicDatasourceCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	datasource, err := service.CreateDatasource("Test Datasource", "Short desc", "Long desc", "icon.png",
		"https://example.com", 75, user.ID, []string{}, "conn_string", "source_type",
		"db-key", "db1", "embed_vendor", "embed_url", "embed-key", "embed_model", true)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "datasource", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, datasource.ID, payload.ObjectID)
}

func TestSystemEvents_Datasource_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "dsowner2@example.com",
		Name:                 "DS Owner 2",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	datasource, err := service.CreateDatasource("Test Datasource 2", "Short desc", "Long desc", "icon.png",
		"https://example.com", 75, user.ID, []string{}, "conn_string", "source_type",
		"db-key", "db1", "embed_vendor", "embed_url", "embed-key", "embed_model", true)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicDatasourceUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateDatasource(datasource.ID, "Updated Datasource", "Updated short", "Updated long", "new-icon.png",
		"https://updated.com", 80, "new_conn_string", "new_source_type",
		"new-db-key", "db2", "new_embed_vendor", "new_embed_url", "new-embed-key", "new_embed_model", false, []string{}, user.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "datasource", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, datasource.ID, payload.ObjectID)
}

func TestSystemEvents_Datasource_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	user, err := service.CreateUser(UserDTO{
		Email:                "dsowner3@example.com",
		Name:                 "DS Owner 3",
		Password:             "password123",
		IsAdmin:              true,
		ShowChat:             true,
		ShowPortal:           true,
		EmailVerified:        true,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
		Groups:               []uint{},
	})
	require.NoError(t, err)

	datasource, err := service.CreateDatasource("Test Datasource 3", "Short desc", "Long desc", "icon.png",
		"https://example.com", 75, user.ID, []string{}, "conn_string", "source_type",
		"db-key", "db1", "embed_vendor", "embed_url", "embed-key", "embed_model", true)
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicDatasourceDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteDatasource(datasource.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "datasource", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, datasource.ID, payload.ObjectID)
}

// ========== Tool Event Tests ==========

func TestSystemEvents_Tool_Created(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicToolCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	tool, err := service.CreateTool("Test Tool", "Tool description", "openapi", `{"openapi":"3.0.0"}`, 75, "test_schema", "api-key")
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "tool", payload.ObjectType)
	assert.Equal(t, "created", payload.Action)
	assert.Equal(t, tool.ID, payload.ObjectID)
}

func TestSystemEvents_Tool_Updated(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	tool, err := service.CreateTool("Test Tool 2", "Tool description", "openapi", `{"openapi":"3.0.0"}`, 75, "test_schema", "api-key")
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicToolUpdated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err = service.UpdateTool(tool.ID, "Updated Tool", "Updated description", "openapi", `{"openapi":"3.0.1"}`, 80, "new_schema", "new-api-key")
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "tool", payload.ObjectType)
	assert.Equal(t, "updated", payload.Action)
	assert.Equal(t, tool.ID, payload.ObjectID)
}

func TestSystemEvents_Tool_Deleted(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	tool, err := service.CreateTool("Test Tool 3", "Tool description", "openapi", `{"openapi":"3.0.0"}`, 75, "test_schema", "api-key")
	require.NoError(t, err)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicToolDeleted, collector.Handler())
	defer bus.Unsubscribe(sub)

	err = service.DeleteTool(tool.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	payload := parseEventPayload(t, events[0])
	assert.Equal(t, "tool", payload.ObjectType)
	assert.Equal(t, "deleted", payload.Action)
	assert.Equal(t, tool.ID, payload.ObjectID)
}

// ========== All Events Subscription Test ==========

func TestSystemEvents_SubscribeAll(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	// Subscribe to ALL events
	collector := NewTestEventCollector()
	collector.Expect(3) // Create, Update, Delete
	sub := bus.SubscribeAll(collector.Handler())
	defer bus.Unsubscribe(sub)

	// Create, update, delete an LLM
	llm, err := service.CreateLLM("All Events Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	require.NoError(t, err)

	_, err = service.UpdateLLM(llm.ID, "Updated All Events LLM", "new-key", "https://new.test.com", 80,
		"New short", "New long", "https://new-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil, "")
	require.NoError(t, err)

	err = service.DeleteLLM(llm.ID)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(2*time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 3)

	// Verify we got create, update, delete events
	topics := make(map[string]bool)
	for _, ev := range events {
		topics[ev.Topic] = true
	}
	assert.True(t, topics[TopicLLMCreated])
	assert.True(t, topics[TopicLLMUpdated])
	assert.True(t, topics[TopicLLMDeleted])
}

// ========== Event Direction Test ==========

func TestSystemEvents_Direction_IsLocal(t *testing.T) {
	service, bus := setupServiceWithEventBus(t)

	collector := NewTestEventCollector()
	collector.Expect(1)
	sub := bus.Subscribe(TopicLLMCreated, collector.Handler())
	defer bus.Unsubscribe(sub)

	_, err := service.CreateLLM("Direction Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	require.NoError(t, err)

	assert.True(t, collector.WaitWithTimeout(time.Second))

	events := collector.GetEvents()
	require.Len(t, events, 1)

	// Verify direction is DirLocal (control-plane only)
	assert.Equal(t, eventbridge.DirLocal, events[0].Dir)
}

// ========== No Event Bus Test ==========

func TestSystemEvents_NoEventBus_NoErrors(t *testing.T) {
	// Setup service WITHOUT event bus
	db := setupTestDB(t)
	service := NewService(db)
	// Don't call SetEventBus

	// Should not panic or error - events are silently skipped
	llm, err := service.CreateLLM("No Bus Test LLM", "api-key", "https://api.test.com", 75,
		"Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, llm)

	_, err = service.UpdateLLM(llm.ID, "Updated No Bus LLM", "new-key", "https://new.test.com", 80,
		"New short", "New long", "https://new-logo.com", models.OPENAI, true, nil, "", []string{}, nil, nil, "")
	assert.NoError(t, err)

	err = service.DeleteLLM(llm.ID)
	assert.NoError(t, err)
}

// ========== Convenience Method Tests ==========

func TestSystemEventEmitter_ConvenienceMethods(t *testing.T) {
	bus := eventbridge.NewBus()
	emitter := NewSystemEventEmitter(bus, "control")

	tests := []struct {
		name       string
		topic      string
		emitFunc   func()
		objectType string
		action     string
	}{
		{
			name:       "EmitLLMCreated",
			topic:      TopicLLMCreated,
			emitFunc:   func() { emitter.EmitLLMCreated(nil, 1, 0) },
			objectType: "llm",
			action:     "created",
		},
		{
			name:       "EmitLLMUpdated",
			topic:      TopicLLMUpdated,
			emitFunc:   func() { emitter.EmitLLMUpdated(nil, 1, 0) },
			objectType: "llm",
			action:     "updated",
		},
		{
			name:       "EmitLLMDeleted",
			topic:      TopicLLMDeleted,
			emitFunc:   func() { emitter.EmitLLMDeleted(1, 0) },
			objectType: "llm",
			action:     "deleted",
		},
		{
			name:       "EmitAppCreated",
			topic:      TopicAppCreated,
			emitFunc:   func() { emitter.EmitAppCreated(nil, 1, 0) },
			objectType: "app",
			action:     "created",
		},
		{
			name:       "EmitAppUpdated",
			topic:      TopicAppUpdated,
			emitFunc:   func() { emitter.EmitAppUpdated(nil, 1, 0) },
			objectType: "app",
			action:     "updated",
		},
		{
			name:       "EmitAppDeleted",
			topic:      TopicAppDeleted,
			emitFunc:   func() { emitter.EmitAppDeleted(1, 0) },
			objectType: "app",
			action:     "deleted",
		},
		{
			name:       "EmitAppApproved",
			topic:      TopicAppApproved,
			emitFunc:   func() { emitter.EmitAppApproved(nil, 1, 0) },
			objectType: "app",
			action:     "approved",
		},
		{
			name:       "EmitDatasourceCreated",
			topic:      TopicDatasourceCreated,
			emitFunc:   func() { emitter.EmitDatasourceCreated(nil, 1, 0) },
			objectType: "datasource",
			action:     "created",
		},
		{
			name:       "EmitDatasourceUpdated",
			topic:      TopicDatasourceUpdated,
			emitFunc:   func() { emitter.EmitDatasourceUpdated(nil, 1, 0) },
			objectType: "datasource",
			action:     "updated",
		},
		{
			name:       "EmitDatasourceDeleted",
			topic:      TopicDatasourceDeleted,
			emitFunc:   func() { emitter.EmitDatasourceDeleted(1, 0) },
			objectType: "datasource",
			action:     "deleted",
		},
		{
			name:       "EmitUserCreated",
			topic:      TopicUserCreated,
			emitFunc:   func() { emitter.EmitUserCreated(nil, 1, 0) },
			objectType: "user",
			action:     "created",
		},
		{
			name:       "EmitUserUpdated",
			topic:      TopicUserUpdated,
			emitFunc:   func() { emitter.EmitUserUpdated(nil, 1, 0) },
			objectType: "user",
			action:     "updated",
		},
		{
			name:       "EmitUserDeleted",
			topic:      TopicUserDeleted,
			emitFunc:   func() { emitter.EmitUserDeleted(1, 0) },
			objectType: "user",
			action:     "deleted",
		},
		{
			name:       "EmitGroupCreated",
			topic:      TopicGroupCreated,
			emitFunc:   func() { emitter.EmitGroupCreated(nil, 1, 0) },
			objectType: "group",
			action:     "created",
		},
		{
			name:       "EmitGroupUpdated",
			topic:      TopicGroupUpdated,
			emitFunc:   func() { emitter.EmitGroupUpdated(nil, 1, 0) },
			objectType: "group",
			action:     "updated",
		},
		{
			name:       "EmitGroupDeleted",
			topic:      TopicGroupDeleted,
			emitFunc:   func() { emitter.EmitGroupDeleted(1, 0) },
			objectType: "group",
			action:     "deleted",
		},
		{
			name:       "EmitToolCreated",
			topic:      TopicToolCreated,
			emitFunc:   func() { emitter.EmitToolCreated(nil, 1, 0) },
			objectType: "tool",
			action:     "created",
		},
		{
			name:       "EmitToolUpdated",
			topic:      TopicToolUpdated,
			emitFunc:   func() { emitter.EmitToolUpdated(nil, 1, 0) },
			objectType: "tool",
			action:     "updated",
		},
		{
			name:       "EmitToolDeleted",
			topic:      TopicToolDeleted,
			emitFunc:   func() { emitter.EmitToolDeleted(1, 0) },
			objectType: "tool",
			action:     "deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewTestEventCollector()
			collector.Expect(1)
			sub := bus.Subscribe(tt.topic, collector.Handler())
			defer bus.Unsubscribe(sub)

			tt.emitFunc()

			assert.True(t, collector.WaitWithTimeout(time.Second))

			events := collector.GetEvents()
			require.Len(t, events, 1)

			payload := parseEventPayload(t, events[0])
			assert.Equal(t, tt.objectType, payload.ObjectType)
			assert.Equal(t, tt.action, payload.Action)
		})
	}
}
