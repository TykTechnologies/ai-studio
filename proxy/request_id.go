package proxy

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
)

// hdrInternalClientRequestID carries the caller's X-Request-ID across the
// gateway's own loopback hop, where requestid.Header carries the request id.
// Both are read only on that hop (IsInternalHop).
const hdrInternalClientRequestID = "X-Tyk-AI-Client-Request-ID"

// withRequestID gives every request the gateway's request id
// (pkg/requestid): the one a front middleware already put in the context
// (the Microgateway's), the outer hop's on the gateway's own loopback hop,
// else a new one. It is answered in requestid.Header; the caller's
// X-Request-ID is left alone and recorded as the client request id.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, client := RequestIDs(r)
		w.Header().Set(requestid.Header, id)
		next.ServeHTTP(w, r.WithContext(requestid.WithID(r.Context(), id, client)))
	})
}

// RequestIDs returns the request id and the caller's request id for r: those
// already in its context, else the ones the gateway's own loopback hop
// carries, else a new id and r's sanitised X-Request-ID. A client cannot set
// the request id: requestid.Header is honoured only from the loopback hop.
func RequestIDs(r *http.Request) (id, client string) {
	id = requestid.FromContext(r.Context())
	client = requestid.ClientFromContext(r.Context())
	if id != "" {
		return id, client
	}
	if IsInternalHop(r) {
		if hop := r.Header.Get(requestid.Header); requestid.Valid(hop) {
			return hop, requestid.SanitizeClient(r.Header.Get(hdrInternalClientRequestID))
		}
	}
	return requestid.New(), requestid.SanitizeClient(r.Header.Get(requestid.ClientHeader))
}

// withRequestIDsFrom carries r's request ids onto ctx: the bridges run their
// loopback attempts on a context detached from the request's.
func withRequestIDsFrom(ctx context.Context, r *http.Request) context.Context {
	if id := requestid.FromContext(r.Context()); id != "" {
		return requestid.WithID(ctx, id, requestid.ClientFromContext(r.Context()))
	}
	return ctx
}

// setLoopbackRequestIDs puts the outer hop's ids on a loopback request, so the
// inner hop (where filters, plugins and analytics run) records the request
// under the id its client was answered with.
func setLoopbackRequestIDs(req *http.Request) {
	req.Header.Del(hdrInternalClientRequestID)
	if id := requestid.FromContext(req.Context()); id != "" {
		req.Header.Set(requestid.Header, id)
	}
	if client := requestid.ClientFromContext(req.Context()); client != "" {
		req.Header.Set(hdrInternalClientRequestID, client)
	}
}
