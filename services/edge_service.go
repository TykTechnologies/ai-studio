package services

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// EdgeService handles edge instance management for hub-and-spoke architecture
type EdgeService struct {
	db *gorm.DB
}

// NewEdgeService creates a new EdgeService
func NewEdgeService(db *gorm.DB) *EdgeService {
	return &EdgeService{
		db: db,
	}
}

// EdgeInstanceWithHealth represents an edge instance with health status
type EdgeInstanceWithHealth struct {
	*models.EdgeInstance
	IsHealthy      bool          `json:"is_healthy"`
	LastHeartbeat  time.Duration `json:"last_heartbeat_ago,omitempty"`
}

// EdgeStatistics contains edge statistics for a namespace
type EdgeStatistics struct {
	TotalEdges      int64 `json:"total_edges"`
	ConnectedEdges  int64 `json:"connected_edges"`
	HealthyEdges    int64 `json:"healthy_edges"`
	DisconnectedEdges int64 `json:"disconnected_edges"`
	UnhealthyEdges  int64 `json:"unhealthy_edges"`
}

// ListEdges returns paginated list of edge instances with optional filtering
func (s *EdgeService) ListEdges(namespace string, status string, page, limit int) ([]EdgeInstanceWithHealth, int64, error) {
	// Build query
	query := s.db.Model(&models.EdgeInstance{})
	
	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count edges: %w", err)
	}

	// Get paginated results
	var edges []models.EdgeInstance
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&edges).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list edges: %w", err)
	}

	// Add health information
	result := make([]EdgeInstanceWithHealth, len(edges))
	for i, edge := range edges {
		result[i] = EdgeInstanceWithHealth{
			EdgeInstance: &edge,
			IsHealthy:    edge.IsHealthy(5 * time.Minute), // 5 minutes healthy threshold
		}
		
		if edge.LastHeartbeat != nil {
			result[i].LastHeartbeat = time.Since(*edge.LastHeartbeat)
		}
	}

	return result, totalCount, nil
}

// GetEdgeByID returns an edge instance by its database ID
func (s *EdgeService) GetEdgeByID(id uint) (*EdgeInstanceWithHealth, error) {
	var edge models.EdgeInstance
	if err := edge.Get(s.db, id); err != nil {
		return nil, fmt.Errorf("failed to get edge: %w", err)
	}

	result := &EdgeInstanceWithHealth{
		EdgeInstance: &edge,
		IsHealthy:    edge.IsHealthy(5 * time.Minute),
	}
	
	if edge.LastHeartbeat != nil {
		result.LastHeartbeat = time.Since(*edge.LastHeartbeat)
	}

	return result, nil
}

// GetEdgeByEdgeID returns an edge instance by its edge ID (string identifier)
func (s *EdgeService) GetEdgeByEdgeID(edgeID string) (*EdgeInstanceWithHealth, error) {
	var edge models.EdgeInstance
	if err := edge.GetByEdgeID(s.db, edgeID); err != nil {
		return nil, fmt.Errorf("failed to get edge: %w", err)
	}

	result := &EdgeInstanceWithHealth{
		EdgeInstance: &edge,
		IsHealthy:    edge.IsHealthy(5 * time.Minute),
	}
	
	if edge.LastHeartbeat != nil {
		result.LastHeartbeat = time.Since(*edge.LastHeartbeat)
	}

	return result, nil
}

// DeleteEdge removes an edge instance
func (s *EdgeService) DeleteEdge(edgeID string) error {
	var edge models.EdgeInstance
	if err := edge.GetByEdgeID(s.db, edgeID); err != nil {
		return fmt.Errorf("edge not found: %w", err)
	}

	if err := edge.Delete(s.db); err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}

	return nil
}

// GetEdgeStatistics returns statistics for edges in a namespace
func (s *EdgeService) GetEdgeStatistics(namespace string) (*EdgeStatistics, error) {
	stats := &EdgeStatistics{}

	// Base query for namespace
	baseQuery := s.db.Model(&models.EdgeInstance{})
	if namespace != "" {
		baseQuery = baseQuery.Where("namespace = ?", namespace)
	}

	// Total edges
	if err := baseQuery.Count(&stats.TotalEdges).Error; err != nil {
		return nil, fmt.Errorf("failed to count total edges: %w", err)
	}

	// Connected edges
	if err := baseQuery.Where("status = ?", models.EdgeStatusConnected).Count(&stats.ConnectedEdges).Error; err != nil {
		return nil, fmt.Errorf("failed to count connected edges: %w", err)
	}

	// Disconnected edges
	if err := baseQuery.Where("status = ?", models.EdgeStatusDisconnected).Count(&stats.DisconnectedEdges).Error; err != nil {
		return nil, fmt.Errorf("failed to count disconnected edges: %w", err)
	}

	// Unhealthy edges
	if err := baseQuery.Where("status = ?", models.EdgeStatusUnhealthy).Count(&stats.UnhealthyEdges).Error; err != nil {
		return nil, fmt.Errorf("failed to count unhealthy edges: %w", err)
	}

	// Healthy edges (connected with recent heartbeat)
	healthyCutoff := time.Now().Add(-5 * time.Minute)
	if err := baseQuery.Where("status = ? AND last_heartbeat > ?", models.EdgeStatusConnected, healthyCutoff).Count(&stats.HealthyEdges).Error; err != nil {
		return nil, fmt.Errorf("failed to count healthy edges: %w", err)
	}

	return stats, nil
}

// GetEdgesInNamespace returns all edges in a specific namespace
func (s *EdgeService) GetEdgesInNamespace(namespace string) ([]EdgeInstanceWithHealth, error) {
	var edges []models.EdgeInstance
	if err := s.db.Where("namespace = ?", namespace).Order("created_at DESC").Find(&edges).Error; err != nil {
		return nil, fmt.Errorf("failed to get edges in namespace: %w", err)
	}

	// Add health information
	result := make([]EdgeInstanceWithHealth, len(edges))
	for i, edge := range edges {
		result[i] = EdgeInstanceWithHealth{
			EdgeInstance: &edge,
			IsHealthy:    edge.IsHealthy(5 * time.Minute),
		}
		
		if edge.LastHeartbeat != nil {
			result[i].LastHeartbeat = time.Since(*edge.LastHeartbeat)
		}
	}

	return result, nil
}

// GetConnectedEdges returns all currently connected edges across all namespaces
func (s *EdgeService) GetConnectedEdges() ([]EdgeInstanceWithHealth, error) {
	var edges []models.EdgeInstance
	if err := s.db.Where("status IN ?", []string{models.EdgeStatusConnected, models.EdgeStatusRegistered}).
		Order("created_at DESC").Find(&edges).Error; err != nil {
		return nil, fmt.Errorf("failed to get connected edges: %w", err)
	}

	// Add health information
	result := make([]EdgeInstanceWithHealth, len(edges))
	for i, edge := range edges {
		result[i] = EdgeInstanceWithHealth{
			EdgeInstance: &edge,
			IsHealthy:    edge.IsHealthy(5 * time.Minute),
		}
		
		if edge.LastHeartbeat != nil {
			result[i].LastHeartbeat = time.Since(*edge.LastHeartbeat)
		}
	}

	return result, nil
}

// CleanupStaleEdges marks edges as disconnected if they haven't sent heartbeat recently
func (s *EdgeService) CleanupStaleEdges(maxAge time.Duration) error {
	var edge models.EdgeInstance
	if err := edge.CleanupStaleEdges(s.db, maxAge); err != nil {
		return fmt.Errorf("failed to cleanup stale edges: %w", err)
	}
	return nil
}

// UpdateEdgeStatus updates the status of an edge instance
func (s *EdgeService) UpdateEdgeStatus(edgeID string, status string) error {
	var edge models.EdgeInstance
	if err := edge.GetByEdgeID(s.db, edgeID); err != nil {
		return fmt.Errorf("edge not found: %w", err)
	}

	if err := edge.UpdateStatus(s.db, status); err != nil {
		return fmt.Errorf("failed to update edge status: %w", err)
	}

	return nil
}

// UpdateEdgeHeartbeat updates the heartbeat timestamp for an edge
func (s *EdgeService) UpdateEdgeHeartbeat(edgeID string) error {
	var edge models.EdgeInstance
	if err := edge.GetByEdgeID(s.db, edgeID); err != nil {
		return fmt.Errorf("edge not found: %w", err)
	}

	if err := edge.UpdateHeartbeat(s.db); err != nil {
		return fmt.Errorf("failed to update edge heartbeat: %w", err)
	}

	return nil
}

// CreateOrUpdateEdge creates a new edge instance or updates existing one
func (s *EdgeService) CreateOrUpdateEdge(edgeID, namespace, version, buildHash string, metadata map[string]interface{}) (*models.EdgeInstance, error) {
	var edge models.EdgeInstance
	err := edge.GetByEdgeID(s.db, edgeID)
	
	if err == gorm.ErrRecordNotFound {
		// Create new edge
		edge = models.EdgeInstance{
			EdgeID:    edgeID,
			Namespace: namespace,
			Version:   version,
			BuildHash: buildHash,
			Metadata:  metadata,
			Status:    models.EdgeStatusRegistered,
		}
		
		if err := edge.Create(s.db); err != nil {
			return nil, fmt.Errorf("failed to create edge: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to check edge existence: %w", err)
	} else {
		// Update existing edge
		edge.Version = version
		edge.BuildHash = buildHash
		edge.Metadata = metadata
		edge.Status = models.EdgeStatusRegistered
		
		if err := edge.Update(s.db); err != nil {
			return nil, fmt.Errorf("failed to update edge: %w", err)
		}
	}

	return &edge, nil
}