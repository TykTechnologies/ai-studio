// internal/grpc/control_payload_queue.go
package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// ControlPayloadQueue manages queuing and batching of plugin payloads for transmission to control
type ControlPayloadQueue struct {
	db            *gorm.DB
	config        *config.ControlPayloadConfig
	edgeID        string
	edgeNamespace string
	grpcClient    pb.ConfigurationSyncServiceClient
	authCtxFunc   func(context.Context) context.Context

	// Synchronization
	mu             sync.Mutex
	sequenceNumber uint64

	// State
	started bool
	stopCh  chan struct{}

	// Stats
	totalQueued uint64
	totalSent   uint64
	totalFailed uint64
	lastError   error
}

// NewControlPayloadQueue creates a new control payload queue
func NewControlPayloadQueue(
	db *gorm.DB,
	cfg *config.ControlPayloadConfig,
	edgeID, edgeNamespace string,
) *ControlPayloadQueue {
	return &ControlPayloadQueue{
		db:             db,
		config:         cfg,
		edgeID:         edgeID,
		edgeNamespace:  edgeNamespace,
		sequenceNumber: 1,
		stopCh:         make(chan struct{}),
	}
}

// SetGRPCClient sets the gRPC client for sending batches
func (q *ControlPayloadQueue) SetGRPCClient(client pb.ConfigurationSyncServiceClient, authCtxFunc func(context.Context) context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.grpcClient = client
	q.authCtxFunc = authCtxFunc
}

// Start begins the queue (runs migrations if needed)
func (q *ControlPayloadQueue) Start() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.started {
		return nil
	}

	// Auto-migrate the ControlPayload model
	if err := q.db.AutoMigrate(&database.ControlPayload{}); err != nil {
		return fmt.Errorf("failed to auto-migrate ControlPayload: %w", err)
	}

	q.started = true

	log.Debug().
		Str("edge_id", q.edgeID).
		Bool("enabled", q.config.Enabled).
		Int64("max_payload_size", q.config.MaxPayloadSizeBytes).
		Int("batch_threshold", q.config.BatchThreshold).
		Msg("Control payload queue started")

	return nil
}

// Stop flushes remaining payloads and stops the queue
func (q *ControlPayloadQueue) Stop() {
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		return
	}
	q.started = false
	q.mu.Unlock()

	close(q.stopCh)

	// Final flush
	if err := q.SendPendingBatch(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to flush control payload queue on shutdown")
	}

	log.Debug().
		Uint64("total_queued", q.totalQueued).
		Uint64("total_sent", q.totalSent).
		Uint64("total_failed", q.totalFailed).
		Msg("Control payload queue stopped")
}

// QueuePayload adds a payload to the queue (persists to database)
func (q *ControlPayloadQueue) QueuePayload(pluginID uint, payload []byte, correlationID string, metadata map[string]string) error {
	if !q.config.Enabled {
		return fmt.Errorf("control payload queue is disabled")
	}

	// Validate payload is not empty
	if len(payload) == 0 {
		return fmt.Errorf("payload cannot be empty")
	}

	// Validate payload size
	if int64(len(payload)) > q.config.MaxPayloadSizeBytes {
		return fmt.Errorf("payload size %d exceeds maximum %d bytes", len(payload), q.config.MaxPayloadSizeBytes)
	}

	// Serialize metadata
	var metadataJSON []byte
	if metadata != nil && len(metadata) > 0 {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize metadata: %w", err)
		}
	}

	// Create database record
	record := &database.ControlPayload{
		PluginID:      pluginID,
		Payload:       payload,
		CorrelationID: correlationID,
		Metadata:      metadataJSON,
		Sent:          false,
		CreatedAt:     time.Now(),
	}

	if err := q.db.Create(record).Error; err != nil {
		return fmt.Errorf("failed to queue payload: %w", err)
	}

	q.mu.Lock()
	q.totalQueued++
	pendingCount := q.getPendingCountLocked()
	q.mu.Unlock()

	log.Debug().
		Uint("plugin_id", pluginID).
		Int("payload_size", len(payload)).
		Str("correlation_id", correlationID).
		Int64("pending_count", pendingCount).
		Msg("Control payload queued")

	// Check if we should trigger immediate send
	if int(pendingCount) >= q.config.BatchThreshold {
		go func() {
			if err := q.SendPendingBatch(context.Background()); err != nil {
				log.Error().Err(err).Msg("Failed to send batch after threshold reached")
			}
		}()
	}

	return nil
}

// SendPendingBatch sends all pending payloads to control
// This should be called from the heartbeat worker
func (q *ControlPayloadQueue) SendPendingBatch(ctx context.Context) error {
	q.mu.Lock()
	if q.grpcClient == nil {
		q.mu.Unlock()
		return fmt.Errorf("gRPC client not set")
	}
	client := q.grpcClient
	authCtxFunc := q.authCtxFunc
	q.mu.Unlock()

	// Get pending payloads
	var payloads []database.ControlPayload
	if err := q.db.Where("sent = ?", false).Order("created_at ASC").Limit(q.config.BatchThreshold).Find(&payloads).Error; err != nil {
		return fmt.Errorf("failed to fetch pending payloads: %w", err)
	}

	if len(payloads) == 0 {
		return nil // Nothing to send
	}

	// Check total batch size
	var totalSize int64
	for _, p := range payloads {
		totalSize += int64(len(p.Payload))
	}

	if totalSize > q.config.MaxBatchSizeBytes {
		// Reduce batch size to fit within limit
		var reducedPayloads []database.ControlPayload
		var reducedSize int64
		for _, p := range payloads {
			if reducedSize+int64(len(p.Payload)) > q.config.MaxBatchSizeBytes {
				break
			}
			reducedPayloads = append(reducedPayloads, p)
			reducedSize += int64(len(p.Payload))
		}
		payloads = reducedPayloads
	}

	// Convert to proto messages
	protoPayloads := make([]*pb.PluginControlPayload, len(payloads))
	payloadIDs := make([]uint, len(payloads))

	for i, p := range payloads {
		var metadata map[string]string
		if p.Metadata != nil && len(p.Metadata) > 0 {
			json.Unmarshal(p.Metadata, &metadata)
		}

		protoPayloads[i] = &pb.PluginControlPayload{
			PluginId:      uint32(p.PluginID),
			Payload:       p.Payload,
			EdgeId:        q.edgeID,
			EdgeNamespace: q.edgeNamespace,
			Timestamp:     timestamppb.New(p.CreatedAt),
			CorrelationId: p.CorrelationID,
			Metadata:      metadata,
		}
		payloadIDs[i] = p.ID
	}

	// Get sequence number
	q.mu.Lock()
	seqNum := q.sequenceNumber
	q.sequenceNumber++
	q.mu.Unlock()

	// Create batch
	batch := &pb.PluginControlBatch{
		EdgeId:         q.edgeID,
		EdgeNamespace:  q.edgeNamespace,
		Payloads:       protoPayloads,
		SequenceNumber: seqNum,
		BatchTimestamp: timestamppb.Now(),
		TotalPayloads:  uint32(len(protoPayloads)),
	}

	log.Debug().
		Uint64("sequence", seqNum).
		Int("payload_count", len(protoPayloads)).
		Int64("total_size", totalSize).
		Msg("Sending plugin control batch to control server")

	// Send to control
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if authCtxFunc != nil {
		sendCtx = authCtxFunc(sendCtx)
	}

	resp, err := client.SendPluginControlBatch(sendCtx, batch)
	if err != nil {
		q.mu.Lock()
		q.totalFailed += uint64(len(payloads))
		q.lastError = err
		q.mu.Unlock()

		log.Error().
			Err(err).
			Uint64("sequence", seqNum).
			Int("payload_count", len(payloads)).
			Msg("Failed to send plugin control batch")
		return fmt.Errorf("failed to send batch: %w", err)
	}

	// Mark payloads as sent
	now := time.Now()
	if err := q.db.Model(&database.ControlPayload{}).
		Where("id IN ?", payloadIDs).
		Updates(map[string]interface{}{
			"sent":    true,
			"sent_at": now,
		}).Error; err != nil {
		log.Error().Err(err).Msg("Failed to mark payloads as sent")
	}

	q.mu.Lock()
	q.totalSent += uint64(len(payloads))
	q.mu.Unlock()

	log.Debug().
		Uint64("sequence", seqNum).
		Uint64("processed", resp.ProcessedCount).
		Bool("success", resp.Success).
		Int("errors", len(resp.Errors)).
		Msg("Plugin control batch sent successfully")

	// Log any per-payload errors
	for _, errMsg := range resp.Errors {
		log.Warn().
			Uint32("plugin_id", errMsg.PluginId).
			Str("correlation_id", errMsg.CorrelationId).
			Str("error", errMsg.ErrorMessage).
			Msg("Control server reported error for payload")
	}

	return nil
}

// CleanupOldPayloads removes sent payloads older than retention period
func (q *ControlPayloadQueue) CleanupOldPayloads() error {
	cutoff := time.Now().Add(-time.Duration(q.config.RetentionHours) * time.Hour)

	result := q.db.Where("sent = ? AND sent_at < ?", true, cutoff).Delete(&database.ControlPayload{})
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old payloads: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Debug().
			Int64("deleted", result.RowsAffected).
			Int("retention_hours", q.config.RetentionHours).
			Msg("Cleaned up old control payloads")
	}

	return nil
}

// GetStats returns queue statistics
func (q *ControlPayloadQueue) GetStats() (queued, sent, failed uint64, lastErr error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalQueued, q.totalSent, q.totalFailed, q.lastError
}

// getPendingCountLocked returns the count of pending payloads (must hold mu lock)
func (q *ControlPayloadQueue) getPendingCountLocked() int64 {
	var count int64
	q.db.Model(&database.ControlPayload{}).Where("sent = ?", false).Count(&count)
	return count
}

// GetPendingCount returns the count of pending payloads
func (q *ControlPayloadQueue) GetPendingCount() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.getPendingCountLocked()
}
