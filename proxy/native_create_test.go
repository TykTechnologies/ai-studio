package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// batchCreateBody is a create body with one request per model.
func batchCreateBody(models ...string) string {
	var reqs []string
	for i, m := range models {
		reqs = append(reqs, `{"custom_id":"req-`+string(rune('a'+i))+`","params":{"model":"`+m+
			`","max_tokens":10,"messages":[{"role":"user","content":"hello `+string(rune('a'+i))+`"}]}}`)
	}
	return `{"requests":[` + strings.Join(reqs, ",") + `]}`
}

const batchesPath = "/llm/rest/claude/v1/messages/batches"

// A batch whose every request is admitted reaches the vendor once, as sent,
// and the App gets a wrapped ID backed by a ledger row it owns.
func TestNativeCreate_AdmittedBatchIsRecordedAndWrapped(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) { l.AllowedModels = []string{"^claude-luna$"} }})
	body := batchCreateBody("claude-luna", "claude-luna")
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body, "anthropic-version", "2023-06-01")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	calls := h.vendor.callsTo(http.MethodPost, "/v1/messages/batches")
	require.Len(t, calls, 1)
	assert.Equal(t, body, string(calls[0].Body), "an unfiltered batch is forwarded byte for byte")
	assert.Equal(t, "anthropic-vendor-key", calls[0].Headers.Get("x-api-key"))

	obj := decodeBatch(t, out)
	assert.Equal(t, h.wrap(h.llm, h.appA, "msgbatch_N1"), obj["id"])

	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_N1")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingPending, row.AccountingState)
	assert.Equal(t, "in_progress", row.State)
	assert.Equal(t, h.appA.UserID, row.UserID)
	assert.Equal(t, string(models.ANTHROPIC), row.Vendor)
	assert.JSONEq(t, string(out), row.Snapshot)
	require.NotNil(t, row.ResultsExpireAt)

	// The wrapped ID works for retrieve.
	resp, _ = h.do(h.keyA, http.MethodGet, batchesPath+"/"+obj["id"].(string), "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// The create is logged, with no chat record (its cost is in the results).
	logs := h.proxyLogs(h.appA)
	require.NotEmpty(t, logs)
	assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
	var n int64
	h.db.Model(&models.LLMChatRecord{}).Where("app_id = ?", h.appA.ID).Count(&n)
	assert.Zero(t, n)
}

// One request asking for a model the LLM does not allow refuses the batch,
// naming it, before anything reaches the vendor. A top-level model on the
// body changes nothing: the native route never reads one.
func TestNativeCreate_DisallowedItemModel(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) { l.AllowedModels = []string{"^claude-luna$"} }})
	for name, body := range map[string]string{
		"plain":                    batchCreateBody("claude-luna", "claude-opus-max"),
		"smuggled top-level model": `{"model":"claude-luna",` + strings.TrimPrefix(batchCreateBody("claude-luna", "claude-opus-max"), "{"),
	} {
		t.Run(name, func(t *testing.T) {
			resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, string(out))
			typ, msg := nativeErr(t, out)
			assert.Equal(t, "permission_error", typ)
			assert.Contains(t, msg, "claude-opus-max")
			assert.Contains(t, msg, `"req-b"`)
		})
	}
	// A duplicate model key cannot show the check one model and the vendor another.
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath,
		`{"requests":[{"custom_id":"a","params":{"model":"claude-luna","model":"claude-opus-max","max_tokens":1,"messages":[]}}]}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Empty(t, h.vendor.recorded())
}

// Malformed bodies are the vendor's 400, not forwarded.
func TestNativeCreate_MalformedBody(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	for _, body := range []string{`{}`, `{"requests":[]}`, `not json`, `{"requests":[{"custom_id":"a"}]}`} {
		resp, out := h.do(h.keyA, http.MethodPost, batchesPath, body)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", body, out)
		typ, _ := nativeErr(t, out)
		assert.Equal(t, "invalid_request_error", typ)
	}
	assert.Empty(t, h.vendor.recorded())
}

// A body over the configured limit is a 413.
func TestNativeCreate_BodyLimit(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{config: &Config{NativeMaxBodyBytes: 512}})
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna", "claude-luna", "claude-luna", "claude-luna", "claude-luna"))
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, string(out))
	assert.Empty(t, h.vendor.recorded())

	resp, out = h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
}

func TestConfigNativeLimits(t *testing.T) {
	var c *Config
	assert.EqualValues(t, 64<<20, c.nativeMaxBodyBytes())
	assert.EqualValues(t, 256<<20, (&Config{NativeMaxBodyBytes: 1 << 40}).nativeMaxBodyBytes())
	assert.Equal(t, 4, c.nativeMaxConcurrentCreates())
	assert.Equal(t, 10000, c.nativeMaxFilteredItems())
	assert.Equal(t, 0, c.nativeMaxInProgressPerApp())
}

// An App out of budget gets exactly the denial a synchronous call gets, and
// so does a budget check that cannot run.
func TestNativeCreate_BudgetDenial(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	for name, err := range map[string]error{
		"exhausted":   fmt.Errorf("budget exceeded: app monthly budget is 0"),
		"unavailable": fmt.Errorf("%w: reading app 3: disk I/O error", services.ErrBudgetCheckUnavailable),
	} {
		t.Run(name, func(t *testing.T) {
			h.proxy.budgetService = denyBudget{err: err}
			syncResp, syncBody := h.do(h.keyA, http.MethodPost, "/llm/rest/claude/v1/messages",
				`{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
			require.NotEqual(t, http.StatusOK, syncResp.StatusCode, string(syncBody))

			resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
			assert.Equal(t, syncResp.StatusCode, resp.StatusCode)
			assert.JSONEq(t, string(syncBody), string(out))
		})
	}
	assert.Empty(t, h.vendor.recorded())
}

// The in-progress cap counts the App's unfinished batches in the ledger.
func TestNativeCreate_InProgressCap(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{config: &Config{NativeMaxInProgressPerApp: 2}})
	for i := 0; i < 2; i++ {
		resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	}
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(out))
	typ, _ := nativeErr(t, out)
	assert.Equal(t, "rate_limit_error", typ)

	// Another App has its own count.
	resp, out = h.do(h.keyB, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	// A batch that ended frees its slot (the retrieve refreshes the row).
	h.vendor.mu.Lock()
	h.vendor.batches["msgbatch_N1"]["processing_status"] = "ended"
	h.vendor.mu.Unlock()
	resp, _ = h.do(h.keyA, http.MethodGet, batchesPath+"/"+h.wrap(h.llm, h.appA, "msgbatch_N1"), "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, out = h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/batches"), 4)
}

// flakyNativeStore fails the calls it is told to.
type flakyNativeStore struct {
	services.NativeResourceStore
	failCreate, failList bool
	creates              atomic.Int32
}

func (s *flakyNativeStore) Create(ctx context.Context, r *models.NativeResource) error {
	s.creates.Add(1)
	if s.failCreate {
		return errors.New("disk full")
	}
	return s.NativeResourceStore.Create(ctx, r)
}

func (s *flakyNativeStore) List(ctx context.Context, q services.NativeResourceListQuery) ([]models.NativeResource, error) {
	if s.failList {
		return nil, errors.New("database is locked")
	}
	return s.NativeResourceStore.List(ctx, q)
}

// A batch the ledger cannot record is cancelled at the vendor and the client
// gets a 503: an unrecorded batch would never be accounted.
func TestNativeCreate_LedgerInsertFailureCancelsUpstream(t *testing.T) {
	store := &flakyNativeStore{failCreate: true}
	h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
		store.NativeResourceStore = s.(*services.Service).NativeResourceStore()
		return serviceWithNativeStore{ServiceInterface: s, store: store}
	}})
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"), "anthropic-version", "2023-06-01")
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, string(out))
	assert.NotContains(t, string(out), "msgbatch_N1")
	assert.EqualValues(t, nativeLedgerAttempts, store.creates.Load())

	cancels := h.vendor.callsTo(http.MethodPost, "/v1/messages/batches/msgbatch_N1/cancel")
	require.Len(t, cancels, 1)
	assert.Equal(t, "anthropic-vendor-key", cancels[0].Headers.Get("x-api-key"))
	h.vendor.mu.Lock()
	assert.Equal(t, "canceling", h.vendor.batches["msgbatch_N1"]["processing_status"])
	h.vendor.mu.Unlock()
}

// A ledger that cannot even be read refuses the create before the vendor is
// called.
func TestNativeCreate_UnhealthyLedgerRefusesBeforeVendor(t *testing.T) {
	store := &flakyNativeStore{failList: true}
	h := newNativeHarness(t, nativeHarnessOpts{wrapService: func(s services.ServiceInterface) services.ServiceInterface {
		store.NativeResourceStore = s.(*services.Service).NativeResourceStore()
		return serviceWithNativeStore{ServiceInterface: s, store: store}
	}})
	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, string(out))
	assert.Empty(t, h.vendor.recorded())
}

// Creates past the concurrency bound wait for a slot rather than failing.
func TestNativeCreate_ConcurrentCreatesWait(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{config: &Config{NativeMaxConcurrentCreates: 1}})
	const n = 5
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			resp, _ := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna"))
			codes <- resp.StatusCode
		}()
	}
	for i := 0; i < n; i++ {
		assert.Equal(t, http.StatusOK, <-codes)
	}
	assert.Len(t, h.vendor.callsTo(http.MethodPost, "/v1/messages/batches"), n)
	var rows int64
	h.db.Model(&models.NativeResource{}).Where("app_id = ?", h.appA.ID).Count(&rows)
	assert.EqualValues(t, n, rows)
}
