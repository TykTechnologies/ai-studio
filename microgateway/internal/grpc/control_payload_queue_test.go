package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// payloadControl answers plugin control batches: a retryable error for the
// payloads whose body is in retry, a final one for those in reject.
type payloadControl struct {
	pb.ConfigurationSyncServiceClient
	retry, reject map[string]bool
	got           []string
}

func (c *payloadControl) SendPluginControlBatch(_ context.Context, b *pb.PluginControlBatch, _ ...grpc.CallOption) (*pb.PluginControlBatchResponse, error) {
	resp := &pb.PluginControlBatchResponse{Success: true}
	for i, p := range b.Payloads {
		c.got = append(c.got, string(p.Payload))
		switch {
		case c.retry[string(p.Payload)]:
			resp.Errors = append(resp.Errors, &pb.PluginPayloadError{PluginId: p.PluginId, ErrorMessage: "awaiting approval", Retry: true, PayloadIndex: uint32(i)})
		case c.reject[string(p.Payload)]:
			resp.Errors = append(resp.Errors, &pb.PluginPayloadError{PluginId: p.PluginId, ErrorMessage: "bad payload", PayloadIndex: uint32(i)})
		default:
			resp.ProcessedCount++
		}
	}
	return resp, nil
}

func newTestPayloadQueue(t *testing.T, control *payloadControl) (*ControlPayloadQueue, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	q := NewControlPayloadQueue(db, &config.ControlPayloadConfig{Enabled: true, MaxPayloadSizeBytes: 1 << 20,
		MaxBatchSizeBytes: 10 << 20, BatchThreshold: 100, RetentionHours: 24}, "edge-1", "default")
	require.NoError(t, q.Start())
	q.SetGRPCClient(control, nil)
	return q, db
}

func unsentPayloads(t *testing.T, db *gorm.DB) []string {
	var rows []database.ControlPayload
	require.NoError(t, db.Where("sent = ?", false).Order("id").Find(&rows).Error)
	var out []string
	for _, r := range rows {
		out = append(out, string(r.Payload))
	}
	return out
}

// rlk: a payload control could not deliver for a reason that may pass (a
// Studio plugin's scopes awaiting approval) stays queued and is sent again
// after payloadRetryDelay; one the plugin refused is final, as is one
// delivered. Meanwhile the payloads behind it still go out.
func TestControlPayloadQueue_RetryableRejectionStaysQueued(t *testing.T) {
	control := &payloadControl{retry: map[string]bool{"saving-1": true}, reject: map[string]bool{"garbage": true}}
	q, db := newTestPayloadQueue(t, control)
	for _, p := range []string{"saving-1", "garbage", "saving-2"} {
		require.NoError(t, q.QueuePayload(42, []byte(p), "", nil))
	}

	require.NoError(t, q.SendPendingBatch(context.Background()))
	assert.Equal(t, []string{"saving-1"}, unsentPayloads(t, db), "only the retryable payload stays queued")

	// Within the delay it is not sent again; newer payloads are.
	require.NoError(t, q.QueuePayload(42, []byte("saving-3"), "", nil))
	control.got = nil
	require.NoError(t, q.SendPendingBatch(context.Background()))
	assert.Equal(t, []string{"saving-3"}, control.got)

	// Once the approval is in and the delay has passed, it is delivered.
	delete(control.retry, "saving-1")
	q.mu.Lock()
	for id := range q.retryAt {
		q.retryAt[id] = time.Now().Add(-time.Second)
	}
	q.mu.Unlock()
	control.got = nil
	require.NoError(t, q.SendPendingBatch(context.Background()))
	assert.Equal(t, []string{"saving-1"}, control.got)
	assert.Empty(t, unsentPayloads(t, db))
}

// A payload that cannot be delivered is retried only within the retention.
func TestControlPayloadQueue_CleanupDropsExpiredUnsent(t *testing.T) {
	q, db := newTestPayloadQueue(t, &payloadControl{})
	require.NoError(t, q.QueuePayload(42, []byte("old"), "", nil))
	require.NoError(t, q.QueuePayload(42, []byte("fresh"), "", nil))
	require.NoError(t, db.Model(&database.ControlPayload{}).Where("payload = ?", []byte("old")).
		Update("created_at", time.Now().Add(-25*time.Hour)).Error)

	require.NoError(t, q.CleanupOldPayloads())
	assert.Equal(t, []string{"fresh"}, unsentPayloads(t, db))
}
