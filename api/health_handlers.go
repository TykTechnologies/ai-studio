package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	"github.com/gin-gonic/gin"
)

// handleHealth reports liveness and the process's FIPS 140-3 status
// ("fips": {enabled, version, enforced}).
func (a *API) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "fips": fipsinfo.Current()})
}

func (a *API) handleReadiness(c *gin.Context) {
	if !a.checkReadiness() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (a *API) checkReadiness() bool {
	if a.config == nil || a.config.DB == nil {
		return false
	}
	
	sqlDB, err := a.config.DB.DB()
	if err != nil {
		return false
	}
	
	if err := sqlDB.Ping(); err != nil {
		return false
	}
	
	return true
}