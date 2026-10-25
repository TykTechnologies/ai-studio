// Package vendorclient builds the HTTP client vendor drivers and embedders
// use for an endpoint an administrator configured.
package vendorclient

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/httputil"
)

// For is the client for an LLM's or embedder's endpoint: guarded by the
// outbound policy (internal addresses refused at dial time), with the
// asset's "Allow internal host" exemption for that endpoint, and the
// langchaingo User-Agent its drivers send by default.
func For(allowInternalHost bool, endpoint string) *http.Client {
	return &http.Client{Transport: &httputil.Transport{
		Transport: netguard.ExemptionFor(allowInternalHost, endpoint).HTTPTransport(),
	}}
}
