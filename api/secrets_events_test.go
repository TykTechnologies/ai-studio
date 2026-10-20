package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Creating, changing and deleting a secret each emit a secret event, which
// the control server turns into a snapshot recompute (edges get referenced
// values). The event carries the secret's ID, never its value or name.
func TestSecretHandlers_EmitSecretEventsWithoutTheSecret(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-secret-key-for-unit-test")
	api, db := setupTestAPI(t)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))

	bus := eventbridge.NewBus()
	api.service.SystemEvents = services.NewSystemEventEmitter(bus, "")
	events := make(chan eventbridge.Event, 8)
	for _, topic := range []string{services.TopicSecretCreated, services.TopicSecretUpdated, services.TopicSecretDeleted} {
		bus.Subscribe(topic, func(e eventbridge.Event) { events <- e })
	}
	next := func(topic string) eventbridge.Event {
		t.Helper()
		select {
		case e := <-events:
			require.Equal(t, topic, e.Topic)
			assert.NotContains(t, string(e.Payload), "engine-token-value")
			assert.NotContains(t, string(e.Payload), "ENGINE_TOKEN")
			return e
		case <-time.After(2 * time.Second):
			t.Fatalf("no %s event", topic)
			return eventbridge.Event{}
		}
	}
	send := func(method, path string, value string) int {
		var in SecretInput
		in.Data.Attributes.VarName = "ENGINE_TOKEN"
		in.Data.Attributes.Value = value
		body, _ := json.Marshal(in)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(method, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		api.router.ServeHTTP(w, req)
		return w.Code
	}

	require.Equal(t, http.StatusCreated, send(http.MethodPost, "/api/v1/secrets", "engine-token-value"))
	next(services.TopicSecretCreated)
	var stored secrets.Secret
	require.NoError(t, db.Where("var_name = ?", "ENGINE_TOKEN").First(&stored).Error)
	path := fmt.Sprintf("/api/v1/secrets/%d", stored.ID)

	require.Equal(t, http.StatusOK, send(http.MethodPatch, path, "engine-token-value-2"))
	next(services.TopicSecretUpdated)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, path, nil)
	api.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	next(services.TopicSecretDeleted)
}
