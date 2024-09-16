package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/gin-gonic/gin"
)

// @Summary Get available LLM drivers
// @Description Get a list of available LLM drivers
// @Tags vendors
// @Accept json
// @Produce json
// @Success 200 {object} VendorListResponse
// @Failure 500 {object} ErrorResponse
// @Router /vendors/llm-drivers [get]
// @Security BearerAuth
func (a *API) getAvailableLLMDrivers(c *gin.Context) {
	vendors := make([]string, len(switches.AVAILABLE_LLM_DRIVERS))
	for i, vendor := range switches.AVAILABLE_LLM_DRIVERS {
		vendors[i] = string(vendor)
	}

	c.JSON(http.StatusOK, VendorListResponse{Data: vendors})
}

// @Summary Get available embedders
// @Description Get a list of available embedders
// @Tags vendors
// @Accept json
// @Produce json
// @Success 200 {object} VendorListResponse
// @Failure 500 {object} ErrorResponse
// @Router /vendors/embedders [get]
// @Security BearerAuth
func (a *API) getAvailableEmbedders(c *gin.Context) {
	vendors := make([]string, len(switches.AVAILABLE_EMBEDDERS))
	for i, vendor := range switches.AVAILABLE_EMBEDDERS {
		vendors[i] = string(vendor)
	}

	c.JSON(http.StatusOK, VendorListResponse{Data: vendors})
}
