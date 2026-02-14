package plugin_sdk

import (
	"net"
	"net/http"
	"time"
)

// DefaultHTTPClient returns an HTTP client with sensible defaults for plugin use.
// Plugins should reuse this client across requests for connection pooling.
//
// Defaults:
//   - 30 second overall timeout
//   - 10 second dial timeout
//   - 10 second TLS handshake timeout
//   - 100 max idle connections, 10 per host
//   - 90 second idle connection timeout
//
// Plugins can also create their own http.Client if different settings are needed.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}
