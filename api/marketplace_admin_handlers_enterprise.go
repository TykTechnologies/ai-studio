//go:build enterprise
// +build enterprise

package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services/marketplace_management"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// MarketplaceAdminHandlers handles marketplace management API endpoints for Enterprise Edition
type MarketplaceAdminHandlers struct {
	service marketplace_management.Service
}

// NewMarketplaceAdminHandlers creates marketplace admin handlers for Enterprise Edition
func NewMarketplaceAdminHandlers(service marketplace_management.Service) *MarketplaceAdminHandlers {
	return &MarketplaceAdminHandlers{
		service: service,
	}
}

// AddMarketplace adds a new marketplace index
// POST /api/v1/admin/marketplaces
func (h *MarketplaceAdminHandlers) AddMarketplace(c *gin.Context) {
	var req struct {
		URL       string `json:"url" binding:"required"`
		IsDefault bool   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid Request", Detail: err.Error()}},
		})
		return
	}

	marketplace, err := h.service.AddMarketplace(req.URL, req.IsDefault)
	if err != nil {
		log.Error().Err(err).Str("url", req.URL).Msg("Failed to add marketplace")

		status := http.StatusInternalServerError
		if err == marketplace_management.ErrInvalidURL {
			status = http.StatusBadRequest
		} else if err == marketplace_management.ErrDuplicateURL {
			status = http.StatusConflict
		}

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Add Marketplace", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": marketplace,
	})
}

// ListMarketplaces returns all marketplace indexes
// GET /api/v1/admin/marketplaces
func (h *MarketplaceAdminHandlers) ListMarketplaces(c *gin.Context) {
	marketplaces, err := h.service.ListMarketplaces()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list marketplaces")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to List Marketplaces", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": marketplaces,
	})
}

// GetMarketplace retrieves a specific marketplace by ID
// GET /api/v1/admin/marketplaces/:id
func (h *MarketplaceAdminHandlers) GetMarketplace(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid ID", Detail: "Marketplace ID must be a number"}},
		})
		return
	}

	marketplace, err := h.service.GetMarketplace(uint(id))
	if err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to get marketplace")

		status := http.StatusInternalServerError
		if err == marketplace_management.ErrMarketplaceNotFound {
			status = http.StatusNotFound
		}

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Get Marketplace", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": marketplace,
	})
}

// UpdateMarketplace updates marketplace properties
// PUT /api/v1/admin/marketplaces/:id
func (h *MarketplaceAdminHandlers) UpdateMarketplace(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid ID", Detail: "Marketplace ID must be a number"}},
		})
		return
	}

	var updates marketplace_management.MarketplaceUpdate
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid Request", Detail: err.Error()}},
		})
		return
	}

	if err := h.service.UpdateMarketplace(uint(id), &updates); err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to update marketplace")

		status := http.StatusInternalServerError
		if err == marketplace_management.ErrMarketplaceNotFound {
			status = http.StatusNotFound
		} else if err == marketplace_management.ErrCannotDeactivateDefault {
			status = http.StatusBadRequest
		}

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Update Marketplace", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Marketplace updated successfully",
	})
}

// RemoveMarketplace removes a marketplace index
// DELETE /api/v1/admin/marketplaces/:id
func (h *MarketplaceAdminHandlers) RemoveMarketplace(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid ID", Detail: "Marketplace ID must be a number"}},
		})
		return
	}

	if err := h.service.RemoveMarketplace(uint(id)); err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to remove marketplace")

		status := http.StatusInternalServerError
		if err == marketplace_management.ErrMarketplaceNotFound {
			status = http.StatusNotFound
		} else if err == marketplace_management.ErrCannotRemoveDefault {
			status = http.StatusBadRequest
		}

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Remove Marketplace", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Marketplace removed successfully",
	})
}

// ValidateMarketplaceURL validates a marketplace URL before adding
// POST /api/v1/admin/marketplaces/validate
func (h *MarketplaceAdminHandlers) ValidateMarketplaceURL(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid Request", Detail: err.Error()}},
		})
		return
	}

	result, err := h.service.ValidateMarketplaceURL(req.URL)
	if err != nil {
		log.Error().Err(err).Str("url", req.URL).Msg("Failed to validate marketplace URL")
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Validate URL", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}

// SyncMarketplace triggers manual sync for a specific marketplace
// POST /api/v1/admin/marketplaces/:id/sync
func (h *MarketplaceAdminHandlers) SyncMarketplace(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid ID", Detail: "Marketplace ID must be a number"}},
		})
		return
	}

	// Verify marketplace exists
	_, err = h.service.GetMarketplace(uint(id))
	if err != nil {
		status := http.StatusInternalServerError
		if err == marketplace_management.ErrMarketplaceNotFound {
			status = http.StatusNotFound
		}

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to Sync Marketplace", Detail: err.Error()}},
		})
		return
	}

	// Note: Actual sync is handled by MarketplaceService.SyncAll() in background
	// This endpoint just validates the marketplace exists and returns success
	// The marketplace service will sync it on the next scheduled run

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Marketplace sync requested - will be synchronized on next scheduled run",
	})
}
