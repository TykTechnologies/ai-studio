package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// listPage is an Anthropic list response.
type listPage struct {
	Data    []map[string]any `json:"data"`
	HasMore bool             `json:"has_more"`
	FirstID *string          `json:"first_id"`
	LastID  *string          `json:"last_id"`
}

func decodeListPage(t *testing.T, body []byte) listPage {
	t.Helper()
	var p listPage
	require.NoError(t, json.Unmarshal(body, &p), "body: %s", body)
	return p
}

func (p listPage) ids() []string {
	out := []string{}
	for _, d := range p.Data {
		s, _ := d["id"].(string)
		out = append(out, s)
	}
	return out
}

// seedListedBatch records a batch created by app on llm at createdAt, with
// the snapshot the gateway would have kept.
func (h *nativeHarness) seedListedBatch(llm *models.LLM, app *models.App, vendorID string, createdAt time.Time) *models.NativeResource {
	h.t.Helper()
	snap, err := json.Marshal(map[string]any{
		"id": h.wrap(llm, app, vendorID), "type": "message_batch", "processing_status": "in_progress",
		"created_at": createdAt.UTC().Format(time.RFC3339),
	})
	require.NoError(h.t, err)
	row := &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: llm.ID, AppID: app.ID, UserID: app.UserID,
		ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: vendorID,
		State: "in_progress", Snapshot: string(snap), AccountingState: models.NativeAccountingPending,
		CreatedAt: createdAt,
	}
	require.NoError(h.t, h.service.NativeResourceStore().Create(context.Background(), row))
	return row
}

func (h *nativeHarness) settleRow(row *models.NativeResource, state string) {
	h.t.Helper()
	store := h.service.NativeResourceStore()
	now := time.Now()
	ok, err := store.Claim(context.Background(), row.ID, "test-settle", now, time.Minute)
	require.NoError(h.t, err)
	require.True(h.t, ok)
	ok, err = store.Settle(context.Background(), row.ID, "test-settle", state, now)
	require.NoError(h.t, err)
	require.True(h.t, ok)
}

func listURL(params ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(params); i += 2 {
		q.Set(params[i], params[i+1])
	}
	if len(q) == 0 {
		return batchesPath
	}
	return batchesPath + "?" + q.Encode()
}

// The listing is the calling App's batches on this LLM from the ledger,
// newest first, paged through wrapped cursors in both directions; the vendor
// is never asked (it would list every App's batches).
func TestNativeList_OwnBatchesNewestFirstWithWrappedCursors(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		h.seedListedBatch(h.llm, h.appA, "msgbatch_L"+strconv.Itoa(i), base.Add(time.Duration(i)*time.Minute))
	}
	h.seedListedBatch(h.llm, h.appB, "msgbatch_B0", base.Add(10*time.Minute))
	h.seedListedBatch(h.llm2, h.appA, "msgbatch_T0", base.Add(11*time.Minute))
	w := func(i int) string { return h.wrap(h.llm, h.appA, "msgbatch_L"+strconv.Itoa(i)) }

	resp, body := h.do(h.keyA, http.MethodGet, listURL("limit", "2"), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p := decodeListPage(t, body)
	assert.Equal(t, []string{w(4), w(3)}, p.ids())
	assert.True(t, p.HasMore)
	require.NotNil(t, p.FirstID)
	require.NotNil(t, p.LastID)
	assert.Equal(t, w(4), *p.FirstID)
	assert.Equal(t, w(3), *p.LastID)
	assert.Equal(t, "message_batch", p.Data[0]["type"], "the ledger snapshot is served")
	assert.Equal(t, "in_progress", p.Data[0]["processing_status"])

	resp, body = h.do(h.keyA, http.MethodGet, listURL("limit", "2", "after_id", *p.LastID), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p = decodeListPage(t, body)
	assert.Equal(t, []string{w(2), w(1)}, p.ids())
	assert.True(t, p.HasMore)

	resp, body = h.do(h.keyA, http.MethodGet, listURL("limit", "2", "after_id", *p.LastID), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p = decodeListPage(t, body)
	assert.Equal(t, []string{w(0)}, p.ids())
	assert.False(t, p.HasMore)

	resp, body = h.do(h.keyA, http.MethodGet, listURL("limit", "2", "before_id", w(1)), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p = decodeListPage(t, body)
	assert.Equal(t, []string{w(3), w(2)}, p.ids(), "the page just newer than the cursor, newest first")
	assert.True(t, p.HasMore, "msgbatch_L4 is newer still")

	resp, body = h.do(h.keyA, http.MethodGet, listURL("limit", "2", "before_id", w(3)), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p = decodeListPage(t, body)
	assert.Equal(t, []string{w(4)}, p.ids())
	assert.False(t, p.HasMore)

	// Default page: all five, none of App B's, none on the other LLM.
	resp, body = h.do(h.keyA, http.MethodGet, listURL(), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	p = decodeListPage(t, body)
	assert.Equal(t, []string{w(4), w(3), w(2), w(1), w(0)}, p.ids())
	assert.False(t, p.HasMore)

	// App B sees only its own; on claude-two App A sees only that one.
	resp, body = h.do(h.keyB, http.MethodGet, listURL(), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, []string{h.wrap(h.llm, h.appB, "msgbatch_B0")}, decodeListPage(t, body).ids())
	resp, body = h.do(h.keyA, http.MethodGet, "/llm/rest/claude-two/v1/messages/batches", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, []string{h.wrap(h.llm2, h.appA, "msgbatch_T0")}, decodeListPage(t, body).ids())

	assert.Empty(t, h.vendor.recorded(), "a listing never reaches the vendor")
	logs := h.proxyLogs(h.appA, 1)
	require.NotEmpty(t, logs, "listings are logged")
	assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
}

// An App with no batches gets an empty page.
func TestNativeList_Empty(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	resp, body := h.do(h.keyA, http.MethodGet, listURL(), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.JSONEq(t, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`, string(body))
}

// A cursor must be an ID the gateway handed this App on this LLM; anything
// else - a raw vendor ID, a tampered ID, another App's - is a 400, and so is
// a bad limit or both cursors at once. (A MAC that fails is still accepted
// when this node's ledger holds the App's row for the vendor ID: the
// key-rotation fallback of unwrapID.)
func TestNativeList_BadCursorsAndLimits(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.seedListedBatch(h.llm, h.appA, "msgbatch_C0", time.Now().Add(-time.Minute))
	h.seedListedBatch(h.llm, h.appB, "msgbatch_C1", time.Now())
	own := h.wrap(h.llm, h.appA, "msgbatch_C0")
	tampered := own[:len(own)-1] + string("AB"[map[bool]int{true: 1, false: 0}[own[len(own)-1] == 'A']])

	for name, u := range map[string]string{
		"raw vendor id":   listURL("after_id", "msgbatch_C0"),
		"tampered":        listURL("after_id", tampered),
		"another app's":   listURL("before_id", h.wrap(h.llm, h.appB, "msgbatch_C1")),
		"another llm's":   listURL("after_id", h.wrap(h.llm2, h.appA, "msgbatch_C1")),
		"unknown on node": listURL("after_id", h.wrap(h.llm, h.appA, "msgbatch_Elsewhere")),
		"both cursors":    listURL("after_id", own, "before_id", own),
		"limit zero":      listURL("limit", "0"),
		"limit too big":   listURL("limit", "1001"),
		"limit not int":   listURL("limit", "ten"),
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := h.do(h.keyA, http.MethodGet, u, "")
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
			typ, _ := nativeErr(t, body)
			assert.Equal(t, "invalid_request_error", typ)
		})
	}
	resp, body := h.do(h.keyA, http.MethodGet, listURL("limit", "1000"), "")
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, h.vendor.recorded())
}

// A delete on a node whose ledger does not hold the batch (it was created
// elsewhere) is refused before the vendor is called.
func TestNativeDelete_NotOnCreatorNodeIs409(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_D0", "ended")
	resp, body := h.do(h.keyA, http.MethodDelete, batchesPath+"/"+h.wrap(h.llm, h.appA, "msgbatch_D0"), "")
	require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
	typ, msg := nativeErr(t, body)
	assert.Equal(t, "invalid_request_error", typ)
	assert.Contains(t, msg, "gateway that created it")
	assert.Empty(t, h.vendor.recorded(), "the vendor is not called")
}

// A settled batch is deleted at the vendor (raw ID, vendor key), the answer
// carries the wrapped ID, and the batch leaves the ledger and its listing.
func TestNativeDelete_SettledForwardsAndLeavesLedger(t *testing.T) {
	t.Parallel()
	for _, state := range []string{models.NativeAccountingAccounted, models.NativeAccountingUnaccountable, models.NativeAccountingLost} {
		t.Run(state, func(t *testing.T) {
			h := newNativeHarness(t, nativeHarnessOpts{})
			h.vendor.addBatch("msgbatch_D1", "ended")
			row := h.seedListedBatch(h.llm, h.appA, "msgbatch_D1", time.Now())
			h.settleRow(row, state)
			wrapped := h.wrap(h.llm, h.appA, "msgbatch_D1")

			resp, body := h.do(h.keyA, http.MethodDelete, batchesPath+"/"+wrapped, "")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			assert.JSONEq(t, `{"id":"`+wrapped+`","type":"message_batch_deleted"}`, string(body))

			calls := h.vendor.callsTo(http.MethodDelete, "/v1/messages/batches/msgbatch_D1")
			require.Len(t, calls, 1)
			assert.Equal(t, "anthropic-vendor-key", calls[0].Headers.Get("x-api-key"))

			_, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_D1")
			assert.Error(t, err, "the row left the ledger")
			resp, body = h.do(h.keyA, http.MethodGet, listURL(), "")
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Empty(t, decodeListPage(t, body).ids())
		})
	}
}

// A vendor refusal of the delete is passed on and the row stays.
func TestNativeDelete_VendorRefusalKeepsRow(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_D2", "in_progress")
	row := h.seedListedBatch(h.llm, h.appA, "msgbatch_D2", time.Now())
	h.settleRow(row, models.NativeAccountingAccounted)

	resp, body := h.do(h.keyA, http.MethodDelete, batchesPath+"/"+h.wrap(h.llm, h.appA, "msgbatch_D2"), "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	_, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_D2")
	assert.NoError(t, err)
}

// Another App's batch, or a tampered ID, is not found, whatever the ledger
// holds.
func TestNativeDelete_ForeignIDNotFound(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_D3", "ended")
	row := h.seedListedBatch(h.llm, h.appA, "msgbatch_D3", time.Now())
	h.settleRow(row, models.NativeAccountingAccounted)

	resp, body := h.do(h.keyB, http.MethodDelete, batchesPath+"/"+h.wrap(h.llm, h.appA, "msgbatch_D3"), "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	resp, body = h.do(h.keyB, http.MethodDelete, batchesPath+"/msgbatch_D3", "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	assert.Empty(t, h.vendor.callsTo(http.MethodDelete, "/v1/messages/batches/msgbatch_D3"))
}

// An unaccounted batch still in progress cannot be deleted: its usage is
// not known yet. The vendor's delete is never called.
func TestNativeDelete_InProgressUnaccountedIs409(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_D4", "in_progress")
	h.seedListedBatch(h.llm, h.appA, "msgbatch_D4", time.Now())

	resp, body := h.do(h.keyA, http.MethodDelete, batchesPath+"/"+h.wrap(h.llm, h.appA, "msgbatch_D4"), "")
	require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
	_, msg := nativeErr(t, body)
	assert.Contains(t, msg, "has ended")
	assert.Empty(t, h.vendor.callsTo(http.MethodDelete, "/v1/messages/batches/msgbatch_D4"))
	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_D4")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingPending, row.AccountingState)
}

// On the creating node, deleting an ended batch that is not accounted yet
// accounts it first - once, at the batch price, with the creating request's
// identity - and only then deletes it at the vendor. A second delete finds
// nothing left to delete on this node.
func TestNativeDelete_AccountsUnsettledEndedBatchOnceThenDeletes(t *testing.T) {
	t.Parallel()
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend }})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_D5", "ended")
	h.vendor.setResults("msgbatch_D5",
		succeededLine("a", "claude-luna", 1000, 200), erroredLine, succeededLine("b", "claude-luna", 500, 100))
	h.seedBatchRow(h.llm, h.appA, "msgbatch_D5")
	wrapped := h.wrap(h.llm, h.appA, "msgbatch_D5")

	resp, body := h.do(h.keyA, http.MethodDelete, batchesPath+"/"+wrapped, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.JSONEq(t, `{"id":"`+wrapped+`","type":"message_batch_deleted"}`, string(body))

	// Accounted before the vendor's delete.
	var order []string
	for _, c := range h.vendor.recorded() {
		order = append(order, c.Method+" "+c.Path)
	}
	require.Contains(t, order, "GET /v1/messages/batches/msgbatch_D5/results")
	require.Contains(t, order, "DELETE /v1/messages/batches/msgbatch_D5")
	assert.Equal(t, "DELETE /v1/messages/batches/msgbatch_D5", order[len(order)-1], "the delete comes last: %v", order)

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = h.chatRecords(h.appA)
		return len(recs) == 1
	}, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, 1500, recs[0].PromptTokens)
	assert.Equal(t, 300, recs[0].ResponseTokens)
	assert.InDelta(t, (300*0.00001+1500*0.000002)*0.5*10000, recs[0].Cost, 1e-9)
	assert.Equal(t, "user:alice", recs[0].OnBehalfOf)

	resp, body = h.do(h.keyA, http.MethodDelete, batchesPath+"/"+wrapped, "")
	assert.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
	assert.Len(t, h.vendor.callsTo(http.MethodDelete, "/v1/messages/batches/msgbatch_D5"), 1)

	assert.Len(t, h.chatRecords(h.appA), 1, "accounted exactly once")
	_, calls := spend.totals()
	assert.Equal(t, 1, calls)
}

// Concurrent deletes of one unsettled batch never delete it unaccounted nor
// account it twice: a loser of the claim is refused (409) or finds the batch
// gone. (The harness database is shared-cache SQLite, whose table locks can
// fail the winner's settle too; the row then stays claimed for the
// reconciler, so the assertions are "at most once" and "never deleted
// unaccounted".)
func TestNativeDelete_ConcurrentDeletesAccountOnce(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_D6", "ended")
	h.vendor.setResults("msgbatch_D6", succeededLine("a", "claude-luna", 100, 10))
	h.seedBatchRow(h.llm, h.appA, "msgbatch_D6")
	path := batchesPath + "/" + h.wrap(h.llm, h.appA, "msgbatch_D6")

	const n = 6
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := h.do(h.keyA, http.MethodDelete, path, "")
			codes <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict, http.StatusNotFound, http.StatusServiceUnavailable:
			// Refused: claim lost, already gone, or (shared-cache SQLite
			// table locks under load) the ledger momentarily unreadable.
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	assert.LessOrEqual(t, ok, 1, "the vendor deletes the batch at most once")
	recs := h.chatRecords(h.appA)
	assert.LessOrEqual(t, len(recs), 1, "accounted at most once")
	if ok == 1 {
		assert.Len(t, recs, 1, "a deleted batch was accounted")
	}
	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_D6")
	if ok == 0 {
		require.NoError(t, err, "a batch not deleted keeps its row")
		assert.NotEqual(t, models.NativeAccountingPending, row.AccountingState)
	} else {
		assert.Error(t, err)
	}
}
