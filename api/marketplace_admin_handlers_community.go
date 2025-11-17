//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MarketplaceAdminHandlers handles marketplace management API endpoints
// Community Edition: Returns 403 Forbidden for all management operations
type MarketplaceAdminHandlers struct {
	service interface{} // unused in CE, but accepted for API compatibility
}

// NewMarketplaceAdminHandlers creates marketplace admin handlers for Community Edition
func NewMarketplaceAdminHandlers(service interface{}) *MarketplaceAdminHandlers {
	return &MarketplaceAdminHandlers{service: service}
}

// enterpriseOnlyResponse returns a consistent 403 response for CE
func (h *MarketplaceAdminHandlers) enterpriseOnlyResponse(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{
		"error": "Multiple marketplace management is only available in Enterprise Edition",
		"feature": "marketplace_management",
		"upgrade_url": "https://tyk.io/ai-studio-enterprise",
	})
}

// AddMarketplace returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) AddMarketplace(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// ListMarketplaces returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) ListMarketplaces(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// GetMarketplace returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) GetMarketplace(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// UpdateMarketplace returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) UpdateMarketplace(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// RemoveMarketplace returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) RemoveMarketplace(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// ValidateMarketplaceURL returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) ValidateMarketplaceURL(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}

// SyncMarketplace returns 403 in Community Edition
func (h *MarketplaceAdminHandlers) SyncMarketplace(c *gin.Context) {
	h.enterpriseOnlyResponse(c)
}
