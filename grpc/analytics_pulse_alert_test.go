package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// An alerting compliance event an edge sends in its analytics pulse raises
// the alert on the hub once stored, exactly once; a name the hub does not
// configure for that filter is not raised.
func TestSendAnalyticsPulse_GuardrailAlertRaisedOnce(t *testing.T) {
	db := setupPulseTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Filter{}, &models.App{}, &models.GuardrailException{}))
	server := setupControlServer(t, db)

	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": guardrails.ProviderBuiltin, "detectors": []any{map[string]any{"name": guardrails.CategoryInjection}},
		"on_detect": guardrails.ActionAlert, "alert": map[string]any{"event": "edge-flag"},
	}}
	require.NoError(t, db.Create(filter).Error)

	svc := services.NewService(db)
	bus := eventbridge.NewBus()
	svc.SystemEvents = services.NewSystemEventEmitter(bus, "hub")
	var mu sync.Mutex
	var got []services.GuardrailAlertPayload
	for _, topic := range []string{"guardrail.alert.edge-flag", "guardrail.alert.forged"} {
		sub := bus.Subscribe(topic, func(ev eventbridge.Event) {
			var p services.GuardrailAlertPayload
			_ = json.Unmarshal(ev.Payload, &p)
			mu.Lock()
			got = append(got, p)
			mu.Unlock()
		})
		defer bus.Unsubscribe(sub)
	}
	analytics.SetComplianceEventsStored(svc.PublishGuardrailAlerts)
	t.Cleanup(func() { analytics.SetComplianceEventsStored(nil) })

	meta := func(alert string) string {
		return fmt.Sprintf(`{"filter_id":%d,"detector":"prompt_injection","action":"log","alert":%q}`, filter.ID, alert)
	}
	now := timestamppb.New(time.Now())
	pulse := &pb.AnalyticsPulse{
		EdgeId: "edge-1", EdgeNamespace: "default", SequenceNumber: 1, TotalRecords: 3,
		ComplianceEvents: []*pb.ComplianceEventProto{
			{AppId: 1, FilterName: "Wolf", EventType: "guardrail.prompt_injection", Severity: "warning", Metadata: meta("edge-flag"), Timestamp: now},
			{AppId: 1, FilterName: "Wolf", EventType: "guardrail.prompt_injection", Severity: "warning", Metadata: meta("forged"), Timestamp: now},
			{AppId: 1, FilterName: "Wolf", EventType: "guardrail.prompt_injection", Severity: "warning", Metadata: fmt.Sprintf(`{"filter_id":%d}`, filter.ID), Timestamp: now},
		},
	}
	resp, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	require.True(t, resp.Success)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1
	}, 3*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1, "one alert for the one configured alerting event")
	assert.Equal(t, "edge-flag", got[0].Alert)
	assert.NotZero(t, got[0].ComplianceEventID)
	assert.Equal(t, filter.ID, got[0].FilterID)
}
