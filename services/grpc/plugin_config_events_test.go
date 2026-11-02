package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForObjectEvent returns the object ID of the first event on topic, or
// fails the test after a timeout.
func waitForObjectEvent(t *testing.T, events <-chan eventbridge.Event) uint {
	t.Helper()
	select {
	case ev := <-events:
		var payload services.ObjectEventPayload
		require.NoError(t, json.Unmarshal(ev.Payload, &payload))
		return payload.ObjectID
	case <-time.After(2 * time.Second):
		t.Fatal("no system event emitted")
		return 0
	}
}

// Config writes made through the plugin SDK (the management gRPC service)
// emit the same system events as the REST handlers, so the control server
// recomputes the edge config checksums and edges show as pending a push
// (53e.27). The event bus is attached after the server is built, as in Studio.
func TestPluginSDKConfigWritesEmitSystemEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Plugin{}, &models.LLM{}, &models.LLMPlugin{}))
	service := services.NewService(db)
	server := NewAIStudioManagementServer(service)

	bus := eventbridge.NewBus()
	service.SetEventBus(bus)

	plugin := &models.Plugin{Name: "lb", Command: "/bin/lb", HookType: models.HookTypePostAuth,
		HookTypes: []string{models.HookTypePostAuth}, IsActive: true,
		Config: map[string]interface{}{"pools": []interface{}{}}}
	require.NoError(t, db.Create(plugin).Error)
	llm := &models.LLM{Name: "mock", Vendor: models.OPENAI, Active: true}
	require.NoError(t, db.Create(llm).Error)

	t.Run("UpdatePluginConfig emits system.plugin.updated", func(t *testing.T) {
		events := make(chan eventbridge.Event, 4)
		sub := bus.Subscribe(services.TopicPluginUpdated, func(ev eventbridge.Event) { events <- ev })
		defer bus.Unsubscribe(sub)

		ctx := SetPluginIDInContext(context.Background(), plugin.ID)
		_, err := server.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{
			PluginId: uint32(plugin.ID), ConfigJson: `{"pools":[{"name":"mockpool"}]}`})
		require.NoError(t, err)
		assert.Equal(t, plugin.ID, waitForObjectEvent(t, events))
	})

	t.Run("UpdateLLMPlugins emits system.llm.updated", func(t *testing.T) {
		events := make(chan eventbridge.Event, 4)
		sub := bus.Subscribe(services.TopicLLMUpdated, func(ev eventbridge.Event) { events <- ev })
		defer bus.Unsubscribe(sub)

		_, err := server.UpdateLLMPlugins(context.Background(), &pb.UpdateLLMPluginsRequest{
			LlmId: uint32(llm.ID), PluginIds: []uint32{uint32(plugin.ID)}})
		require.NoError(t, err)
		assert.Equal(t, llm.ID, waitForObjectEvent(t, events))
	})
}
