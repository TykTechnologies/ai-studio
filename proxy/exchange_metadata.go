package proxy

import (
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
func ExchangeMetadata(vendor, path string, body []byte) map[string]string {
	md := map[string]string{MetadataStream: strconv.FormatBool(requestformat.IsStreaming(vendor, path, body))}
	if model := requestformat.Model(vendor, path, body); model != "" {
		md[MetadataModel] = model
	}
	return md
}

type exchangeMetadataKey struct{}

// withExchangeMetadata keeps a request's ExchangeMetadata on its context for
// the response hooks, which run after its body has gone upstream.
func withExchangeMetadata(r *http.Request, vendor string, body []byte) *http.Request {
	md := ExchangeMetadata(vendor, r.URL.EscapedPath(), body)
	return r.WithContext(context.WithValue(r.Context(), exchangeMetadataKey{}, md))
}

// addExchangeMetadata copies the request's ExchangeMetadata, if its handler
// kept any, into a response hook context's metadata.
func addExchangeMetadata(ctx context.Context, dst map[string]string) {
	md, _ := ctx.Value(exchangeMetadataKey{}).(map[string]string)
	for k, v := range md {
		dst[k] = v
	}
}
