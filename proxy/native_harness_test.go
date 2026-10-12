package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
)

// --- native API harness ------------------------------------------------------

// callsTo returns the recorded calls with method and path.
func (f *fakeBatchAPI) callsTo(method, path string) []nativeVendorCall {
	var out []nativeVendorCall
	for _, c := range f.recorded() {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// nativeVendorCall is one request the fake vendor received.
type nativeVendorCall struct {
	Method, Path, RawQuery string
	Headers                http.Header
	Body                   []byte
}

// fakeBatchAPI is an in-memory Anthropic Messages + Message Batches API.
type fakeBatchAPI struct {
	mu      sync.Mutex
	calls   []nativeVendorCall
	batches map[string]map[string]any
	server  *httptest.Server
	// results are the JSONL lines served for a batch's results.
	results map[string][]string
	// resultsGate, when set, holds the results after their first line until
	// it is closed.
	resultsGate chan struct{}
	// resultsStream, when set, writes a batch's results instead of results.
	resultsStream func(w io.Writer)
	// resultsStatus, when set, is the status every results download gets
	// (with an error body) instead of the results.
	resultsStatus int
}

// setResults sets the results lines of a batch.
func (f *fakeBatchAPI) setResults(id string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[id] = lines
}

func (f *fakeBatchAPI) serveResults(w http.ResponseWriter, r *http.Request, id string) {
	f.mu.Lock()
	lines, gate, stream, status := f.results[id], f.resultsGate, f.resultsStream, f.resultsStatus
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write(anthropicVendor.NativeError(status, "api_error", "results unavailable"))
		return
	}
	w.Header().Set("Content-Type", "application/binary")
	var out io.Writer = w
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		out = gz
	}
	flush := func() {
		if gz, ok := out.(*gzip.Writer); ok {
			_ = gz.Flush()
		}
		w.(http.Flusher).Flush()
	}
	if stream != nil {
		stream(out)
		return
	}
	for i, l := range lines {
		_, _ = io.WriteString(out, l+"\n")
		flush()
		if i == 0 && gate != nil {
			<-gate
		}
	}
}

func newFakeBatchAPI(t *testing.T) *fakeBatchAPI {
	f := &fakeBatchAPI{batches: map[string]map[string]any{}, results: map[string][]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

// addBatch seeds a batch; ended batches carry Anthropic's absolute results_url.
func (f *fakeBatchAPI) addBatch(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addBatchLocked(id, status)
}

func (f *fakeBatchAPI) addBatchLocked(id, status string) {
	b := map[string]any{
		"id": id, "type": "message_batch", "processing_status": status,
		"request_counts": map[string]any{"processing": 0, "succeeded": 2, "errored": 0, "canceled": 0, "expired": 0},
		"created_at":     "2026-10-12T10:00:00Z", "expires_at": "2026-10-13T10:00:00Z",
		"ended_at": nil, "cancel_initiated_at": nil, "archived_at": nil, "results_url": nil,
	}
	if status == "ended" {
		b["ended_at"] = "2026-10-12T11:00:00Z"
		b["results_url"] = "https://api.anthropic.com/v1/messages/batches/" + id + "/results"
	}
	f.batches[id] = b
}

func (f *fakeBatchAPI) recorded() []nativeVendorCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]nativeVendorCall(nil), f.calls...)
}

func (f *fakeBatchAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, nativeVendorCall{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Headers: r.Header.Clone(), Body: body})
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Request-Id", "req_vendor_1")
	notFound := func() {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(anthropicVendor.NativeError(http.StatusNotFound, "not_found_error", "not found"))
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, ev := range []string{
				`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-luna","content":[],"usage":{"input_tokens":5,"output_tokens":1}}}`,
				`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
				`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
			} {
				_, _ = w.Write([]byte(ev + "\n\n"))
				w.(http.Flusher).Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-luna",` +
			`"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages/batches":
		f.mu.Lock()
		id := "msgbatch_N" + strconv.Itoa(len(f.batches)+1)
		f.addBatchLocked(id, "in_progress")
		out, _ := json.Marshal(f.batches[id])
		f.mu.Unlock()
		_, _ = w.Write(out)
	case strings.HasPrefix(r.URL.Path, "/v1/messages/batches/"):
		rest := strings.TrimPrefix(r.URL.Path, "/v1/messages/batches/")
		id, action, _ := strings.Cut(rest, "/")
		if action == "results" && r.Method == http.MethodGet {
			f.mu.Lock()
			b, ok := f.batches[id]
			ended := ok && b["processing_status"] == "ended"
			f.mu.Unlock()
			if !ended {
				notFound()
				return
			}
			f.serveResults(w, r, id)
			return
		}
		if action == "" && r.Method == http.MethodDelete {
			f.mu.Lock()
			b, ok := f.batches[id]
			ended := ok && b["processing_status"] == "ended"
			if ended {
				delete(f.batches, id)
			}
			f.mu.Unlock()
			switch {
			case !ok:
				notFound()
			case !ended:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(anthropicVendor.NativeError(http.StatusBadRequest, "invalid_request_error", "batch has not ended"))
			default:
				_, _ = w.Write([]byte(`{"id":"` + id + `","type":"message_batch_deleted"}`))
			}
			return
		}
		f.mu.Lock()
		b, ok := f.batches[id]
		if ok && r.Method == http.MethodPost && action == "cancel" && b["processing_status"] == "in_progress" {
			b["processing_status"] = "canceling"
			b["cancel_initiated_at"] = "2026-10-12T10:30:00Z"
		}
		var out []byte
		if ok {
			out, _ = json.Marshal(b)
		}
		f.mu.Unlock()
		if !ok || (action != "" && action != "cancel") || (action == "cancel" && r.Method != http.MethodPost) ||
			(action == "" && r.Method != http.MethodGet) {
			notFound()
			return
		}
		_, _ = w.Write(out)
	default:
		notFound()
	}
}

// nativeHarness is a live proxy with an Anthropic LLM ("claude") backed by
// fakeBatchAPI, a second Anthropic LLM ("claude-two") on the same vendor key,
// an OpenAI "primary" whose failover waterfall lists "claude", and three
// Apps: A holds claude and claude-two, B holds claude, C holds only primary.
type nativeHarness struct {
	t                *testing.T
	db               *gorm.DB
	port             int
	proxy            *Proxy
	service          *services.Service
	vendor           *fakeBatchAPI
	llm, llm2        *models.LLM
	primary          *models.LLM
	appA, appB, appC *models.App
	keyA, keyB, keyC string
}

type nativeHarnessOpts struct {
	config      *Config
	vendorKey   string
	wrapService func(services.ServiceInterface) services.ServiceInterface
	// endpointPath is appended to the fake vendor's URL as the LLM endpoint.
	endpointPath string
	// mutateLLM adjusts the "claude" LLM before it is stored.
	mutateLLM func(*models.LLM)
	// wrapBudget wraps the budget service the gateway records spend on.
	wrapBudget func(budget.Service) budget.Service
}

func newNativeHarness(t *testing.T, opts nativeHarnessOpts) *nativeHarness {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })
	if opts.vendorKey == "" {
		opts.vendorKey = "anthropic-vendor-key"
	}
	if opts.vendorKey == "-" {
		opts.vendorKey = ""
	}

	h := &nativeHarness{t: t, db: db, vendor: newFakeBatchAPI(t)}
	h.service = services.NewService(db)
	var budgetSvc budget.Service = budget.NewService(db, services.NewTestNotificationService(db))
	if opts.wrapBudget != nil {
		budgetSvc = opts.wrapBudget(budgetSvc)
	}
	h.service.Budget = budgetSvc

	user, err := h.service.CreateUser(services.UserDTO{Email: "native@example.com", Name: "native", Password: "password123"})
	require.NoError(t, err)

	mkLLM := func(name string) *models.LLM {
		l := &models.LLM{
			Name: name, Vendor: models.ANTHROPIC, Active: true,
			APIEndpoint: h.vendor.server.URL + opts.endpointPath, APIKey: opts.vendorKey,
			DefaultModel: "claude-luna",
		}
		require.NoError(t, db.Create(l).Error)
		return l
	}
	h.llm = mkLLM("Claude")
	if opts.mutateLLM != nil {
		opts.mutateLLM(h.llm)
		require.NoError(t, db.Save(h.llm).Error)
	}
	h.llm2 = mkLLM("Claude-Two")
	h.primary = &models.LLM{
		Name: "Primary", Vendor: models.OPENAI, Active: true,
		APIEndpoint: h.vendor.server.URL, APIKey: "openai-key", DefaultModel: "gpt-4o",
		Failover: models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: h.llm.ID, Model: "claude-luna"}}},
	}
	require.NoError(t, db.Create(h.primary).Error)

	mkApp := func(name string, llmIDs ...uint) (*models.App, string) {
		app, err := h.service.CreateApp(name, "", user.ID, nil, llmIDs, nil, nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.service.ActivateAppCredential(app.ID))
		app, err = h.service.GetAppByID(app.ID)
		require.NoError(t, err)
		return app, app.Credential.Secret
	}
	h.appA, h.keyA = mkApp("Native A", h.llm.ID, h.llm2.ID)
	h.appB, h.keyB = mkApp("Native B", h.llm.ID)
	h.appC, h.keyC = mkApp("Native C", h.primary.ID)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h.port = ln.Addr().(*net.TCPAddr).Port

	cfg := opts.config
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.Port = h.port
	var svc services.ServiceInterface = h.service
	if opts.wrapService != nil {
		svc = opts.wrapService(svc)
	}
	p := New(svc, budgetSvc, cfg)
	require.NoError(t, p.loadResources())
	h.proxy = p
	srv := &http.Server{Handler: p.createHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		p.waitForAnalyzers()
	})
	return h
}

// do sends a request as the App holding key.
func (h *nativeHarness) do(key, method, path, body string, headers ...string) (*http.Response, []byte) {
	h.t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, rdr)
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)
	return resp, out
}

// wrap mints the ID an App would have been handed for a batch on llm.
func (h *nativeHarness) wrap(llm *models.LLM, app *models.App, vendorID string) string {
	h.t.Helper()
	return wrapBatchID(h.t, llm.APIKey, llm.ID, app.ID, vendorID)
}

func wrapBatchID(t *testing.T, key string, llmID, appID uint, vendorID string) string {
	t.Helper()
	codec, err := nativeapi.NewCodec(key)
	require.NoError(t, err)
	rt, ok := (&anthropicVendor.Anthropic{}).NativeAPI().Resource(anthropicVendor.ResourceMessageBatch)
	require.True(t, ok)
	id, err := codec.Wrap(rt, vendorID, nativeapi.Scope{LLMID: llmID, AppID: appID})
	require.NoError(t, err)
	return id
}

func (h *nativeHarness) proxyLogs(app *models.App) []models.ProxyLog {
	h.t.Helper()
	h.proxy.waitForAnalyzers()
	var logs []models.ProxyLog
	require.Eventually(h.t, func() bool {
		return h.db.Where("app_id = ?", app.ID).Order("id asc").Find(&logs).Error == nil
	}, 2*time.Second, 20*time.Millisecond)
	return logs
}

// nativeErr decodes an Anthropic-shaped error body.
func nativeErr(t *testing.T, body []byte) (typ, msg string) {
	t.Helper()
	var e struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &e), "body: %s", body)
	require.Equal(t, "error", e.Type, "body: %s", body)
	return e.Error.Type, e.Error.Message
}

// seedBatchRow records msgbatch vendorID as created by app on llm on this
// node, pending accounting.
func (h *nativeHarness) seedBatchRow(llm *models.LLM, app *models.App, vendorID string) *models.NativeResource {
	h.t.Helper()
	row := &models.NativeResource{
		Vendor: string(models.ANTHROPIC), LLMID: llm.ID, AppID: app.ID, UserID: app.UserID,
		ResourceType: anthropicVendor.ResourceMessageBatch, VendorResourceID: vendorID,
		State: "ended", AccountingState: models.NativeAccountingPending,
		OnBehalfOf: "user:alice", ActingAgent: "agent:bot",
	}
	require.NoError(h.t, h.service.NativeResourceStore().Create(context.Background(), row))
	return row
}

func (h *nativeHarness) addPrice(model string, cpt, cpit float64) {
	h.t.Helper()
	require.NoError(h.t, h.db.Create(&models.ModelPrice{ModelName: model, Vendor: string(models.ANTHROPIC), CPT: cpt, CPIT: cpit, Currency: "USD"}).Error)
}

func (h *nativeHarness) chatRecords(app *models.App) []models.LLMChatRecord {
	h.t.Helper()
	h.proxy.waitForAnalyzers()
	// The shared-cache SQLite refuses a read that overlaps the analytics
	// writer ("database table is locked") instead of waiting; retry it.
	var (
		recs []models.LLMChatRecord
		err  error
	)
	for attempt := 0; attempt < 100; attempt++ {
		recs = nil
		if err = h.db.Where("app_id = ?", app.ID).Order("id asc").Find(&recs).Error; err == nil {
			return recs
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(h.t, err)
	return recs
}
