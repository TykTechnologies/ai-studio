package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/gin-gonic/gin"
)

// Configuration pushes ("reloads") are recorded in the database and
// delivered by whichever replica holds each edge's stream; see
// features/ClusterControlPlane.md. The handlers here are shared by CE and
// ENT; the editions differ only in which scopes they expose.

// recentPushWindow is how far back the operations listing looks.
const recentPushWindow = 24 * time.Hour

func initiatedBy(c *gin.Context) string {
	if user, ok := c.Get("user"); ok {
		if u, ok := user.(*models.User); ok {
			return u.Email
		}
	}
	return "unknown"
}

// sendPushError maps a push error to its HTTP status.
func sendPushError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pushes.ErrEdgeNotFound):
		helpers.SendErrorResponse(c, helpers.NewNotFoundError(err.Error()))
	case errors.Is(err, pushes.ErrNoTargets):
		helpers.SendErrorResponse(c, helpers.ErrorResponse{StatusCode: http.StatusConflict, Title: "Conflict", Message: err.Error()})
	case errors.Is(err, services.ErrPushesUnavailable):
		helpers.SendErrorResponse(c, helpers.ErrorResponse{StatusCode: http.StatusServiceUnavailable, Title: "Service Unavailable", Message: err.Error()})
	case errors.Is(err, pushes.ErrOperationNotFound):
		helpers.SendErrorResponse(c, helpers.NewNotFoundError(err.Error()))
	default:
		// Database and other internal errors are logged, not returned.
		logger.Errorf("Edge push request %s %s failed: %v", c.Request.Method, c.Request.URL.Path, err)
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("The push could not be recorded or read; see the server log."))
	}
}

// pushAccepted answers a push request: the operation, which edges it
// targets and whether each is reachable now, and any warnings.
func pushAccepted(c *gin.Context, res *pushes.Result) {
	op := res.Operation
	edges := make([]string, len(res.Targets))
	reachable := 0
	for i, t := range res.Targets {
		edges[i] = t.EdgeID
		if t.Reachable {
			reachable++
		}
	}
	message := fmt.Sprintf("Push recorded for %d edge(s); %d connected now.", len(res.Targets), reachable)
	if reachable == len(res.Targets) {
		message = fmt.Sprintf("Push recorded for %d edge(s).", len(res.Targets))
	}
	c.JSON(http.StatusAccepted, gin.H{
		"data": gin.H{
			"type": "reload-operations",
			"id":   op.OperationID,
			"attributes": gin.H{
				"operation_id":     op.OperationID,
				"scope":            op.Scope,
				"target_namespace": op.Namespace,
				"target_edges":     edges,
				"initiated_by":     op.InitiatedBy,
				"initiated_at":     op.CreatedAt,
				"deadline_at":      op.DeadlineAt,
				"status":           op.Status,
				"progress":         0,
				"message":          message,
				"targets":          res.Targets,
				"skipped":          nonNilTargets(res.Skipped),
				"skipped_total":    res.SkippedTotal,
				"warnings":         nonNilStrings(res.Warnings),
			},
		},
	})
}

// pushStatusAttributes is the full report of one push, per edge.
func pushStatusAttributes(st *pushes.OperationStatus) gin.H {
	reach := map[string]pushes.Target{}
	for _, t := range st.Targets {
		reach[t.EdgeID] = t
	}
	edges := make([]gin.H, len(st.Commands))
	ids := make([]string, len(st.Commands))
	var warnings []string
	for i, cmd := range st.Commands {
		ids[i] = cmd.EdgeID
		e := gin.H{
			"edge_id":           cmd.EdgeID,
			"namespace":         cmd.Namespace,
			"status":            cmd.Status,
			"phase":             cmd.Phase,
			"message":           cmd.Message,
			"warning":           cmd.Warning,
			"attempts":          cmd.Attempts,
			"max_attempts":      cmd.MaxAttempts,
			"sent_at":           cmd.SentAt,
			"completed_at":      cmd.CompletedAt,
			"expected_checksum": cmd.ExpectedChecksum,
			"loaded_checksum":   cmd.LoadedChecksum,
			"history":           cmd.History,
		}
		if t, ok := reach[cmd.EdgeID]; ok {
			e["reachable"] = t.Reachable
			e["waiting_reason"] = t.Reason
		}
		edges[i] = e
		if cmd.Warning != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s", cmd.EdgeID, cmd.Warning))
		}
	}
	op := st.Operation
	return gin.H{
		"operation_id":     op.OperationID,
		"scope":            op.Scope,
		"target_namespace": op.Namespace,
		"target_edges":     ids,
		"initiated_by":     op.InitiatedBy,
		"initiated_at":     op.CreatedAt,
		"deadline_at":      op.DeadlineAt,
		"completed_at":     op.CompletedAt,
		"status":           op.Status,
		"progress":         st.Progress,
		"message":          st.Message,
		"counts":           st.Counts,
		"edges":            edges,
		"warnings":         nonNilStrings(warnings),
	}
}

// pushStatus reports one push from the database, so any replica answers.
func (a *API) pushStatus(c *gin.Context) {
	operationID := c.Param("operation_id")
	if err := validateOperationID(operationID); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}
	coord := a.service.NamespaceService.Pushes()
	if coord == nil {
		sendPushError(c, services.ErrPushesUnavailable)
		return
	}
	st, err := coord.Status(c.Request.Context(), operationID)
	if err != nil {
		sendPushError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"type":       "reload-operations",
		"id":         st.Operation.OperationID,
		"attributes": pushStatusAttributes(st),
	}})
}

// listPushes lists the last day's pushes with their outcome counts.
func (a *API) listPushes(c *gin.Context) {
	coord := a.service.NamespaceService.Pushes()
	if coord == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}, "message": services.ErrPushesUnavailable.Error()})
		return
	}
	ops, err := coord.Recent(c.Request.Context(), time.Now().Add(-recentPushWindow), 50)
	if err != nil {
		sendPushError(c, err)
		return
	}
	data := make([]gin.H, len(ops))
	for i, s := range ops {
		op := s.Operation
		data[i] = gin.H{
			"type": "reload-operations",
			"id":   op.OperationID,
			"attributes": gin.H{
				"operation_id":     op.OperationID,
				"scope":            op.Scope,
				"target_namespace": op.Namespace,
				"initiated_by":     op.InitiatedBy,
				"initiated_at":     op.CreatedAt,
				"deadline_at":      op.DeadlineAt,
				"completed_at":     op.CompletedAt,
				"total":            op.Total,
				"status":           op.Status,
				"progress":         s.Progress,
				"message":          s.Message,
				"counts":           s.Counts,
			},
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilTargets(t []pushes.Target) []pushes.Target {
	if t == nil {
		return []pushes.Target{}
	}
	return t
}
