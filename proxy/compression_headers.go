package proxy

import (
	"net/http"
	"strings"
)

// hdrCompression is the request header a client sends to a gateway plugin
// that compresses request bodies (X-Tyk-Compression: off asks the Prompt
// Compressor to leave the body alone). It is an instruction to the gateway:
// it crosses the /ai/ -> /llm/call/ loopback, where the plugin runs, and
// never reaches a vendor.
const hdrCompression = "X-Tyk-Compression"

// isCompressionHeader reports whether key is X-Tyk-Compression or an
// X-Tyk-Compression-* header.
func isCompressionHeader(key string) bool {
	return strings.EqualFold(key, hdrCompression) ||
		len(key) > len(hdrCompression)+1 && strings.EqualFold(key[:len(hdrCompression)+1], hdrCompression+"-")
}

// stripCompressionHeaders removes the compression headers before a request
// leaves for the vendor.
func stripCompressionHeaders(h http.Header) {
	for k := range h {
		if isCompressionHeader(k) {
			delete(h, k)
		}
	}
}

// withClientLoopbackHeaders adds to extra (the loopback markers) the client's
// X-Tyk-Compression header, so the plugin on the inner /llm/call/ hop sees
// what the client asked for. The SDK driver builds the loopback request
// itself and copies no client header otherwise. Only that one header is
// carried, as the client sent it; the plugin decides whether to honour it.
func withClientLoopbackHeaders(r *http.Request, extra http.Header) http.Header {
	vs := r.Header.Values(hdrCompression)
	if len(vs) == 0 {
		return extra
	}
	if extra == nil {
		extra = http.Header{}
	}
	extra[hdrCompression] = append([]string(nil), vs...)
	return extra
}
