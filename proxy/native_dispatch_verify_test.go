package proxy

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adversarial cases added during independent verification of the native
// dispatcher (ut0.14).

// A ".." anywhere in the path, plain or percent-encoded, is never claimed as a
// native operation and never reaches the vendor's batch API (the router may
// redirect to the cleaned path; that request is checked on its own merits).
func TestNativeVerify_TraversalNeverReachesVendor(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{endpointPath: "/corp/anthropic/v1"})
	h.vendor.addBatch("msgbatch_T1", "ended")
	id := h.wrap(h.llm, h.appA, "msgbatch_T1")

	for _, p := range []string{
		"/llm/rest/claude/v1/messages/batches/../batches/" + id,
		"/llm/rest/claude/v1/messages/batches/%2e%2e/batches/" + id,
		"/llm/rest/claude/../../v1/messages/batches/" + id,
		"/llm/rest/claude/v1/../../../v1/messages/batches/" + id,
	} {
		t.Run(p, func(t *testing.T) {
			before := len(h.vendor.recorded())
			// Do not follow redirects: the router answers an unclean path with a
			// 301 to its cleaned form, which is a different (legitimate) request.
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+p, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+h.keyA)
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			assert.NotEqual(t, http.StatusOK, resp.StatusCode)
			for _, c := range h.vendor.recorded()[before:] {
				assert.NotContains(t, c.Path, "/messages/batches", "traversal reached the vendor batch API: %s", c.Path)
			}
		})
	}
}

// Every ID that does not verify for the caller gets the same answer, so the
// check is no oracle for which batches exist or whose they are.
func TestNativeVerify_NotFoundIsUniform(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_U1", "in_progress")
	good := h.wrap(h.llm, h.appA, "msgbatch_U1")
	tampered := good[:len(good)-1] + map[bool]string{true: "0", false: "1"}[strings.HasSuffix(good, "1")]

	var first []byte
	for name, id := range map[string]string{
		"other app's id":    good, // requested as App B
		"tampered mac":      tampered,
		"raw vendor id":     "msgbatch_U1",
		"garbage":           "nope",
		"wrong llm's id":    h.wrap(h.llm2, h.appB, "msgbatch_U1"),
		"charset violation": "msgbatch_bad!id.t1k00000000.00000000000000000000000000000000",
	} {
		resp, body := h.do(h.keyB, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", name, body)
		if first == nil {
			first = body
		}
		assert.Equal(t, string(first), string(body), "%s: the 404 body must not differ", name)
	}
	for _, c := range h.vendor.recorded() {
		assert.NotContains(t, c.Path, "msgbatch_U1", "an unverified id reached the vendor")
	}
}

// A vendor answer whose id is not a valid batch id is never handed to the
// client raw: the call fails instead.
func TestNativeVerify_UnwrappableVendorIDIs502(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_V1", "ended")
	h.vendor.mu.Lock()
	h.vendor.batches["msgbatch_V1"]["id"] = "msgbatch_../../evil"
	h.vendor.mu.Unlock()

	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+h.wrap(h.llm, h.appA, "msgbatch_V1"), "")
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "body: %s", body)
	assert.NotContains(t, string(body), "evil")
	assert.NotContains(t, string(body), "api.anthropic.com")
}

// An oversized vendor object is refused, not buffered without bound or passed
// through.
func TestNativeVerify_OversizedVendorObjectIs502(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_W1", "ended")
	h.vendor.mu.Lock()
	h.vendor.batches["msgbatch_W1"]["padding"] = strings.Repeat("x", nativeMaxObjectBody+1)
	h.vendor.mu.Unlock()

	resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+h.wrap(h.llm, h.appA, "msgbatch_W1"), "")
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.Less(t, len(body), 4096)
}

// A public URL that is not http(s) or has no host is ignored: results_url
// stays root-relative and never takes a hostile scheme.
func TestNativeVerify_PublicURLMustBeHTTP(t *testing.T) {
	t.Parallel()
	for _, pub := range []string{"javascript:alert(1)", "ftp://gw.example.com", "//gw.example.com", "gw.example.com"} {
		t.Run(pub, func(t *testing.T) {
			h := newNativeHarness(t, nativeHarnessOpts{config: &Config{NativePublicURL: pub}})
			h.vendor.addBatch("msgbatch_P1", "ended")
			id := h.wrap(h.llm, h.appA, "msgbatch_P1")
			resp, body := h.do(h.keyA, http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id, "")
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
			assert.Equal(t, "/v1/messages/batches/"+id+"/results", decodeBatch(t, body)["results_url"])
		})
	}
}

// Concurrent retrieves and cancels of one batch by its owner are race-free
// (run with -race) and each one answers with the wrapped id.
func TestNativeVerify_ConcurrentRetrieveAndCancel(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_C1", "in_progress")
	id := h.wrap(h.llm, h.appA, "msgbatch_C1")

	var wg sync.WaitGroup
	errs := make(chan string, 40)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method, p := http.MethodGet, "/llm/rest/claude/v1/messages/batches/"+id
			if i%4 == 0 {
				method, p = http.MethodPost, p+"/cancel"
			}
			resp, body := h.do(h.keyA, method, p, "")
			if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"id":"`+id+`"`) {
				errs <- fmt.Sprintf("%s %s: %d %s", method, p, resp.StatusCode, body)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
