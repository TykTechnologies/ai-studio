package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

func decodeBatch(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	require.NoError(t, json.Unmarshal(body, &obj), "body: %s", body)
	return obj
}

// Retrieve through /llm/rest and /llm/call answers the owning App with the
// vendor's batch, its id wrapped and its results_url pointing at the gateway.
// The vendor sees the raw ID, the LLM's key and only allow-listed headers and
// query parameters.
func TestNative_RetrieveWrapsIDsAndPinsTheUpstream(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	id := h.wrap(h.llm, h.appA, "msgbatch_A1")

	for _, mode := range []string{"rest", "call", "stream"} {
		t.Run(mode, func(t *testing.T) {
			before := len(h.vendor.recorded())
			resp, body := h.do(h.keyA, http.MethodGet, "/llm/"+mode+"/claude/v1/messages/batches/"+id+"?beta=true&after_id=x&evil=1", "",
				"anthropic-version", "2023-06-01",
				"anthropic-beta", "message-batches-2024-09-24",
				"anthropic-workspace-id", "wrkspc_other",
				"anthropic-user-profile-id", "prof_1",
				"X-Tyk-Something", "internal",
				"X-Custom", "nope",
				"x-api-key", "client-supplied")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			assert.Equal(t, "req_vendor_1", resp.Header.Get("Request-Id"), "vendor response headers reach the client")

			obj := decodeBatch(t, body)
			assert.Equal(t, id, obj["id"])
			assert.Equal(t, "/v1/messages/batches/"+id+"/results", obj["results_url"],
				"results_url is root-relative to the client's base URL without a configured public URL")
			assert.NotContains(t, string(body), "api.anthropic.com")
			assert.NotContains(t, string(body), `"msgbatch_A1"`, "the raw vendor id never reaches the client")

			calls := h.vendor.recorded()
			require.Len(t, calls, before+1)
			got := calls[before]
			assert.Equal(t, http.MethodGet, got.Method)
			assert.Equal(t, "/v1/messages/batches/msgbatch_A1", got.Path)
			assert.Equal(t, "beta=true", got.RawQuery)
			assert.Equal(t, "anthropic-vendor-key", got.Headers.Get("x-api-key"))
			assert.Equal(t, "2023-06-01", got.Headers.Get("anthropic-version"))
			assert.Equal(t, "message-batches-2024-09-24", got.Headers.Get("anthropic-beta"))
			assert.Empty(t, got.Headers.Get("Authorization"))
			assert.Empty(t, got.Headers.Get("anthropic-workspace-id"))
			assert.Empty(t, got.Headers.Get("anthropic-user-profile-id"))
			assert.Empty(t, got.Headers.Get("X-Custom"))
			for k := range got.Headers {
				assert.False(t, strings.HasPrefix(strings.ToLower(k), "x-tyk"), "header %s reached the vendor", k)
			}
		})
	}

	// One proxy log row per call, bodies as the client saw them.
	logs := h.proxyLogs(h.appA, 3)
	require.Len(t, logs, 3)
	for _, l := range logs {
		assert.Equal(t, http.StatusOK, l.ResponseCode)
		assert.Equal(t, h.llm.ID, l.LLMID)
		assert.Contains(t, l.ResponseBody, id)
	}
}

// An in-progress batch has no results_url; it stays null.
func TestNative_RetrieveInProgressKeepsNullResultsURL(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_RUN", "in_progress")
	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+h.wrap(h.llm, h.appA, "msgbatch_RUN"), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	obj := decodeBatch(t, body)
	v, present := obj["results_url"]
	assert.True(t, present)
	assert.Nil(t, v)
}

// With a configured public URL the results_url is absolute, on the route the
// request came in on; the request's Host never feeds it.
func TestNative_ResultsURLFromConfiguredPublicURL(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{config: &Config{NativePublicURL: "https://gw.example.com/"}})
	h.vendor.addBatch("msgbatch_A1", "ended")
	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	resp, body := h.do(h.keyA, http.MethodGet, "/llm/call/claude/v1/messages/batches/"+id, "",
		"X-Forwarded-Host", "evil.example.com")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, "https://gw.example.com/llm/call/claude/v1/messages/batches/"+id+"/results", decodeBatch(t, body)["results_url"])
}

// An endpoint behind a corporate prefix still matches on the canonical path,
// and the upstream keeps the prefix.
func TestNative_EndpointWithPathPrefix(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{endpointPath: "/corp/anthropic/v1"})
	h.vendor.addBatch("msgbatch_A1", "ended")
	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
	// The fake serves /v1/... only, so a 404 from it proves the path was
	// joined under the prefix; what matters is the path it was sent.
	_ = body
	calls := h.vendor.recorded()
	require.Len(t, calls, 1, "status %d", resp.StatusCode)
	assert.Equal(t, "/corp/anthropic/v1/messages/batches/msgbatch_A1", calls[0].Path)
}

// Cancel forwards a POST to the vendor and answers with the wrapped batch.
func TestNative_Cancel(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_RUN", "in_progress")
	id := h.wrap(h.llm, h.appA, "msgbatch_RUN")
	resp, body := h.do(h.keyA, http.MethodPost, "/llm/rest/claude/v1/messages/batches/"+id+"/cancel", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	obj := decodeBatch(t, body)
	assert.Equal(t, id, obj["id"])
	assert.Equal(t, "canceling", obj["processing_status"])
	calls := h.vendor.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/v1/messages/batches/msgbatch_RUN/cancel", calls[0].Path)
}

// Every ID that does not verify for the calling App and LLM gets the same 404
// and never reaches the vendor: another App's, another LLM's on the same
// vendor key, a tampered MAC, a raw vendor ID.
func TestNative_ForeignOrForgedIDsAreNotFound(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	idA := h.wrap(h.llm, h.appA, "msgbatch_A1")
	tampered := idA[:len(idA)-1] + map[bool]string{true: "1", false: "0"}[strings.HasSuffix(idA, "0")]

	for name, tc := range map[string]struct{ key, path string }{
		"another app":          {h.keyB, "/llm/rest/claude/v1/messages/batches/" + idA},
		"another llm same key": {h.keyA, "/llm/rest/claude-two/v1/messages/batches/" + idA},
		"tampered mac":         {h.keyA, "/llm/rest/claude/v1/messages/batches/" + tampered},
		"raw vendor id":        {h.keyA, "/llm/rest/claude/v1/messages/batches/msgbatch_A1"},
		"garbage":              {h.keyA, "/llm/rest/claude/v1/messages/batches/x.t1kzz.yy"},
		"cancel another app":   {h.keyB, "/llm/rest/claude/v1/messages/batches/" + idA + "/cancel"},
	} {
		t.Run(name, func(t *testing.T) {
			method := http.MethodGet
			if strings.HasSuffix(tc.path, "/cancel") {
				method = http.MethodPost
			}
			resp, body := h.do(tc.key, method, tc.path, "")
			require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
			typ, msg := nativeErr(t, body)
			assert.Equal(t, "not_found_error", typ)
			assert.Equal(t, nativeNotFoundMsg, msg)
		})
	}
	assert.Empty(t, h.vendor.recorded(), "no unverified ID reaches the vendor")
}

// Access inherited as a failover rung passes the ACL for synchronous calls
// but not for native resources.
func TestNative_FailoverInheritedAccessRefused(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	id := h.wrap(h.llm, h.appC, "msgbatch_A1")
	resp, body := h.do(h.keyC, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "",
		hdrFailoverOrigin, "primary", hdrFailoverAttempt, "1", hdrFailoverToken, h.proxy.failoverToken)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	typ, _ := nativeErr(t, body)
	assert.Equal(t, "permission_error", typ)

	// Without the marker the ACL itself refuses.
	resp, _ = h.do(h.keyC, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, h.vendor.recorded())
}

// A native operation without its prerequisites is a 501 in the vendor's
// shape; it never falls through to the legacy chain (which would answer a
// GET with 405 and a POST by forwarding it).
func TestNative_MissingPrerequisitesAre501(t *testing.T) {
	for name, opts := range map[string]nativeHarnessOpts{
		"no ledger": {wrapService: func(s services.ServiceInterface) services.ServiceInterface {
			return serviceWithoutNativeStore{s}
		}},
		"no vendor key": {vendorKey: "-"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newNativeHarness(t, opts)
			h.vendor.addBatch("msgbatch_A1", "in_progress")
			id := wrapBatchID(t, "some-key", h.llm.ID, h.appA.ID, "msgbatch_A1")
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				path := "/llm/rest/claude/v1/messages/batches/" + id
				if method == http.MethodPost {
					path += "/cancel"
				}
				resp, body := h.do(h.keyA, method, path, "")
				require.Equal(t, http.StatusNotImplemented, resp.StatusCode, string(body))
				typ, _ := nativeErr(t, body)
				assert.Equal(t, "api_error", typ)
			}
			assert.Empty(t, h.vendor.recorded(), "the legacy chain was reached")
		})
	}
}

// A native path asked with a method no operation declares is the vendor's
// 405, with Allow.
func TestNative_WrongMethodOnNativePath(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	resp, body := h.do(h.keyA, http.MethodPut, "/llm/rest/claude/v1/messages/batches/"+id, `{}`)
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, string(body))
	assert.Equal(t, "GET, DELETE", resp.Header.Get("Allow"))
	typ, _ := nativeErr(t, body)
	assert.Equal(t, "invalid_request_error", typ)
	assert.Empty(t, h.vendor.recorded())
}

// The native route changes nothing for the calls it does not claim.
func TestNative_LegacyChainUnchanged(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	msg := `{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`

	for _, mode := range []string{"rest", "call"} {
		resp, body := h.do(h.keyA, http.MethodPost, "/llm/"+mode+"/claude/v1/messages", msg)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"msg_1"`)
	}

	// /llm/call still detects a streaming body and streams.
	resp, body := h.do(h.keyA, http.MethodPost, "/llm/call/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	assert.Contains(t, string(body), "message_stop")

	// A GET on a path no extension declares is still the legacy 405.
	resp, _ = h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/models", "")
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	resp, _ = h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages", "")
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	// The Messages calls still produce chat records.
	require.Eventually(t, func() bool {
		var n int64
		h.db.Model(&models.LLMChatRecord{}).Where("app_id = ?", h.appA.ID).Count(&n)
		return n == 3
	}, 5*time.Second, 50*time.Millisecond)
	for _, c := range h.vendor.recorded() {
		assert.Equal(t, "/v1/messages", c.Path)
	}
}

// On the creator node a retrieve refreshes the ledger row with the vendor's
// latest view, IDs wrapped, so list can serve it.
func TestNative_RetrieveUpdatesLedgerSnapshot(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	store := h.service.NativeResourceStore()
	row := &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID,
		ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: "msgbatch_A1",
		State: "in_progress", AccountingState: models.NativeAccountingPending,
	}
	require.NoError(t, store.Create(context.Background(), row))

	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	got, err := store.GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_A1")
	require.NoError(t, err)
	assert.Equal(t, "ended", got.State)
	require.NotNil(t, got.EndedAt)
	require.NotNil(t, got.ResultsExpireAt)
	assert.Equal(t, time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC), got.ResultsExpireAt.UTC())
	assert.JSONEq(t, string(body), got.Snapshot)
	assert.Equal(t, models.NativeAccountingPending, got.AccountingState, "a retrieve never settles accounting")
}

// A rotated vendor key breaks the MAC of IDs minted before; the creator
// node's ledger row for the same App still authorises them. Without the row
// (another node, or another App) they are not found.
func TestNative_KeyRotationFallbackToLedger(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	oldID := wrapBatchID(t, "the-old-vendor-key", h.llm.ID, h.appA.ID, "msgbatch_A1")

	resp, _ := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+oldID, "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "no ledger row: the old ID does not verify")

	require.NoError(t, h.service.NativeResourceStore().Create(context.Background(), &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: h.llm.ID, AppID: h.appA.ID,
		ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: "msgbatch_A1",
		AccountingState: models.NativeAccountingPending,
	}))
	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+oldID, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, h.wrap(h.llm, h.appA, "msgbatch_A1"), decodeBatch(t, body)["id"], "the answer carries an ID under the current key")

	// App B's old-key ID for the same batch has no row of its own.
	oldB := wrapBatchID(t, "the-old-vendor-key", h.llm.ID, h.appB.ID, "msgbatch_A1")
	resp, _ = h.do(h.keyB, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+oldB, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// The edge mounts the proxy under gin's Any("/llm/*path"); native GETs reach
// it there as on Studio.
func TestNative_EdgeGinMount(t *testing.T) {
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_A1", "ended")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/llm/*path", gin.WrapH(h.proxy.createHandler()))
	srv := httptest.NewServer(engine)
	defer srv.Close()

	id := h.wrap(h.llm, h.appA, "msgbatch_A1")
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/llm/call/claude/v1/messages/batches/"+id, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.keyA)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var obj map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&obj))
	assert.Equal(t, id, obj["id"])
}

func TestRewriteBodyRef(t *testing.T) {
	obj, err := decodeNativeObject([]byte(`{"id":"a","n":1,"data":[{"id":"b"},{"id":"c"},{"other":1}],"ids":["d","e"],"nested":{"x":{"id":"f"}},"null":null}`))
	require.NoError(t, err)
	up := func(s string) (string, error) { return strings.ToUpper(s), nil }
	for _, p := range []string{"id", "data[].id", "ids[]", "nested.x.id", "missing.id", "null"} {
		require.NoError(t, rewriteBodyRef(obj, p, up), p)
	}
	out, err := json.Marshal(obj)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"A","n":1,"data":[{"id":"B"},{"id":"C"},{"other":1}],"ids":["D","E"],"nested":{"x":{"id":"F"}},"null":null}`, string(out))

	assert.Error(t, rewriteBodyRef(obj, "n", up), "a non-string ID is an error")
	assert.Error(t, rewriteBodyRef(obj, "id[]", up), "a non-array is an error")

	_, err = decodeNativeObject([]byte(`{"a":1} {"b":2}`))
	assert.Error(t, err)
	_, err = decodeNativeObject([]byte(`null`))
	assert.Error(t, err)
}

func TestSplitNativePath(t *testing.T) {
	for path, want := range map[string][3]string{
		"/llm/rest/claude/v1/messages/batches": {"rest", "claude", "/v1/messages/batches"},
		"/llm/call/c/x":                        {"call", "c", "/x"},
		"/llm/stream/c/x/":                     {"stream", "c", "/x/"},
	} {
		mode, slug, rest, ok := splitNativePath(path)
		require.True(t, ok, path)
		assert.Equal(t, want, [3]string{mode, slug, rest}, path)
	}
	for _, path := range []string{"/llm/rest/claude", "/llm/rest/claude/", "/llm/other/c/x", "/ai/c/v1/messages", "/llm/rest//x"} {
		_, _, _, ok := splitNativePath(path)
		assert.False(t, ok, path)
	}
}
