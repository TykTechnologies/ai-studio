package services

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// NamespaceService handles namespace operations for hub-and-spoke architecture
type NamespaceService struct {
	db          *gorm.DB
	edgeService *EdgeService
}

// NewNamespaceService creates a new NamespaceService
func NewNamespaceService(db *gorm.DB, edgeService *EdgeService) *NamespaceService {
	return &NamespaceService{
		db:          db,
		edgeService: edgeService,
	}
}

// NamespaceInfo contains information about a namespace
type NamespaceInfo struct {
	Name         string `json:"name"`
	IsGlobal     bool   `json:"is_global"`
	EdgeCount    int64  `json:"edge_count"`
	LLMCount     int64  `json:"llm_count"`
	AppCount     int64  `json:"app_count"`
	TokenCount   int64  `json:"token_count"`
	FilterCount  int64  `json:"filter_count"`
	PluginCount  int64  `json:"plugin_count"`
}

// ReloadOperation represents a configuration reload operation
type ReloadOperation struct {
	OperationID     string    `json:"operation_id"`
	TargetNamespace string    `json:"target_namespace"`
	TargetEdges     []string  `json:"target_edges,omitempty"`
	InitiatedBy     string    `json:"initiated_by"`
	InitiatedAt     time.Time `json:"initiated_at"`
	Status          string    `json:"status"`
	Progress        int       `json:"progress"`
	Message         string    `json:"message"`
}

// ListNamespaces returns all available namespaces with statistics
func (s *NamespaceService) ListNamespaces() ([]NamespaceInfo, error) {
	// Get distinct namespaces from all relevant tables
	var namespaces []string
	
	err := s.db.Raw(`
		SELECT DISTINCT namespace FROM (
			SELECT namespace FROM llms WHERE namespace != ''
			UNION ALL
			SELECT namespace FROM apps WHERE namespace != ''
			UNION ALL
			SELECT namespace FROM filters WHERE namespace != ''
			UNION ALL
			SELECT namespace FROM edge_instances WHERE namespace != ''
		) AS all_namespaces
		ORDER BY namespace
	`).Scan(&namespaces).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get namespaces: %w", err)
	}

	// Always include global namespace (empty string)
	result := make([]NamespaceInfo, 0, len(namespaces)+1)

	// Add global namespace
	globalInfo := NamespaceInfo{
		Name:     "global",
		IsGlobal: true,
	}
	
	// Get counts for global namespace (empty string)
	s.db.Model(&models.EdgeInstance{}).Where("namespace = ''").Count(&globalInfo.EdgeCount)
	s.db.Model(&models.LLM{}).Where("namespace = ''").Count(&globalInfo.LLMCount)
	s.db.Model(&models.App{}).Where("namespace = ''").Count(&globalInfo.AppCount)
	s.db.Model(&models.Credential{}).Where("active = ?", true).Count(&globalInfo.TokenCount) // Use credentials instead
	s.db.Model(&models.Filter{}).Where("namespace = ''").Count(&globalInfo.FilterCount)
	s.db.Model(&models.Plugin{}).Where("namespace = ''").Count(&globalInfo.PluginCount)
	
	result = append(result, globalInfo)

	// Add specific namespaces
	for _, ns := range namespaces {
		nsInfo := NamespaceInfo{
			Name:     ns,
			IsGlobal: false,
		}

		// Get counts for this namespace
		s.db.Model(&models.EdgeInstance{}).Where("namespace = ?", ns).Count(&nsInfo.EdgeCount)
		s.db.Model(&models.LLM{}).Where("namespace = ?", ns).Count(&nsInfo.LLMCount)
		s.db.Model(&models.App{}).Where("namespace = ?", ns).Count(&nsInfo.AppCount)
		// Note: Credentials are global in AI Studio, so count all active credentials
		s.db.Model(&models.Credential{}).Where("active = ?", true).Count(&nsInfo.TokenCount)
		s.db.Model(&models.Filter{}).Where("namespace = ?", ns).Count(&nsInfo.FilterCount)
		s.db.Model(&models.Plugin{}).Where("namespace = ?", ns).Count(&nsInfo.PluginCount)

		result = append(result, nsInfo)
	}

	return result, nil
}

// GetNamespaceInfo returns detailed information about a specific namespace
func (s *NamespaceService) GetNamespaceInfo(namespace string) (*NamespaceInfo, error) {
	// Convert "global" to empty string for database queries
	dbNamespace := namespace
	if namespace == "global" {
		dbNamespace = ""
	}

	info := &NamespaceInfo{
		Name:     namespace,
		IsGlobal: namespace == "global" || namespace == "",
	}

	// Get counts for this namespace
	if err := s.db.Model(&models.EdgeInstance{}).Where("namespace = ?", dbNamespace).Count(&info.EdgeCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count edges: %w", err)
	}
	
	if err := s.db.Model(&models.LLM{}).Where("namespace = ?", dbNamespace).Count(&info.LLMCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count LLMs: %w", err)
	}
	
	if err := s.db.Model(&models.App{}).Where("namespace = ?", dbNamespace).Count(&info.AppCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count apps: %w", err)
	}
	
	// Credentials are global in AI Studio, so just count all active credentials
	if err := s.db.Model(&models.Credential{}).Where("active = ?", true).Count(&info.TokenCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count credentials: %w", err)
	}
	
	if err := s.db.Model(&models.Filter{}).Where("namespace = ?", dbNamespace).Count(&info.FilterCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count filters: %w", err)
	}

	return info, nil
}

// GetEdgesInNamespace returns all edges in a specific namespace
func (s *NamespaceService) GetEdgesInNamespace(namespace string) ([]EdgeInstanceWithHealth, error) {
	// Convert "global" to empty string for database queries
	dbNamespace := namespace
	if namespace == "global" {
		dbNamespace = ""
	}

	return s.edgeService.GetEdgesInNamespace(dbNamespace)
}

// TriggerNamespaceReload initiates a configuration reload for all edges in a namespace
func (s *NamespaceService) TriggerNamespaceReload(namespace string, initiatedBy string) (*ReloadOperation, error) {
	// Convert "global" to empty string for database queries
	dbNamespace := namespace
	if namespace == "global" {
		dbNamespace = ""
	}

	// Check if namespace has any active edges
	var edgeCount int64
	if err := s.db.Model(&models.EdgeInstance{}).
		Where("namespace = ? AND status IN ?", dbNamespace, []string{models.EdgeStatusConnected, models.EdgeStatusRegistered}).
		Count(&edgeCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check namespace edges: %w", err)
	}

	if edgeCount == 0 {
		return nil, fmt.Errorf("no active edges found in namespace '%s'", namespace)
	}

	// Create reload operation
	operationID := fmt.Sprintf("ns-reload-%s-%d", namespace, time.Now().Unix())
	
	operation := &ReloadOperation{
		OperationID:     operationID,
		TargetNamespace: namespace,
		InitiatedBy:     initiatedBy,
		InitiatedAt:     time.Now(),
		Status:          "initiated",
		Progress:        0,
		Message:         fmt.Sprintf("Reload operation initiated for namespace '%s'", namespace),
	}

	// TODO: Implement actual reload coordination with gRPC control server
	// For now, just return the operation details

	return operation, nil
}

// TriggerEdgeReload initiates a configuration reload for a specific edge
func (s *NamespaceService) TriggerEdgeReload(edgeID string, initiatedBy string) (*ReloadOperation, error) {
	// Check if edge exists and is active
	edge, err := s.edgeService.GetEdgeByEdgeID(edgeID)
	if err != nil {
		return nil, fmt.Errorf("edge not found: %w", err)
	}

	if edge.Status != models.EdgeStatusConnected && edge.Status != models.EdgeStatusRegistered {
		return nil, fmt.Errorf("edge '%s' is not in a reloadable state (status: %s)", edgeID, edge.Status)
	}

	// Create reload operation
	operationID := fmt.Sprintf("edge-reload-%s-%d", edgeID, time.Now().Unix())
	
	operation := &ReloadOperation{
		OperationID:     operationID,
		TargetNamespace: edge.Namespace,
		TargetEdges:     []string{edgeID},
		InitiatedBy:     initiatedBy,
		InitiatedAt:     time.Now(),
		Status:          "initiated",
		Progress:        0,
		Message:         fmt.Sprintf("Reload operation initiated for edge '%s'", edgeID),
	}

	// TODO: Implement actual reload coordination with gRPC control server
	// For now, just return the operation details

	return operation, nil
}

// GetNamespaceStatistics returns comprehensive statistics for all namespaces
func (s *NamespaceService) GetNamespaceStatistics() (map[string]*EdgeStatistics, error) {
	namespaces, err := s.ListNamespaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	result := make(map[string]*EdgeStatistics)
	
	for _, ns := range namespaces {
		dbNamespace := ns.Name
		if ns.IsGlobal {
			dbNamespace = ""
		}
		
		stats, err := s.edgeService.GetEdgeStatistics(dbNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get statistics for namespace '%s': %w", ns.Name, err)
		}
		
		result[ns.Name] = stats
	}

	return result, nil
}

// ValidateNamespace checks if a namespace exists (has any resources)
func (s *NamespaceService) ValidateNamespace(namespace string) (bool, error) {
	// Convert "global" to empty string for database queries
	dbNamespace := namespace
	if namespace == "global" {
		dbNamespace = ""
	}

	// Check if namespace has any resources
	var totalCount int64
	
	// Count across all entity types
	queries := []string{
		"SELECT COUNT(*) FROM llms WHERE namespace = ?",
		"SELECT COUNT(*) FROM apps WHERE namespace = ?", 
		"SELECT COUNT(*) FROM filters WHERE namespace = ?",
		"SELECT COUNT(*) FROM edge_instances WHERE namespace = ?",
	}

	for _, query := range queries {
		var count int64
		if err := s.db.Raw(query, dbNamespace).Scan(&count).Error; err != nil {
			return false, fmt.Errorf("failed to validate namespace: %w", err)
		}
		totalCount += count
	}

	return totalCount > 0, nil
}

// GetActiveNamespaces returns only namespaces that have active edges
func (s *NamespaceService) GetActiveNamespaces() ([]NamespaceInfo, error) {
	// Get namespaces that have active edges
	var namespaces []string
	
	err := s.db.Raw(`
		SELECT DISTINCT namespace
		FROM edge_instances 
		WHERE status IN (?, ?)
		ORDER BY namespace
	`, models.EdgeStatusConnected, models.EdgeStatusRegistered).Scan(&namespaces).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get active namespaces: %w", err)
	}

	result := make([]NamespaceInfo, 0, len(namespaces))

	for _, ns := range namespaces {
		displayName := ns
		if ns == "" {
			displayName = "global"
		}
		
		nsInfo := NamespaceInfo{
			Name:     displayName,
			IsGlobal: ns == "",
		}

		// Get counts for this namespace
		s.db.Model(&models.EdgeInstance{}).Where("namespace = ?", ns).Count(&nsInfo.EdgeCount)
		s.db.Model(&models.LLM{}).Where("namespace = ?", ns).Count(&nsInfo.LLMCount)
		s.db.Model(&models.App{}).Where("namespace = ?", ns).Count(&nsInfo.AppCount)
		// Note: Credentials are global in AI Studio, so count all active credentials
		s.db.Model(&models.Credential{}).Where("active = ?", true).Count(&nsInfo.TokenCount)
		s.db.Model(&models.Filter{}).Where("namespace = ?", ns).Count(&nsInfo.FilterCount)
		s.db.Model(&models.Plugin{}).Where("namespace = ?", ns).Count(&nsInfo.PluginCount)

		result = append(result, nsInfo)
	}

	return result, nil
}