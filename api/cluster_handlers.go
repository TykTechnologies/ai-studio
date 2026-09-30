package api

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/gin-gonic/gin"
)

// ClusterStatusFunc reports the control plane's replicas (pkg/cluster.Status).
type ClusterStatusFunc func(ctx context.Context) (interface{}, error)

// SetClusterStatus supplies the cluster status the /cluster/status
// endpoint reports (set by pkg/studio).
func (a *API) SetClusterStatus(f ClusterStatusFunc) {
	a.clusterStatus = f
}

// @Summary Control plane status
// @Description Live Studio replicas, the edges each holds, the leader, the cluster event log and relay on this replica, the push backlog, and warnings
// @Tags edges
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} ErrorResponse
// @Router /api/v1/cluster/status [get]
// @Security BearerAuth
func (a *API) getClusterStatus(c *gin.Context) {
	if a.clusterStatus == nil {
		helpers.SendErrorResponse(c, helpers.ErrorResponse{StatusCode: http.StatusServiceUnavailable, Title: "Service Unavailable", Message: "cluster status is not available on this server"})
		return
	}
	st, err := a.clusterStatus(c.Request.Context())
	if err != nil {
		logger.Errorf("Cluster status failed: %v", err)
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("The cluster status could not be read; see the server log."))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": st})
}
