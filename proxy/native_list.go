package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Listings and deletes of native resources.
//
// A listing is served from this node's ledger, never forwarded: the vendor
// would list every App's resources in the LLM's workspace. It shows the
// calling App's resources on the LLM that this node created, newest first,
// from the snapshot of each one the gateway last returned. On a gateway with
// several replicas or edges, each lists only its own.
//
// A delete reaches the vendor only once this node's ledger shows the
// resource settled, so deleting it cannot be used to dodge its accounting:
// on the creating node an unsettled resource is accounted first; any other
// node refuses (it cannot account a resource it did not create).

const (
	nativeDeleteNotOwnedMsg  = "this batch can only be deleted through the gateway that created it, once its usage has been accounted"
	nativeDeleteUnsettledMsg = "this batch cannot be deleted until it has ended and its usage has been accounted; retry once it has ended"
)

// listedResource is the resource type a listing operation pages through.
func (c *nativeCall) listedResource() (nativeapi.ResourceType, bool) {
	if len(c.op.ResponseIDs) == 0 {
		return nativeapi.ResourceType{}, false
	}
	return c.nr.vendor.matcher.Spec().Resource(c.op.ResponseIDs[0].Resource)
}

// serveList answers a listing from the ledger.
func (c *nativeCall) serveList(w http.ResponseWriter) {
	ls := c.op.List
	rt, ok := c.listedResource()
	if !ok {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI, "operation not supported by this gateway")
		return
	}
	q := c.r.URL.Query()
	limit := ls.DefaultLimit
	if limit <= 0 {
		limit = 20
	}
	maxLimit := ls.MaxLimit
	if maxLimit <= 0 || maxLimit > services.MaxNativeResourceListLimit {
		maxLimit = services.MaxNativeResourceListLimit
	}
	if v := q.Get(ls.LimitParam); ls.LimitParam != "" && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, fmt.Sprintf("%s must be an integer between 1 and %d", ls.LimitParam, maxLimit))
			return
		}
		limit = n
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	query := services.NativeResourceListQuery{LLMID: c.nr.llm.ID, AppID: c.app.ID, ResourceType: rt.Name, Limit: limit}
	after, before := c.listParam(q, ls.AfterParam), c.listParam(q, ls.BeforeParam)
	if after != "" && before != "" {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, fmt.Sprintf("%s and %s cannot be combined", ls.AfterParam, ls.BeforeParam))
		return
	}
	// Cursors are IDs the gateway handed out: verified for this App and
	// LLM, then unwrapped. A raw vendor ID or one that does not verify is
	// a bad request.
	for _, cur := range []struct {
		param, value string
		dst          *string
	}{{ls.AfterParam, after, &query.AfterVendorID}, {ls.BeforeParam, before, &query.BeforeVendorID}} {
		if cur.value == "" {
			continue
		}
		vendorID, ok := c.unwrapID(rt, cur.value)
		if !ok {
			c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, fmt.Sprintf("invalid %s", cur.param))
			return
		}
		*cur.dst = vendorID
	}

	ctx := c.r.Context()
	rows, err := c.store.List(ctx, query)
	if errors.Is(err, services.ErrNativeResourceNotFound) {
		// A verified ID of a resource this node did not create.
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "invalid pagination cursor for this gateway")
		return
	}
	if err != nil {
		slog.Error("native list", "llm", c.nr.llm.Name, "op", c.op.Name, "error", err)
		c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "the gateway could not read its resource ledger")
		return
	}

	items := make([][]byte, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for i := range rows {
		item, id, err := c.listItem(rt, &rows[i])
		if err != nil {
			slog.Error("native list item", "row", rows[i].ID, "error", err)
			c.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to render the listing")
			return
		}
		items = append(items, item)
		ids = append(ids, id)
	}

	// More pages in the requested direction: anything beyond the page's last
	// row (older), or for a before cursor beyond its first (newer).
	hasMore := false
	if len(rows) == limit {
		next := services.NativeResourceListQuery{LLMID: query.LLMID, AppID: query.AppID, ResourceType: query.ResourceType, Limit: 1}
		if query.BeforeVendorID != "" {
			next.BeforeVendorID = rows[0].VendorResourceID
		} else {
			next.AfterVendorID = rows[len(rows)-1].VendorResourceID
		}
		more, err := c.store.List(ctx, next)
		if err != nil && !errors.Is(err, services.ErrNativeResourceNotFound) {
			slog.Error("native list", "llm", c.nr.llm.Name, "op", c.op.Name, "error", err)
			c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "the gateway could not read its resource ledger")
			return
		}
		hasMore = len(more) > 0
	}

	firstID, lastID := "", ""
	if len(ids) > 0 {
		firstID, lastID = ids[0], ids[len(ids)-1]
	}
	body, err := ls.Envelope(items, firstID, lastID, hasMore)
	if err != nil {
		c.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to render the listing")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	c.status, c.respBody = http.StatusOK, body
}

// listParam is a query parameter's value ("" when the param is unnamed).
func (c *nativeCall) listParam(q map[string][]string, name string) string {
	if name == "" {
		return ""
	}
	if vs := q[name]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// listItem renders one listed resource: its last snapshot with the ID
// freshly wrapped for the calling App and LLM (a snapshot taken before a
// vendor key rotation carries the old signature).
func (c *nativeCall) listItem(rt nativeapi.ResourceType, row *models.NativeResource) ([]byte, string, error) {
	id, err := c.codec.Wrap(rt, row.VendorResourceID, c.scope)
	if err != nil {
		return nil, "", err
	}
	obj := map[string]any{}
	if row.Snapshot != "" {
		if o, err := decodeNativeObject([]byte(row.Snapshot)); err == nil {
			obj = o
		} else {
			slog.Warn("native ledger snapshot unreadable; listing the ID only", "row", row.ID, "error", err)
		}
	}
	obj["id"] = id
	out, err := json.Marshal(obj)
	return out, id, err
}

// serveDelete forwards a delete once this node's ledger shows the resource
// settled, accounting it first when it can.
func (c *nativeCall) serveDelete(w http.ResponseWriter, path string, body []byte) {
	rt, ok := c.nr.vendor.matcher.Spec().Resource(c.op.PathResource)
	if !ok {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI, "operation not supported by this gateway")
		return
	}
	// The ledger work outlives a client that gives up.
	ctx := context.WithoutCancel(c.r.Context())
	row, err := c.store.GetByOwner(ctx, c.nr.llm.ID, c.app.ID, rt.Name, c.vendorID)
	if errors.Is(err, services.ErrNativeResourceNotFound) {
		c.fail(w, http.StatusConflict, nativeErrInvalidRequest, nativeDeleteNotOwnedMsg)
		return
	}
	if err != nil {
		slog.Error("native delete: ledger read", "llm", c.nr.llm.Name, "error", err)
		c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "the gateway could not read its resource ledger")
		return
	}
	if !models.NativeAccountingTerminal(row.AccountingState) {
		if rt.Async == nil {
			c.fail(w, http.StatusConflict, nativeErrInvalidRequest, nativeDeleteUnsettledMsg)
			return
		}
		c.accountBeforeDelete(ctx, row)
		// Whatever the outcome, only the ledger decides.
		row, err = c.store.GetByOwner(ctx, c.nr.llm.ID, c.app.ID, rt.Name, c.vendorID)
		if errors.Is(err, services.ErrNativeResourceNotFound) {
			// A concurrent delete removed it meanwhile.
			c.fail(w, http.StatusNotFound, nativeErrNotFound, nativeNotFoundMsg)
			return
		}
		if err != nil {
			c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "the gateway could not read its resource ledger")
			return
		}
		if !models.NativeAccountingTerminal(row.AccountingState) {
			c.fail(w, http.StatusConflict, nativeErrInvalidRequest, nativeDeleteUnsettledMsg)
			return
		}
	}

	c.forward(w, path, body)
	if c.status < 200 || c.status >= 300 {
		return
	}
	// Gone at the vendor: gone from the ledger (and its listings). A row
	// left behind is settled, so it is only listed until swept.
	for attempt := 0; attempt < nativeLedgerAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(50<<attempt) * time.Millisecond)
		}
		if err = c.store.Delete(ctx, row.ID); err == nil || errors.Is(err, services.ErrNativeResourceNotFound) {
			return
		}
	}
	slog.Warn("native delete: ledger row not removed", "row", row.ID, "error", err)
}

// accountBeforeDelete tries to settle row (on its creating node) before a
// delete, with the reconciler's synchronous accounting of one row. The
// caller re-reads the ledger: only a settled row may be deleted.
func (c *nativeCall) accountBeforeDelete(ctx context.Context, row *models.NativeResource) {
	outcome, err := c.p.accountNativeRow(ctx, row)
	if err != nil {
		slog.Warn("native delete: accounting before the delete failed", "row", row.ID, "outcome", outcome, "error", err)
	}
}
