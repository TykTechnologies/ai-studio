package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"gorm.io/gorm"
)

// CreateTool creates a new tool with validity checks
// CreateTool creates a new tool using the default DB connection.
func (s *Service) CreateTool(name, description, toolType string, oasSpec string, privacyScore int, schemaName, APIKey string) (*models.Tool, error) {
	return s.CreateToolWithDB(s.DB, name, description, toolType, oasSpec, privacyScore, schemaName, APIKey)
}

// CreateToolWithDB creates a new tool using the provided DB connection (supports transactions).
func (s *Service) CreateToolWithDB(db *gorm.DB, name, description, toolType string, oasSpec string, privacyScore int, schemaName, APIKey string) (*models.Tool, error) {
	tool := &models.Tool{
		Name:           name,
		Description:    description,
		ToolType:       toolType,
		OASSpec:        oasSpec,
		PrivacyScore:   privacyScore,
		AuthSchemaName: schemaName,
		AuthKey:        APIKey,
	}

	// Execute "before_create" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookBeforeCreate,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.Tool); ok {
				tool = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(tool, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	if err := tool.Create(db); err != nil {
		return nil, err
	}

	// Auto-assign to Default tool catalogue if not in any catalogue
	if err := s.ensureToolInDefaultCatalogue(tool); err != nil {
		// Log but don't fail - this is a convenience feature
		logger.Warn(fmt.Sprintf("Failed to add tool to default catalogue: %v", err))
	}

	// Execute "after_create" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookAfterCreate,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-create hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitToolCreated(tool, tool.ID, 0)
	}

	return tool, nil
}

// UpdateTool updates an existing tool with validity checks
func (s *Service) UpdateTool(id uint, name, description, toolType string, oasSpec string, privacyScore int, schemaName, APIKey string) (*models.Tool, error) {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return nil, err
	}

	tool.Name = name
	tool.Description = description
	tool.ToolType = toolType
	tool.OASSpec = oasSpec
	tool.PrivacyScore = privacyScore
	tool.AuthSchemaName = schemaName
	tool.AuthKey = APIKey

	// Execute "before_update" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookBeforeUpdate,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.Tool); ok {
				tool = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(tool, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	if err := tool.Update(s.DB); err != nil {
		return nil, err
	}

	// Execute "after_update" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookAfterUpdate,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-update hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitToolUpdated(tool, tool.ID, 0)
	}

	return tool, nil
}

// GetToolByID retrieves a tool by its ID
func (s *Service) GetToolByID(id uint) (*models.Tool, error) {
	tool := models.NewTool()
	if err := tool.Get(s.DB, id); err != nil {
		return nil, err
	}

	tool.AuthKey = secrets.GetValue(tool.AuthKey, true) // preserve reference for API responses
	return tool, nil
}

// DeleteTool deletes a tool
func (s *Service) DeleteTool(id uint) error {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return err
	}

	// Execute "before_delete" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookBeforeDelete,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			return fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}
	}

	if err := tool.Delete(s.DB); err != nil {
		return err
	}

	// Execute "after_delete" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeTool,
			HookAfterDelete,
			tool,
			0, // No user context in this method
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-delete hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitToolDeleted(id, 0)
	}

	return nil
}

// GetToolByName retrieves a tool by its name
func (s *Service) GetToolByName(name string) (*models.Tool, error) {
	tool := models.NewTool()
	if err := tool.GetByName(s.DB, name); err != nil {
		return nil, err
	}

	tool.AuthKey = secrets.GetValue(tool.AuthKey, true) // preserve reference for API responses
	return tool, nil
}

// GetToolBySlug retrieves a tool by its slug (pre-computed from name using slug.Make)
func (s *Service) GetToolBySlug(slug string) (*models.Tool, error) {
	var tool models.Tool

	// Use the pre-computed slug column for efficient indexed lookup
	err := s.DB.Where("slug = ?", slug).
		Preload("FileStores").
		Preload("Filters").
		Preload("Dependencies").
		Preload("Apps").
		First(&tool).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tool not found with slug: %s", slug)
		}
		return nil, fmt.Errorf("error retrieving tool: %w", err)
	}

	tool.AuthKey = secrets.GetValue(tool.AuthKey, true) // preserve reference for API responses
	return &tool, nil
}

// GetAllTools retrieves all tools
func (s *Service) GetAllTools(pageSize int, pageNumber int, all bool) ([]models.Tool, int64, int, error) {
	var tools models.Tools
	totalCount, totalPages, err := tools.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return tools, totalCount, totalPages, nil
}

// GetToolsByType retrieves all tools of a specific type
func (s *Service) GetToolsByType(toolType string) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByType(s.DB, toolType); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreMin retrieves all tools with a privacy score greater than or equal to the given minimum
func (s *Service) GetToolsByPrivacyScoreMin(minScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreMin(s.DB, minScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreMax retrieves all tools with a privacy score less than or equal to the given maximum
func (s *Service) GetToolsByPrivacyScoreMax(maxScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreMax(s.DB, maxScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// GetToolsByPrivacyScoreRange retrieves all tools with a privacy score within the given range
func (s *Service) GetToolsByPrivacyScoreRange(minScore, maxScore int) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.GetByPrivacyScoreRange(s.DB, minScore, maxScore); err != nil {
		return nil, err
	}
	return tools, nil
}

// SearchTools searches for tools matching the given query in name or description
func (s *Service) SearchTools(query string) ([]models.Tool, error) {
	var tools models.Tools
	if err := tools.Search(s.DB, query); err != nil {
		return nil, err
	}
	return tools, nil
}

// AddOperationToTool adds an operation to a tool
func (s *Service) AddOperationToTool(toolID uint, operation string) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	tool.AddOperation(operation)
	return tool.Update(s.DB)
}

// RemoveOperationFromTool removes an operation from a tool
func (s *Service) RemoveOperationFromTool(toolID uint, operation string) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	tool.RemoveOperation(operation)
	return tool.Update(s.DB)
}

// GetToolOperations retrieves all operations associated with a tool
func (s *Service) GetToolOperations(toolID uint) ([]string, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	return tool.GetOperations(), nil
}

// AddFileStoreToTool adds a FileStore to a Tool
func (s *Service) AddFileStoreToTool(toolID uint, fileStoreID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return tool.AddFileStore(s.DB, fileStore)
}

// RemoveFileStoreFromTool removes a FileStore from a Tool
func (s *Service) RemoveFileStoreFromTool(toolID uint, fileStoreID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	fileStore := &models.FileStore{}
	if err := fileStore.Get(s.DB, fileStoreID); err != nil {
		return err
	}

	return tool.RemoveFileStore(s.DB, fileStore)
}

// GetToolFileStores gets all FileStores associated with a Tool
func (s *Service) GetToolFileStores(toolID uint) ([]models.FileStore, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	return tool.GetFileStores(s.DB)
}

// SetToolFileStores replaces all existing FileStore associations with new ones
func (s *Service) SetToolFileStores(toolID uint, fileStoreIDs []uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	fileStores := make([]models.FileStore, len(fileStoreIDs))
	for i, id := range fileStoreIDs {
		fileStore := models.FileStore{}
		if err := fileStore.Get(s.DB, id); err != nil {
			return err
		}
		fileStores[i] = fileStore
	}

	return tool.SetFileStores(s.DB, fileStores)
}

// AddFilterToTool adds a Filter to a Tool
func (s *Service) AddFilterToTool(toolID uint, filterID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	filter := &models.Filter{}
	if err := filter.Get(s.DB, filterID); err != nil {
		return err
	}

	return tool.AddFilter(s.DB, filter)
}

// RemoveFilterFromTool removes a Filter from a Tool
func (s *Service) RemoveFilterFromTool(toolID uint, filterID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	filter := &models.Filter{}
	if err := filter.Get(s.DB, filterID); err != nil {
		return err
	}

	return tool.RemoveFilter(s.DB, filter)
}

// GetToolFilters gets all Filters associated with a Tool
func (s *Service) GetToolFilters(toolID uint) ([]models.Filter, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	return tool.GetFilters(s.DB)
}

// SetToolFilters replaces all existing Filter associations with new ones
func (s *Service) SetToolFilters(toolID uint, filterIDs []uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	filters := make([]models.Filter, len(filterIDs))
	for i, id := range filterIDs {
		filter := models.Filter{}
		if err := filter.Get(s.DB, id); err != nil {
			return err
		}
		filters[i] = filter
	}

	return tool.SetFilters(s.DB, filters)
}

// AddDependencyToTool adds a dependency to a Tool
func (s *Service) AddDependencyToTool(toolID uint, dependencyID uint) error {
	// Prevent self-dependency
	if toolID == dependencyID {
		return fmt.Errorf("tool cannot depend on itself")
	}

	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	dependency := models.NewTool()
	if err := dependency.Get(s.DB, dependencyID); err != nil {
		return err
	}

	err = tool.AddDependency(s.DB, dependency)
	if err != nil {
		if strings.Contains(err.Error(), "circular reference") {
			return fmt.Errorf("cannot add dependency: would create a circular reference")
		}
		return err
	}

	return nil
}

// RemoveDependencyFromTool removes a dependency from a Tool
func (s *Service) RemoveDependencyFromTool(toolID uint, dependencyID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	dependency := models.NewTool()
	if err := dependency.Get(s.DB, dependencyID); err != nil {
		return err
	}

	return tool.RemoveDependency(s.DB, dependency)
}

// GetToolDependencies gets all dependencies associated with a Tool
func (s *Service) GetToolDependencies(toolID uint) ([]*models.Tool, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	return tool.GetDependencies(s.DB)
}

// SetToolDependencies replaces all existing Tool dependencies with new ones
func (s *Service) SetToolDependencies(toolID uint, dependencyIDs []uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	dependencies := make([]*models.Tool, len(dependencyIDs))
	for i, id := range dependencyIDs {
		dependency := models.NewTool()
		if err := dependency.Get(s.DB, id); err != nil {
			return err
		}
		dependencies[i] = dependency
	}

	return tool.SetDependencies(s.DB, dependencies)
}

// ClearToolDependencies removes all dependencies from a Tool
func (s *Service) ClearToolDependencies(toolID uint) error {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return err
	}

	return tool.ClearDependencies(s.DB)
}

// HasToolDependency checks if a specific Tool is a dependency
func (s *Service) HasToolDependency(toolID uint, dependencyID uint) (bool, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return false, err
	}

	return tool.HasDependency(s.DB, dependencyID)
}

// parsedSpecCache caches parsed OpenAPI specs to avoid repeated parsing
type parsedSpecCache struct {
	operations  []string
	toolVersion int64
	createdAt   time.Time
}

// clientCacheEntry caches universalclient instances for tool operations
type clientCacheEntry struct {
	client      *universalclient.Client
	toolVersion int64
	authHash    string
	createdAt   time.Time
}

var (
	specCache     = make(map[uint]*parsedSpecCache)
	specCacheMu   sync.RWMutex
	clientCache   = make(map[string]*clientCacheEntry)
	clientCacheMu sync.RWMutex
)

// ListToolOperationsFromSpec retrieves all operations from the tool's OpenAPI spec
func (s *Service) ListToolOperationsFromSpec(toolID uint) ([]string, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	if tool.OASSpec == "" {
		return nil, fmt.Errorf("tool has no OpenAPI specification")
	}

	// Check cache first
	specCacheMu.RLock()
	cache, exists := specCache[toolID]
	specCacheMu.RUnlock()

	cacheExpiry := 30 * time.Minute
	if exists && cache.toolVersion == tool.UpdatedAt.UnixNano() &&
		time.Since(cache.createdAt) < cacheExpiry {
		return cache.operations, nil
	}

	// Cache miss or expired - parse the spec
	decodedSpec, err := base64.StdEncoding.DecodeString(tool.OASSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
	}

	client, err := universalclient.NewClient(decodedSpec, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	operations, err := client.ListOperations()
	if err != nil {
		return nil, err
	}

	// Update cache
	specCacheMu.Lock()
	specCache[toolID] = &parsedSpecCache{
		operations:  operations,
		toolVersion: tool.UpdatedAt.UnixNano(),
		createdAt:   time.Now(),
	}

	// Clean up old cache entries to prevent memory leaks
	if len(specCache) > 100 {
		oldestKey := uint(0)
		oldestTime := time.Now()
		for k, v := range specCache {
			if v.createdAt.Before(oldestTime) {
				oldestTime = v.createdAt
				oldestKey = k
			}
		}
		if oldestKey != 0 {
			delete(specCache, oldestKey)
		}
	}
	specCacheMu.Unlock()

	return operations, nil
}

// getCachedUniversalClient returns a cached universalclient or creates a new one
func (s *Service) getCachedUniversalClient(tool *models.Tool, authSchemaName, authKey string) (*universalclient.Client, error) {
	if tool.OASSpec == "" {
		return nil, fmt.Errorf("tool has no OpenAPI specification")
	}

	// Create cache key from tool ID + auth info
	authHash := ""
	if authSchemaName != "" && authKey != "" {
		authHash = fmt.Sprintf("%s:%s", authSchemaName, authKey)
	}
	cacheKey := fmt.Sprintf("tool_%d_auth_%s", tool.ID, authHash)

	// Check cache first
	clientCacheMu.RLock()
	cache, exists := clientCache[cacheKey]
	clientCacheMu.RUnlock()

	cacheExpiry := 30 * time.Minute
	if exists && cache.toolVersion == tool.UpdatedAt.UnixNano() &&
		time.Since(cache.createdAt) < cacheExpiry {
		return cache.client, nil
	}

	// Cache miss or expired - create new client
	decodedSpec, err := base64.StdEncoding.DecodeString(tool.OASSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
	}

	// Create options slice for the client
	options := []universalclient.ClientOption{
		universalclient.WithResponseFormat(universalclient.ResponseFormatJSON),
	}

	// Add auth option if provided
	if authSchemaName != "" && authKey != "" {
		options = append(options, universalclient.WithAuth(authSchemaName, authKey))
	}

	client, err := universalclient.NewClient(decodedSpec, "", options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create universalclient: %w", err)
	}

	// Update cache
	clientCacheMu.Lock()
	clientCache[cacheKey] = &clientCacheEntry{
		client:      client,
		toolVersion: tool.UpdatedAt.UnixNano(),
		authHash:    authHash,
		createdAt:   time.Now(),
	}

	// Clean up old cache entries to prevent memory leaks
	if len(clientCache) > 50 { // Smaller limit for client cache since clients are larger
		oldestKey := ""
		oldestTime := time.Now()
		for k, v := range clientCache {
			if v.createdAt.Before(oldestTime) {
				oldestTime = v.createdAt
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(clientCache, oldestKey)
		}
	}
	clientCacheMu.Unlock()

	return client, nil
}

// CallToolOperation calls an operation from the tool's OpenAPI spec
func (s *Service) CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	// Use cached client for better performance
	client, err := s.getCachedUniversalClient(tool, tool.AuthSchemaName, tool.AuthKey)
	if err != nil {
		return nil, err
	}

	result, err := client.CallOperation(operationID, params, payload, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to call operation: %w", err)
	}

	return result, nil
}

// ToolOperationDetail represents detailed information about a tool operation
type ToolOperationDetail struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
}

// GetToolOperationDetails retrieves detailed information about tool operations from OpenAPI spec
func (s *Service) GetToolOperationDetails(toolID uint) ([]ToolOperationDetail, error) {
	tool, err := s.GetToolByID(toolID)
	if err != nil {
		return nil, err
	}

	if tool.OASSpec == "" {
		return nil, fmt.Errorf("tool has no OpenAPI specification")
	}

	// Get basic operations list first
	operations, err := s.ListToolOperationsFromSpec(toolID)
	if err != nil {
		return nil, err
	}

	// Decode the spec for detailed parsing
	decodedSpec, err := base64.StdEncoding.DecodeString(tool.OASSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 OpenAPI spec: %w", err)
	}

	// Create universal client for parsing
	client, err := universalclient.NewClient(decodedSpec, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create universal client: %w", err)
	}

	// Extract detailed information for each operation
	details := make([]ToolOperationDetail, 0, len(operations))
	for _, operationID := range operations {
		detail := ToolOperationDetail{
			OperationID: operationID,
			Method:      "POST", // Default for tool calls through proxy
			Path:        "",
			Summary:     "",
			Description: "",
		}

		// Try to get tool definition for this operation
		tools, err := client.AsTool(operationID)
		if err == nil && len(tools) > 0 {
			tool := tools[0]
			if tool.Function != nil {
				detail.Summary = tool.Function.Name
				detail.Description = tool.Function.Description
			}
		}

		details = append(details, detail)
	}

	return details, nil
}

// ensureToolInDefaultCatalogue adds a tool to the Default tool catalogue if it's not in any catalogue
func (s *Service) ensureToolInDefaultCatalogue(tool *models.Tool) error {
	// Check if tool is in any catalogue
	count := s.DB.Model(tool).Association("ToolCatalogues").Count()

	if count == 0 {
		// Get or create default tool catalogue
		defaultCatalogue, err := models.GetOrCreateDefaultToolCatalogue(s.DB)
		if err != nil {
			logger.Errorf("Failed to get default tool catalogue: %v", err)
			return fmt.Errorf("failed to get default tool catalogue: %w", err)
		}

		// Add tool to default catalogue
		if err := s.DB.Model(defaultCatalogue).Association("Tools").Append(tool); err != nil {
			logger.Errorf("Failed to append tool to default catalogue: %v", err)
			return fmt.Errorf("failed to add tool to default catalogue: %w", err)
		}

	} else {
		logger.Infof("Tool '%s' already in %d catalogue(s), skipping auto-assignment", tool.Name, count)
	}

	return nil
}
