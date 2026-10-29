package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Updating a price with recalculation emits system.model_price.updated, as a
// standard update does: without it edges were not flagged for a push and kept
// pricing at the old price, and subscribed plugins missed the change.
func TestUpdateModelPriceAndRecalculate_EmitsUpdatedEvent(t *testing.T) {
	api, _ := setupTestAPI(t)
	bus := eventbridge.NewBus()
	api.service.SystemEvents = services.NewSystemEventEmitter(bus, "")
	events := make(chan eventbridge.Event, 4)
	bus.Subscribe(services.TopicModelPriceUpdated, func(e eventbridge.Event) { events <- e })

	price, err := api.service.CreateModelPrice("gemini-2.5-flash", "google_ai", 0.000001, 0.000001, 0, 0, "USD")
	require.NoError(t, err)

	body := map[string]interface{}{"data": map[string]interface{}{
		"type":       "model-prices",
		"attributes": map[string]interface{}{"cpt": 0.000002},
	}}
	w := performRequest(api.router, http.MethodPatch, fmt.Sprintf("/api/v1/model-prices/%d/recalculate", price.ID), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	select {
	case e := <-events:
		assert.Equal(t, services.TopicModelPriceUpdated, e.Topic)
		var payload struct {
			ObjectID uint `json:"object_id"`
		}
		require.NoError(t, json.Unmarshal(e.Payload, &payload))
		assert.Equal(t, price.ID, payload.ObjectID)
	case <-time.After(2 * time.Second):
		t.Fatal("no system.model_price.updated event after PATCH /model-prices/:id/recalculate")
	}
}
