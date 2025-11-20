// internal/services/filter_service.go
package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"gorm.io/gorm"
)

// FilterServiceInterface defines the interface for filter operations
type FilterServiceInterface interface {
	// CreateFilter creates a new filter
	CreateFilter(req *CreateFilterRequest) (*database.Filter, error)
	
	// GetFilter retrieves a filter by ID
	GetFilter(id uint) (*database.Filter, error)
	
	// ListFilters lists filters with pagination
	ListFilters(page, limit int, isActive bool) ([]database.Filter, int64, error)
	
	// UpdateFilter updates an existing filter
	UpdateFilter(id uint, req *UpdateFilterRequest) (*database.Filter, error)
	
	// DeleteFilter soft deletes a filter
	DeleteFilter(id uint) error
	
	// GetFiltersForLLM returns filters associated with an LLM
	GetFiltersForLLM(llmID uint) ([]database.Filter, error)
	
	// UpdateLLMFilters updates filter associations for an LLM
	UpdateLLMFilters(llmID uint, filterIDs []uint) error
	
	// ExecuteFilter executes a filter script (placeholder for actual filter engine)
	ExecuteFilter(filterID uint, payload map[string]interface{}) (map[string]interface{}, error)
}

// FilterService implements filter management
type FilterService struct {
	db   *gorm.DB
	repo *database.Repository
}

// NewFilterService creates a new filter service
func NewFilterService(db *gorm.DB, repo *database.Repository) FilterServiceInterface {
	return &FilterService{
		db:   db,
		repo: repo,
	}
}

// CreateFilterRequest for creating filters
type CreateFilterRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Script      string `json:"script" binding:"required"`
	IsActive    bool   `json:"is_active"`
	OrderIndex  int    `json:"order_index"`
}

// UpdateFilterRequest for updating filters
type UpdateFilterRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Script      *string `json:"script"`
	IsActive    *bool   `json:"is_active"`
	OrderIndex  *int    `json:"order_index"`
}

// CreateFilter creates a new filter
func (s *FilterService) CreateFilter(req *CreateFilterRequest) (*database.Filter, error) {
	// Only check for empty scripts
	if strings.TrimSpace(req.Script) == "" {
		return nil, fmt.Errorf("script cannot be empty")
	}

	filter := &database.Filter{
		Name:        req.Name,
		Description: req.Description,
		Script:      req.Script,
		IsActive:    req.IsActive,
		OrderIndex:  req.OrderIndex,
	}

	if err := s.db.Create(filter).Error; err != nil {
		return nil, fmt.Errorf("failed to create filter: %w", err)
	}

	return filter, nil
}

// GetFilter retrieves a filter by ID
func (s *FilterService) GetFilter(id uint) (*database.Filter, error) {
	var filter database.Filter
	err := s.db.Preload("LLMs").First(&filter, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("filter not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get filter: %w", err)
	}

	return &filter, nil
}

// ListFilters lists filters with pagination
func (s *FilterService) ListFilters(page, limit int, isActive bool) ([]database.Filter, int64, error) {
	var filters []database.Filter
	var total int64

	query := s.db.Model(&database.Filter{}).Where("is_active = ?", isActive)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).
		Preload("LLMs").
		Order("order_index ASC, created_at DESC").
		Find(&filters).Error

	return filters, total, err
}

// UpdateFilter updates an existing filter
func (s *FilterService) UpdateFilter(id uint, req *UpdateFilterRequest) (*database.Filter, error) {
	// Get existing filter
	filter, err := s.GetFilter(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Name != nil {
		filter.Name = *req.Name
	}
	if req.Description != nil {
		filter.Description = *req.Description
	}
	if req.Script != nil {
		filter.Script = *req.Script
	}
	if req.IsActive != nil {
		filter.IsActive = *req.IsActive
	}
	if req.OrderIndex != nil {
		filter.OrderIndex = *req.OrderIndex
	}

	// Save changes
	if err := s.db.Save(filter).Error; err != nil {
		return nil, fmt.Errorf("failed to update filter: %w", err)
	}

	return filter, nil
}

// DeleteFilter soft deletes a filter
func (s *FilterService) DeleteFilter(id uint) error {
	result := s.db.Delete(&database.Filter{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete filter: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("filter not found: %d", id)
	}
	return nil
}

// GetFiltersForLLM returns filters associated with an LLM
func (s *FilterService) GetFiltersForLLM(llmID uint) ([]database.Filter, error) {
	var filters []database.Filter
	
	err := s.db.Joins("JOIN llm_filters lf ON lf.filter_id = filters.id").
		Where("lf.llm_id = ? AND lf.is_active = ? AND filters.is_active = ?", llmID, true, true).
		Order("lf.order_index ASC").
		Find(&filters).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get filters for LLM: %w", err)
	}

	return filters, nil
}

// UpdateLLMFilters updates filter associations for an LLM
func (s *FilterService) UpdateLLMFilters(llmID uint, filterIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove existing associations
		if err := tx.Where("llm_id = ?", llmID).Delete(&database.LLMFilter{}).Error; err != nil {
			return fmt.Errorf("failed to remove existing filter associations: %w", err)
		}

		// Add new associations
		for i, filterID := range filterIDs {
			llmFilter := database.LLMFilter{
				LLMID:      llmID,
				FilterID:   filterID,
				IsActive:   true,
				OrderIndex: i, // Use array index as order
				CreatedAt:  time.Now(),
			}
			if err := tx.Create(&llmFilter).Error; err != nil {
				return fmt.Errorf("failed to create LLM-filter association: %w", err)
			}
		}

		return nil
	})
}

// ExecuteFilter executes a filter script (placeholder implementation)
func (s *FilterService) ExecuteFilter(filterID uint, payload map[string]interface{}) (map[string]interface{}, error) {
	// Get filter
	filter, err := s.GetFilter(filterID)
	if err != nil {
		return nil, err
	}

	if !filter.IsActive {
		return payload, nil // Pass through if filter is inactive
	}

	// Execute filter using Tengo scripting engine
	result, err := s.executeFilterScript(filter, payload)
	if err != nil {
		return nil, fmt.Errorf("filter execution failed: %w", err)
	}

	// If result is false, the filter blocks the request
	if !result {
		return nil, fmt.Errorf("request blocked by filter: %s", filter.Name)
	}

	// Filter passed, return payload unchanged
	return payload, nil
}

// executeFilterScript is implemented in edition-specific files (filter_service_ce.go and filter_service_ent.go)
// CE: Always returns true (filters disabled)
// ENT: Executes Tengo script and returns result