package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
)

// Native API extensions (pkg/nativeapi) on the native /llm routes.
//
// The native route is registered ahead of the legacy /llm routes and claims a
// request only when its vendor declares an operation on exactly that method
// and canonical path (or, for a vendor-shaped 405, on that path). Anything
// else - POST /v1/messages included - falls through to the legacy chain
// untouched. It sits inside the credential middleware, so authentication,
// the PreAuth/PostAuth plugins and the LLM ACL have all run; it runs before
// stream detection, model validation and the /llm/call rewrite, which assume
// a single synchronous call with a top-level model.
//
// A claimed request never falls back to the legacy chain: missing
// prerequisites (no ledger, no vendor key) are a 501.

const (
	// nativeMaxRequestBody caps the body of an operation without an Admit
	// step (retrieve, cancel: no body at all).
	nativeMaxRequestBody = 1 << 20
	// nativeMaxObjectBody caps a vendor object read back for ID wrapping.
	nativeMaxObjectBody = 16 << 20
)

// Vendor-neutral error types; an operation's ErrorShape renders them in the
// vendor's error format (they are Anthropic's names).
const (
	nativeErrInvalidRequest = "invalid_request_error"
	nativeErrAuthentication = "authentication_error"
	nativeErrPermission     = "permission_error"
	nativeErrNotFound       = "not_found_error"
	nativeErrAPI            = "api_error"
)

// nativeNotFoundMsg is the one answer to every ID that does not verify, so
// the check cannot be used as an oracle.
const nativeNotFoundMsg = "resource not found"

// nativeVendor is a vendor's compiled native spec.
type nativeVendor struct {
	matcher *nativeapi.Matcher
}

// nativeVendors caches nativeVendorFor per vendor. Specs are static.
var nativeVendors sync.Map // models.Vendor -> *nativeVendor (nil: none)

// nativeVendorFor returns the compiled native spec of vendor, or nil when the
// vendor declares none (or an invalid one, which is logged once).
func nativeVendorFor(vendor models.Vendor) *nativeVendor {
	if v, ok := nativeVendors.Load(vendor); ok {
		return v.(*nativeVendor)
	}
	var nv *nativeVendor
	if ctor, ok := switches.VendorMap[vendor]; ok {
		if prov, ok := ctor().(nativeapi.Provider); ok {
			m, err := nativeapi.NewMatcher(prov.NativeAPI())
			if err != nil {
				slog.Error("native API spec invalid; native operations disabled", "vendor", vendor, "error", err)
			} else {
				nv = &nativeVendor{matcher: m}
			}
		}
	}
	actual, _ := nativeVendors.LoadOrStore(vendor, nv)
	return actual.(*nativeVendor)
}

// nativeRoute is a request the native route claims.
type nativeRoute struct {
	mode, slug, rest string
	llm              *models.LLM
	vendor           *nativeVendor
	// canonical is the vendor-canonical path (canonicalVendorPath).
	canonical string
	// match is the operation; its Op is nil for a native path requested with
	// a method no operation declares.
	match nativeapi.Match
	// variant is a non-canonical spelling of a declared path.
	variant bool
}

// splitNativePath splits /llm/{mode}/{slug}/{rest...}. rest keeps its leading
// slash and is never empty.
func splitNativePath(p string) (mode, slug, rest string, ok bool) {
	tail, found := strings.CutPrefix(p, "/llm/")
	if !found {
		return "", "", "", false
	}
	mode, tail, found = strings.Cut(tail, "/")
	if !found || (mode != "rest" && mode != "stream" && mode != "call") {
		return "", "", "", false
	}
	slug, tail, found = strings.Cut(tail, "/")
	if !found || slug == "" || tail == "" {
		return "", "", "", false
	}
	return mode, slug, "/" + tail, true
}

// resolveNative reports whether r is a native operation of its LLM's vendor.
func (p *Proxy) resolveNative(r *http.Request) (*nativeRoute, bool) {
	mode, slug, rest, ok := splitNativePath(r.URL.Path)
	if !ok || hasTraversalSegment(r.URL.Path) {
		return nil, false
	}
	llm, ok := p.GetLLM(slug)
	if !ok {
		return nil, false
	}
	nv := nativeVendorFor(llm.Vendor)
	if nv == nil {
		return nil, false
	}
	endpoint, err := url.Parse(switches.UpstreamEndpoint(llm))
	if err != nil {
		return nil, false
	}
	canonical := canonicalVendorPath(endpoint.Path, rest)
	nr := &nativeRoute{mode: mode, slug: slug, rest: rest, llm: llm, vendor: nv, canonical: canonical}
	if m, ok := nv.matcher.Match(r.Method, canonical); ok {
		nr.match = m
		return nr, true
	}
	// A path a chain hook declares for the method stays on the synchronous
	// chain even when an operation's pattern also matches it (OpenAI's POST
	// /v1/responses/compact against /v1/responses/{id}).
	if _, hooked := nv.matcher.Spec().ChainHooks.Hook(r.Method, canonical); hooked {
		return nil, false
	}
	if nv.matcher.MatchesPath(canonical) {
		return nr, true
	}
	// Another spelling of a declared path (case, empty segments) would reach
	// the vendor around the native handling, and an upstream that routes
	// case-insensitively would serve it: it is claimed and refused. The
	// caller's own path is checked too: joining it to the endpoint's path is
	// case-sensitive, so /V1/responses on a .../v1 endpoint canonicalises to
	// /v1/V1/responses, which no declared path folds to.
	if !nv.matcher.Spec().ChainHooks.Declares(canonical) &&
		(nv.matcher.MatchesFold(canonical) || nv.matcher.MatchesFold(rest)) {
		nr.variant = true
		return nr, true
	}
	return nil, false
}

// nativeMatch is the mux matcher of the native route.
func (p *Proxy) nativeMatch(r *http.Request, _ *mux.RouteMatch) bool {
	_, ok := p.resolveNative(r)
	return ok
}

// nativeErrorShape renders errors for the vendor: the operation's shape, or
// any the vendor declares for a path with no operation for the method.
func (nr *nativeRoute) errorShape() func(status int, typ, msg string) []byte {
	if nr.match.Op != nil && nr.match.Op.ErrorShape != nil {
		return nr.match.Op.ErrorShape
	}
	for _, op := range nr.vendor.matcher.Spec().Operations {
		if op.ErrorShape != nil {
			return op.ErrorShape
		}
	}
	return func(status int, typ, msg string) []byte {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"type": typ, "message": msg}})
		return b
	}
}

// nativeCall is one native operation in flight.
type nativeCall struct {
	p     *Proxy
	nr    *nativeRoute
	op    *nativeapi.Operation
	app   *models.App
	store services.NativeResourceStore
	codec *nativeapi.Codec
	scope nativeapi.Scope
	// vendorID is the unwrapped {id} of the path, once verified.
	vendorID string
	r        *http.Request
	status   int
	// reqBody and respBody are what the proxy log records.
	reqBody, respBody []byte
}

// opName names the call's operation for metrics.
func (c *nativeCall) opName() string {
	if c.nr.variant {
		return "path_variant"
	}
	if c.op == nil {
		return "method_not_allowed"
	}
	return c.op.Name
}

func (c *nativeCall) fail(w http.ResponseWriter, status int, typ, msg string) {
	body := c.nr.errorShape()(status, typ, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	c.status, c.respBody = status, body
}

// handleNative serves a request nativeMatch claimed.
func (p *Proxy) handleNative(w http.ResponseWriter, r *http.Request) {
	nr, ok := p.resolveNative(r)
	if !ok {
		// The LLM changed between matching and serving (a reload).
		respondWithError(w, http.StatusNotFound, "LLM not found", nil, false)
		return
	}
	c := &nativeCall{p: p, nr: nr, op: nr.match.Op, r: r}
	r, span := startProxySpan(r, nr.llm)
	c.r = r
	defer func() { endProxySpan(span, c.status) }()
	start := time.Now()
	defer func() {
		metrics.RecordNativeRequest(r.Context(), string(nr.llm.Vendor), c.opName(), c.status, time.Since(start).Seconds())
	}()

	app, _ := r.Context().Value("app").(*models.App)
	if app == nil {
		c.fail(w, http.StatusUnauthorized, nativeErrAuthentication, "authentication required")
		return
	}
	c.app = app
	defer c.log()

	if nr.variant {
		c.fail(w, http.StatusNotFound, nativeErrNotFound, "unknown path: native API paths are matched exactly (lower case, no empty segments)")
		return
	}

	if c.op == nil {
		w.Header().Set("Allow", strings.Join(nr.allowedMethods(), ", "))
		c.fail(w, http.StatusMethodNotAllowed, nativeErrInvalidRequest, "method not allowed")
		return
	}
	if name, refused := refusedQuery(c.op, r.URL.Query()); refused {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest,
			fmt.Sprintf("query parameter %q is not supported by this gateway on this endpoint", name))
		return
	}
	// An operation that touches no vendor resource (Anthropic's
	// count_tokens) needs neither a direct grant nor the ledger and codec.
	resourceFree := nativeResourceFree(c.op)
	if !resourceFree && !c.directGrant() {
		c.fail(w, http.StatusForbidden, nativeErrPermission, "this endpoint requires direct access to the LLM")
		return
	}
	if !resourceFree && !c.prerequisites(w) {
		return
	}
	if !nativeOpImplemented(c.op) {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI, "operation not supported by this gateway")
		return
	}
	if c.op.Budget && !c.checkBudget(w) {
		return
	}
	if c.op.Ledger == nativeapi.LedgerCreate && !c.checkInProgressCap(w) {
		return
	}
	if c.op.Admit != nil && c.op.Ledger == nativeapi.LedgerCreate {
		release, ok := c.acquireCreate(w)
		if !ok {
			return
		}
		defer release()
	}

	upstreamPath, ok := c.upstreamPath(w)
	if !ok {
		return
	}
	body, ok := c.requestBody(w)
	if !ok {
		return
	}
	if c.op.Admit != nil {
		if body, ok = c.admit(w, body); !ok {
			return
		}
		// Log what the filters let through, as the synchronous chain does.
		c.reqBody = body
	}
	if c.op.Ledger == nativeapi.LedgerCreate && !c.storeHealthy(w) {
		return
	}
	if c.op.List != nil {
		c.serveList(w)
		return
	}
	if c.op.Ledger == nativeapi.LedgerDelete {
		c.serveDelete(w, upstreamPath, body)
		return
	}
	if c.op.Results != nil {
		c.serveResults(w, upstreamPath)
		return
	}
	c.forward(w, upstreamPath, body)
}

// nativeResourceFree reports whether op neither names, hands out nor lists a
// vendor resource: a single synchronous call the gateway only admits (model
// allow-list, filters) and logs, such as Anthropic's count_tokens.
func nativeResourceFree(op *nativeapi.Operation) bool {
	return op.Ledger == nativeapi.LedgerNone && op.PathResource == "" && len(op.RequestRefs) == 0 &&
		len(op.ResponseIDs) == 0 && op.List == nil && op.Results == nil && op.Transform == nil
}

// refusedQuery returns the first query parameter of op's Upstream.RefuseQuery
// the request carries with a value other than "false".
func refusedQuery(op *nativeapi.Operation, q url.Values) (string, bool) {
	for _, name := range op.Upstream.RefuseQuery {
		for _, v := range q[name] {
			if !strings.EqualFold(v, "false") {
				return name, true
			}
		}
	}
	return "", false
}

// allowedMethods lists the methods the vendor declares on the route's path.
func (nr *nativeRoute) allowedMethods() []string {
	var out []string
	seen := map[string]bool{}
	for _, op := range nr.vendor.matcher.Spec().Operations {
		if seen[op.Method] {
			continue
		}
		if m, ok := nr.vendor.matcher.Match(op.Method, nr.canonical); ok && m.Op != nil {
			seen[op.Method] = true
			out = append(out, op.Method)
		}
	}
	return out
}

// nativeOpImplemented reports whether the dispatcher serves op: the
// pass-through ops on an existing resource (read, mutate), creates, results
// downloads, ledger-served listings and settlement-guarded deletes.
func nativeOpImplemented(op *nativeapi.Operation) bool {
	if op.List != nil {
		// The listed type is the one of the page's IDs (ResponseIDs).
		return op.Ledger == nativeapi.LedgerList && op.PathResource == "" && len(op.ResponseIDs) > 0
	}
	if op.Results != nil {
		return op.Ledger == nativeapi.LedgerRead && op.PathResource != ""
	}
	switch op.Ledger {
	case nativeapi.LedgerNone, nativeapi.LedgerRead, nativeapi.LedgerMutate:
		return true
	case nativeapi.LedgerCreate:
		return len(op.ResponseIDs) > 0
	case nativeapi.LedgerDelete:
		return op.PathResource != ""
	}
	return false
}

// directGrant reports whether the App holds the LLM itself. Native resources
// live in one vendor account, so the access inherited through a router or as
// a failover rung (which the ACL honours for synchronous calls) does not
// extend to them, and a loopback hop is refused outright.
func (c *nativeCall) directGrant() bool {
	if _, ok := c.p.parseFailoverMarker(c.r); ok {
		return false
	}
	if _, ok := c.p.parseRouterMarker(c.r); ok {
		return false
	}
	if _, ok := failoverMarkerFromContext(c.r.Context()); ok {
		return false
	}
	if _, ok := routerMarkerFromContext(c.r.Context()); ok {
		return false
	}
	return appHoldsLLM(c.app, c.nr.llm.ID)
}

// prerequisites sets up the ledger and the ID codec, answering 501 when the
// gateway cannot serve native operations for the LLM.
func (c *nativeCall) prerequisites(w http.ResponseWriter) bool {
	c.store = nativeStoreOf(c.p.gatewayService)
	if c.store == nil {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI, "native API operations are not available on this gateway")
		return false
	}
	codec, err := nativeapi.CodecForLLM(c.nr.llm, time.Now())
	if err != nil {
		c.fail(w, http.StatusNotImplemented, nativeErrAPI, "native API operations need the LLM's signing secret or vendor credential")
		return false
	}
	c.codec = codec
	c.scope = nativeapi.Scope{LLMID: c.nr.llm.ID, AppID: c.app.ID}
	return true
}

// unwrapID verifies a wrapped ID of resource type rt for the calling App and
// LLM and returns the vendor ID. A MAC that fails verification is still
// accepted when this node's ledger holds the App's row for the vendor ID: the
// signing key was rotated past its grace period (or, on an LLM without a
// signing secret, the vendor credential) since the ID was minted.
func (c *nativeCall) unwrapID(rt nativeapi.ResourceType, wrapped string) (string, bool) {
	vendorID, err := c.codec.Unwrap(rt, wrapped, c.scope)
	if err == nil {
		return vendorID, true
	}
	parsed, perr := nativeapi.ParseID(rt, wrapped)
	if perr != nil || c.store == nil {
		return "", false
	}
	if _, gerr := c.store.GetByOwner(c.r.Context(), c.nr.llm.ID, c.app.ID, rt.Name, parsed.VendorID); gerr == nil {
		slog.Info("native resource id accepted by the ledger after a signature mismatch (rotated signing key?)",
			"llm_id", c.nr.llm.ID, "app_id", c.app.ID, "type", rt.Name, "kid", parsed.KID, "error", err)
		return parsed.VendorID, true
	}
	return "", false
}

// upstreamPath fills the operation's canonical path with the request's
// parameters, the {id} unwrapped to the vendor ID.
func (c *nativeCall) upstreamPath(w http.ResponseWriter) (string, bool) {
	segs := strings.Split(strings.TrimPrefix(c.op.Path, "/"), "/")
	for i, seg := range segs {
		name, isParam := strings.CutPrefix(seg, "{")
		if !isParam {
			continue
		}
		name = strings.TrimSuffix(name, "}")
		raw := c.nr.match.Params[name]
		var value string
		if name == nativeapi.IDParam {
			rt, ok := c.nr.vendor.matcher.Spec().Resource(c.op.PathResource)
			if !ok {
				c.fail(w, http.StatusNotImplemented, nativeErrAPI, "operation not supported by this gateway")
				return "", false
			}
			vendorID, ok := c.unwrapID(rt, raw)
			if !ok {
				c.fail(w, http.StatusNotFound, nativeErrNotFound, nativeNotFoundMsg)
				return "", false
			}
			value, c.vendorID = vendorID, vendorID
		} else {
			if !(nativeapi.ResourceType{}).ValidVendorID(raw) {
				c.fail(w, http.StatusNotFound, nativeErrNotFound, nativeNotFoundMsg)
				return "", false
			}
			value = raw
		}
		segs[i] = url.PathEscape(value)
	}
	return "/" + strings.Join(segs, "/"), true
}

// requestBody reads the request body (bounded) and unwraps the IDs the
// operation declares in it.
func (c *nativeCall) requestBody(w http.ResponseWriter) ([]byte, bool) {
	if c.r.Body == nil {
		return nil, true
	}
	limit := int64(nativeMaxRequestBody)
	if c.op.Admit != nil {
		limit = c.p.config.nativeMaxBodyBytes()
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, c.r.Body, limit))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			c.fail(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body too large")
		} else {
			c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "failed to read request body")
		}
		return nil, false
	}
	c.reqBody = body
	if len(c.op.RequestRefs) == 0 || len(bytes.TrimSpace(body)) == 0 {
		return body, true
	}
	obj, err := decodeNativeObject(body)
	if err != nil {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, "request body is not a JSON object")
		return nil, false
	}
	if k, ok := refKeyVariant(obj, c.op.RequestRefs); ok {
		c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, fmt.Sprintf("unsupported parameter %q", k))
		return nil, false
	}
	spec := c.nr.vendor.matcher.Spec()
	for _, ref := range c.op.RequestRefs {
		rt, _ := spec.Resource(ref.Resource)
		if err := rewriteRef(obj, ref, func(wrapped string) (string, error) {
			v, ok := c.unwrapID(rt, wrapped)
			if !ok {
				return "", errNativeRef
			}
			if ref.Screened {
				if err := c.checkScreened(rt, v); err != nil {
					return "", err
				}
			}
			return v, nil
		}); err != nil {
			switch {
			case errors.Is(err, errNativeScreenedElsewhere):
				metrics.RecordPolicyBlock(c.r.Context(), "request_filter", "firewall")
				c.fail(w, http.StatusBadRequest, nativeErrInvalidRequest, nativeScreenedElsewhereMsg)
			case errors.Is(err, errNativeScreeningUnchecked):
				c.fail(w, http.StatusServiceUnavailable, nativeErrAPI, "the input file could not be checked; retry shortly")
			default:
				c.fail(w, http.StatusNotFound, nativeErrNotFound, nativeNotFoundMsg)
			}
			return nil, false
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		c.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to encode request body")
		return nil, false
	}
	return out, true
}

var errNativeRef = errors.New("native resource reference does not verify")

// refKeyVariant returns a top-level key of obj that differs only in case from
// the field of one of refs ("Previous_Response_Id"). The gateway verifies the
// exact field; an upstream that matched parameters case-insensitively would
// read the variant unverified, so it is refused.
func refKeyVariant(obj map[string]any, refs []nativeapi.BodyRef) (string, bool) {
	for k := range obj {
		for _, ref := range refs {
			top, _, _ := strings.Cut(ref.Path, ".")
			top = strings.TrimSuffix(top, "[]")
			if k != top && strings.EqualFold(k, top) {
				return k, true
			}
		}
	}
	return "", false
}

// upstreamRequest builds the vendor request: the LLM's configured endpoint
// (never a plugin's upstream_override: the resources live in this LLM's
// vendor account), the canonical path, the allow-listed query parameters and
// headers, and the vendor credential.
func (c *nativeCall) upstreamRequest(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Request, error) {
	return c.upstreamRequestWith(ctx, method, path, query, body, nil)
}

// upstreamRequestWith is upstreamRequest with vendor default headers for the
// ones the client did not send (nativeapi.UpstreamReq.Header).
func (c *nativeCall) upstreamRequestWith(ctx context.Context, method, path string, query url.Values, body []byte, defaults http.Header) (*http.Request, error) {
	q := url.Values{}
	for _, name := range c.op.Upstream.QueryAllow {
		if vs, ok := query[name]; ok {
			q[name] = vs
		}
	}
	header := http.Header{}
	for _, name := range c.op.Upstream.HeaderAllow {
		if vs := c.r.Header.Values(name); len(vs) > 0 {
			header[http.CanonicalHeaderKey(name)] = append([]string(nil), vs...)
		}
	}
	return c.p.nativeUpstreamRequest(ctx, c.nr.llm, method, path, q, body, header, defaults)
}

// nativeUpstreamRequest builds a request to llm's configured endpoint (SSRF
// guarded) at a canonical vendor path, with header, then defaults for the
// headers header lacks, then the vendor credential. Query and header are
// sent as given: callers allow-list what comes from a client.
func (p *Proxy) nativeUpstreamRequest(ctx context.Context, llm *models.LLM, method, path string, query url.Values, body []byte, header, defaults http.Header) (*http.Request, error) {
	endpoint, err := url.Parse(switches.UpstreamEndpoint(llm))
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL: %w", err)
	}
	if err := netguard.ValidateUpstreamURL(endpoint); err != nil {
		return nil, fmt.Errorf("upstream endpoint not permitted: %w", err)
	}
	u := &url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}
	u.RawPath = joinUpstreamPath(endpoint.EscapedPath(), path)
	if p, err := url.PathUnescape(u.RawPath); err == nil {
		u.Path = p
	} else {
		return nil, fmt.Errorf("invalid upstream path: %w", err)
	}
	u.RawQuery = query.Encode()

	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		req.Header[http.CanonicalHeaderKey(k)] = append([]string(nil), vs...)
	}
	for k, vs := range defaults {
		if k = http.CanonicalHeaderKey(k); len(req.Header.Values(k)) == 0 {
			req.Header[k] = append([]string(nil), vs...)
		}
	}
	injectUpstreamTrace(req.Context(), req.Header, llm)
	if err := p.setVendorAuthHeader(req, llm); err != nil {
		return nil, fmt.Errorf("vendor auth: %w", err)
	}
	return req, nil
}

// forward sends the operation upstream and answers the client with the
// vendor's response, its IDs wrapped and its URLs rewritten.
func (c *nativeCall) forward(w http.ResponseWriter, path string, body []byte) {
	ctx, cancel := context.WithTimeout(c.r.Context(), c.p.config.llmTimeout())
	defer cancel()
	req, err := c.upstreamRequest(ctx, c.r.Method, path, c.r.URL.Query(), body)
	if err != nil {
		slog.Error("native upstream request", "llm", c.nr.llm.Name, "op", c.op.Name, "error", err)
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "upstream endpoint not permitted")
		return
	}
	resp, err := upstreamTransport.RoundTrip(req)
	if err != nil {
		c.fail(w, upstreamTransportStatus(err), nativeErrAPI, "failed to make upstream request")
		return
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, nativeMaxObjectBody+1))
	if err != nil {
		c.fail(w, upstreamTransportStatus(err), nativeErrAPI, "failed to read upstream response")
		return
	}
	if len(raw) > nativeMaxObjectBody {
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "upstream response too large")
		return
	}
	// The transport asked for gzip itself (the client's Accept-Encoding is not
	// forwarded) and decompressed; an encoding still set is one it could not.
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		c.fail(w, http.StatusBadGateway, nativeErrAPI, "unsupported upstream content encoding")
		return
	}

	out := raw
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		obj, err := decodeNativeObject(raw)
		if err != nil {
			c.fail(w, http.StatusBadGateway, nativeErrAPI, "unexpected upstream response")
			return
		}
		if !c.wrapResponse(obj) {
			c.fail(w, http.StatusBadGateway, nativeErrAPI, "unexpected upstream response")
			return
		}
		if out, err = json.Marshal(obj); err != nil {
			c.fail(w, http.StatusInternalServerError, nativeErrAPI, "failed to encode response")
			return
		}
		if c.op.Ledger == nativeapi.LedgerCreate {
			if !c.recordCreated(w, raw, out) {
				return
			}
		} else {
			c.updateLedger(raw, out)
		}
	}

	copyNativeResponseHeaders(w.Header(), resp.Header)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(out)
	c.status, c.respBody = resp.StatusCode, out
}

// wrapResponse wraps the IDs the operation declares in a vendor object and
// then runs its Transform. A vendor ID that cannot be wrapped fails the call
// rather than reaching the client raw.
func (c *nativeCall) wrapResponse(obj map[string]any) bool {
	spec := c.nr.vendor.matcher.Spec()
	wrap := func(resourceType, vendorID string) (string, error) {
		rt, ok := spec.Resource(resourceType)
		if !ok {
			return "", fmt.Errorf("unknown resource type %q", resourceType)
		}
		return c.codec.Wrap(rt, vendorID, c.scope)
	}
	for _, ref := range c.op.ResponseIDs {
		if err := rewriteRef(obj, ref, func(id string) (string, error) { return wrap(ref.Resource, id) }); err != nil {
			slog.Error("native response id not wrapped", "op", c.op.Name, "path", ref.Path, "error", err)
			return false
		}
	}
	if c.op.Transform != nil {
		c.op.Transform(obj, nativeapi.TransformEnv{PublicBaseURL: c.p.nativePublicBaseURL(c.nr), Wrap: wrap})
	}
	return true
}

// updateLedger refreshes this node's ledger row of the path resource, when it
// holds one (only the creating node does), with the vendor's latest view.
func (c *nativeCall) updateLedger(raw, wrapped []byte) {
	if c.op.Ledger != nativeapi.LedgerRead && c.op.Ledger != nativeapi.LedgerMutate {
		return
	}
	rt, ok := c.nr.vendor.matcher.Spec().Resource(c.op.PathResource)
	if !ok {
		return
	}
	vendorID, ok := c.unwrapID(rt, c.nr.match.ID())
	if !ok {
		return
	}
	ctx := context.WithoutCancel(c.r.Context())
	row, err := c.store.GetByOwner(ctx, c.nr.llm.ID, c.app.ID, rt.Name, vendorID)
	if err != nil {
		return
	}
	// A resource without a lifecycle (a file) keeps its state; only the
	// listed snapshot is refreshed.
	snap := services.NativeResourceSnapshot{
		State: row.State, Snapshot: string(wrapped), EndedAt: row.EndedAt, ResultsExpireAt: row.ResultsExpireAt,
	}
	if rt.Async != nil && rt.Async.State != nil {
		info, err := rt.Async.State(raw)
		if err != nil {
			return
		}
		snap.State, snap.EndedAt, snap.ResultsExpireAt = string(info.State), info.EndedAt, info.ResultsExpireAt
	}
	if err := c.store.UpdateSnapshot(ctx, row.ID, snap); err != nil {
		slog.Warn("native ledger snapshot not updated", "id", row.ID, "error", err)
	}
	c.p.recordNativeChildren(ctx, c.store, row, c.nr.llm, c.nr.vendor, c.codec, c.op.ResponseIDs, raw)
}

// log records the call's proxy log row.
func (c *nativeCall) log() {
	if c.app == nil {
		return
	}
	llm, app, r := c.nr.llm, c.app, c.r
	l := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    time.Now(),
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    llm.Name,
		RequestBody:  truncateString(string(c.reqBody), maxBodySize),
		ResponseBody: truncateString(string(c.respBody), maxBodySize),
		ResponseCode: c.status,
	}
	if llm.DontLogBodies {
		l.RequestBody, l.ResponseBody = "", ""
	}
	ctx := context.WithoutCancel(r.Context())
	c.p.goAnalyze(func() { analytics.RecordExchange(ctx, l, nil) })
}

// nativePublicBaseURL is the absolute base of the native route the request
// came in on, from the configured public URL; "" (root-relative URLs) when
// none is configured. The request's Host and forwarding headers are never
// trusted for it.
func (p *Proxy) nativePublicBaseURL(nr *nativeRoute) string {
	if p.config == nil || p.config.NativePublicURL == "" {
		return ""
	}
	u, err := url.Parse(p.config.NativePublicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return strings.TrimSuffix(u.String(), "/") + "/llm/" + nr.mode + "/" + url.PathEscape(nr.slug)
}

// copyNativeResponseHeaders copies the vendor's response headers, less the
// hop-by-hop ones, the framing the gateway recomputes and cookies.
func copyNativeResponseHeaders(dst, src http.Header) {
	for k, vs := range src {
		switch http.CanonicalHeaderKey(k) {
		case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
			"Transfer-Encoding", "Upgrade", "Content-Length", "Content-Encoding", "Set-Cookie":
			continue
		}
		dst[k] = append([]string(nil), vs...)
	}
}

// decodeNativeObject decodes a JSON object, keeping numbers exact.
func decodeNativeObject(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("not a JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after JSON object")
	}
	return obj, nil
}

// rewriteBodyRef replaces every string a nativeapi.BodyRef path names in obj
// with fn's result. A segment with a "[]" suffix iterates an array; a missing
// or null field is skipped, a field of another type is an error.
func rewriteBodyRef(obj map[string]any, path string, fn func(string) (string, error)) error {
	return rewriteRefSegs(obj, strings.Split(path, "."), false, fn)
}

// rewriteRef is rewriteBodyRef for ref, honouring ref.ObjectID: the field
// may also be an object whose "id" is the reference.
func rewriteRef(obj map[string]any, ref nativeapi.BodyRef, fn func(string) (string, error)) error {
	return rewriteRefSegs(obj, strings.Split(ref.Path, "."), ref.ObjectID, fn)
}

func rewriteRefSegs(obj map[string]any, segs []string, objectID bool, fn func(string) (string, error)) error {
	name, isArr := strings.CutSuffix(segs[0], "[]")
	v, ok := obj[name]
	if !ok || v == nil {
		return nil
	}
	last := len(segs) == 1
	if !isArr {
		if last {
			if m, isObj := v.(map[string]any); isObj && objectID {
				return rewriteRefSegs(m, []string{"id"}, false, fn)
			}
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s is not a string", name)
			}
			out, err := fn(s)
			if err != nil {
				return err
			}
			obj[name] = out
			return nil
		}
		child, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not an object", name)
		}
		return rewriteRefSegs(child, segs[1:], objectID, fn)
	}
	arr, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", name)
	}
	for i, el := range arr {
		if last {
			s, ok := el.(string)
			if !ok {
				return fmt.Errorf("%s[%d] is not a string", name, i)
			}
			out, err := fn(s)
			if err != nil {
				return err
			}
			arr[i] = out
			continue
		}
		child, ok := el.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] is not an object", name, i)
		}
		if err := rewriteRefSegs(child, segs[1:], objectID, fn); err != nil {
			return err
		}
	}
	return nil
}
