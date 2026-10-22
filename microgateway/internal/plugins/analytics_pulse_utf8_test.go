package plugins

import (
	"context"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// marshalingSyncClient marshals each pulse as the gRPC codec does, so a pulse
// gRPC could not send fails here too.
type marshalingSyncClient struct {
	pb.ConfigurationSyncServiceClient
	mu     sync.Mutex
	pulses []*pb.AnalyticsPulse
}

func (c *marshalingSyncClient) SendAnalyticsPulse(_ context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	if _, err := proto.Marshal(pulse); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pulses = append(c.pulses, pulse)
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

// A body stored cut inside a multi-byte character failed every pulse on prod
// (2026-10-21, "grpc: error while marshaling: string field contains invalid
// UTF-8"): the failed pulse is kept for the next one, so the edge delivered
// no analytics, budget usage or compliance events from then on.
func TestAnalyticsPulse_InvalidUTF8IsReplacedAndThePulseIsDelivered(t *testing.T) {
	client := &marshalingSyncClient{}
	p := &AnalyticsPulsePlugin{
		config: &PulsePluginConfig{MaxBufferSize: 100, TimeoutSeconds: 1, MaxRetries: 0,
			IntervalSeconds: 3600, IncludeRequestResponseData: true, EdgeRetentionHours: 24},
		edgeID: "edge-1", edgeNamespace: "test", clients: staticClient{client},
		sequenceNumber: 1, lastPulseTime: time.Now(), retryInterval: time.Millisecond,
	}
	cut := "caf\xc3" // "café" cut after the first byte of "é"
	now := time.Now()
	p.analyticsBuffer = []database.AnalyticsEvent{{RequestID: "r1", TimeStamp: now, ErrorMessage: cut}}
	p.analyticsMetadata = []AnalyticsMetadata{{RequestID: "r1", RequestBody: "ok body", ResponseBody: cut}}
	p.complianceBuffer = []ComplianceEventBuffer{{FilterName: "guard", Description: cut, Metadata: `{"tool":"` + cut + `"}`, Timestamp: now}}

	p.sendPulse()

	require.Len(t, client.pulses, 1, "the pulse is sent")
	got := client.pulses[0]
	require.Len(t, got.AnalyticsEvents, 1)
	assert.Equal(t, "caf\uFFFD", got.AnalyticsEvents[0].ResponseBody)
	assert.Equal(t, "caf\uFFFD", got.AnalyticsEvents[0].ErrorMessage)
	assert.Equal(t, "ok body", got.AnalyticsEvents[0].RequestBody, "valid text is untouched")
	require.Len(t, got.ComplianceEvents, 1)
	assert.True(t, utf8.ValidString(got.ComplianceEvents[0].Description))
	assert.True(t, utf8.ValidString(got.ComplianceEvents[0].Metadata))

	p.bufferMutex.RLock()
	defer p.bufferMutex.RUnlock()
	assert.Empty(t, p.analyticsBuffer, "delivered data leaves the buffer")
	assert.Empty(t, p.complianceBuffer)
	assert.NoError(t, p.lastError)
}

func TestMakeValidUTF8_ListsMapsAndNestedMessages(t *testing.T) {
	m := &pb.AnalyticsPulse{
		EdgeId:          "edge\xff",
		AnalyticsEvents: []*pb.AnalyticsEvent{{RequestId: "a"}, {RequestId: "b\xfe"}},
		ProxySummaries:  []*pb.ProxyLogSummary{{UniqueModels: []string{"gpt", "x\xc3"}}},
	}
	_, err := proto.Marshal(m)
	require.Error(t, err, "the fixture really is unmarshalable")

	assert.Equal(t, 3, makeValidUTF8(m.ProtoReflect()))
	_, err = proto.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, "edge\uFFFD", m.EdgeId)
	assert.Equal(t, "a", m.AnalyticsEvents[0].RequestId)
	assert.Equal(t, "b\uFFFD", m.AnalyticsEvents[1].RequestId)
	assert.Equal(t, []string{"gpt", "x\uFFFD"}, m.ProxySummaries[0].UniqueModels)
	assert.Equal(t, 0, makeValidUTF8(m.ProtoReflect()), "nothing left to fix")
}

func TestMakeValidUTF8_MapKeysAndValues(t *testing.T) {
	m := &pb.EdgeRegistrationRequest{Metadata: map[string]string{"region": "eu\xff", "k\xfe": "v", "ok": "fine"}}
	assert.Equal(t, 2, makeValidUTF8(m.ProtoReflect()))
	_, err := proto.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"region": "eu�", "k�": "v", "ok": "fine"}, m.Metadata)
}
