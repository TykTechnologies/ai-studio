package proxy

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// scrapeMetric sums the samples of metric whose labels include every one of
// labels (name="value" pairs).
func scrapeMetric(t *testing.T, metric string, labels ...string) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	total := 0.0
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, metric+"{") {
			continue
		}
		match := true
		for i := 0; i+1 < len(labels); i += 2 {
			if !strings.Contains(line, labels[i]+`="`+labels[i+1]+`"`) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		v, err := strconv.ParseFloat(line[strings.LastIndex(line, " ")+1:], 64)
		require.NoError(t, err, line)
		total += v
	}
	return total
}

// A create, a results download and a reconciler pass each show up in the
// native API metrics.
func TestNativeMetrics_CreateResultsReconcile(t *testing.T) {
	metrics.Init()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	vendor := string(models.ANTHROPIC)

	const (
		createOp  = "anthropic.message_batches.create"
		resultsOp = "anthropic.message_batches.results"
	)
	creates0 := scrapeMetric(t, "aistudio_native_requests_total", "vendor", vendor, "operation", createOp, "status_code", "200")
	results0 := scrapeMetric(t, "aistudio_native_requests_total", "vendor", vendor, "operation", resultsOp, "status_code", "200")
	durations0 := scrapeMetric(t, "aistudio_native_request_duration_seconds_count", "operation", createOp)
	bytes0 := scrapeMetric(t, "aistudio_native_results_bytes_total", "vendor", vendor, "operation", resultsOp)
	reconciled0 := scrapeMetric(t, "aistudio_native_reconcile_outcomes_total",
		"vendor", vendor, "resource_type", "anthropic.message_batch", "outcome", "accounted")

	// Two batches; the first is downloaded, the second left to the reconciler.
	for i := 0; i < 2; i++ {
		resp, body := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	}
	line := succeededLine("a", "claude-luna", 100, 10)
	for _, id := range []string{"msgbatch_N1", "msgbatch_N2"} {
		h.vendor.addBatch(id, "ended")
		h.vendor.setResults(id, line)
	}
	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_N1")), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	h.proxy.reconcileNative(context.Background(), h.service.NativeResourceStore(), time.Now())

	assert.Equal(t, 2.0, scrapeMetric(t, "aistudio_native_requests_total", "vendor", vendor, "operation", createOp, "status_code", "200")-creates0)
	assert.Equal(t, 1.0, scrapeMetric(t, "aistudio_native_requests_total", "vendor", vendor, "operation", resultsOp, "status_code", "200")-results0)
	assert.Equal(t, 2.0, scrapeMetric(t, "aistudio_native_request_duration_seconds_count", "operation", createOp)-durations0)
	assert.Equal(t, float64(len(line)+1), scrapeMetric(t, "aistudio_native_results_bytes_total", "vendor", vendor, "operation", resultsOp)-bytes0)
	assert.Equal(t, 1.0, scrapeMetric(t, "aistudio_native_reconcile_outcomes_total",
		"vendor", vendor, "resource_type", "anthropic.message_batch", "outcome", "accounted")-reconciled0,
		"the reconciler accounted the batch nobody downloaded")
}
