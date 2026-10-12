package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gosimple/slug"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Child resources (nativeapi.BodyRef.Child): resources an async resource
// produces, such as an OpenAI batch's output and error files. The node that
// holds the parent's ledger row records each child as it first sees it in
// the parent's object (a retrieve, a cancel, a reconciler poll), with
// ParentID set, so the child is listed with the App's other resources and a
// delete of it waits until the parent is accounted: a deleted output file
// would leave the batch's usage unreadable.

// recordNativeChildren records the children refs name in raw (the parent's
// vendor object) that this node's ledger does not hold yet. Each child's
// snapshot is the vendor's object for it when it can be fetched.
func (p *Proxy) recordNativeChildren(ctx context.Context, store services.NativeResourceStore, parent *models.NativeResource,
	llm *models.LLM, nv *nativeVendor, codec *nativeapi.Codec, refs []nativeapi.BodyRef, raw []byte) {
	var obj map[string]any
	for _, ref := range refs {
		if !ref.Child {
			continue
		}
		if obj == nil {
			o, err := decodeNativeObject(raw)
			if err != nil {
				return
			}
			obj = o
		}
		var vendorID string
		_ = rewriteBodyRef(obj, ref.Path, func(s string) (string, error) { vendorID = s; return s, nil })
		rt, ok := nv.matcher.Spec().Resource(ref.Resource)
		if vendorID == "" || !ok || !rt.ValidVendorID(vendorID) {
			continue
		}
		if _, err := store.GetByOwner(ctx, parent.LLMID, parent.AppID, rt.Name, vendorID); err == nil {
			continue
		} else if !errors.Is(err, services.ErrNativeResourceNotFound) {
			slog.Warn("native child resource not recorded", "parent", parent.ID, "type", rt.Name, "error", err)
			continue
		}
		parentID := parent.ID
		row := &models.NativeResource{
			Vendor: parent.Vendor, LLMID: parent.LLMID, AppID: parent.AppID, UserID: parent.UserID,
			OnBehalfOf: parent.OnBehalfOf, ActingAgent: parent.ActingAgent,
			ResourceType: rt.Name, VendorResourceID: vendorID, ParentID: &parentID,
			Snapshot:        p.nativeChildSnapshot(ctx, llm, nv, codec, parent.AppID, rt, vendorID),
			AccountingState: models.NativeAccountingNone,
		}
		if err := store.Create(ctx, row); err != nil && !errors.Is(err, services.ErrNativeResourceExists) {
			slog.Warn("native child resource not recorded", "parent", parent.ID, "type", rt.Name, "vendor_id", vendorID, "error", err)
		}
	}
}

// nativeChildSnapshot fetches a child's vendor object through its type's
// retrieve operation and wraps it for the parent's App; "" when it cannot.
func (p *Proxy) nativeChildSnapshot(ctx context.Context, llm *models.LLM, nv *nativeVendor, codec *nativeapi.Codec,
	appID uint, rt nativeapi.ResourceType, vendorID string) string {
	op := nativeRetrieveOp(nv, rt.Name)
	if op == nil || codec == nil {
		return ""
	}
	status, raw, err := p.nativeFetch(ctx, llm, nativeapi.UpstreamReq{
		Method: http.MethodGet, Path: strings.Replace(op.Path, "{"+nativeapi.IDParam+"}", url.PathEscape(vendorID), 1),
	})
	if err != nil || status != http.StatusOK {
		return ""
	}
	c := &nativeCall{
		p: p, nr: &nativeRoute{mode: "call", slug: slug.Make(llm.Name), llm: llm, vendor: nv},
		op: op, codec: codec, scope: nativeapi.Scope{LLMID: llm.ID, AppID: appID},
	}
	m, err := decodeNativeObject(raw)
	if err != nil || !c.wrapResponse(m) {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}
