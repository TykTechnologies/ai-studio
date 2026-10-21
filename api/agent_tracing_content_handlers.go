package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
	"github.com/gin-gonic/gin"
)

// Agent Call Tracing content API (spec §8, §9.2, §9.4; Enterprise). Payload
// reads need trace-content read and are audited by the implementation
// before any content is returned, whatever AUDIT_RECORD_READS says; when
// the access cannot be audited the content is refused (503). Purges are
// audited the same way. The end user of an erasure arrives only in a POST
// body. Responses carry Cache-Control: no-store (the group's noStore).

func (a *API) registerAgentTracingContentRoutes(g *permRouter) {
	g.GET("/spans/:span_id/payloads/:part", authz.Read("trace-content"), a.getAgentPayload)
	g.DELETE("/sessions/:session_id", authz.Delete("traces"), a.purgeAgentSession)
	g.POST("/purge", authz.Delete("traces"), a.purgeAgentEndUser)
	g.GET("/config", authz.Read("traces"), a.getAgentTracingConfig)
	g.GET("/sessions/:session_id/export", authz.Execute("traces"), a.exportAgentSession)
	g.GET("/traces/:trace_id/export", authz.Execute("traces"), a.exportAgentTrace)
}

// exportQuery parses format (json, the default, or markdown) and content
// (true asks for captured content, which needs trace-content read; without
// it the export is content-stripped and its audit record says so).
func exportQuery(c *gin.Context) (agenttracing.ExportQuery, bool) {
	q := agenttracing.ExportQuery{Format: c.DefaultQuery("format", agenttracing.ExportJSON)}
	if q.Format != agenttracing.ExportJSON && q.Format != agenttracing.ExportMarkdown {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", "format must be json or markdown")
		return q, false
	}
	switch c.DefaultQuery("content", "false") {
	case "true":
		q.Content = true
	case "false":
	default:
		agentTracingError(c, http.StatusBadRequest, "Bad Request", "content must be true or false")
		return q, false
	}
	return q, true
}

// respondExport writes an export as an attachment (the group sets no-store).
func respondExport(c *gin.Context, res *agenttracing.ExportResult, err error) {
	if err != nil {
		respondAgentContent(c, nil, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+res.Filename+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	if res.ContentIncluded {
		c.Header("X-Tyk-Export-Content", "included")
	} else {
		c.Header("X-Tyk-Export-Content", "not-included")
	}
	c.Data(http.StatusOK, res.ContentType, res.Body)
}

// @Summary Export an agent session
// @Description JSON (application/vnd.tyk.agent-trace+json; v=1) or a Markdown transcript of one session. content=true includes captured content when the caller holds trace-content read; otherwise the export is content-stripped (X-Tyk-Export-Content says which). Every export is audited; it is refused (503) when the audit trail cannot record it.
// @Tags agent-tracing
// @Produce json
// @Param session_id path string true "Session id (UUID)"
// @Param format query string false "json (default) or markdown"
// @Param content query bool false "Include captured content"
// @Success 200 {file} file
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /agent-tracing/sessions/{session_id}/export [get]
// @Security BearerAuth
func (a *API) exportAgentSession(c *gin.Context) {
	id, ok := sessionIDParam(c)
	if !ok {
		return
	}
	q, ok := exportQuery(c)
	if !ok {
		return
	}
	res, err := a.agentTracing().ExportSession(c.Request.Context(), agentTracingViewer(c), id, q, agentAccessMeta(c))
	respondExport(c, res, err)
}

// @Summary Export an agent trace (turn)
// @Description As the session export, for one trace of one App.
// @Tags agent-tracing
// @Produce json
// @Param trace_id path string true "Trace id (32 lowercase hex)"
// @Param app_id query int true "The trace's App"
// @Param format query string false "json (default) or markdown"
// @Param content query bool false "Include captured content"
// @Success 200 {file} file
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /agent-tracing/traces/{trace_id}/export [get]
// @Security BearerAuth
func (a *API) exportAgentTrace(c *gin.Context) {
	id, ok := hexIDParam(c, "trace_id", 16)
	if !ok {
		return
	}
	appID, ok := appIDQuery(c)
	if !ok {
		return
	}
	q, ok := exportQuery(c)
	if !ok {
		return
	}
	res, err := a.agentTracing().ExportTrace(c.Request.Context(), agentTracingViewer(c), appID, id, q, agentAccessMeta(c))
	respondExport(c, res, err)
}

// agentAccessMeta is the request context an audited access records.
func agentAccessMeta(c *gin.Context) agenttracing.AccessMeta {
	m := agenttracing.AccessMeta{
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("audit_request_id"),
	}
	if u, ok := c.Get("user"); ok {
		if user, ok := u.(*models.User); ok && user != nil {
			m.UserEmail, m.UserName = user.Email, user.Name
		}
	}
	return m
}

// respondAgentContent maps the content errors, then the read API's.
func respondAgentContent(c *gin.Context, body any, err error) {
	switch {
	case errors.Is(err, agenttracing.ErrAuditUnavailable):
		agentTracingError(c, http.StatusServiceUnavailable, "Service Unavailable", err.Error())
	case errors.Is(err, agenttracing.ErrForbidden):
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("trace-content")))
	default:
		respondAgentTracing(c, body, err)
	}
}

// @Summary Read a captured payload part
// @Description One captured part of a span (request, response, system_instructions, tool_definitions, tool_args, tool_result, error_body), as stored after the content pipeline. Needs trace-content read; every read is audited, and the content is refused (503) when the audit trail cannot record it. Content over max_bytes (and the UI view cap) comes back truncated.
// @Tags agent-tracing
// @Produce json
// @Param span_id path string true "Span id (16 lowercase hex)"
// @Param part path string true "Payload part"
// @Param app_id query int true "The span's App (span ids are unique only within an App)"
// @Param max_bytes query int false "Cap on the returned content"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /agent-tracing/spans/{span_id}/payloads/{part} [get]
// @Security BearerAuth
func (a *API) getAgentPayload(c *gin.Context) {
	id, ok := hexIDParam(c, "span_id", 8)
	if !ok {
		return
	}
	appID, ok := appIDQuery(c)
	if !ok {
		return
	}
	q := agenttracing.PayloadQuery{Part: c.Param("part")}
	if raw := c.Query("max_bytes"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			agentTracingError(c, http.StatusBadRequest, "Bad Request", "max_bytes must be a positive integer")
			return
		}
		q.MaxBytes = n
	}
	res, err := a.agentTracing().GetPayload(c.Request.Context(), agentTracingViewer(c), appID, id, q, agentAccessMeta(c))
	respondAgentContent(c, res, err)
}

// @Summary Purge an agent session
// @Description Deletes a session and everything captured for it: spans, links, payloads, correlation keys, agents and staged events (right to erasure). Audited; refused (503) when the audit trail cannot record it.
// @Tags agent-tracing
// @Produce json
// @Param session_id path string true "Session id (UUID)"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /agent-tracing/sessions/{session_id} [delete]
// @Security BearerAuth
func (a *API) purgeAgentSession(c *gin.Context) {
	id, ok := sessionIDParam(c)
	if !ok {
		return
	}
	res, err := a.agentTracing().PurgeSession(c.Request.Context(), agentTracingViewer(c), id, agentAccessMeta(c))
	respondAgentContent(c, res, err)
}

// PurgeBody names what to erase. The end user is personal data, so it is
// accepted only here, in a body.
type PurgeBody struct {
	EndUser string `json:"end_user"`
}

// @Summary Purge an end user's agent sessions
// @Description Deletes every session whose declared end user is end_user (right to erasure). Audited by session ids, never by the end user itself.
// @Tags agent-tracing
// @Accept json
// @Produce json
// @Param body body PurgeBody true "What to erase"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /agent-tracing/purge [post]
// @Security BearerAuth
func (a *API) purgeAgentEndUser(c *gin.Context) {
	var body PurgeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", "the body must be {\"end_user\": \"...\"}")
		return
	}
	body.EndUser = strings.TrimSpace(body.EndUser)
	res, err := a.agentTracing().PurgeEndUser(c.Request.Context(), agentTracingViewer(c), body.EndUser, agentAccessMeta(c))
	respondAgentContent(c, res, err)
}

// @Summary Agent tracing settings
// @Description The effective content and retention settings (from AGENT_TRACING_* environment variables; Apps, LLMs, tools and datasources override them). content_ceiling bounds every App, default and request header; content_default_mode applies to Apps without their own mode. Never includes keys.
// @Tags agent-tracing
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /agent-tracing/config [get]
// @Security BearerAuth
func (a *API) getAgentTracingConfig(c *gin.Context) {
	res, err := a.agentTracing().GetConfig(c.Request.Context(), agentTracingViewer(c))
	respondAgentContent(c, res, err)
}
