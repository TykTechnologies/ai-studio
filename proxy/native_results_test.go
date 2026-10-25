package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

func succeededLine(customID, model string, in, out int) string {
	return `{"custom_id":"` + customID + `","result":{"type":"succeeded","message":{"id":"msg_` + customID +
		`","type":"message","role":"assistant","model":"` + model + `","content":[{"type":"text","text":"answer ` + customID +
		`"}],"stop_reason":"end_turn","usage":{"input_tokens":` + strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) +
		`,"service_tier":"batch"}}}}`
}

const (
	erroredLine  = `{"custom_id":"err","result":{"type":"errored","error":{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}}}`
	canceledLine = `{"custom_id":"can","result":{"type":"canceled"}}`
	expiredLine  = `{"custom_id":"exp","result":{"type":"expired"}}`
)

func resultsPath(id string) string { return "/llm/rest/claude/v1/messages/batches/" + id + "/results" }

// spendCounter counts the spend the gateway records.
type spendCounter struct {
	budget.Service
	mu    sync.Mutex
	spend float64
	calls int
}

func (s *spendCounter) RecordSpend(app *models.App, llm *models.LLM, cost float64) {
	s.mu.Lock()
	s.spend += cost
	s.calls++
	s.mu.Unlock()
	if sr, ok := s.Service.(budget.SpendRecorder); ok {
		sr.RecordSpend(app, llm, cost)
	}
}

func (s *spendCounter) totals() (float64, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spend, s.calls
}

// The results reach the client as the vendor sent them, and on the creator
// node a complete download records the batch's usage once: one chat record
// per model at half the list price, billed lines only, identity from the
// ledger row. A second download records nothing more.
func TestNativeResults_AccountedOnceAtBatchPrice(t *testing.T) {
	t.Parallel()
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend }})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.addPrice("claude-sol", 0.00002, 0.000004)
	h.vendor.addBatch("msgbatch_R1", "ended")
	lines := []string{
		succeededLine("a", "claude-luna", 1000, 200),
		erroredLine,
		succeededLine("b", "claude-luna", 500, 100),
		canceledLine,
		succeededLine("c", "claude-sol", 100, 10),
		expiredLine,
	}
	h.vendor.setResults("msgbatch_R1", lines...)
	row := h.seedBatchRow(h.llm, h.appA, "msgbatch_R1")
	id := h.wrap(h.llm, h.appA, "msgbatch_R1")

	for i := 0; i < 2; i++ {
		resp, body := h.do(h.keyA, http.MethodGet, resultsPath(id), "")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Empty(t, resp.Header.Get("Content-Length"))
		assert.Equal(t, strings.Join(lines, "\n")+"\n", string(body), "lines pass through unchanged")
	}

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = h.chatRecords(h.appA)
		return len(recs) == 2
	}, 5*time.Second, 50*time.Millisecond)
	recs = h.chatRecords(h.appA)
	require.Len(t, recs, 2, "exactly one record per model across two downloads")

	byModel := map[string]models.LLMChatRecord{}
	for _, r := range recs {
		byModel[r.Name] = r
	}
	luna, sol := byModel["claude-luna"], byModel["claude-sol"]
	assert.Equal(t, 1500, luna.PromptTokens)
	assert.Equal(t, 300, luna.ResponseTokens)
	// Half of sync pricing: (300×0.00001 + 1500×0.000002) × 0.5, ×10000.
	assert.InDelta(t, (300*0.00001+1500*0.000002)*0.5*10000, luna.Cost, 1e-9)
	assert.InDelta(t, (10*0.00002+100*0.000004)*0.5*10000, sol.Cost, 1e-9)
	assert.Equal(t, "user:alice", luna.OnBehalfOf)
	assert.Equal(t, "agent:bot", luna.ActingAgent)

	total, calls := spend.totals()
	assert.Equal(t, 2, calls, "spend recorded once per model, once per batch")
	assert.InDelta(t, (luna.Cost+sol.Cost)/10000, total, 1e-12)

	got, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_R1")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingAccounted, got.AccountingState)
	assert.Equal(t, row.ID, got.ID)
}

// The same usage through the synchronous chain costs twice as much: the
// batch discount is the multiplier, not a different price.
func TestNativeResults_CostIsHalfOfSyncForSameUsage(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	resp, body := h.do(h.keyB, http.MethodPost, "/llm/rest/claude/v1/messages",
		`{"model":"claude-luna","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	h.vendor.addBatch("msgbatch_R2", "ended")
	h.vendor.setResults("msgbatch_R2", succeededLine("a", "claude-luna", 5, 3)) // the fake's sync usage
	h.seedBatchRow(h.llm, h.appA, "msgbatch_R2")
	resp, body = h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_R2")), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var syncRecs, batchRecs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		syncRecs, batchRecs = h.chatRecords(h.appB), h.chatRecords(h.appA)
		return len(syncRecs) == 1 && len(batchRecs) == 1
	}, 5*time.Second, 50*time.Millisecond)
	require.Greater(t, syncRecs[0].Cost, 0.0)
	assert.InDelta(t, syncRecs[0].Cost/2, batchRecs[0].Cost, 1e-9)
}

// First result bytes reach the client while the vendor is still sending.
func TestNativeResults_StreamsBeforeUpstreamFinishes(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_S1", "ended")
	h.vendor.setResults("msgbatch_S1", succeededLine("a", "claude-luna", 1, 1), succeededLine("b", "claude-luna", 1, 1))
	gate := make(chan struct{})
	h.vendor.mu.Lock()
	h.vendor.resultsGate = gate
	h.vendor.mu.Unlock()
	defer func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+resultsPath(h.wrap(h.llm, h.appA, "msgbatch_S1")), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.keyA)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	br := bufio.NewReader(resp.Body)
	first := make(chan string, 1)
	go func() {
		l, _ := br.ReadString('\n')
		first <- l
	}()
	select {
	case l := <-first:
		assert.Contains(t, l, `"custom_id":"a"`)
	case <-time.After(5 * time.Second):
		t.Fatal("the first line did not arrive while the vendor was still streaming")
	}
	close(gate)
	rest, err := io.ReadAll(br)
	require.NoError(t, err)
	assert.Contains(t, string(rest), `"custom_id":"b"`)
}

// A large results file streams through without being held in memory: the
// vendor never gets more than a few MiB ahead of what the client has
// received. A gateway that buffered the body (or any amount proportional to
// it) would leave the vendor waiting for a client that is waiting for the
// gateway, and the download would stall. This is measured on the bytes in
// flight, not on the process heap, which other tests' garbage moves.
func TestNativeResults_LargeFileMemoryBounded(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("large stream")
	}
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_BIG", "ended")
	h.seedBatchRow(h.llm, h.appA, "msgbatch_BIG")

	const (
		lineSize = 4096
		n        = 16000 // ~64 MiB
		// maxAhead is how far the vendor may run ahead of the client: the
		// gateway's line buffers plus the kernel's socket buffers on two hops.
		maxAhead = 12 << 20
	)
	pad := strings.Repeat("x", lineSize-200)
	var received atomic.Int64
	stalled := make(chan struct{})
	h.vendor.mu.Lock()
	h.vendor.resultsStream = func(w io.Writer) {
		written := int64(0)
		for i := 0; i < n; i++ {
			for deadline := time.Now().Add(10 * time.Second); written-received.Load() > maxAhead; {
				if time.Now().After(deadline) {
					close(stalled)
					return
				}
				time.Sleep(time.Millisecond)
			}
			k, _ := fmt.Fprintf(w, `{"custom_id":"r%d","result":{"type":"succeeded","message":{"model":"claude-luna","content":[{"type":"text","text":"%s"}],"usage":{"input_tokens":1,"output_tokens":1}}}}`+"\n", i, pad)
			written += int64(k)
			if f, ok := w.(http.Flusher); ok && i%16 == 0 {
				f.Flush()
			}
		}
	}
	h.vendor.mu.Unlock()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+resultsPath(h.wrap(h.llm, h.appA, "msgbatch_BIG")), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.keyA)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	lines := 0
	br := bufio.NewReaderSize(resp.Body, 64<<10)
	for {
		chunk, err := br.ReadSlice('\n')
		received.Add(int64(len(chunk)))
		if err == io.EOF {
			break
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		require.NoError(t, err)
		lines++
	}
	select {
	case <-stalled:
		t.Fatalf("the vendor stalled %d MiB ahead of the client: the gateway is buffering the results", maxAhead>>20)
	default:
	}
	assert.Equal(t, n, lines)

	require.Eventually(t, func() bool {
		recs := h.chatRecords(h.appA)
		return len(recs) == 1 && recs[0].PromptTokens == n
	}, 5*time.Second, 50*time.Millisecond)
}

// A gzip-encoded upstream (the transport asks for it) is decoded and parsed.
func TestNativeResults_GzipUpstream(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_GZ", "ended")
	h.vendor.setResults("msgbatch_GZ", succeededLine("a", "claude-luna", 10, 5), succeededLine("b", "claude-luna", 10, 5))
	h.seedBatchRow(h.llm, h.appA, "msgbatch_GZ")
	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_GZ")), "", "Accept-Encoding", "identity")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, resp.Header.Get("Content-Encoding"))
	assert.Contains(t, string(body), `"custom_id":"b"`)
	calls := h.vendor.recorded()
	assert.Contains(t, calls[len(calls)-1].Headers.Get("Accept-Encoding"), "gzip", "the vendor was asked for gzip")
	require.Eventually(t, func() bool {
		recs := h.chatRecords(h.appA)
		return len(recs) == 1 && recs[0].PromptTokens == 20
	}, 5*time.Second, 50*time.Millisecond)
}

// A client that goes away before the end records nothing and leaves the row
// claimable for the next download or the reconciler.
func TestNativeResults_InterruptedDownloadRecordsNothing(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_INT", "ended")
	h.vendor.setResults("msgbatch_INT", succeededLine("a", "claude-luna", 1, 1), succeededLine("b", "claude-luna", 1, 1))
	gate := make(chan struct{})
	h.vendor.mu.Lock()
	h.vendor.resultsGate = gate
	h.vendor.mu.Unlock()
	h.seedBatchRow(h.llm, h.appA, "msgbatch_INT")

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(h.port)+resultsPath(h.wrap(h.llm, h.appA, "msgbatch_INT")), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+h.keyA)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	l, err := bufio.NewReader(resp.Body).ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, l, `"custom_id":"a"`)
	cancel()
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)
	close(gate)

	assert.Empty(t, h.chatRecords(h.appA))
	got, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_INT")
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingPending, got.AccountingState)

	// The next complete download accounts it.
	h.vendor.mu.Lock()
	h.vendor.resultsGate = nil
	h.vendor.mu.Unlock()
	resp2, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_INT")), "")
	require.Equal(t, http.StatusOK, resp2.StatusCode, string(body))
	require.Eventually(t, func() bool { return len(h.chatRecords(h.appA)) == 1 }, 5*time.Second, 50*time.Millisecond)
}

// A node without the ledger row (another edge, another replica's ledger)
// passes the results through and records only an audit proxy log.
func TestNativeResults_NonCreatorNodePassesThrough(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.addPrice("claude-luna", 0.00001, 0.000002)
	h.vendor.addBatch("msgbatch_NC", "ended")
	h.vendor.setResults("msgbatch_NC", succeededLine("a", "claude-luna", 1, 1))
	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_NC")), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, h.chatRecords(h.appA))
	logs := h.proxyLogs(h.appA, 1)
	require.Len(t, logs, 1)
	assert.Equal(t, http.StatusOK, logs[0].ResponseCode)
	assert.Contains(t, logs[0].ResponseBody, `"results_lines":1`)
}

// Results of a batch still in progress are the vendor's error, passed on.
func TestNativeResults_NotEndedIsVendorError(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_RUN", "in_progress")
	resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_RUN")), "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	typ, _ := nativeErr(t, body)
	assert.Equal(t, "not_found_error", typ)
}

// Results of another App's batch are not found, like any foreign ID.
func TestNativeResults_ForeignIDNotFound(t *testing.T) {
	t.Parallel()
	h := newNativeHarness(t, nativeHarnessOpts{})
	h.vendor.addBatch("msgbatch_R1", "ended")
	h.vendor.setResults("msgbatch_R1", succeededLine("a", "claude-luna", 1, 1))
	resp, _ := h.do(h.keyB, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_R1")), "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_R1/results"))
}

// bodyHook is a response hook that may rewrite bodies.
type bodyHook struct{}

func (bodyHook) OnBeforeWriteHeaders(context.Context, *HeadersRequest) (*HeadersResponse, error) {
	return &HeadersResponse{}, nil
}
func (bodyHook) OnBeforeWrite(_ context.Context, r *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	return &ResponseWriteResponse{Body: r.Body, Headers: r.Headers}, nil
}
func (bodyHook) OnStreamComplete(context.Context, *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{}, nil
}
func (bodyHook) GetName() string { return "body-hook" }

type scopedBodyHook struct {
	bodyHook
	applies bool
}

func (h scopedBodyHook) HasHooksForLLM(uint) bool { return h.applies }

// An LLM with response plugins refuses results downloads, which those
// plugins could not see; a plugin scoped to other LLMs does not.
func TestNativeResults_ResponsePluginsRefuseDownload(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		hook ResponseHook
		want int
	}{
		"unscoped hook":           {bodyHook{}, http.StatusNotImplemented},
		"hook on this llm":        {scopedBodyHook{applies: true}, http.StatusNotImplemented},
		"hook on other llms only": {scopedBodyHook{applies: false}, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			h := newNativeHarness(t, nativeHarnessOpts{})
			h.proxy.AddResponseHook(tc.hook)
			h.vendor.addBatch("msgbatch_R1", "ended")
			h.vendor.setResults("msgbatch_R1", succeededLine("a", "claude-luna", 1, 1))
			resp, body := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_R1")), "")
			require.Equal(t, tc.want, resp.StatusCode, string(body))
			if tc.want != http.StatusOK {
				_, msg := nativeErr(t, body)
				assert.Contains(t, msg, "response plugins")
				assert.Empty(t, h.vendor.callsTo(http.MethodGet, "/v1/messages/batches/msgbatch_R1/results"))
			}
		})
	}
}

func TestNativeUsageAggregation(t *testing.T) {
	u := newNativeUsage()
	rt, _ := (&anthropicVendor.Anthropic{}).NativeAPI().Resource(anthropicVendor.ResourceMessageBatch)
	for _, l := range []string{succeededLine("a", "m1", 10, 1), succeededLine("b", "m1", 5, 2), erroredLine, succeededLine("c", "m2", 1, 1), canceledLine} {
		x, ok, err := rt.Async.LineUsage([]byte(l))
		require.NoError(t, err)
		if ok {
			u.add(x)
		}
	}
	require.Len(t, u.order, 2)
	m1 := u.byKey[u.order[0]]
	assert.Equal(t, 15, m1.usage.InputTokens)
	assert.Equal(t, 3, m1.usage.OutputTokens)
	assert.Equal(t, 2, m1.requests)
	assert.Equal(t, 0.5, m1.usage.PriceMultiplier())
}
