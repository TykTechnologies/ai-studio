package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Native creates: an operation with an Admit step (an Anthropic batch
// create). The body is split into items, each admitted as a synchronous call
// would be (model allow-list, request filters), the budget is checked, the
// resource is created upstream and recorded in this node's ledger before its
// wrapped ID is returned.

const (
	nativeDefaultMaxBody       = 64 << 20
	nativeHardMaxBody          = config.NativeMaxBodyBytesLimit
	nativeDefaultCreates       = 4
	nativeDefaultFilteredItems = 10000
	// nativeDefaultGuardrailCalls bounds the remote guardrail provider calls
	// one native create may make across its items.
	nativeDefaultGuardrailCalls = 10000
	// nativeFilterWorkers bounds the items filtered at once per create.
	nativeFilterWorkers = 8
	// nativeLedgerAttempts is how often a ledger insert is tried before the
	// created resource is cancelled upstream.
	nativeLedgerAttempts = 3
)

func (c *Config) nativeMaxBodyBytes() int64 {
	if c == nil || c.NativeMaxBodyBytes <= 0 {
		return nativeDefaultMaxBody
	}
	return min(c.NativeMaxBodyBytes, nativeHardMaxBody)
}

func (c *Config) nativeMaxConcurrentCreates() int {
	if c == nil || c.NativeMaxConcurrentCreates <= 0 {
		return nativeDefaultCreates
	}
	return c.NativeMaxConcurrentCreates
}

func (c *Config) nativeMaxFilteredItems() int {
	if c == nil || c.NativeMaxFilteredItems <= 0 {
		return nativeDefaultFilteredItems
	}
	return c.NativeMaxFilteredItems
}

func (c *Config) nativeMaxGuardrailCalls() int {
	if c == nil || c.NativeMaxGuardrailCalls <= 0 {
		return nativeDefaultGuardrailCalls
	}
	return c.NativeMaxGuardrailCalls
}

func (c *Config) nativeMaxInProgressPerApp() int {
	if c == nil {
		return 0
	}
	return c.NativeMaxInProgressPerApp
}

// acquireCreate takes a native create slot, waiting for one while the client
// waits. Taken before the body is read, so waiting creates hold no body.
func (c *nativeCall) acquireCreate(w http.ResponseWriter) (release func(), ok bool) {
	p := c.p
	p.nativeCreatesOnce.Do(func() {
		p.nativeCreates = make(chan struct{}, p.config.nativeMaxConcurrentCreates())
	})
	select {
	case p.nativeCreates <- struct{}{}:
		return func() { <-p.nativeCreates }, true
	case <-c.r.Context().Done():
		c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "request canceled while waiting to be admitted")
		return nil, false
	}
}

// checkBudget is the synchronous calls' budget pre-check, with their denial.
func (c *nativeCall) checkBudget(w http.ResponseWriter) bool {
	_, _, err := c.p.budgetService.CheckBudget(c.app, c.nr.llm)
	if err == nil {
		return true
	}
	status, msg := budgetDenial(err, "Budget limit exceeded")
	if status == http.StatusForbidden && !errors.Is(err, services.ErrAppInactive) {
		metrics.RecordPolicyBlock(c.r.Context(), "budget", "budget")
	}
	respondBudgetDenial(w, c.r, status, msg, err)
	c.status, c.respBody = status, []byte(msg)
	return false
}

// checkInProgressCap refuses a create while the App already has the
// configured number of the resource type in progress on the LLM.
func (c *nativeCall) checkInProgressCap(w http.ResponseWriter) bool {
	limit := c.p.config.nativeMaxInProgressPerApp()
	rt, ok := c.createdResource()
	if limit <= 0 || !ok || rt.Async == nil {
		return true
	}
	ctx := c.r.Context()
	q := services.NativeResourceListQuery{LLMID: c.nr.llm.ID, AppID: c.app.ID, ResourceType: rt.Name, Limit: services.MaxNativeResourceListLimit}
	count := 0
	for {
		rows, err := c.store.List(ctx, q)
		if err != nil {
			c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "resource ledger unavailable, retry shortly")
			return false
		}
		for _, row := range rows {
			if row.State != string(nativeapi.StateEnded) {
				count++
			}
		}
		if count >= limit {
			c.fail(w, http.StatusTooManyRequests, "rate_limit_error",
				fmt.Sprintf("at most %d batches may be in progress per app; wait for one to end", limit))
			return false
		}
		if len(rows) < q.Limit {
			return true
		}
		q.AfterVendorID = rows[len(rows)-1].VendorResourceID
	}
}

// storeHealthy checks the ledger can be read before anything is created
// upstream, so a create is not paid for and then lost.
func (c *nativeCall) storeHealthy(w http.ResponseWriter) bool {
	rt, _ := c.createdResource()
	if _, err := c.store.List(c.r.Context(), services.NativeResourceListQuery{
		LLMID: c.nr.llm.ID, AppID: c.app.ID, ResourceType: rt.Name, Limit: 1,
	}); err != nil {
		slog.Error("native resource ledger unavailable", "llm", c.nr.llm.Name, "error", err)
		c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "resource ledger unavailable, retry shortly")
		return false
	}
	return true
}

// createdResource is the resource type a create mints: its first response ID.
func (c *nativeCall) createdResource() (nativeapi.ResourceType, bool) {
	if len(c.op.ResponseIDs) == 0 {
		return nativeapi.ResourceType{}, false
	}
	return c.nr.vendor.matcher.Spec().Resource(c.op.ResponseIDs[0].Resource)
}

// admit splits the body into items and admits each: its model must be on the
// LLM's allow-list, and the LLM's request filters run on it with its key
// (custom_id) in the script context. One refused item refuses the whole
// body. Only items a filter rewrote are rewritten in the forwarded body.
func (c *nativeCall) admit(w http.ResponseWriter, body []byte) ([]byte, bool) {
	if c.op.Admit.Multipart && !multipartOpensWithBoundary(c.r.Header.Get("Content-Type"), body) {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest,
			"the body must be multipart/form-data opening with the boundary its Content-Type names")
		return nil, false
	}
	items, err := c.op.Admit.Split(body)
	if err != nil {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, err.Error())
		return nil, false
	}
	mv := NewModelValidator(c.nr.llm.AllowedModels)
	for _, it := range items {
		// An item that invokes no model (conversation items) has none to check.
		if it.Model != "" && !mv.IsModelAllowed(it.Model) {
			msg := fmt.Sprintf("model '%s' is not allowed", it.Model)
			if it.Key != "" {
				msg += fmt.Sprintf(" (custom_id %q)", it.Key)
			}
			c.fail(w, http.StatusForbidden, nativeErrPermission, msg)
			return nil, false
		}
	}

	runners := requestFilterRunners(c.nr.llm)
	if len(runners) == 0 {
		return body, true
	}
	if limit := c.p.config.nativeMaxFilteredItems(); len(items) > limit {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest,
			fmt.Sprintf("this LLM screens every request: a batch may hold at most %d requests here, this one holds %d", limit, len(items)))
		return nil, false
	}

	// One budget of remote guardrail calls for all the items: without it a
	// batch could make the per-request budget in billed vendor calls once
	// per item. Spending it refuses the create (400 naming the limit), never
	// admits part of the batch.
	r := c.r.WithContext(guardrails.WithCallBudget(c.r.Context(), c.p.config.nativeMaxGuardrailCalls()))
	outs := make([][]byte, len(items))
	errs := make([]error, len(items))
	var (
		next    int
		blocked bool
		mu      sync.Mutex
		wg      sync.WaitGroup
	)
	take := func() (int, bool) {
		mu.Lock()
		defer mu.Unlock()
		if blocked || next >= len(items) {
			return 0, false
		}
		i := next
		next++
		return i, true
	}
	workers := min(nativeFilterWorkers, runtime.GOMAXPROCS(0), len(items))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i, ok := take()
				if !ok {
					return
				}
				it := items[i]
				// A single-body operation's item has no key: its filters
				// see the call as a synchronous one would.
				var extra map[string]interface{}
				if it.Key != "" {
					extra = map[string]interface{}{"custom_id": it.Key, "batch_index": int64(i)}
				}
				var out []byte
				var err error
				// These workers are not HTTP handler goroutines: a panic
				// in an item's filters would end the process, so it
				// refuses the item instead.
				if safe.CallRedacted("native item filters", func() {
					out, err = c.p.runRequestFiltersWith(runners, c.nr.llm, r, it.Body, it.Format, it.Model, extra)
				}) {
					out, err = nil, errItemFiltersFailed
				}
				outs[i], errs[i] = out, err
				if err != nil {
					mu.Lock()
					blocked = true
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	// The guardrail call budget is the whole create's, not an item's.
	for _, err := range errs {
		if errors.Is(err, guardrails.ErrCallBudget) {
			metrics.RecordPolicyBlock(c.r.Context(), "request_filter", "firewall")
			c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest,
				fmt.Sprintf("this LLM screens every request with a guardrail: screening this batch needs more than the %d guardrail provider calls one upload may make here (NATIVE_API_MAX_GUARDRAIL_CALLS)", c.p.config.nativeMaxGuardrailCalls()))
			return nil, false
		}
	}
	// Report the first refused item in body order, whichever worker saw it.
	for i, err := range errs {
		if err != nil {
			metrics.RecordPolicyBlock(c.r.Context(), "request_filter", "firewall")
			msg := fmt.Sprintf("request refused: %v", err)
			if items[i].Key != "" {
				msg = fmt.Sprintf("request %q refused: %v", items[i].Key, err)
			}
			c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, msg)
			return nil, false
		}
	}
	// A filter that hands its messages back unchanged still reconstructs the
	// body (key order, spacing); only a different value is a rewrite.
	changed := false
	for i := range items {
		if !bytes.Equal(outs[i], items[i].Body) && !jsonEquivalent(outs[i], items[i].Body) {
			items[i].Body, changed = outs[i], true
		}
	}
	if !changed {
		return body, true
	}
	joined, err := c.op.Admit.Join(body, items)
	if err != nil {
		c.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to rebuild the filtered request")
		return nil, false
	}
	return joined, true
}

// recordCreated writes the ledger row of a resource the vendor just created
// (wrapped is the object as the client gets it). If the row cannot be
// written the resource is cancelled upstream and the client gets a 503: an
// unrecorded batch would never be accounted.
func (c *nativeCall) recordCreated(w http.ResponseWriter, raw, wrapped []byte) bool {
	rt, ok := c.createdResource()
	if !ok {
		return true
	}
	err := c.ledgerCreated(rt, raw, wrapped)
	switch {
	case err == nil:
		return true
	case errors.Is(err, errNativeCreatedUnexpected):
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "unexpected upstream response")
	default:
		c.fail(w, http.StatusServiceUnavailable, nativeErrAPI,
			fmt.Sprintf("the %s could not be recorded by the gateway and was canceled; retry shortly", nativeNoun(rt.Name)))
	}
	return false
}

// errNativeCreatedUnexpected is a created object the gateway cannot read.
var errNativeCreatedUnexpected = errors.New("unexpected created object")

// errItemFiltersFailed refuses an item whose request filters panicked. It
// carries nothing of the panic.
var errItemFiltersFailed = errors.New("Policy error: the request could not be screened")

// nativeScreeningNone is the screening digest of an LLM without request
// filters: its content was not screened.
const nativeScreeningNone = "none"

// nativeScreeningVersion prefixes every screening digest. Bump it whenever
// what a filter screens changes without the filter being edited (the
// built-in pattern library, the segmenter or the scan pipeline in a
// release): files uploaded before then are then refused and uploaded again.
const nativeScreeningVersion = "native-screening-v2"

// requestFilterDigest identifies llm's request filters as they screen
// content: each one's ID, kind, script and config, in the order they run;
// nativeScreeningNone when there are none, "" when a config cannot be read
// (never matches). It hashes content rather than UpdatedAt, whose precision
// differs between a saved object and one read back, so the same filters
// give the same digest across restarts and replicas. A guardrail's
// connection block is reduced to its field names: an edge receives it with
// secrets resolved, and neither a rotated key nor a hash of one belongs in
// the digest, which identifies a policy and is stored unkeyed. It is never
// returned to a client.
func requestFilterDigest(llm *models.LLM) string {
	h := sha256.New()
	h.Write([]byte(nativeScreeningVersion))
	n := 0
	for _, f := range llm.Filters {
		if f.ResponseFilter {
			continue
		}
		n++
		kind := f.Kind
		if kind == "" {
			kind = models.FilterKindScript
		}
		cfg, err := json.Marshal(screeningConfig(f.Config))
		if err != nil {
			return ""
		}
		fmt.Fprintf(h, "|%d|%s|%d:", f.ID, kind, len(f.Script))
		h.Write(f.Script)
		fmt.Fprintf(h, "|%d:", len(cfg))
		h.Write(cfg)
	}
	if n == 0 {
		return nativeScreeningNone
	}
	return hex.EncodeToString(h.Sum(nil))
}

// screeningConfig is cfg with the values of its connection block left out:
// the field names stay (which connection fields a filter sets), the values
// (keys, endpoints, references or their resolved secrets) do not.
func screeningConfig(cfg models.JSONMap) models.JSONMap {
	conn, ok := cfg["connection"]
	if !ok {
		return cfg
	}
	out := maps.Clone(cfg)
	var names []string
	switch c := conn.(type) {
	case map[string]any:
		names = slices.Sorted(maps.Keys(c))
	case models.JSONMap:
		names = slices.Sorted(maps.Keys(c))
	case map[string]string:
		names = slices.Sorted(maps.Keys(c))
	}
	out["connection"] = names
	return out
}

var (
	// errNativeScreenedElsewhere refuses a reference to content screened
	// under other request filters than the LLM's current ones (or not
	// screened by this gateway at all).
	errNativeScreenedElsewhere = errors.New("referenced content screened under other request filters")
	// errNativeScreeningUnchecked is a ledger read that failed.
	errNativeScreeningUnchecked = errors.New("screening of referenced content could not be checked")
)

// nativeScreenedElsewhereMsg names neither the filters nor the content.
const nativeScreenedElsewhereMsg = "the input file was screened under a different request policy than this LLM now applies (or by another gateway); upload it again through this gateway"

// checkScreened verifies that the content of the resource vendorID (type
// rt), which this call runs at the vendor, was screened by the LLM's
// current request filters. An LLM without request filters runs anything
// its App may reference. A row without a digest (written before the gateway
// recorded one, or a resource nothing screened such as a batch output file)
// and a resource this node has no row of count as screened elsewhere.
func (c *nativeCall) checkScreened(rt nativeapi.ResourceType, vendorID string) error {
	current := requestFilterDigest(c.nr.llm)
	switch current {
	case nativeScreeningNone:
		return nil
	case "":
		return errNativeScreenedElsewhere
	}
	row, err := c.store.GetByOwner(c.r.Context(), c.nr.llm.ID, c.app.ID, rt.Name, vendorID)
	switch {
	case errors.Is(err, services.ErrNativeResourceNotFound):
		return errNativeScreenedElsewhere
	case err != nil:
		slog.Error("native resource screening check failed", "llm_id", c.nr.llm.ID, "app_id", c.app.ID, "type", rt.Name, "error", err)
		return errNativeScreeningUnchecked
	case row.ScreeningDigest != current:
		return errNativeScreenedElsewhere
	}
	return nil
}

// ledgerCreated writes the ledger row of a resource of type rt the vendor
// just created (raw as the vendor sent it, wrapped as the client gets it).
// If the row cannot be written the resource is cancelled upstream and the
// error returned: an unrecorded async resource would never be accounted.
func (c *nativeCall) ledgerCreated(rt nativeapi.ResourceType, raw, wrapped []byte) error {
	vendorID := createdVendorID(c.op, raw)
	if vendorID == "" {
		return errNativeCreatedUnexpected
	}

	row := &models.NativeResource{
		Vendor:           string(c.nr.llm.Vendor),
		LLMID:            c.nr.llm.ID,
		AppID:            c.app.ID,
		UserID:           c.app.UserID,
		ResourceType:     rt.Name,
		VendorResourceID: vendorID,
		Snapshot:         string(wrapped),
		AccountingState:  models.NativeAccountingNone,
	}
	if id := authidentity.From(c.r.Context()); id != nil {
		row.OnBehalfOf, row.ActingAgent = id.OnBehalfOf(), id.ActingAgent()
	}
	if c.op.Admit != nil {
		// admit screened the content with the filters of this same LLM
		// snapshot.
		row.ScreeningDigest = requestFilterDigest(c.nr.llm)
	}
	if rt.Async != nil {
		row.AccountingState = models.NativeAccountingPending
		if rt.Async.State != nil {
			if info, err := rt.Async.State(raw); err == nil {
				row.State, row.EndedAt, row.ResultsExpireAt = string(info.State), info.EndedAt, info.ResultsExpireAt
			}
		}
	}

	// The client may give up; the row must still be written (or the batch
	// cancelled), so this does not follow the request's cancellation.
	ctx := context.WithoutCancel(c.r.Context())
	var err error
	for attempt := 0; attempt < nativeLedgerAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(50<<attempt) * time.Millisecond)
		}
		r := *row
		if err = c.store.Create(ctx, &r); err == nil || errors.Is(err, services.ErrNativeResourceExists) {
			return nil
		}
	}
	slog.Error("native resource not recorded; cancelling it upstream",
		"llm", c.nr.llm.Name, "app_id", c.app.ID, "type", rt.Name, "vendor_id", vendorID, "error", err)
	c.cancelUpstream(ctx, rt, vendorID)
	return err
}

// cancelUpstream cancels a resource the gateway could not record.
func (c *nativeCall) cancelUpstream(ctx context.Context, rt nativeapi.ResourceType, vendorID string) {
	if rt.Async == nil || rt.Async.Cancel == nil {
		slog.Error("native resource cannot be cancelled upstream; it will not be accounted", "type", rt.Name, "vendor_id", vendorID)
		return
	}
	creq := rt.Async.Cancel(vendorID)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := c.upstreamRequestWith(ctx, creq.Method, creq.Path, creq.Query, nil, creq.Header)
	if err == nil {
		var resp *http.Response
		if resp, err = upstreamTransport.RoundTrip(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				err = fmt.Errorf("vendor answered %d", resp.StatusCode)
			}
		}
	}
	if err != nil {
		slog.Error("native resource cancel failed; it will not be accounted", "type", rt.Name, "vendor_id", vendorID, "error", err)
	}
}

// jsonEquivalent reports whether a and b are the same JSON value.
func jsonEquivalent(a, b []byte) bool {
	var va, vb any
	da, db := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	db.UseNumber()
	if da.Decode(&va) != nil || db.Decode(&vb) != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

// multipartOpensWithBoundary reports whether contentType is multipart/form-data
// and body opens with its boundary line. A preamble before it would let the
// vendor (reading the Content-Type boundary) and the gateway (reading the
// body's first line) see different parts.
func multipartOpensWithBoundary(contentType string, body []byte) bool {
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil || mt != "multipart/form-data" || params["boundary"] == "" {
		return false
	}
	open := "--" + params["boundary"]
	return bytes.HasPrefix(body, []byte(open+"\r\n")) || bytes.HasPrefix(body, []byte(open+"\n"))
}
