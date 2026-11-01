package proxy

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
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
// what the client asked for, and its X-Tyk-Prompt-Template hint, so the
// request guardrails there see it. The SDK driver builds the loopback
// request itself and copies no client header otherwise. Only those headers
// are carried, as the client sent them; the plugin decides whether to honour
// the one, and the other only orders the guardrails' template search.
func withClientLoopbackHeaders(r *http.Request, extra http.Header) http.Header {
	for _, name := range []string{hdrCompression, templates.HintHeader} {
		vs := r.Header.Values(name)
		if len(vs) == 0 {
			continue
		}
		if extra == nil {
			extra = http.Header{}
		}
		extra[http.CanonicalHeaderKey(name)] = append([]string(nil), vs...)
	}
	return extra
}
