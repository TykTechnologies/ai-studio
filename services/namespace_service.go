package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// NamespaceService handles namespace operations for hub-and-spoke architecture
type NamespaceService struct {
	db          *gorm.DB
	edgeService *EdgeService
	pushes      *pushes.Coordinator
}

// NewNamespaceService creates a new NamespaceService
func NewNamespaceService(db *gorm.DB, edgeService *EdgeService) *NamespaceService {
	return &NamespaceService{
		db:          db,
		edgeService: edgeService,
		// pushes is set later via SetPushes
	}
}

// ErrPushesUnavailable: this Studio has no control server, so nothing can
// deliver a push.
var ErrPushesUnavailable = errors.New("configuration pushes are not available: the edge control server is not running")

// SetPushes sets the coordinator that records and delivers configuration
// pushes (see features/ClusterControlPlane.md).
func (s *NamespaceService) SetPushes(c *pushes.Coordinator) {
	s.pushes = c
}

// Pushes returns the push coordinator, or nil when there is none.
func (s *NamespaceService) Pushes() *pushes.Coordinator {
	return s.pushes
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

// TriggerNamespaceReload pushes the current configuration to every edge in
// the namespace. It returns once the push is recorded; delivery and the
// edges' answers are tracked on the returned operation.
func (s *NamespaceService) TriggerNamespaceReload(namespace string, initiatedBy string) (*pushes.Result, error) {
	return s.push(pushes.Request{Scope: pushes.ScopeNamespace, Namespace: namespace, InitiatedBy: initiatedBy})
}

// TriggerEdgeReload pushes the current configuration to one edge. An edge
// that is not connected is waited for until the push's deadline.
func (s *NamespaceService) TriggerEdgeReload(edgeID string, initiatedBy string) (*pushes.Result, error) {
	return s.push(pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{edgeID}, InitiatedBy: initiatedBy})
}

// TriggerAllReload pushes the current configuration to every edge in every
// namespace, as one operation.
func (s *NamespaceService) TriggerAllReload(initiatedBy string) (*pushes.Result, error) {
	return s.push(pushes.Request{Scope: pushes.ScopeAll, InitiatedBy: initiatedBy})
}

func (s *NamespaceService) push(req pushes.Request) (*pushes.Result, error) {
	if s.pushes == nil {
		return nil, ErrPushesUnavailable
	}
	res, err := s.pushes.Push(context.Background(), req)
	if err != nil {
		return nil, err
	}
	// The pending-changes preview measures from the push. Each namespace
	// pushed to is stamped under the spelling its edges are stored with.
	stamped := map[string]bool{}
	for _, t := range res.Targets {
		if !stamped[t.Namespace] {
			stamped[t.Namespace] = true
			s.markNamespacePushed(t.Namespace)
		}
	}
	return res, nil
}

// markNamespacePushed stamps NamespaceSyncStatus.LastPushAt for the
// namespace (DB spelling: "" for global). A failure is logged, not returned:
// the reload itself must not be blocked by the bookkeeping.
func (s *NamespaceService) markNamespacePushed(dbNamespace string) {
	if err := models.MarkNamespacePushed(s.db, dbNamespace, time.Now()); err != nil {
		fmt.Printf("Warning: failed to record push for namespace '%s': %v\n", dbNamespace, err)
	}
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