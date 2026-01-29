package services

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// SyncStatusService manages configuration sync status between control and edge gateways
type SyncStatusService struct {
	db *gorm.DB
}

// NewSyncStatusService creates a new SyncStatusService
func NewSyncStatusService(db *gorm.DB) *SyncStatusService {
	return &SyncStatusService{db: db}
}

// NamespaceSyncSummary represents sync status for a namespace with edge counts
type NamespaceSyncSummary struct {
	Namespace        string    `json:"namespace"`
	ExpectedChecksum string    `json:"expected_checksum"`
	ConfigVersion    string    `json:"config_version"`
	LastConfigChange time.Time `json:"last_config_change"`
	SyncedCount      int64     `json:"synced_count"`
	PendingCount     int64     `json:"pending_count"`
	StaleCount       int64     `json:"stale_count"`
	UnknownCount     int64     `json:"unknown_count"`
	TotalEdges       int64     `json:"total_edges"`
}

// GetNamespaceSyncSummary returns sync status for all namespaces
func (s *SyncStatusService) GetNamespaceSyncSummary() ([]NamespaceSyncSummary, error) {
	var results []NamespaceSyncSummary

	// Get all namespace sync statuses
	var namespaces []models.NamespaceSyncStatus
	status := &models.NamespaceSyncStatus{}
	namespaces, err := status.GetAll(s.db)
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces {
		summary := NamespaceSyncSummary{
			Namespace:        ns.Namespace,
			ExpectedChecksum: ns.ExpectedChecksum,
			ConfigVersion:    ns.ConfigVersion,
			LastConfigChange: ns.LastConfigChange,
		}

		// Count edges by sync status for this namespace
		edgeInstance := &models.EdgeInstance{}
		counts, err := edgeInstance.CountEdgesBySyncStatus(s.db, ns.Namespace)
		if err == nil {
			summary.SyncedCount = counts[models.EdgeSyncStatusInSync]
			summary.PendingCount = counts[models.EdgeSyncStatusPending]
			summary.StaleCount = counts[models.EdgeSyncStatusStale]
			summary.UnknownCount = counts[models.EdgeSyncStatusUnknown]
			summary.TotalEdges = summary.SyncedCount + summary.PendingCount + summary.StaleCount + summary.UnknownCount
		}

		results = append(results, summary)
	}

	return results, nil
}

// GetNamespaceSyncStatus returns detailed sync status for a specific namespace
func (s *SyncStatusService) GetNamespaceSyncStatus(namespace string) (*NamespaceSyncSummary, []models.EdgeInstance, error) {
	// Get namespace sync status
	var status models.NamespaceSyncStatus
	if err := status.GetByNamespace(s.db, namespace); err != nil {
		return nil, nil, err
	}

	summary := &NamespaceSyncSummary{
		Namespace:        status.Namespace,
		ExpectedChecksum: status.ExpectedChecksum,
		ConfigVersion:    status.ConfigVersion,
		LastConfigChange: status.LastConfigChange,
	}

	// Get edges in namespace with their sync status
	var edges models.EdgeInstances
	if err := edges.ListEdgesInNamespace(s.db, namespace); err != nil {
		return summary, nil, err
	}

	// Count edges by sync status
	for _, edge := range edges {
		if edge.Status == models.EdgeStatusConnected || edge.Status == models.EdgeStatusRegistered {
			switch edge.SyncStatus {
			case models.EdgeSyncStatusInSync:
				summary.SyncedCount++
			case models.EdgeSyncStatusPending:
				summary.PendingCount++
			case models.EdgeSyncStatusStale:
				summary.StaleCount++
			default:
				summary.UnknownCount++
			}
		}
	}
	summary.TotalEdges = summary.SyncedCount + summary.PendingCount + summary.StaleCount + summary.UnknownCount

	return summary, edges, nil
}

// HasPendingSyncs returns true if any namespace has edges waiting for sync
func (s *SyncStatusService) HasPendingSyncs() (bool, error) {
	var count int64
	err := s.db.Model(&models.EdgeInstance{}).
		Where("sync_status IN ? AND status IN ?",
			[]string{models.EdgeSyncStatusPending, models.EdgeSyncStatusStale},
			[]string{models.EdgeStatusConnected, models.EdgeStatusRegistered}).
		Count(&count).Error
	return count > 0, err
}

// GetPendingNamespaces returns namespaces that have edges pending sync
func (s *SyncStatusService) GetPendingNamespaces() ([]string, error) {
	var namespaces []string
	err := s.db.Model(&models.EdgeInstance{}).
		Distinct("namespace").
		Where("sync_status IN ? AND status IN ?",
			[]string{models.EdgeSyncStatusPending, models.EdgeSyncStatusStale},
			[]string{models.EdgeStatusConnected, models.EdgeStatusRegistered}).
		Pluck("namespace", &namespaces).Error
	return namespaces, err
}

// GetSyncAuditLog returns audit log entries with optional filtering
func (s *SyncStatusService) GetSyncAuditLog(namespace, edgeID, eventType string, limit int) ([]models.SyncAuditLog, error) {
	auditLog := &models.SyncAuditLog{}
	return auditLog.GetFiltered(s.db, namespace, edgeID, eventType, limit)
}

// MarkStaleEdges marks edges that have been out of sync for too long as stale
// staleThreshold is the duration after which pending edges are marked as stale (default 15 minutes)
func (s *SyncStatusService) MarkStaleEdges(staleThreshold time.Duration) error {
	edgeInstance := &models.EdgeInstance{}
	return edgeInstance.MarkStaleEdges(s.db, staleThreshold)
}
