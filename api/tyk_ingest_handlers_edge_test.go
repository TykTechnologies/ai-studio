package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bodies the two real producers send, captured live from Gateway 5.15.1
// and Pump 1.16.0: the Vector bridge (gzip, records unwrapped from the stdout
// pump's tyk-analytics-record) and the http pump (gzip).
func TestIngestTykAnalytics_LiveProducerBodies(t *testing.T) {
	h := setupIngestAPI(t)
	for _, file := range []string{"pump_records_live_stdout_vector.json", "pump_records_live_http_pump.json"} {
		raw, err := os.ReadFile("../services/tykmcp/testdata/" + file)
		require.NoError(t, err)
		w := h.ingest(t, "Bearer tai_ing_good", gzipped(t, raw), map[string]string{"Content-Encoding": "gzip"})
		require.Equal(t, http.StatusAccepted, w.Code, "%s: %s", file, w.Body.String())
	}
	require.Len(t, h.fake.batches, 2)
	for _, b := range h.fake.batches {
		assert.Len(t, b, 12)
	}
}

// The token is only read from the Authorization header, never from the URL
// (where it would end up in access logs).
func TestIngestTykAnalytics_TokenNotFromQuery(t *testing.T) {
	h := setupIngestAPI(t)
	for _, q := range []string{"?token=tai_ing_good", "?access_token=tai_ing_good", "?api_key=tai_ing_good"} {
		req := httptest.NewRequest(http.MethodPost, tykmcp.IngestPath+q, strings.NewReader(`[]`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, q)
	}
	assert.Empty(t, h.fake.batches)
}

// erroringIngestService fails IngestAnalytics with a fixed error.
type erroringIngestService struct {
	*fakeIngestService
	err error
}

func (e *erroringIngestService) IngestAnalytics(context.Context, uint, []tykmcp.TykAnalyticsRecord) (*tykmcp.IngestResult, error) {
	return nil, e.err
}

// The status tells the pump what to do: 5xx is retried, any other 4xx drops
// the batch. A storage failure must be retryable; a validation failure or
// a token revoked mid-request must not be; internal detail never leaks.
func TestIngestTykAnalytics_ServiceErrorStatuses(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"storage failure": {fmt.Errorf("insert: %w", errors.New("pq: connection refused at 10.0.0.5")), http.StatusInternalServerError},
		"validation":      {fmt.Errorf("%w: unknown record", tykmcp.ErrValidation), http.StatusBadRequest},
		"revoked":         {tykmcp.ErrIngestUnauthorized, http.StatusUnauthorized},
		"disabled":        {tykmcp.ErrDisabled, http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			db := apitest.SetupTestDB(t)
			service := apitest.SetupTestService(db)
			cfg := apitest.SetupTestAuthConfig(db, service)
			cfg.TestMode = false
			authService := apitest.SetupTestAuthService(db, service)
			authService.Config.TestMode = false
			a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)
			fake := &fakeIngestService{tokens: map[string]uint{"tai_ing_good": 7}}
			service.TykMCP = &erroringIngestService{fakeIngestService: fake, err: tc.err}
			h := &ingestHarness{r: a.router, fake: fake}

			w := h.ingest(t, "Bearer tai_ing_good", []byte(`[{"api_id":"a"}]`), nil)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.NotContains(t, w.Body.String(), "10.0.0.5")
		})
	}
}
