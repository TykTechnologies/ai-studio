package proxy

import (
	"bytes"
	"context"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
)

// Plugin context metadata keys for the model a request names and whether it
// asks for a stream, set on post_auth and response hook contexts for every
// vendor (8xf.11). A host convenience only: hosts before 2.3 do not set them,
// so a plugin that must run there reads the request with plugin_sdk's
// RequestModel and IsStreamingRequest instead.
const (
	MetadataModel  = "model"
	MetadataStream = "stream"
)

// ExchangeMetadata is the model and stream metadata of a request to vendor:
// path is the request path (escaped, so a Bedrock ARN model ID comes back
// whole), body its body. The model is left out when the request names none.
// The body is parsed at most once, and not at all on a path that decides
// both (requestformat.ModelAndStreaming).
func ExchangeMetadata(vendor, path string, body []byte) map[string]string {
	return exchangeMetadata(vendor, path, func() (string, bool) { return requestformat.BodyFields(body) })
}

// ExchangeMetadataFor is ExchangeMetadata of r, reusing the body fields an
// earlier step of r read from the same bytes, and keeping what it reads on
// the returned request for the later steps (the edge's post_auth hook runs
// before the proxy handler, which needs them again).
func ExchangeMetadataFor(r *http.Request, vendor string, body []byte) (*http.Request, map[string]string) {
	r, fields := withRequestFields(r)
	return r, exchangeMetadata(vendor, r.URL.EscapedPath(), fields.reader(body))
}

func exchangeMetadata(vendor, path string, bodyFields func() (string, bool)) map[string]string {
	model, stream := requestformat.ModelAndStreamingFunc(vendor, path, bodyFields)
	md := map[string]string{MetadataStream: strconv.FormatBool(stream)}
	if model != "" {
		md[MetadataModel] = model
	}
	return md
}

type exchangeMetadataKey struct{}

// withExchangeMetadata keeps a request's ExchangeMetadata on its context for
// the response hooks, which run after its body has gone upstream.
func withExchangeMetadata(r *http.Request, vendor string, body []byte) *http.Request {
	md := exchangeMetadata(vendor, r.URL.EscapedPath(), requestFieldsOf(r.Context()).reader(body))
	return r.WithContext(context.WithValue(r.Context(), exchangeMetadataKey{}, md))
}

// requestFields is the "model" and "stream" body fields of a request as one
// step read them (requestformat.BodyFields semantics), kept on the request's
// context so a later step need not parse the body again: the model
// extractors decode the whole body to validate the model, and v2.2.2 parsed
// a request no more often than that (53e.45). They are keyed by the bytes
// they were read from, so a filter or plugin that rewrote the body in
// between makes them miss, and the body is parsed again.
type requestFields struct {
	body   []byte
	model  string
	stream bool
	set    bool
}

type requestFieldsKey struct{}

// withRequestFields is r with a requestFields on its context, the one it
// already has or a new one, which later steps fill and read.
func withRequestFields(r *http.Request) (*http.Request, *requestFields) {
	if f := requestFieldsOf(r.Context()); f != nil {
		return r, f
	}
	f := &requestFields{}
	return r.WithContext(context.WithValue(r.Context(), requestFieldsKey{}, f)), f
}

// requestFieldsOf is ctx's requestFields; nil when it has none, which keeps
// and finds nothing.
func requestFieldsOf(ctx context.Context) *requestFields {
	f, _ := ctx.Value(requestFieldsKey{}).(*requestFields)
	return f
}

// keepObject keeps the fields of body, which a model extractor decoded into
// obj, unless obj cannot say what BodyFields would (requestformat.ObjectFields).
func (f *requestFields) keepObject(body []byte, obj map[string]any) {
	if f == nil {
		return
	}
	if model, stream, ok := requestformat.ObjectFields(obj); ok {
		f.body, f.model, f.stream, f.set = body, model, stream, true
	}
}

// reader reads body's fields: the kept ones when they were read from the
// same bytes, else from body (once), which it then keeps.
func (f *requestFields) reader(body []byte) func() (string, bool) {
	return func() (string, bool) {
		if f == nil {
			return requestformat.BodyFields(body)
		}
		if f.set && bytes.Equal(f.body, body) {
			return f.model, f.stream
		}
		model, stream := requestformat.BodyFields(body)
		f.body, f.model, f.stream, f.set = body, model, stream, true
		return model, stream
	}
}

// addExchangeMetadata copies the request's ExchangeMetadata, if its handler
// kept any, into a response hook context's metadata.
func addExchangeMetadata(ctx context.Context, dst map[string]string) {
	md, _ := ctx.Value(exchangeMetadataKey{}).(map[string]string)
	for k, v := range md {
		dst[k] = v
	}
}
