package api

import (
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Agent Call Tracing read API (spec §8; Enterprise). The handlers parse and
// bound the request and build the Viewer; the queries run in the Enterprise
// implementation behind services/agenttracing, which the Community stub
// answers with 402. Responses carry Cache-Control: no-store.
//
// Client-supplied identifiers that may be personal data (end_user,
// external_id) and the in-session find term are accepted only in POST
// bodies, so they never reach URLs, browser history or access logs.

func (a *API) registerAgentTracingRoutes(v1 *permRouter) {
	g := v1.Group("/agent-tracing", noStore)
	g.GET("/sessions", authz.Read("traces"), a.listAgentSessions)
	g.POST("/sessions/query", authz.Read("traces"), a.queryAgentSessions)
	g.GET("/sessions/:session_id", authz.Read("traces"), a.getAgentSession)
	g.GET("/sessions/:session_id/spans", authz.Read("traces"), a.listAgentSessionSpans)
	g.POST("/sessions/:session_id/search", authz.Read("traces"), a.searchAgentSession)
	g.GET("/traces/:trace_id", authz.Read("traces"), a.getAgentTrace)
	g.GET("/spans/:span_id", authz.Read("traces"), a.getAgentSpan)
}

// agentTracing is the tracing service, or an inert one when none is wired.
func (a *API) agentTracing() agenttracing.Service {
	if a.service == nil || a.service.AgentTracing == nil {
		return agenttracing.Unavailable()
	}
	return a.service.AgentTracing
}

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}

var (
	sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	maxNameFilterLen = 255
)

// agentTracingViewer is the caller's view: holders of traces:read see every
// App (the route already required it). The flags only tell the UI what to
// offer; the metadata reads never return content.
func agentTracingViewer(c *gin.Context) agenttracing.Viewer {
	v := agenttracing.Viewer{
		AllApps:        true,
		CanReadContent: authz.Can(c, authz.Read("trace-content")),
		CanExport:      authz.Can(c, authz.Execute("traces")),
		CanPurge:       authz.Can(c, authz.Delete("traces")),
	}
	if u, ok := c.Get("user"); ok {
		if user, ok := u.(*models.User); ok {
			v.UserID = user.ID
		}
	}
	return v
}

// respondAgentTracing writes a query result or maps its error.
func respondAgentTracing(c *gin.Context, body any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, body)
		return
	}
	var bad *agenttracing.BadRequestError
	switch {
	case errors.As(err, &bad):
		agentTracingError(c, http.StatusBadRequest, "Bad Request", bad.Msg)
	case errors.Is(err, agenttracing.ErrEnterpriseFeature):
		c.JSON(http.StatusPaymentRequired, authz.EnterpriseRequired(err.Error()))
	case errors.Is(err, agenttracing.ErrDisabled):
		agentTracingError(c, http.StatusServiceUnavailable, "Service Unavailable", err.Error())
	case errors.Is(err, agenttracing.ErrNotFound):
		agentTracingError(c, http.StatusNotFound, "Not Found", "Not found")
	default:
		log.Error().Err(err).Str("path", c.FullPath()).Msg("agent tracing query failed")
		agentTracingError(c, http.StatusInternalServerError, "Internal Server Error", "The agent tracing query failed")
	}
}

func agentTracingError(c *gin.Context, status int, title, detail string) {
	c.JSON(status, ErrorResponse{Errors: []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}{{Title: title, Detail: detail}}})
}

// SessionQueryBody is the POST form of the sessions list query: every GET
// filter, plus the ones that may carry personal data.
type SessionQueryBody struct {
	From             *time.Time `json:"from,omitempty"`
	To               *time.Time `json:"to,omitempty"`
	AppID            uint       `json:"app_id,omitempty"`
	UserID           uint       `json:"user_id,omitempty"`
	EndUser          string     `json:"end_user,omitempty"`
	ExternalID       string     `json:"external_id,omitempty"`
	SessionID        string     `json:"session_id,omitempty"`
	Agent            string     `json:"agent,omitempty"`
	Model            string     `json:"model,omitempty"`
	Tool             string     `json:"tool,omitempty"`
	Status           string     `json:"status,omitempty"`
	Source           string     `json:"source,omitempty"`
	WeakLinks        bool       `json:"weak_links,omitempty"`
	HasErrors        *bool      `json:"has_errors,omitempty"`
	HasGatewayEvents *bool      `json:"has_gateway_events,omitempty"`
	CostMinMicros    *int64     `json:"cost_min_micros,omitempty"`
	CostMaxMicros    *int64     `json:"cost_max_micros,omitempty"`
	DurationMinMs    *int64     `json:"duration_min_ms,omitempty"`
	DurationMaxMs    *int64     `json:"duration_max_ms,omitempty"`
	Sort             string     `json:"sort,omitempty"` // a SessionSorts column, "-" prefix for descending
	Cursor           string     `json:"cursor,omitempty"`
	Limit            int        `json:"limit,omitempty"`
}

// listAgentSessions godoc
// @Summary List agent sessions (Enterprise)
// @Description Sessions in a time range (default the last 24 h), newest first, with the spec §6.1 filters and cursor pagination. end_user and external_id are refused here: use POST /agent-tracing/sessions/query so they stay out of URLs.
// @Tags agent-tracing
// @Produce json
// @Param from query string false "RFC 3339 start (default 24 h before to)"
// @Param to query string false "RFC 3339 end (default now)"
// @Param app_id query int false "App"
// @Param user_id query int false "Studio user"
// @Param session_id query string false "Exact session id"
// @Param agent query string false "Agent name"
// @Param model query string false "Model"
// @Param tool query string false "Tool called"
// @Param status query string false "active, closed or error"
// @Param source query string false "declared or inferred"
// @Param weak_links query bool false "Only sessions with weak links"
// @Param has_errors query bool false "Has errors"
// @Param has_gateway_events query bool false "Has gateway events"
// @Param cost_min_micros query int false "Minimum cost (micros)"
// @Param cost_max_micros query int false "Maximum cost (micros)"
// @Param duration_min_ms query int false "Minimum duration (ms)"
// @Param duration_max_ms query int false "Maximum duration (ms)"
// @Param sort query string false "started_at, cost, duration, tokens, turns, llm_calls, tool_calls or errors; prefix - for descending (default -started_at)"
// @Param cursor query string false "next_cursor of the previous page"
// @Param limit query int false "Page size (1-200, default 50)"
// @Success 200 {object} map[string]interface{} "{data: [session], next_cursor}"
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse
// @Router /agent-tracing/sessions [get]
func (a *API) listAgentSessions(c *gin.Context) {
	if c.Query("end_user") != "" || c.Query("external_id") != "" {
		agentTracingError(c, http.StatusBadRequest, "Bad Request",
			"end_user and external_id are not accepted in the URL; use POST /api/v1/agent-tracing/sessions/query")
		return
	}
	var body SessionQueryBody
	if err := sessionQueryFromURL(c, &body); err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	a.runSessionQuery(c, body)
}

// queryAgentSessions godoc
// @Summary Query agent sessions (Enterprise)
// @Description The sessions list with every filter in the body, including end_user and external_id.
// @Tags agent-tracing
// @Accept json
// @Produce json
// @Param query body SessionQueryBody true "Filters"
// @Success 200 {object} map[string]interface{} "{data: [session], next_cursor}"
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse
// @Router /agent-tracing/sessions/query [post]
func (a *API) queryAgentSessions(c *gin.Context) {
	var body SessionQueryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	a.runSessionQuery(c, body)
}

func (a *API) runSessionQuery(c *gin.Context, body SessionQueryBody) {
	q, err := sessionQueryFromBody(body, time.Now().UTC())
	if err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := a.agentTracing().ListSessions(c.Request.Context(), agentTracingViewer(c), q)
	respondAgentTracing(c, res, err)
}

// getAgentSession godoc
// @Summary Get an agent session (Enterprise)
// @Description The session with its agents and turns, and what the caller may do with it (viewer).
// @Tags agent-tracing
// @Produce json
// @Param session_id path string true "Session id (UUID)"
// @Success 200 {object} map[string]interface{} "{session, agents, traces, viewer}"
// @Failure 404 {object} ErrorResponse
// @Router /agent-tracing/sessions/{session_id} [get]
func (a *API) getAgentSession(c *gin.Context) {
	id, ok := sessionIDParam(c)
	if !ok {
		return
	}
	res, err := a.agentTracing().GetSession(c.Request.Context(), agentTracingViewer(c), id)
	respondAgentTracing(c, res, err)
}

// listAgentSessionSpans godoc
// @Summary List a session's spans (Enterprise)
// @Description The span skeleton (no content), keyset-paged by (start_time, span_id).
// @Tags agent-tracing
// @Produce json
// @Param session_id path string true "Session id (UUID)"
// @Param kind query string false "Span kind"
// @Param status query string false "Span status"
// @Param agent_id query string false "Agent id"
// @Param from query string false "RFC 3339"
// @Param to query string false "RFC 3339"
// @Param include query string false "previews: ask for inline content previews"
// @Param cursor query string false "next_cursor of the previous page"
// @Param limit query int false "Page size (1-1000, default 1000)"
// @Success 200 {object} map[string]interface{} "{data: [span], next_cursor, previews}"
// @Failure 404 {object} ErrorResponse
// @Router /agent-tracing/sessions/{session_id}/spans [get]
func (a *API) listAgentSessionSpans(c *gin.Context) {
	id, ok := sessionIDParam(c)
	if !ok {
		return
	}
	q, err := spanQueryFromURL(c)
	if err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := a.agentTracing().ListSessionSpans(c.Request.Context(), agentTracingViewer(c), id, q)
	respondAgentTracing(c, res, err)
}

// SessionSearchBody is the in-session find request.
type SessionSearchBody struct {
	Q     string `json:"q"`
	Limit int    `json:"limit,omitempty"`
}

// searchAgentSession godoc
// @Summary Find in a session (Enterprise)
// @Description Matches span names, tool names, models, operations and error types. The term is in the body so it never reaches a URL.
// @Tags agent-tracing
// @Accept json
// @Produce json
// @Param session_id path string true "Session id (UUID)"
// @Param query body SessionSearchBody true "Find term (1-256 characters)"
// @Success 200 {object} map[string]interface{} "{matches: [{span_id, field, snippet}], total, truncated}"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /agent-tracing/sessions/{session_id}/search [post]
func (a *API) searchAgentSession(c *gin.Context) {
	id, ok := sessionIDParam(c)
	if !ok {
		return
	}
	var body SessionSearchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	term := strings.TrimSpace(body.Q)
	if term == "" || utf8.RuneCountInString(term) > agenttracing.MaxSearchTermLength {
		agentTracingError(c, http.StatusBadRequest, "Bad Request",
			"q must be 1 to "+strconv.Itoa(agenttracing.MaxSearchTermLength)+" characters")
		return
	}
	limit, err := boundedLimit(body.Limit, agenttracing.DefaultSearchResults, agenttracing.MaxSearchResults)
	if err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := a.agentTracing().SearchSession(c.Request.Context(), agentTracingViewer(c), id,
		agenttracing.SearchQuery{Term: term, Limit: limit})
	respondAgentTracing(c, res, err)
}

// getAgentTrace godoc
// @Summary Get an agent trace (Enterprise)
// @Description One turn with its first page of spans (keyset-paged like the session spans).
// @Tags agent-tracing
// @Produce json
// @Param trace_id path string true "Trace id (32 lowercase hex)"
// @Param cursor query string false "next_cursor of the previous page"
// @Param limit query int false "Page size (1-1000, default 1000)"
// @Success 200 {object} map[string]interface{} "{trace, session_id, spans, next_cursor}"
// @Failure 404 {object} ErrorResponse
// @Router /agent-tracing/traces/{trace_id} [get]
func (a *API) getAgentTrace(c *gin.Context) {
	id, ok := hexIDParam(c, "trace_id", 16)
	if !ok {
		return
	}
	q, err := spanQueryFromURL(c)
	if err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	res, err := a.agentTracing().GetTrace(c.Request.Context(), agentTracingViewer(c), id, q)
	respondAgentTracing(c, res, err)
}

// getAgentSpan godoc
// @Summary Get an agent span (Enterprise)
// @Description One span with all its links (candidates included) and gateway events. No content.
// @Tags agent-tracing
// @Produce json
// @Param span_id path string true "Span id (16 lowercase hex)"
// @Success 200 {object} map[string]interface{} "{span, events, session_id}"
// @Failure 404 {object} ErrorResponse
// @Router /agent-tracing/spans/{span_id} [get]
func (a *API) getAgentSpan(c *gin.Context) {
	id, ok := hexIDParam(c, "span_id", 8)
	if !ok {
		return
	}
	res, err := a.agentTracing().GetSpan(c.Request.Context(), agentTracingViewer(c), id)
	respondAgentTracing(c, res, err)
}

func sessionIDParam(c *gin.Context) (string, bool) {
	id := c.Param("session_id")
	if !sessionIDPattern.MatchString(id) {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", "session_id must be a lowercase UUID")
		return "", false
	}
	return id, true
}

func hexIDParam(c *gin.Context, name string, size int) ([]byte, bool) {
	raw := c.Param(name)
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != size || strings.ToLower(raw) != raw {
		agentTracingError(c, http.StatusBadRequest, "Bad Request",
			name+" must be "+strconv.Itoa(size*2)+" lowercase hex characters")
		return nil, false
	}
	return b, true
}

// boundedLimit applies the default to 0 and refuses anything outside 1..max.
func boundedLimit(n, def, max int) (int, error) {
	if n == 0 {
		return def, nil
	}
	if n < 1 || n > max {
		return 0, agenttracing.BadRequest("limit must be between 1 and " + strconv.Itoa(max))
	}
	return n, nil
}

// sessionQueryFromURL fills body from the GET query string.
func sessionQueryFromURL(c *gin.Context, body *SessionQueryBody) error {
	var err error
	str := func(name string) string { return c.Query(name) }
	if s := str("from"); s != "" {
		t, e := time.Parse(time.RFC3339, s)
		if e != nil {
			return agenttracing.BadRequest("from must be an RFC 3339 time")
		}
		body.From = &t
	}
	if s := str("to"); s != "" {
		t, e := time.Parse(time.RFC3339, s)
		if e != nil {
			return agenttracing.BadRequest("to must be an RFC 3339 time")
		}
		body.To = &t
	}
	uintParam := func(name string, dst *uint) {
		if s := str(name); s != "" && err == nil {
			n, e := strconv.ParseUint(s, 10, 32)
			if e != nil {
				err = agenttracing.BadRequest(name + " must be a positive integer")
				return
			}
			*dst = uint(n)
		}
	}
	int64Param := func(name string) *int64 {
		s := str(name)
		if s == "" || err != nil {
			return nil
		}
		n, e := strconv.ParseInt(s, 10, 64)
		if e != nil {
			err = agenttracing.BadRequest(name + " must be an integer")
			return nil
		}
		return &n
	}
	boolParam := func(name string) *bool {
		s := str(name)
		if s == "" || err != nil {
			return nil
		}
		b, e := strconv.ParseBool(s)
		if e != nil {
			err = agenttracing.BadRequest(name + " must be true or false")
			return nil
		}
		return &b
	}
	uintParam("app_id", &body.AppID)
	uintParam("user_id", &body.UserID)
	body.SessionID = str("session_id")
	body.Agent, body.Model, body.Tool = str("agent"), str("model"), str("tool")
	body.Status, body.Source = str("status"), str("source")
	if w := boolParam("weak_links"); w != nil {
		body.WeakLinks = *w
	}
	body.HasErrors = boolParam("has_errors")
	body.HasGatewayEvents = boolParam("has_gateway_events")
	body.CostMinMicros = int64Param("cost_min_micros")
	body.CostMaxMicros = int64Param("cost_max_micros")
	body.DurationMinMs = int64Param("duration_min_ms")
	body.DurationMaxMs = int64Param("duration_max_ms")
	body.Sort, body.Cursor = str("sort"), str("cursor")
	if s := str("limit"); s != "" && err == nil {
		n, e := strconv.Atoi(s)
		if e != nil {
			return agenttracing.BadRequest("limit must be an integer")
		}
		if n == 0 {
			return agenttracing.BadRequest("limit must be between 1 and " + strconv.Itoa(agenttracing.MaxSessionPageSize))
		}
		body.Limit = n
	}
	return err
}

// sessionQueryFromBody validates the filters and applies the defaults.
func sessionQueryFromBody(b SessionQueryBody, now time.Time) (agenttracing.SessionQuery, error) {
	q := agenttracing.SessionQuery{
		AppID: b.AppID, UserID: b.UserID,
		EndUser: b.EndUser, ExternalID: b.ExternalID, SessionID: b.SessionID,
		Agent: b.Agent, Model: b.Model, Tool: b.Tool,
		Status: b.Status, Source: b.Source, WeakLinks: b.WeakLinks,
		HasErrors: b.HasErrors, HasGatewayEvents: b.HasGatewayEvents,
		CostMinMicros: b.CostMinMicros, CostMaxMicros: b.CostMaxMicros,
		DurationMinMs: b.DurationMinMs, DurationMaxMs: b.DurationMaxMs,
		Cursor: b.Cursor,
	}
	q.To = now
	if b.To != nil {
		q.To = b.To.UTC()
	}
	q.From = q.To.Add(-24 * time.Hour)
	if b.From != nil {
		q.From = b.From.UTC()
	}
	if !q.From.Before(q.To) {
		return q, agenttracing.BadRequest("from must be before to")
	}
	if q.SessionID != "" && !sessionIDPattern.MatchString(q.SessionID) {
		return q, agenttracing.BadRequest("session_id must be a lowercase UUID")
	}
	for name, v := range map[string]string{"end_user": q.EndUser, "external_id": q.ExternalID, "agent": q.Agent, "model": q.Model, "tool": q.Tool} {
		if utf8.RuneCountInString(v) > maxNameFilterLen {
			return q, agenttracing.BadRequest(name + " must be at most 255 characters")
		}
	}
	switch q.Status {
	case "", "active", "closed", "error":
	default:
		return q, agenttracing.BadRequest("status must be active, closed or error")
	}
	switch q.Source {
	case "", "declared", "inferred":
	default:
		return q, agenttracing.BadRequest("source must be declared or inferred")
	}
	for name, p := range map[string]*int64{"cost_min_micros": q.CostMinMicros, "cost_max_micros": q.CostMaxMicros,
		"duration_min_ms": q.DurationMinMs, "duration_max_ms": q.DurationMaxMs} {
		if p != nil && *p < 0 {
			return q, agenttracing.BadRequest(name + " must not be negative")
		}
	}
	sort := b.Sort
	if sort == "" {
		sort = "-started_at"
	}
	q.Desc = strings.HasPrefix(sort, "-")
	q.Sort = strings.TrimPrefix(sort, "-")
	known := false
	for _, s := range agenttracing.SessionSorts {
		known = known || s == q.Sort
	}
	if !known {
		return q, agenttracing.BadRequest("sort must be one of " + strings.Join(agenttracing.SessionSorts, ", ") + " (prefix - for descending)")
	}
	limit, err := boundedLimit(b.Limit, agenttracing.DefaultSessionPageSize, agenttracing.MaxSessionPageSize)
	if err != nil {
		return q, err
	}
	q.Limit = limit
	return q, nil
}

// spanQueryFromURL parses the span page parameters.
func spanQueryFromURL(c *gin.Context) (agenttracing.SpanQuery, error) {
	q := agenttracing.SpanQuery{
		Kind: c.Query("kind"), Status: c.Query("status"), AgentID: c.Query("agent_id"),
		Cursor: c.Query("cursor"), Previews: c.Query("include") == "previews",
	}
	if inc := c.Query("include"); inc != "" && inc != "previews" {
		return q, agenttracing.BadRequest("include must be previews")
	}
	for _, p := range []struct {
		name string
		dst  *time.Time
	}{{"from", &q.From}, {"to", &q.To}} {
		if s := c.Query(p.name); s != "" {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				return q, agenttracing.BadRequest(p.name + " must be an RFC 3339 time")
			}
			*p.dst = t.UTC()
		}
	}
	n := 0
	if s := c.Query("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v == 0 {
			return q, agenttracing.BadRequest("limit must be between 1 and " + strconv.Itoa(agenttracing.MaxSpanPageSize))
		}
		n = v
	}
	limit, err := boundedLimit(n, agenttracing.DefaultSpanPageSize, agenttracing.MaxSpanPageSize)
	if err != nil {
		return q, err
	}
	q.Limit = limit
	return q, nil
}
