// services/reload_coordinator.go
package services

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ControlServerInterface defines the interface for control server operations (avoids import cycle)
type ControlServerInterface interface {
	GetConnectedEdges() map[string]interface{} // Returns edge instances as interface{}
	SendReloadRequest(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error
	SetReloadCoordinator(coordinator interface{})
}

// ReloadCoordinator orchestrates distributed configuration reloads across edge instances
type ReloadCoordinator struct {
	controlServer    ControlServerInterface
	activeOperations map[string]*DistributedReloadOperation // operation_id -> operation status
	mu              sync.RWMutex
}

// DistributedReloadOperation tracks the state of a distributed reload operation
type DistributedReloadOperation struct {
	OperationID     string                         `json:"operation_id"`
	TargetNamespace string                         `json:"target_namespace"`
	TargetEdges     []string                       `json:"target_edges"`
	InitiatedBy     string                         `json:"initiated_by"`
	StartTime       time.Time                      `json:"start_time"`
	TimeoutAt       time.Time                      `json:"timeout_at"`
	Status          string                         `json:"status"` // "initiated", "in_progress", "completed", "failed", "timed_out"
	EdgeStatus      map[string]*EdgeReloadStatus   `json:"edge_status"` // edge_id -> status
}

// EdgeReloadStatus tracks reload status for individual edge instances
type EdgeReloadStatus struct {
	EdgeID          string                                 `json:"edge_id"`
	CurrentPhase    pb.ReloadPhase                        `json:"current_phase"`
	Success         *bool                                  `json:"success,omitempty"` // nil=in_progress, true/false=completed
	ErrorMessage    string                                 `json:"error_message,omitempty"`
	LastUpdate      time.Time                              `json:"last_update"`
	ResponseHistory []*pb.ConfigurationReloadResponse     `json:"response_history"`
}

// NewReloadCoordinator creates a new reload coordinator
func NewReloadCoordinator(controlServer ControlServerInterface) *ReloadCoordinator {
	return &ReloadCoordinator{
		controlServer:    controlServer,
		activeOperations: make(map[string]*DistributedReloadOperation),
	}
}

// InitiateNamespaceReload initiates a configuration reload for all edges in a namespace
func (rc *ReloadCoordinator) InitiateNamespaceReload(namespace string, initiatedBy string, timeoutSeconds int64) (*ReloadOperation, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Generate unique operation ID
	operationID := fmt.Sprintf("reload-%s-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano()%1000)

	log.Info().
		Str("operation_id", operationID).
		Str("target_namespace", namespace).
		Str("initiated_by", initiatedBy).
		Msg("Initiating distributed configuration reload")

	// Find target edges based on namespace
	targetEdges := rc.getTargetEdgesByNamespace(namespace)
	if len(targetEdges) == 0 {
		return nil, fmt.Errorf("no connected edges found in namespace '%s'", namespace)
	}

	// Create operation
	now := time.Now()
	operation := &DistributedReloadOperation{
		OperationID:     operationID,
		TargetNamespace: namespace,
		TargetEdges:     targetEdges,
		InitiatedBy:     initiatedBy,
		StartTime:       now,
		TimeoutAt:       now.Add(time.Duration(timeoutSeconds) * time.Second),
		Status:          "initiated",
		EdgeStatus:      make(map[string]*EdgeReloadStatus),
	}

	// Initialize edge status tracking
	for _, edgeID := range targetEdges {
		operation.EdgeStatus[edgeID] = &EdgeReloadStatus{
			EdgeID:          edgeID,
			CurrentPhase:    pb.ReloadPhase_PONG, // Will be updated when edge responds
			LastUpdate:      now,
			ResponseHistory: make([]*pb.ConfigurationReloadResponse, 0),
		}
	}

	// Send reload requests to target edges
	reloadReq := &pb.ConfigurationReloadRequest{
		OperationId:      operationID,
		TargetNamespace:  namespace,
		TargetEdges:      targetEdges,
		InitiatedBy:      initiatedBy,
		TimeoutSeconds:   timeoutSeconds,
		InitiatedAt:      timestamppb.New(now),
	}

	successCount := 0
	for _, edgeID := range targetEdges {
		if err := rc.sendReloadRequestToEdge(edgeID, reloadReq); err != nil {
			log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to send reload request to edge")
			operation.EdgeStatus[edgeID].CurrentPhase = pb.ReloadPhase_FAILED
			operation.EdgeStatus[edgeID].ErrorMessage = err.Error()
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to send reload request to any edges")
	}

	operation.Status = "in_progress"
	rc.activeOperations[operationID] = operation

	log.Info().
		Str("operation_id", operationID).
		Int("target_edge_count", len(targetEdges)).
		Int("requests_sent", successCount).
		Msg("Distributed reload operation initiated")

	// Start timeout monitoring
	go rc.monitorOperationTimeout(operationID)

	// Convert to simplified ReloadOperation for return
	simpleOperation := &ReloadOperation{
		OperationID:     operation.OperationID,
		TargetNamespace: operation.TargetNamespace,
		TargetEdges:     operation.TargetEdges,
		InitiatedBy:     operation.InitiatedBy,
		InitiatedAt:     operation.StartTime,
		Status:          operation.Status,
		Progress:        0, // TODO: Calculate progress based on edge status
		Message:         fmt.Sprintf("Reload operation initiated for namespace '%s'", namespace),
	}

	return simpleOperation, nil
}

// InitiateEdgeReload initiates a configuration reload for specific edge instances
func (rc *ReloadCoordinator) InitiateEdgeReload(edgeIDs []string, initiatedBy string, timeoutSeconds int64) (*ReloadOperation, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Generate unique operation ID
	operationID := fmt.Sprintf("edge-reload-%s-%d", time.Now().Format("20060102-150405"), time.Now().UnixNano()%1000)

	log.Info().
		Str("operation_id", operationID).
		Strs("target_edges", edgeIDs).
		Str("initiated_by", initiatedBy).
		Msg("Initiating edge-specific configuration reload")

	// Validate that all edges exist and are connected
	connectedEdges := rc.validateEdgesConnected(edgeIDs)
	if len(connectedEdges) == 0 {
		return nil, fmt.Errorf("no connected edges found in specified list")
	}

	// Create operation  
	now := time.Now()
	operation := &DistributedReloadOperation{
		OperationID:     operationID,
		TargetNamespace: "mixed", // Multiple namespaces possible
		TargetEdges:     connectedEdges,
		InitiatedBy:     initiatedBy,
		StartTime:       now,
		TimeoutAt:       now.Add(time.Duration(timeoutSeconds) * time.Second),
		Status:          "initiated",
		EdgeStatus:      make(map[string]*EdgeReloadStatus),
	}

	// Initialize edge status tracking and send reload requests
	reloadReq := &pb.ConfigurationReloadRequest{
		OperationId:     operationID,
		TargetEdges:     connectedEdges,
		InitiatedBy:     initiatedBy,
		TimeoutSeconds:  timeoutSeconds,
		InitiatedAt:     timestamppb.New(now),
	}

	successCount := 0
	for _, edgeID := range connectedEdges {
		operation.EdgeStatus[edgeID] = &EdgeReloadStatus{
			EdgeID:          edgeID,
			CurrentPhase:    pb.ReloadPhase_PONG,
			LastUpdate:      now,
			ResponseHistory: make([]*pb.ConfigurationReloadResponse, 0),
		}

		if err := rc.sendReloadRequestToEdge(edgeID, reloadReq); err != nil {
			log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to send reload request to edge")
			operation.EdgeStatus[edgeID].CurrentPhase = pb.ReloadPhase_FAILED
			operation.EdgeStatus[edgeID].ErrorMessage = err.Error()
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to send reload request to any edges")
	}

	operation.Status = "in_progress"
	rc.activeOperations[operationID] = operation

	// Start timeout monitoring
	go rc.monitorOperationTimeout(operationID)

	// Convert to simplified ReloadOperation for return
	simpleOperation := &ReloadOperation{
		OperationID:     operation.OperationID,
		TargetNamespace: operation.TargetNamespace,
		TargetEdges:     operation.TargetEdges,
		InitiatedBy:     operation.InitiatedBy,
		InitiatedAt:     operation.StartTime,
		Status:          operation.Status,
		Progress:        0, // TODO: Calculate progress based on edge status
		Message:         fmt.Sprintf("Reload operation initiated for edges: %v", connectedEdges),
	}

	return simpleOperation, nil
}

// GetOperationStatus retrieves the status of a reload operation
func (rc *ReloadCoordinator) GetOperationStatus(operationID string) (*ReloadOperation, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	operation, exists := rc.activeOperations[operationID]
	if !exists {
		return nil, fmt.Errorf("operation not found: %s", operationID)
	}

	// Convert to simplified ReloadOperation for return
	simpleOperation := &ReloadOperation{
		OperationID:     operation.OperationID,
		TargetNamespace: operation.TargetNamespace,
		TargetEdges:     operation.TargetEdges,
		InitiatedBy:     operation.InitiatedBy,
		InitiatedAt:     operation.StartTime,
		Status:          operation.Status,
		Progress:        rc.calculateProgress(operation),
		Message:         rc.generateStatusMessage(operation),
	}

	return simpleOperation, nil
}

// ListActiveOperations returns all active reload operations
func (rc *ReloadCoordinator) ListActiveOperations() []*ReloadOperation {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	operations := make([]*ReloadOperation, 0, len(rc.activeOperations))
	for _, operation := range rc.activeOperations {
		// Convert to simplified ReloadOperation for return
		simpleOperation := &ReloadOperation{
			OperationID:     operation.OperationID,
			TargetNamespace: operation.TargetNamespace,
			TargetEdges:     operation.TargetEdges,
			InitiatedBy:     operation.InitiatedBy,
			InitiatedAt:     operation.StartTime,
			Status:          operation.Status,
			Progress:        rc.calculateProgress(operation),
			Message:         rc.generateStatusMessage(operation),
		}
		operations = append(operations, simpleOperation)
	}

	return operations
}

// ProcessReloadResponse processes reload status responses from edge instances
func (rc *ReloadCoordinator) ProcessReloadResponse(response *pb.ConfigurationReloadResponse) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	operation, exists := rc.activeOperations[response.OperationId]
	if !exists {
		log.Warn().
			Str("operation_id", response.OperationId).
			Str("edge_id", response.EdgeId).
			Msg("Received reload response for unknown operation")
		return
	}

	edgeStatus, exists := operation.EdgeStatus[response.EdgeId]
	if !exists {
		log.Warn().
			Str("operation_id", response.OperationId).
			Str("edge_id", response.EdgeId).
			Msg("Received reload response for unknown edge")
		return
	}

	// Update edge status
	edgeStatus.CurrentPhase = response.Phase
	edgeStatus.LastUpdate = time.Now()
	edgeStatus.ResponseHistory = append(edgeStatus.ResponseHistory, response)

	// Set success/failure based on phase
	if response.Phase == pb.ReloadPhase_READY {
		success := true
		edgeStatus.Success = &success
	} else if response.Phase == pb.ReloadPhase_FAILED {
		failure := false
		edgeStatus.Success = &failure
		edgeStatus.ErrorMessage = response.Message
	}

	log.Info().
		Str("operation_id", response.OperationId).
		Str("edge_id", response.EdgeId).
		Str("phase", response.Phase.String()).
		Bool("success", response.Success).
		Str("message", response.Message).
		Msg("Processed reload response from edge")

	// Check if operation is complete
	rc.checkOperationCompletion(operation)
}

// Helper methods

func (rc *ReloadCoordinator) getTargetEdgesByNamespace(namespace string) []string {
	edges := rc.controlServer.GetConnectedEdges()
	var targetEdges []string

	log.Debug().
		Str("target_namespace", namespace).
		Int("total_connected_edges", len(edges)).
		Msg("Filtering edges by namespace")

	for edgeID, edgeInterface := range edges {
		// Extract namespace from edge info map
		edgeNamespace := ""
		if edgeInfoMap, ok := edgeInterface.(map[string]interface{}); ok {
			if ns, exists := edgeInfoMap["namespace"]; exists {
				edgeNamespace = fmt.Sprintf("%v", ns)
			}
		}

		log.Debug().
			Str("edge_id", edgeID).
			Str("edge_namespace", edgeNamespace).
			Str("target_namespace", namespace).
			Msg("Evaluating edge for namespace match")

		// Namespace matching logic
		shouldInclude := false
		if namespace == "all" {
			shouldInclude = true
		} else if namespace == "global" && edgeNamespace == "" {
			shouldInclude = true // Global namespace match
		} else if namespace == edgeNamespace {
			shouldInclude = true // Exact match
		}

		if shouldInclude {
			targetEdges = append(targetEdges, edgeID)
			log.Debug().Str("edge_id", edgeID).Msg("Edge included in reload target")
		} else {
			log.Debug().Str("edge_id", edgeID).Msg("Edge excluded from reload target")
		}
	}

	log.Info().
		Str("target_namespace", namespace).
		Int("total_edges", len(edges)).
		Int("target_edges", len(targetEdges)).
		Strs("target_edge_ids", targetEdges).
		Msg("Namespace filtering completed")

	return targetEdges
}

func (rc *ReloadCoordinator) validateEdgesConnected(edgeIDs []string) []string {
	edges := rc.controlServer.GetConnectedEdges()
	var connectedEdges []string

	for _, edgeID := range edgeIDs {
		if _, exists := edges[edgeID]; exists {
			connectedEdges = append(connectedEdges, edgeID)
		}
	}

	return connectedEdges
}

func (rc *ReloadCoordinator) sendReloadRequestToEdge(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error {
	return rc.controlServer.SendReloadRequest(edgeID, reloadReq)
}

func (rc *ReloadCoordinator) checkOperationCompletion(operation *DistributedReloadOperation) {
	allComplete := true
	successCount := 0
	failureCount := 0

	for _, edgeStatus := range operation.EdgeStatus {
		if edgeStatus.CurrentPhase != pb.ReloadPhase_READY && edgeStatus.CurrentPhase != pb.ReloadPhase_FAILED {
			allComplete = false
		} else if edgeStatus.Success != nil {
			if *edgeStatus.Success {
				successCount++
			} else {
				failureCount++
			}
		}
	}

	if allComplete {
		if failureCount == 0 {
			operation.Status = "completed"
			log.Info().
				Str("operation_id", operation.OperationID).
				Int("success_count", successCount).
				Msg("Distributed reload operation completed successfully")
		} else {
			operation.Status = "failed"
			log.Warn().
				Str("operation_id", operation.OperationID).
				Int("success_count", successCount).
				Int("failure_count", failureCount).
				Msg("Distributed reload operation completed with failures")
		}
	}
}

func (rc *ReloadCoordinator) monitorOperationTimeout(operationID string) {
	rc.mu.RLock()
	operation, exists := rc.activeOperations[operationID]
	if !exists {
		rc.mu.RUnlock()
		return
	}
	timeoutAt := operation.TimeoutAt
	rc.mu.RUnlock()

	// Wait for timeout
	time.Sleep(time.Until(timeoutAt))

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Check if operation is still active
	operation, exists = rc.activeOperations[operationID]
	if !exists || operation.Status == "completed" || operation.Status == "failed" {
		return
	}

	// Mark operation as timed out
	operation.Status = "timed_out"
	
	// Mark any incomplete edges as failed
	for _, edgeStatus := range operation.EdgeStatus {
		if edgeStatus.CurrentPhase != pb.ReloadPhase_READY && edgeStatus.CurrentPhase != pb.ReloadPhase_FAILED {
			edgeStatus.CurrentPhase = pb.ReloadPhase_FAILED
			failure := false
			edgeStatus.Success = &failure
			edgeStatus.ErrorMessage = "Operation timed out"
		}
	}

	log.Warn().
		Str("operation_id", operationID).
		Str("target_namespace", operation.TargetNamespace).
		Msg("Reload operation timed out")
}

// calculateProgress calculates the progress percentage for a distributed reload operation
func (rc *ReloadCoordinator) calculateProgress(operation *DistributedReloadOperation) int {
	if len(operation.EdgeStatus) == 0 {
		return 0
	}

	completedCount := 0
	for _, edgeStatus := range operation.EdgeStatus {
		if edgeStatus.CurrentPhase == pb.ReloadPhase_READY || edgeStatus.CurrentPhase == pb.ReloadPhase_FAILED {
			completedCount++
		}
	}

	return (completedCount * 100) / len(operation.EdgeStatus)
}

// generateStatusMessage generates a status message for a distributed reload operation
func (rc *ReloadCoordinator) generateStatusMessage(operation *DistributedReloadOperation) string {
	totalEdges := len(operation.EdgeStatus)
	
	if totalEdges == 0 {
		return "No target edges"
	}

	successCount := 0
	failureCount := 0
	inProgressCount := 0

	for _, edgeStatus := range operation.EdgeStatus {
		if edgeStatus.CurrentPhase == pb.ReloadPhase_READY {
			successCount++
		} else if edgeStatus.CurrentPhase == pb.ReloadPhase_FAILED {
			failureCount++
		} else {
			inProgressCount++
		}
	}

	switch operation.Status {
	case "initiated":
		return fmt.Sprintf("Reload operation initiated for %d edges", totalEdges)
	case "in_progress":
		if inProgressCount > 0 {
			return fmt.Sprintf("Reload in progress: %d successful, %d failed, %d in progress", successCount, failureCount, inProgressCount)
		} else {
			return fmt.Sprintf("Reload completing: %d successful, %d failed", successCount, failureCount)
		}
	case "completed":
		return fmt.Sprintf("Reload completed successfully on all %d edges", totalEdges)
	case "failed":
		return fmt.Sprintf("Reload failed: %d successful, %d failed out of %d edges", successCount, failureCount, totalEdges)
	case "timed_out":
		return fmt.Sprintf("Reload timed out: %d successful, %d failed, %d incomplete out of %d edges", successCount, failureCount, inProgressCount, totalEdges)
	default:
		return fmt.Sprintf("Unknown status: %s", operation.Status)
	}
}