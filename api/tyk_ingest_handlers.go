package api

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Limits of the Tyk analytics ingest endpoint. Tyk Pump batches are a few
// hundred records; these bound what one request can cost.
var (
	// ingestMaxBodyBytes caps the body as sent and, separately, after gzip
	// expansion.
	ingestMaxBodyBytes int64 = 16 << 20
	// ingestMaxRecords caps the records in one batch.
	ingestMaxRecords = 5000
	// ingestRate and ingestBurst limit requests per connection (token).
	ingestRate  rate.Limit = 20
	ingestBurst            = 40
)

var errIngestBodyTooLarge = errors.New("request body too large")

// ingestLimiters rate-limits ingest requests per connection. A connection
// has one live token, so this is per token; the map is bounded by the
// number of connections.
type ingestLimiters struct {
	mu sync.Mutex
	m  map[uint]*rate.Limiter
}

func (l *ingestLimiters) allow(connectionID uint) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = make(map[uint]*rate.Limiter)
	}
	lim, ok := l.m[connectionID]
	if !ok {
		lim = rate.NewLimiter(ingestRate, ingestBurst)
		l.m[connectionID] = lim
	}
	return lim.Allow()
}

// cappedReader fails with errIngestBodyTooLarge once more than n bytes
// have been read, rather than silently truncating like io.LimitReader.
type cappedReader struct {
	r io.Reader
	n int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	if c.n < 0 {
		return 0, errIngestBodyTooLarge
	}
	if int64(len(p)) > c.n+1 {
		p = p[:c.n+1]
	}
	n, err := c.r.Read(p)
	c.n -= int64(n)
	if c.n < 0 {
		return n, errIngestBodyTooLarge
	}
	return n, err
}

// ingestTykAnalytics godoc
// @Summary Ingest Tyk Gateway analytics records
// @Description Machine-to-machine endpoint for Tyk Pump (the http pump, or the stdout pump through a log shipper). The body is a JSON array of Tyk AnalyticsRecord, optionally gzip-encoded (Content-Encoding: gzip). Only MCP records are kept. Authenticated with the connection's ingest token (Authorization: Bearer). A bad or revoked token gets 401 so the pump drops the batch. Enterprise Edition; 404 in Community Edition.
// @Tags TykMCP
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer ingest token"
// @Param body body []tykmcp.TykAnalyticsRecord true "Analytics records"
// @Success 202 {object} tykmcp.IngestResult
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 413 {object} models.ErrorResponse
// @Failure 415 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Router /ingest/tyk-analytics [post]
func (a *API) ingestTykAnalytics(c *gin.Context) {
	svc := a.tykMCPService()

	// A missing or malformed header still goes to the service, so Community
	// Edition answers 404 (the endpoint does not exist there) rather than
	// 401; the service refuses an empty token.
	token, _ := bearerToken(c.GetHeader("Authorization"))
	connectionID, err := svc.AuthenticateIngest(c.Request.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, tykmcp.ErrIngestUnauthorized):
			ingestUnauthorized(c)
		case errors.Is(err, tykmcp.ErrEnterpriseFeature), errors.Is(err, tykmcp.ErrDisabled):
			// No ingest here (CE, or the integration is off): a 4xx the
			// pump drops rather than a 5xx it retries.
			webhookError(c, http.StatusNotFound, "Not Found", "not found")
		default:
			logger.Warnf("Tyk analytics ingest: authenticating the token failed: %v", err)
			webhookError(c, http.StatusInternalServerError, "Internal Server Error", "Failed to authenticate the ingest token")
		}
		return
	}
	if !a.ingestLimits.allow(connectionID) {
		c.Header("Retry-After", "1")
		webhookError(c, http.StatusTooManyRequests, "Too Many Requests", "ingest rate limit exceeded for this connection")
		return
	}

	records, status, err := decodeIngestBody(c)
	if err != nil {
		webhookError(c, status, http.StatusText(status), err.Error())
		return
	}

	result, err := svc.IngestAnalytics(c.Request.Context(), connectionID, records)
	if err != nil {
		switch {
		case errors.Is(err, tykmcp.ErrIngestUnauthorized):
			ingestUnauthorized(c)
		case errors.Is(err, tykmcp.ErrDisabled):
			webhookError(c, http.StatusNotFound, "Not Found", "not found")
		case errors.Is(err, tykmcp.ErrValidation):
			webhookError(c, http.StatusBadRequest, "Bad Request", err.Error())
		default:
			logger.Warnf("Tyk analytics ingest for connection %d failed: %v", connectionID, err)
			webhookError(c, http.StatusInternalServerError, "Internal Server Error", "Failed to ingest analytics")
		}
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func ingestUnauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", `Bearer realm="tyk-analytics-ingest"`)
	webhookError(c, http.StatusUnauthorized, "Unauthorized", tykmcp.ErrIngestUnauthorized.Error())
}

// bearerToken reads "Bearer <token>" (scheme case-insensitive).
func bearerToken(h string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(h), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// decodeIngestBody reads the request's JSON array of records within the
// size and count limits. On failure it returns the status to answer with.
func decodeIngestBody(c *gin.Context) ([]tykmcp.TykAnalyticsRecord, int, error) {
	var body io.Reader = http.MaxBytesReader(c.Writer, c.Request.Body, ingestMaxBodyBytes)
	switch enc := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip":
		zr, err := gzip.NewReader(body)
		if err != nil {
			if isBodyTooLarge(err) {
				return nil, http.StatusRequestEntityTooLarge, errIngestBodyTooLarge
			}
			return nil, http.StatusBadRequest, errors.New("invalid gzip body")
		}
		defer zr.Close()
		body = &cappedReader{r: zr, n: ingestMaxBodyBytes}
	default:
		return nil, http.StatusUnsupportedMediaType, errors.New("unsupported Content-Encoding (use gzip or none)")
	}

	dec := json.NewDecoder(body)
	bad := func(err error) ([]tykmcp.TykAnalyticsRecord, int, error) {
		if isBodyTooLarge(err) {
			return nil, http.StatusRequestEntityTooLarge, errIngestBodyTooLarge
		}
		return nil, http.StatusBadRequest, errors.New("body must be a JSON array of Tyk analytics records")
	}
	tok, err := dec.Token()
	if err != nil {
		return bad(err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return bad(nil)
	}
	records := make([]tykmcp.TykAnalyticsRecord, 0, 64)
	for dec.More() {
		if len(records) >= ingestMaxRecords {
			return nil, http.StatusRequestEntityTooLarge, errors.New("too many records in one batch")
		}
		var r tykmcp.TykAnalyticsRecord
		if err := dec.Decode(&r); err != nil {
			return bad(err)
		}
		records = append(records, r)
	}
	if _, err := dec.Token(); err != nil {
		return bad(err)
	}
	return records, 0, nil
}

func isBodyTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.Is(err, errIngestBodyTooLarge) || errors.As(err, &mbe)
}

// --- Ingest token administration ---

// getTykIngestStatus godoc
// @Summary Analytics ingest status of a Tyk connection
// @Description Whether an ingest token is set, when the last batch arrived and what the last 24 hours brought.
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} tykmcp.IngestStatus
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /tyk-connections/{id}/ingest-status [get]
func (a *API) getTykIngestStatus(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	st, err := a.tykMCPService().GetIngestStatus(c.Request.Context(), id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to get the ingest status")
		return
	}
	c.JSON(http.StatusOK, st)
}

// rotateTykIngestToken godoc
// @Summary Generate or rotate a Tyk connection's analytics ingest token
// @Description Returns the new token's plaintext exactly once; only its hash is stored. The previous token stops working immediately.
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} tykmcp.RotatedIngestToken
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /tyk-connections/{id}/ingest-token [post]
func (a *API) rotateTykIngestToken(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	res, err := a.tykMCPService().RotateIngestToken(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to generate the ingest token")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, res)
}

// revokeTykIngestToken godoc
// @Summary Revoke a Tyk connection's analytics ingest token
// @Tags TykMCP
// @Produce json
// @Param id path int true "Connection ID"
// @Success 200 {object} tykmcp.IngestStatus
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /tyk-connections/{id}/ingest-token [delete]
func (a *API) revokeTykIngestToken(c *gin.Context) {
	actor, ok := requireTykMCPActor(c, "tyk-connections")
	if !ok {
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	st, err := a.tykMCPService().RevokeIngestToken(c.Request.Context(), actor, id)
	if err != nil {
		tykMCPErrorResponse(c, err, "Failed to revoke the ingest token")
		return
	}
	c.JSON(http.StatusOK, st)
}
